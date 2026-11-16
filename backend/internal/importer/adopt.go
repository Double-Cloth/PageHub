package importer

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/Double-Cloth/PageHub/backend/internal/registry"
)

type existingContainer struct {
	ID     string `json:"Id"`
	Image  string `json:"Image"`
	Config struct {
		Labels     map[string]string `json:"Labels"`
		Env        []string          `json:"Env"`
		Cmd        []string          `json:"Cmd"`
		Entrypoint []string          `json:"Entrypoint"`
		WorkingDir string            `json:"WorkingDir"`
		User       string            `json:"User"`
	} `json:"Config"`
	State struct {
		Running bool `json:"Running"`
	} `json:"State"`
	HostConfig struct {
		Privileged     bool           `json:"Privileged"`
		NetworkMode    string         `json:"NetworkMode"`
		PortBindings   map[string]any `json:"PortBindings"`
		PidMode        string         `json:"PidMode"`
		IpcMode        string         `json:"IpcMode"`
		CapAdd         []string       `json:"CapAdd"`
		Devices        []any          `json:"Devices"`
		DeviceRequests []any          `json:"DeviceRequests"`
		RestartPolicy  struct {
			Name string `json:"Name"`
		} `json:"RestartPolicy"`
	} `json:"HostConfig"`
	Mounts []struct {
		Type        string `json:"Type"`
		Name        string `json:"Name"`
		Destination string `json:"Destination"`
	} `json:"Mounts"`
}

// 接管仅支持已运行的受控镜像工程，并保留真实环境变量和卷引用。
func (i Importer) CaptureAdoption(ctx context.Context, a registry.Record) (registry.Record, error) {
	var original registry.Record
	if a.Manifest.Kind != "compose" {
		return original, errors.New("接管仅支持 Compose 应用")
	}
	file := filepath.Join(a.Directory, "pagehub.runtime.json")
	b, err := os.ReadFile(file)
	if err != nil {
		return original, err
	}
	var candidate map[string]any
	if err = json.Unmarshal(b, &candidate); err != nil {
		return original, err
	}
	services := candidate["services"].(map[string]any)
	out, err := i.Adapter.Docker.Run(ctx, "", "ps", "--all", "--filter", "label=com.docker.compose.project="+a.Project, "--format", "{{.ID}}")
	if err != nil {
		return original, err
	}
	ids := strings.Fields(string(out))
	if len(ids) != len(services) {
		return original, errors.New("既有项目的服务数量与应用包不一致，拒绝接管")
	}
	args := append([]string{"inspect"}, ids...)
	out, err = i.Adapter.Docker.Run(ctx, "", args...)
	if err != nil {
		return original, err
	}
	var containers []existingContainer
	if err = json.Unmarshal(out, &containers); err != nil {
		return original, err
	}
	seen := map[string]bool{}
	for _, container := range containers {
		name := container.Config.Labels["com.docker.compose.service"]
		service, ok := services[name].(map[string]any)
		if !ok || seen[name] || !container.State.Running || container.HostConfig.Privileged || container.HostConfig.NetworkMode == "host" || len(container.HostConfig.PortBindings) > 0 {
			return original, errors.New("接管要求每个声明服务各一个运行实例，无提升权限或宿主端口")
		}
		if container.HostConfig.PidMode != "" || container.HostConfig.IpcMode == "host" || len(container.HostConfig.CapAdd) > 0 || len(container.HostConfig.Devices) > 0 || len(container.HostConfig.DeviceRequests) > 0 {
			return original, errors.New("现有服务使用共享命名空间、额外 capabilities 或设备，需要专门接管方案")
		}
		seen[name] = true
		if service["build"] != nil {
			return original, errors.New("接管包须以固定镜像声明现有服务，请先准备镜像描述文件")
		}
		image, _ := service["image"].(string)
		imageOut, err := i.Adapter.Docker.Run(ctx, "", "image", "inspect", image, "--format", "{{.Id}}")
		if err != nil || strings.TrimSpace(string(imageOut)) != container.Image {
			return original, errors.New("包内镜像与现有容器镜像不一致")
		}
		service["image"] = container.Image
		if declared, exists := service["command"]; exists && declared != nil {
			left, _ := json.Marshal(declared)
			right, _ := json.Marshal(container.Config.Cmd)
			if string(left) != string(right) {
				return original, errors.New("启动命令与现有服务不一致")
			}
		}
		service["command"] = container.Config.Cmd
		service["entrypoint"] = container.Config.Entrypoint
		service["working_dir"] = container.Config.WorkingDir
		service["user"] = container.Config.User
		environment := map[string]string{}
		for _, entry := range container.Config.Env {
			key, value, found := strings.Cut(entry, "=")
			if found {
				environment[key] = value
			}
		}
		declared, _ := service["environment"].(map[string]any)
		for key, value := range declared {
			if value != nil && value != environment[key] {
				return original, errors.New("应用包环境配置与现有服务不一致")
			}
		}
		service["environment"] = environment
		expected := map[string]string{}
		volumes, _ := candidate["volumes"].(map[string]any)
		mounts, _ := service["volumes"].([]any)
		for _, mount := range mounts {
			v := mount.(map[string]any)
			if v["type"] != "volume" {
				return original, errors.New("接管包含 bind mount，需要独立受信任迁移方案")
			}
			source, _ := v["source"].(string)
			target, _ := v["target"].(string)
			definition, _ := volumes[source].(map[string]any)
			actual, _ := definition["name"].(string)
			if actual == "" {
				actual = a.Project + "_" + source
			}
			expected[target] = actual
		}
		actual := map[string]string{}
		for _, mount := range container.Mounts {
			if mount.Type != "volume" {
				return original, errors.New("现有工程包含宿主 bind mount，请先完成受信任迁移")
			}
			actual[mount.Destination] = mount.Name
		}
		if len(expected) != len(actual) {
			return original, errors.New("持久卷数量不一致，拒绝创建空卷替代现有数据")
		}
		for target, volume := range expected {
			if actual[target] != volume {
				return original, errors.New("持久卷名称或挂载位置不一致，拒绝接管")
			}
		}
	}
	if err = registry.WriteJSON(file, candidate, 0600); err != nil {
		return original, err
	}
	// 原始配置快照用于接管失败时恢复原网络和重启策略。
	copyBytes, _ := json.Marshal(candidate)
	var previous map[string]any
	_ = json.Unmarshal(copyBytes, &previous)
	previousServices := previous["services"].(map[string]any)
	for _, container := range containers {
		name := container.Config.Labels["com.docker.compose.service"]
		service := previousServices[name].(map[string]any)
		service["restart"] = container.HostConfig.RestartPolicy.Name
		if networks, ok := service["networks"].(map[string]any); ok {
			delete(networks, "pagehub-ingress")
		}
	}
	delete(previous["networks"].(map[string]any), "pagehub-ingress")
	original = a
	original.Directory = filepath.Join(a.Directory, "adoption-original")
	if err = registry.WriteJSON(filepath.Join(original.Directory, "pagehub.runtime.json"), previous, 0600); err != nil {
		return original, err
	}
	return original, nil
}
