package lifecycle

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/double-cloth-studio/PageHub/backend/internal/docker"

	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
)

func (s Service) Backup(ctx context.Context, id string) (destination string, err error) {
	a, err := s.Store.Get(id)
	if err != nil {
		return "", err
	}
	if err = s.Store.Validate(a); err != nil {
		return "", err
	}
	if a.Manifest.Backup.Mode == "none" {
		return "", errors.New("该应用声明 backup.mode=none")
	}
	if a.Manifest.Kind != "static" && a.Manifest.Backup.Mode != "volume" {
		return "", errors.New("该应用需要外部业务备份，请使用应用自身的备份工具")
	}
	u, err := registry.Lock(s.Store.Path("state", "locks", id+".lock"))
	if err != nil {
		return "", err
	}
	defer u()
	a, err = s.Store.Get(id)
	if err != nil {
		return "", err
	}
	if err = s.Store.Validate(a); err != nil {
		return "", err
	}
	return s.backupLocked(ctx, a)
}

func (s Service) backupLocked(ctx context.Context, a registry.Record) (destination string, err error) {
	id := a.Manifest.ID
	dest := s.Store.Path("backups", id, time.Now().UTC().Format("20060102T150405.000000000Z"))
	if err = os.MkdirAll(dest, 0700); err != nil {
		return "", err
	}
	if a.Manifest.Kind == "static" {
		file, err := os.Create(filepath.Join(dest, "bundle.zip"))
		if err != nil {
			return "", err
		}
		writer := zip.NewWriter(file)
		err = filepath.WalkDir(a.ContentDirectory(), func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if !entry.Type().IsRegular() {
				return errors.New("备份目录含特殊文件")
			}
			rel, err := filepath.Rel(a.ContentDirectory(), path)
			if err != nil {
				return err
			}
			if strings.HasPrefix(rel, "pagehub.") {
				return nil
			}
			output, err := writer.Create(filepath.ToSlash(rel))
			if err != nil {
				return err
			}
			input, err := os.Open(path)
			if err != nil {
				return err
			}
			defer input.Close()
			_, err = io.Copy(output, input)
			return err
		})
		closeErr := writer.Close()
		fileErr := file.Close()
		if err != nil {
			return "", err
		}
		if closeErr != nil {
			return "", closeErr
		}
		if fileErr != nil {
			return "", fileErr
		}
	} else {
		states, queryErr := s.Adapter.Docker.Compose(ctx, a, "ps", "--all", "--format", "json")
		if queryErr != nil {
			return "", queryErr
		}
		containers, queryErr := docker.DecodeObjects(states)
		if queryErr != nil {
			return "", queryErr
		}
		runningServices := []string{}
		for _, container := range containers {
			if container["State"] == "running" || container["State"] == "restarting" {
				name, ok := container["Service"].(string)
				if !ok || name == "" {
					return "", errors.New("无法识别原运行服务，拒绝备份")
				}
				runningServices = append(runningServices, name)
			}
		}
		if err = s.Adapter.Stop(ctx, a); err != nil {
			return "", err
		}
		if len(runningServices) > 0 {
			defer func() {
				restoreCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				_, restoreErr := s.Adapter.Docker.Compose(restoreCtx, a, append([]string{"start"}, runningServices...)...)
				if restoreErr != nil {
					err = errors.Join(err, fmt.Errorf("备份后恢复原运行服务失败: %w", restoreErr))
				}
			}()
		}
		out, err := s.Adapter.Docker.Run(ctx, "", "volume", "ls", "--filter", "label=com.docker.compose.project="+a.Project, "--format", "{{.Name}}")
		if err != nil {
			return "", err
		}
		volumes := strings.Fields(string(out))
		if len(volumes) == 0 {
			return "", errors.New("未发现持久卷，未生成备份收据")
		}
		for _, volume := range volumes {
			if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]+$`).MatchString(volume) {
				return "", errors.New("卷名无效")
			}
			_, err = s.Adapter.Docker.Run(ctx, "", "run", "--rm", "--pull", "never", "--network", "none", "--mount", "type=volume,src="+volume+",dst=/data,readonly", "--mount", "type=bind,src="+dest+",dst=/backup", "--entrypoint", "tar", s.Gateway.Image, "-C", "/data", "-czf", "/backup/"+volume+".tar.gz", ".")
			if err != nil {
				return "", err
			}
		}
	}
	checksums := map[string]string{}
	files, err := os.ReadDir(dest)
	if err != nil {
		return "", err
	}
	for _, file := range files {
		input, err := os.Open(filepath.Join(dest, file.Name()))
		if err != nil {
			return "", err
		}
		h := sha256.New()
		_, err = io.Copy(h, input)
		input.Close()
		if err != nil {
			return "", err
		}
		checksums[file.Name()] = hex.EncodeToString(h.Sum(nil))
	}
	err = registry.WriteJSON(filepath.Join(dest, "receipt.json"), map[string]any{"id": id, "digest": a.Digest, "created_at": time.Now().UTC(), "files": checksums, "mode": a.Manifest.Backup.Mode, "restore_verified": false}, 0600)
	return dest, err
}
