package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
	"path/filepath"
)

func (b Builtin) externalArgs(a registry.Record) []string {
	s := a.Manifest.Source
	return []string{"compose", "--project-name", s.Project, "--project-directory", s.ComposeDirectory, "-f", filepath.Join(s.ComposeDirectory, s.ComposeFile)}
}

func (b Builtin) validateExternalCompose(ctx context.Context, a registry.Record) error {
	if err := a.Manifest.Validate(b.DevImages); err != nil {
		return err
	}
	out, err := b.Docker.Run(ctx, a.Manifest.Source.ComposeDirectory, append(b.externalArgs(a), "config", "--format", "json")...)
	if err != nil {
		return err
	}
	var config map[string]any
	if err = json.Unmarshal(out, &config); err != nil {
		return err
	}
	canonical, err := json.Marshal(config)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(canonical)
	if hex.EncodeToString(sum[:]) != a.Manifest.Source.ConfigSHA256 {
		return errors.New("既有 Compose 配置摘要已变化，请重新审核")
	}
	services, ok := config["services"].(map[string]any)
	if !ok || len(services) == 0 {
		return errors.New("Compose 缺少服务")
	}
	for name, value := range services {
		service, ok := value.(map[string]any)
		if !ok {
			return errors.New("Compose 服务无效")
		}
		for key, value := range service {
			// 普通字段沿用标准白名单，挂载和安全相关字段必须逐项确认具体值。
			if key != "volumes" && CheckRaw(map[string]any{"services": map[string]any{name: map[string]any{key: value}}}) == nil {
				continue
			}
			approved, exists := a.Manifest.Source.Approvals[name+"."+key]
			left, _ := json.Marshal(approved)
			right, _ := json.Marshal(value)
			if !exists || string(left) != string(right) {
				return fmt.Errorf("需要精确授权 source.approvals[%s.%s]", name, key)
			}
		}
	}
	return nil
}

func (b Builtin) externalComposeAction(ctx context.Context, a registry.Record, action string) error {
	if err := b.validateExternalCompose(ctx, a); err != nil {
		return err
	}
	_, err := b.Docker.Run(ctx, a.Manifest.Source.ComposeDirectory, append(b.externalArgs(a), action)...)
	return err
}
