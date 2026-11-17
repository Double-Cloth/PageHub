package managed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/double-cloth-studio/PageHub/backend/internal/desired"
	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
)

type SelfRequest struct {
	Stage        string    `json:"stage,omitempty"`
	ID           string    `json:"id"`
	Commit       string    `json:"commit"`
	ConfigSHA256 string    `json:"config_sha256"`
	RequestedAt  time.Time `json:"requested_at"`
}

func (s Service) ScheduleSelf(ctx context.Context, c desired.Config, commit, stage string) (SelfRequest, error) {
	request := SelfRequest{ID: fmt.Sprint(time.Now().UnixNano()), Commit: commit, Stage: stage, ConfigSHA256: desired.Digest(c), RequestedAt: time.Now().UTC()}
	if !commitPattern.MatchString(commit) {
		return request, errors.New("平台更新需要冻结 commit")
	}
	u, err := registry.Lock(s.Legacy.Store.Path("state", "self-request.lock"))
	if err != nil {
		return request, err
	}
	defer u()
	unit := "pagehub-updater.service"
	state, err := exec.CommandContext(ctx, "systemctl", "show", "--property=ActiveState", "--value", unit).Output()
	if err != nil {
		return request, errors.New("独立 updater 服务未安装或 systemd 不可用")
	}
	if value := strings.TrimSpace(string(state)); value != "inactive" && value != "failed" {
		return request, errors.New("平台更新正在执行")
	}
	if err = registry.WriteJSON(s.Legacy.Store.Path("state", "self-request.json"), request, 0600); err != nil {
		return request, err
	}
	cmd := exec.CommandContext(ctx, "systemctl", "start", "--no-block", unit)
	if err = cmd.Run(); err != nil {
		return request, errors.New("无法启动独立 systemd updater，请检查本机服务日志")
	}
	return request, nil
}

