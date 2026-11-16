package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/double-cloth-studio/PageHub/backend/internal/docker"
	"github.com/double-cloth-studio/PageHub/backend/internal/manifests"
	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
	"gopkg.in/yaml.v3"
)

type Status struct {
	ID      string `json:"id"`
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
	Running bool   `json:"-"`
}

type ExecutionPlan struct {
	ID          string `json:"id"`
	Action      string `json:"action"`
	Fingerprint string `json:"fingerprint"`
}

type Adapter interface {
	Validate(context.Context, registry.Record) error
	Plan(registry.Record, string) (ExecutionPlan, error)
	Inspect(context.Context, registry.Record) Status
	Start(context.Context, registry.Record) error
	Stop(context.Context, registry.Record) error
	Probe(context.Context, registry.Record) Status
}

type Builtin struct {
	Docker     docker.Client
	GatewayURL string
	DevImages  bool
}

func (b Builtin) Plan(a registry.Record, action string) (ExecutionPlan, error) {
	if !a.Manifest.Allows(action) {
		return ExecutionPlan{}, errors.New("应用未授权此生命周期操作")
	}
	return ExecutionPlan{ID: a.Manifest.ID, Action: action, Fingerprint: registry.Fingerprint(a)}, nil
}

func (b Builtin) Validate(ctx context.Context, a registry.Record) error {
	m := a.Manifest
	if m.Kind == "external" {
		return m.Validate(b.DevImages)
	}
	if m.Kind == "external-compose" {
		return b.validateExternalCompose(ctx, a)
	}
	if err := m.Validate(b.DevImages); err != nil {
		return err
	}
	if m.Kind == "static" {
		root, err := os.OpenRoot(a.Directory)
		if err != nil {
			return err
		}
		defer root.Close()
		st, err := root.Stat(m.Source.StaticDir)
		if err != nil || !st.IsDir() {
			return errors.New("static_dir 不存在或不是目录")
		}
		if m.Source.Fallback != "" {
			st, err = root.Stat(filepath.Join(m.Source.StaticDir, m.Source.Fallback))
			if err != nil || !st.Mode().IsRegular() {
				return errors.New("fallback 文件不存在")
			}
		}
		return nil
	}
	var config map[string]any
	if m.Kind == "image" {
		config = map[string]any{"services": map[string]any{"web": map[string]any{"image": m.Source.Image}}}
	} else {
		file := filepath.Join(a.Directory, m.Source.ComposeFile)
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		var raw map[string]any
		if err = yaml.Unmarshal(data, &raw); err != nil {
			return err
		}
		if err = CheckRaw(raw); err != nil {
			return err
		}
		out, err := b.Docker.Run(ctx, a.Directory, "compose", "--project-name", a.Project, "--project-directory", a.Directory, "-f", file, "config", "--format", "json")
		if err != nil {
			return err
		}
		if err = json.Unmarshal(out, &config); err != nil {
			return err
		}
	}
	if err := CheckResolved(config, a.Directory, a.Project, b.DevImages); err != nil {
		return err
	}
	services := config["services"].(map[string]any)
	web := m.Source.WebService
	if m.Kind == "image" {
		web = "web"
	}
	if _, ok := services[web]; !ok {
		return errors.New("web_service 未出现在 Compose 配置中")
	}
	networks, _ := config["networks"].(map[string]any)
	if networks == nil {
		networks = map[string]any{}
	}
	networks["pagehub-ingress"] = map[string]any{"external": true, "name": "pagehub-ingress"}
	config["networks"] = networks
	for name, value := range services {
		service := value.(map[string]any)
		service["restart"] = "no"
		if m.Startup == "always" {
			service["restart"] = "unless-stopped"
		}
		if name == web {
			nets, _ := service["networks"].(map[string]any)
			if nets == nil {
				nets = map[string]any{"default": nil}
				if _, ok := networks["default"]; !ok {
					networks["default"] = map[string]any{}
				}
			}
			nets["pagehub-ingress"] = map[string]any{"aliases": []string{"pagehub-" + m.ID + "-web"}}
			service["networks"] = nets
		}
	}
	return registry.WriteJSON(filepath.Join(a.Directory, "pagehub.runtime.json"), config, 0600)
}

