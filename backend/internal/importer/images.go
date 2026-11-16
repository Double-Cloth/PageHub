package importer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/Double-Cloth/PageHub/backend/internal/registry"
)

func (i Importer) PinImages(ctx context.Context, a registry.Record) error {
	file := filepath.Join(a.Directory, "pagehub.runtime.json")
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	var config map[string]any
	if err = json.Unmarshal(data, &config); err != nil {
		return err
	}
	services := config["services"].(map[string]any)
	volumes, _ := config["volumes"].(map[string]any)
	if volumes == nil {
		volumes = map[string]any{}
	}
	offlineImages := map[string]string{}
	if os.Getenv("PAGEHUB_OFFLINE") == "true" {
		mapping, err := os.ReadFile(i.Store.Path("config", "offline-images.json"))
		if err != nil {
			return errors.New("离线安装缺少已校验的镜像映射")
		}
		if err = json.Unmarshal(mapping, &offlineImages); err != nil {
			return err
		}
	}
	for name, value := range services {
		service := value.(map[string]any)
		image, _ := service["image"].(string)
		if image == "" {
			image = a.Project + "-" + name
		}
		if os.Getenv("PAGEHUB_OFFLINE") == "true" {
			mapped, ok := offlineImages[image]
			if !ok {
				return errors.New("离线镜像映射缺少服务 " + name)
			}
			image = mapped
		}
		out, err := i.Adapter.Docker.Run(ctx, "", "image", "inspect", image, "--format", "{{.Id}}")
		if err != nil {
			return err
		}
		id := strings.TrimSpace(string(out))
		if !strings.HasPrefix(id, "sha256:") || len(id) != 71 {
			return errors.New("镜像 ID 无效")
		}
		service["image"] = id
		metadata, err := i.Adapter.Docker.Run(ctx, "", "image", "inspect", id, "--format", "{{json .Config.Volumes}}")
		if err != nil {
			return err
		}
		var declared map[string]any
		if err = json.Unmarshal(metadata, &declared); err != nil {
			return err
		}
		mounts, _ := service["volumes"].([]any)
		targets := map[string]bool{}
		for _, value := range mounts {
			mount := value.(map[string]any)
			target, _ := mount["target"].(string)
			targets[target] = true
		}
		for target := range declared {
			if targets[target] {
				continue
			}
			hash := sha256.Sum256([]byte(name + "\x00" + target))
			source := "image-" + hex.EncodeToString(hash[:8])
			volumes[source] = map[string]any{"name": a.Project + "_" + source}
			mounts = append(mounts, map[string]any{"type": "volume", "source": source, "target": target})
		}
		if len(mounts) > 0 {
			service["volumes"] = mounts
		}
	}
	if len(volumes) > 0 {
		config["volumes"] = volumes
	}
	return registry.WriteJSON(file, config, 0600)
}

func (i Importer) PrepareImages(ctx context.Context, a registry.Record) error {
	data, err := os.ReadFile(filepath.Join(a.Directory, "pagehub.runtime.json"))
	if err != nil {
		return err
	}
	var config map[string]any
	if err = json.Unmarshal(data, &config); err != nil {
		return err
	}
	for name, value := range config["services"].(map[string]any) {
		service := value.(map[string]any)
		if service["build"] != nil {
			continue
		}
		image, _ := service["image"].(string)
		if strings.HasPrefix(image, "sha256:") {
			if _, err = i.Adapter.Docker.Run(ctx, "", "image", "inspect", image); err != nil {
				return err
			}
		} else {
			if _, err = i.Adapter.Docker.Compose(ctx, a, "pull", name); err != nil {
				return err
			}
		}
	}
	_, err = i.Adapter.Docker.Compose(ctx, a, "build")
	return err
}
