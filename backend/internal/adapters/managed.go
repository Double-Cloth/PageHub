package adapters

import (
	"context"
	"encoding/json"

	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
)

func (b Builtin) inspectManaged(ctx context.Context, a registry.Record, status Status, items []map[string]any) Status {
	if len(items) != 1 {
		return Status{ID: a.Manifest.ID, State: "error", Message: "托管项目的实际服务数量与登记不一致", Running: status.Running}
	}
	id, ok := items[0]["ID"].(string)
	if !ok || id == "" {
		status.State = "unknown"
		status.Message = "无法识别实际容器 ID"
		return status
	}
	out, err := b.Docker.Run(ctx, "", "inspect", id)
	if err != nil {
		status.State = "unknown"
		status.Message = "无法核对实际容器镜像与挂载"
		return status
	}
	var containers []struct {
		Image  string `json:"Image"`
		Mounts []struct {
			Type        string `json:"Type"`
			Name        string `json:"Name"`
			Destination string `json:"Destination"`
			RW          bool   `json:"RW"`
		} `json:"Mounts"`
		HostConfig struct {
			Privileged   bool           `json:"Privileged"`
			PortBindings map[string]any `json:"PortBindings"`
			CapAdd       []string       `json:"CapAdd"`
			Devices      []any          `json:"Devices"`
			NetworkMode  string         `json:"NetworkMode"`
		} `json:"HostConfig"`
	}
	if json.Unmarshal(out, &containers) != nil || len(containers) != 1 {
		status.State = "unknown"
		return status
	}
	c := containers[0]
	status.Image = c.Image
	if c.Image != a.Managed.Image || len(c.Mounts) != 1 || c.Mounts[0].Type != "volume" || c.Mounts[0].Name != a.Project+"_runtime" || c.Mounts[0].Destination != "/var/lib/travel-diary" || !c.Mounts[0].RW || c.HostConfig.Privileged || c.HostConfig.NetworkMode == "host" || len(c.HostConfig.PortBindings) > 0 || len(c.HostConfig.CapAdd) > 0 || len(c.HostConfig.Devices) > 0 {
		status.State = "error"
		status.Message = "实际容器镜像、权限或数据卷与托管登记不一致"
	}
	return status
}
