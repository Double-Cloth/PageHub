package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Double-Cloth/PageHub/backend/internal/registry"
)

type Client struct{}

func (Client) Run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	configureProcess(cmd)
	cmd.Dir = dir
	// 不继承宿主机 Compose 控制变量和秘密环境变量。
	for _, key := range []string{"PATH", "HOME", "DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_CONFIG", "SYSTEMROOT"} {
		if value, ok := os.LookupEnv(key); ok {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		reason := "请在本机检查 Docker 日志"
		detail := strings.ToLower(stderr.String())
		switch {
		case strings.Contains(detail, "no such image"), strings.Contains(detail, "not found"), strings.Contains(detail, "pull access denied"):
			reason = "镜像或资源不存在，请核对固定镜像和离线产物"
		case strings.Contains(detail, "permission denied"), strings.Contains(detail, "denied"):
			reason = "Docker 或挂载路径权限不足"
		case strings.Contains(detail, "already allocated"), strings.Contains(detail, "address already in use"):
			reason = "宿主端口已占用"
		case strings.Contains(detail, "unhealthy"):
			reason = "容器健康检查失败"
		case ctx.Err() != nil:
			reason = "操作超时或被取消"
		}
		return nil, fmt.Errorf("docker %s 失败，%s", strings.Join(args[:min(len(args), 2)], " "), reason)
	}
	return out, nil
}

func (d Client) Compose(ctx context.Context, a registry.Record, args ...string) ([]byte, error) {
	base := []string{"compose", "--project-name", a.Project, "--project-directory", a.Directory, "-f", filepath.Join(a.Directory, "pagehub.runtime.json")}
	return d.Run(ctx, a.Directory, append(base, args...)...)
}

func DecodeObjects(data []byte) ([]map[string]any, error) {
	var objects []map[string]any
	if len(bytes.TrimSpace(data)) == 0 {
		return objects, nil
	}
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("[")) {
		err := json.Unmarshal(data, &objects)
		return objects, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	for dec.More() {
		var o map[string]any
		if err := dec.Decode(&o); err != nil {
			return nil, err
		}
		objects = append(objects, o)
	}
	return objects, nil
}
