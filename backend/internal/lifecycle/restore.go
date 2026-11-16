package lifecycle

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/double-cloth-studio/PageHub/backend/internal/importer"
	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

type backupReceipt struct {
	ID     string            `json:"id"`
	Digest string            `json:"digest"`
	Files  map[string]string `json:"files"`
}

// VerifyBackup 在隔离目录或卷中实际解包，并保留文件校验清单。
func (s Service) VerifyBackup(ctx context.Context, id, directory string) error {
	a, err := s.Store.Get(id)
	if err != nil {
		return err
	}
	if err = s.Store.Validate(a); err != nil {
		return err
	}
	receipt, err := readBackup(directory, id, a.Digest)
	if err != nil {
		return err
	}
	if a.Manifest.Kind == "static" {
		if len(receipt.Files) != 1 || receipt.Files["bundle.zip"] == "" {
			return errors.New("静态备份文件清单无效")
		}
		stage, err := os.MkdirTemp("", "pagehub-verify-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(stage)
		if _, err = importer.Extract(ctx, filepath.Join(directory, "bundle.zip"), stage); err != nil {
			return err
		}
	} else {
		if a.Manifest.Backup.Mode != "volume" {
			return errors.New("该应用须使用外部恢复工具")
		}
		for file := range receipt.Files {
			if !strings.HasSuffix(file, ".tar.gz") {
				return errors.New("卷归档名称无效")
			}
			if err = validateVolumeArchive(filepath.Join(directory, file)); err != nil {
				return err
			}
			volume := fmt.Sprintf("pagehub-verify-%d", time.Now().UnixNano())
			if _, err = s.Adapter.Docker.Run(ctx, "", "volume", "create", volume); err != nil {
				return err
			}
			result, restoreErr := s.restoreVolume(ctx, directory, file, volume)
			cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
			_, cleanupErr := s.Adapter.Docker.Run(cleanupCtx, "", "volume", "rm", volume)
			cancel()
			if err = errors.Join(restoreErr, cleanupErr); err != nil {
				return err
			}
			if err = registry.AtomicWrite(filepath.Join(directory, file+".verified-files"), result, 0600); err != nil {
				return err
			}
		}
	}
	return registry.WriteJSON(filepath.Join(directory, "verification.json"), map[string]any{"id": id, "digest": a.Digest, "files": receipt.Files, "restore_verified": true, "verified_at": time.Now().UTC()}, 0600)
}

func readBackup(directory, id, digest string) (backupReceipt, error) {
	var r backupReceipt
	data, err := os.ReadFile(filepath.Join(directory, "receipt.json"))
	if err != nil {
		return r, err
	}
	if json.Unmarshal(data, &r) != nil || r.ID != id || r.Digest != digest || len(r.Files) == 0 {
		return r, errors.New("备份与当前应用版本不匹配")
	}
	for name, expected := range r.Files {
		if filepath.Base(name) != name || strings.ContainsAny(name, "/\\") || len(expected) != 64 {
			return r, errors.New("备份文件清单无效")
		}
		f, err := os.Open(filepath.Join(directory, name))
		if err != nil {
			return r, err
		}
		h := sha256.New()
		_, err = io.Copy(h, f)
		f.Close()
		if err != nil {
			return r, err
		}
		if hex.EncodeToString(h.Sum(nil)) != expected {
			return r, errors.New("备份 SHA-256 校验失败")
		}
	}
	return r, nil
}

func validateVolumeArchive(file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := path.Clean(h.Name)
		if path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, "\\") || (h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeDir) {
			return errors.New("卷归档含越界路径、链接或特殊文件")
		}
	}
}

func (s Service) restoreVolume(ctx context.Context, directory, file, volume string) ([]byte, error) {
	return s.Adapter.Docker.Run(ctx, "", "run", "--rm", "--pull", "never", "--network", "none", "--mount", "type=volume,src="+volume+",dst=/data", "--mount", "type=bind,src="+directory+",dst=/backup,readonly", "--entrypoint", "sh", s.Gateway.Image, "-ec", `find /data -mindepth 1 -maxdepth 1 -exec rm -rf {} +; tar -xzf "/backup/$1" -C /data; cd /data; find . -type f -exec sha256sum {} \; | sort`, "restore", file)
}

func (s Service) Restore(ctx context.Context, id, directory string, dry bool) (err error) {
	directory, err = filepath.Abs(directory)
	if err != nil {
		return err
	}
	unlock, err := registry.Lock(s.Store.Path("state", "locks", id+".lock"))
	if err != nil {
		return err
	}
	defer unlock()
	if err = s.VerifyBackup(ctx, id, directory); err != nil {
		return err
	}
	if dry {
		return nil
	}
	a, err := s.Store.Get(id)
	if err != nil {
		return err
	}
	safety, err := s.backupLocked(ctx, a)
	if err != nil {
		return err
	}
	defer func() {
		state := "succeeded"
		if err != nil {
			state = "failed"
		}
		err = errors.Join(err, registry.WriteJSON(s.Store.Path("state", "restores", fmt.Sprintf("%d.json", time.Now().UnixNano())), map[string]any{"id": id, "backup": directory, "safety_backup": safety, "state": state, "at": time.Now().UTC()}, 0600))
	}()
	if a.Manifest.Kind == "static" {
		stage, err := os.MkdirTemp(filepath.Dir(a.Directory), "restore-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(stage)
		if _, err = importer.Extract(ctx, filepath.Join(directory, "bundle.zip"), stage); err != nil {
			return err
		}
		target := filepath.Join(a.Directory, a.Manifest.Source.StaticDir)
		previous := target + fmt.Sprintf(".before-restore-%d", time.Now().UnixNano())
		if err = os.Rename(target, previous); err != nil {
			return err
		}
		if err = os.Rename(filepath.Join(stage, a.Manifest.Source.StaticDir), target); err != nil {
			return errors.Join(err, os.Rename(previous, target))
		}
		return nil
	}
	// 正式恢复要求应用已停止，避免数据库与卷写入发生竞争。
	out, err := s.Adapter.Docker.Compose(ctx, a, "ps", "--status", "running", "--quiet")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(out)) != "" {
		return errors.New("请先停止应用后执行正式恢复，当前数据已自动备份")
	}
	receipt, err := readBackup(directory, id, a.Digest)
	if err != nil {
		return err
	}
	volumes, err := s.Adapter.Docker.Run(ctx, "", "volume", "ls", "--filter", "label=com.docker.compose.project="+a.Project, "--format", "{{.Name}}")
	if err != nil {
		return err
	}
	allowed := map[string]bool{}
	for _, v := range strings.Fields(string(volumes)) {
		allowed[v] = true
	}
	for file := range receipt.Files {
		if !allowed[strings.TrimSuffix(file, ".tar.gz")] {
			return errors.New("备份包含非当前工程卷")
		}
	}
	for file := range receipt.Files {
		result, restoreErr := s.restoreVolume(ctx, directory, file, strings.TrimSuffix(file, ".tar.gz"))
		if restoreErr != nil {
			return restoreErr
		}
		expected, readErr := os.ReadFile(filepath.Join(directory, file+".verified-files"))
		if readErr != nil {
			return readErr
		}
		if string(result) != string(expected) {
			return errors.New("恢复后文件校验不一致，应用保持停止，请使用安全备份恢复")
		}
	}
	return nil
}