func CheckRaw(config map[string]any) error {
	allowedTop := map[string]bool{"services": true, "volumes": true, "networks": true, "name": true, "version": true}
	for key := range config {
		if !allowedTop[key] {
			return fmt.Errorf("不支持的 Compose 顶层字段 %s", key)
		}
	}
	services, ok := config["services"].(map[string]any)
	if !ok || len(services) == 0 {
		return errors.New("Compose 缺少 services")
	}
	allowed := map[string]bool{}
	for _, key := range strings.Fields("image build command environment volumes networks healthcheck depends_on restart expose working_dir user read_only tmpfs cap_drop security_opt stop_grace_period init shm_size mem_limit cpus pids_limit labels") {
		allowed[key] = true
	}
	for name, value := range services {
		s, ok := value.(map[string]any)
		if !ok {
			return errors.New("服务格式无效")
		}
		for key := range s {
			if key == "entrypoint" && s[key] == nil {
				continue
			}
			if !allowed[key] {
				return fmt.Errorf("服务 %s 的字段 %s 不在权限白名单中", name, key)
			}
		}
	}
	return nil
}

func inside(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func CheckResolved(config map[string]any, dir, project string, dev bool) error {
	services, ok := config["services"].(map[string]any)
	if !ok || len(services) == 0 {
		return errors.New("Compose 缺少 services")
	}
	for _, value := range services {
		s, ok := value.(map[string]any)
		if !ok {
			return errors.New("服务格式无效")
		}
		for _, key := range strings.Fields("privileged network_mode pid ipc devices cap_add ports entrypoint volumes_from external_links container_name develop post_start pre_stop provider profiles") {
			if v, exists := s[key]; exists && v != nil && v != false {
				return fmt.Errorf("拒绝危险 Compose 字段 %s", key)
			}
		}
		if image, ok := s["image"].(string); ok && !manifests.ValidImage(image, dev) {
			return errors.New("所有服务镜像必须固定摘要")
		}
		if build, ok := s["build"].(map[string]any); ok {
			for key := range build {
				if key != "context" && key != "dockerfile" && key != "args" {
					return fmt.Errorf("拒绝构建字段 %s", key)
				}
			}
			contextDir, _ := build["context"].(string)
			if !filepath.IsAbs(contextDir) || !inside(dir, contextDir) {
				return errors.New("构建 context 必须位于应用包内")
			}
			dockerfile, _ := build["dockerfile"].(string)
			if dockerfile != "" && (!manifests.RelativeFile(dockerfile) || !inside(dir, filepath.Join(contextDir, dockerfile))) {
				return errors.New("Dockerfile 路径无效")
			}
		} else if _, ok := s["image"]; !ok {
			return errors.New("服务必须有镜像或受控本地构建")
		}
		if opts, ok := s["security_opt"].([]any); ok {
			for _, opt := range opts {
				if opt != "no-new-privileges:true" && opt != "no-new-privileges" {
					return errors.New("security_opt 不在白名单")
				}
			}
		}
		if mounts, ok := s["volumes"].([]any); ok {
			for _, mount := range mounts {
				v, ok := mount.(map[string]any)
				if !ok {
					return errors.New("卷配置无效")
				}
				typeName, _ := v["type"].(string)
				source, _ := v["source"].(string)
				target, _ := v["target"].(string)
				if strings.Contains(target, "docker.sock") {
					return errors.New("拒绝 Docker Socket")
				}
				if typeName == "bind" {
					if !inside(dir, source) || v["read_only"] != true {
						return errors.New("bind mount 必须指向包内只读路径")
					}
				} else if typeName != "volume" || source == "" {
					return errors.New("只允许明确命名的持久卷")
				}
			}
		}
	}
	for _, section := range []string{"volumes", "networks"} {
		items, _ := config[section].(map[string]any)
		for key, value := range items {
			v, _ := value.(map[string]any)
			if key == "pagehub-ingress" || v["external"] == true || v["driver_opts"] != nil {
				return errors.New("拒绝外部卷、网络或自定义 driver_opts")
			}
			if name, ok := v["name"].(string); ok && !strings.HasPrefix(name, project+"_") {
				return errors.New("卷或网络名称超出项目边界")
			}
			if driver, ok := v["driver"].(string); ok && driver != "local" && driver != "bridge" {
				return errors.New("不支持的存储或网络驱动")
			}
		}
	}
	return nil
}

func (b Builtin) Start(ctx context.Context, a registry.Record) error {
	if a.Manifest.Kind == "external-compose" {
		return b.externalComposeAction(ctx, a, "start")
	}
	if a.Manifest.Kind == "external" {
		return nil
	}
	if a.Manifest.Kind == "static" {
		return nil
	}
	_, err := b.Docker.Compose(ctx, a, "up", "-d", "--no-build", "--pull", "never")
	return err
}

func (b Builtin) Stop(ctx context.Context, a registry.Record) error {
	if a.Manifest.Kind == "external-compose" {
		return b.externalComposeAction(ctx, a, "stop")
	}
	if a.Manifest.Kind == "external" {
		return nil
	}
	if a.Manifest.Kind == "static" {
		return errors.New("静态应用通过网关托管，无独立容器")
	}
	_, err := b.Docker.Compose(ctx, a, "stop", "--timeout", "30")
	return err
}

func (b Builtin) Inspect(ctx context.Context, a registry.Record) Status {
	status := Status{ID: a.Manifest.ID, State: "unknown"}
	if a.Manifest.Kind == "external" || a.Manifest.Kind == "external-compose" {
		return b.Probe(ctx, a)
	}
	if a.Manifest.Kind == "static" {
		st, err := os.Stat(filepath.Join(a.Directory, a.Manifest.Source.StaticDir))
		if err != nil || !st.IsDir() {
			status.State = "error"
		} else {
			return b.Probe(ctx, a)
		}
		return status
	}
	out, err := b.Docker.Compose(ctx, a, "ps", "--all", "--format", "json")
	if err != nil {
		status.Message = "Docker 查询失败"
		return status
	}
	items, err := docker.DecodeObjects(out)
	if err != nil {
		return status
	}
	if len(items) == 0 {
		status.State = "stopped"
		return status
	}
	running := false
	for _, item := range items {
		state, _ := item["State"].(string)
		if state == "restarting" {
			status.State = "starting"
			return status
		}
		if state != "running" {
			status.State = "stopped"
			return status
		}
		if item["Health"] == "unhealthy" {
			status.State = "unhealthy"
			return status
		}
		if item["Health"] == "starting" {
			status.State = "starting"
			return status
		}
		running = true
	}
	if running {
		probed := b.Probe(ctx, a)
		probed.Running = true
		return probed
	}
	return status
}

func (b Builtin) Probe(ctx context.Context, a registry.Record) Status {
	s := Status{ID: a.Manifest.ID, State: "unknown"}
	if a.Manifest.Visibility == "private" {
		s.Message = "私有路由默认拒绝公网访问，需通过独立可信入口探测"
		return s
	}
	path := strings.TrimSuffix(a.Manifest.Path, "/") + a.Manifest.Health.Path
	if a.Manifest.RouteMode == "preserve_prefix" {
		path = a.Manifest.Health.Path
		if !strings.HasPrefix(path, a.Manifest.Path) {
			path = strings.TrimSuffix(a.Manifest.Path, "/") + path
		}
	}
	req, err := http.NewRequestWithContext(ctx, "GET", b.GatewayURL+path, nil)
	if err != nil {
		return s
	}
	client := http.Client{Timeout: time.Duration(a.Manifest.Health.TimeoutSeconds) * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		s.State = "unhealthy"
		return s
	}
	defer resp.Body.Close()
	s.State = "unhealthy"
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		s.State = "healthy"
	}
	return s
}