func (s Service) WaitSelf(ctx context.Context, request SelfRequest) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		var result struct {
			ID    string `json:"id"`
			State string `json:"state"`
		}
		b, err := os.ReadFile(s.Legacy.Store.Path("state", "self-result.json"))
		if err == nil && json.Unmarshal(b, &result) == nil && result.ID == request.ID {
			if result.State == "succeeded" {
				return nil
			}
			if result.State == "error" {
				return errors.New("平台更新失败，请检查独立运维记录和 pagehub-updater 日志")
			}
		}
		if time.Since(request.RequestedAt) > 5*time.Second {
			out, err := exec.CommandContext(ctx, "systemctl", "show", "--property=ActiveState", "--value", "pagehub-updater.service").Output()
			if err != nil || strings.TrimSpace(string(out)) == "failed" || strings.TrimSpace(string(out)) == "inactive" {
				return errors.New("独立 updater 已退出且缺少成功收据，请检查更新日志")
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s Service) PrepareSelf(ctx context.Context, c desired.Config, request SelfRequest) (string, error) {
	if request.ConfigSHA256 != desired.Digest(c) || !commitPattern.MatchString(request.Commit) {
		return "", errors.New("平台更新配置已变化或请求无效")
	}
	u, err := registry.Lock(s.Legacy.Store.Path("state", "platform.lock"))
	if err != nil {
		return "", err
	}
	defer u()
	if request.Stage != "" {
		base := s.Legacy.Store.Path("state", "self-staging")
		if filepath.Dir(request.Stage) != base || !strings.HasPrefix(filepath.Base(request.Stage), request.Commit+"-") {
			return "", errors.New("平台暂存路径越界")
		}
		real, err := filepath.EvalSymlinks(request.Stage)
		if err != nil || real != request.Stage {
			return "", errors.New("平台暂存目录包含链接或不存在")
		}
		b, err := os.ReadFile(filepath.Join(request.Stage, "source-version.json"))
		if err != nil {
			return "", err
		}
		var version struct {
			Commit string `json:"commit"`
		}
		if json.Unmarshal(b, &version) != nil || version.Commit != request.Commit {
			return "", errors.New("暂存平台 commit 与审批不一致")
		}
		return request.Stage, nil
	}
	dir := s.Legacy.Store.Path("state", "self-staging", request.Commit+"-"+fmt.Sprint(time.Now().UnixNano()))
	if err = os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(filepath.Dir(dir), ".prepare-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	if err = checkout(ctx, c.Platform.SourceDir, c.Platform.Repository, c.Platform.Branch, request.Commit, stage, nil); err != nil {
		return "", err
	}
	if _, err = os.Stat(filepath.Join(stage, "backend", "cmd", "runner", "main.go")); err != nil {
		return "", errors.New("平台源码缺少 Runner")
	}
	if err = registry.WriteJSON(filepath.Join(stage, "source-version.json"), map[string]string{"commit": request.Commit, "repository": c.Platform.Repository}, 0600); err != nil {
		return "", err
	}
	if _, err = os.Stat(dir); err == nil {
		return "", errors.New("该平台暂存版本已存在，请检查上次更新记录")
	}
	if err = os.Rename(stage, dir); err != nil {
		return "", err
	}
	return dir, nil
}

type ComponentStatus struct {
	ExpectedImage string `json:"expected_image,omitempty"`
	AssetSHA256   string `json:"asset_sha256,omitempty"`
	RouteEnabled  bool   `json:"route_enabled"`
	Dirty         bool   `json:"dirty,omitempty"`
	Version       string `json:"version,omitempty"`
	ID            string `json:"id"`
	Commit        string `json:"commit,omitempty"`
	Image         string `json:"image,omitempty"`
	Digest        string `json:"digest,omitempty"`
	Path          string `json:"path,omitempty"`
	Driver        string `json:"driver"`
	State         string `json:"state"`
	Running       bool   `json:"running"`
}

type SourceVersion struct {
	Commit string `json:"commit"`
	Dirty  bool   `json:"dirty"`
}

func (s Service) SourceVersion() SourceVersion {
	var version SourceVersion
	b, err := os.ReadFile(s.Legacy.Store.Path("current", "source-version.json"))
	if err != nil || json.Unmarshal(b, &version) != nil {
		version.Dirty = true
	}
	return version
}

func (s Service) Status(ctx context.Context) ([]ComponentStatus, error) {
	r, err := s.Legacy.Store.Load()
	if err != nil {
		return nil, err
	}
	result := []ComponentStatus{}
	for _, a := range r.Apps {
		if err = s.Legacy.Store.Validate(a); err != nil {
			result = append(result, ComponentStatus{ID: a.Manifest.ID, Driver: a.Manifest.Kind, State: "invalid"})
			continue
		}
		status := s.Legacy.Adapter.Inspect(ctx, a)
		entry := ComponentStatus{ID: a.Manifest.ID, Digest: a.Digest, Path: a.Manifest.Path, Driver: a.Manifest.Kind, State: status.State, Running: status.Running}
		entry.RouteEnabled = !a.Disabled
		if a.Managed != nil {
			entry.Commit = a.Managed.Commit
			entry.ExpectedImage = a.Managed.Image
			entry.Image = status.Image
			entry.Driver = a.Managed.Config.Deployment.Driver
			entry.AssetSHA256 = a.Managed.AssetSHA256
		}
		result = append(result, entry)
	}
	version := s.SourceVersion()
	out, serviceErr := exec.CommandContext(ctx, "systemctl", "is-active", "pagehub-manager", "pagehub-runner").Output()
	state := "unhealthy"
	routeEnabled := endpointHealthy(ctx, s.Legacy.Adapter.GatewayURL+"/healthz") && endpointHealthy(ctx, s.Legacy.Adapter.GatewayURL+"/")
	if serviceErr == nil && strings.Count(string(out), "active\n") == 2 && routeEnabled {
		state = "running"
	}
	releaseVersion, _ := os.ReadFile(s.Legacy.Store.Path("current", "VERSION"))
	result = append(result, ComponentStatus{ID: "self", Commit: version.Commit, Dirty: version.Dirty, Version: strings.TrimSpace(string(releaseVersion)), Driver: "platform", State: state, Running: serviceErr == nil, RouteEnabled: routeEnabled})
	return result, nil
}

func endpointHealthy(ctx context.Context, address string) bool {
	request, err := http.NewRequestWithContext(ctx, "GET", address, nil)
	if err != nil {
		return false
	}
	client := http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode == http.StatusOK
}

func (s Service) Doctor(ctx context.Context) error {
	c, err := s.Config()
	if err != nil {
		return err
	}
	if err = s.checkPlatformConfig(c); err != nil {
		return err
	}
	if err = s.Legacy.Doctor(ctx); err != nil {
		return err
	}
	if !endpointHealthy(ctx, s.Legacy.Adapter.GatewayURL+"/healthz") || !endpointHealthy(ctx, s.Legacy.Adapter.GatewayURL+"/") {
		return errors.New("共享网关或门户 HTTP 健康检查失败")
	}
	if !endpointHealthy(ctx, fmt.Sprintf("http://127.0.0.1:%d/_pagehub/api/health", c.Platform.Admin.Port)) {
		return errors.New("Manager 管理入口健康检查失败")
	}
	r, err := s.Legacy.Store.Load()
	if err != nil {
		return err
	}
	for _, a := range r.Apps {
		if a.Managed != nil && a.Manifest.Kind == "compose" {
			if _, err = s.Legacy.Adapter.Docker.Run(ctx, "", "volume", "inspect", a.Project+"_runtime"); err != nil {
				return errors.New("托管 runtime 数据卷缺失，请检查备份恢复")
			}
		}
	}
	return nil
}
