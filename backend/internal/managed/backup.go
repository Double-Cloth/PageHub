package managed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/double-cloth-studio/PageHub/backend/internal/manifests"
	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
)

func composeArgs(a registry.Record, args ...string) []string {
	base := []string{"compose", "--project-name", a.Project, "--project-directory", a.Directory, "-f", filepath.Join(a.Directory, "pagehub.runtime.json")}
	return append(base, args...)
}

func (s Service) initialize(ctx context.Context, a registry.Record) error {
	if _, err := s.Legacy.Adapter.Docker.Compose(ctx, a, "run", "--rm", "--no-deps", "--pull", "never", "-T", "app", "node", "-e", `require("/opt/travel-diary/js/server.js").ensureDataStructure("/var/lib/travel-diary").catch(()=>process.exit(1))`); err != nil {
		return err
	}
	_, err := s.Legacy.Adapter.Docker.Compose(ctx, a, "run", "--rm", "--no-deps", "--pull", "never", "-T", "app", "node", "-e", `process.exit(require("node:fs").existsSync("/var/lib/travel-diary/.secrets/auth.json")?0:1)`)
	if err == nil {
		return nil
	}
	// 原生工具从终端读取密码，密码不经过参数、配置和日志。
	cmd := exec.CommandContext(ctx, "docker", composeArgs(a, "run", "--rm", "--no-deps", "--pull", "never", "app", "node", "/opt/travel-diary/scripts/set-auth-password.js", "--dir", "/var/lib/travel-diary")...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	for _, key := range []string{"PATH", "HOME", "DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_CONFIG", "SYSTEMROOT"} {
		if value, ok := os.LookupEnv(key); ok {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	if err = cmd.Run(); err != nil {
		return errors.New("日记管理员密码初始化失败，请使用交互终端重新执行 apply")
	}
	return nil
}

type BackupReceipt struct {
	ID        string    `json:"id"`
	Digest    string    `json:"digest"`
	Commit    string    `json:"commit"`
	Image     string    `json:"image"`
	Volume    string    `json:"volume"`
	Verified  bool      `json:"verified"`
	CreatedAt time.Time `json:"created_at"`
}

func (s Service) native(ctx context.Context, a registry.Record, dir string, args ...string) error {
	if _, err := s.Legacy.Adapter.Docker.Run(ctx, "", "volume", "inspect", a.Project+"_runtime"); err != nil {
		return errors.New("runtime 卷缺失，拒绝创建空数据卷")
	}
	base := []string{"run", "--rm", "--pull", "never", "--network", "none", "--read-only", "--user", "1000:1000", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true", "--tmpfs", "/tmp:rw,nosuid,nodev", "--mount", "type=volume,src=" + a.Project + "_runtime,dst=/var/lib/travel-diary", "--mount", "type=bind,src=" + dir + ",dst=/backups", a.Managed.Image, "node", "/opt/travel-diary/scripts/deployment-backup.js"}
	_, err := s.Legacy.Adapter.Docker.Run(ctx, "", append(base, args...)...)
	return err
}

func (s Service) Backup(ctx context.Context, id string) (string, error) {
	global, err := registry.Lock(s.Legacy.Store.Path("state", "platform.lock"))
	if err != nil {
		return "", err
	}
	defer global()
	current, err := s.Legacy.Store.Get(id)
	if err != nil {
		return "", err
	}
	if err = s.checkRecord(current); err != nil {
		return "", err
	}
	if current.Manifest.Kind == "static" {
		dir, err := s.Legacy.Backup(ctx, id)
		if err != nil {
			return "", err
		}
		return dir, s.Legacy.VerifyBackup(ctx, id, dir)
	}
	u, err := registry.Lock(s.Legacy.Store.Path("state", "locks", id+".lock"))
	if err != nil {
		return "", err
	}
	defer u()
	a, err := s.Legacy.Store.Get(id)
	if err != nil {
		return "", err
	}
	if err = s.checkRecord(a); err != nil {
		return "", err
	}
	return s.backupLocked(ctx, a, true)
}

func (s Service) backupLocked(ctx context.Context, a registry.Record, resume bool) (destination string, err error) {
	if a.Managed == nil || a.Manifest.Kind != "compose" {
		return "", errors.New("该驱动没有持久化数据备份")
	}
	status := s.Legacy.Adapter.Inspect(ctx, a)
	if status.State == "error" {
		return "", errors.New("日记实际容器与托管登记不一致，请先核对原数据卷与镜像")
	}
	if err = s.Legacy.Adapter.Stop(ctx, a); err != nil {
		return "", err
	}
	if resume && status.Running && !a.Disabled {
		defer func() {
			restartCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
			defer cancel()
			err = errors.Join(err, s.Legacy.Adapter.Start(restartCtx, a))
		}()
	}
	dest := s.Legacy.Store.Path("backups", a.Manifest.ID, "travel-diary-"+time.Now().UTC().Format("20060102T150405-000000000"))
	if err = os.MkdirAll(dest, 0700); err != nil {
		return "", err
	}
	if err = os.Chown(dest, 1000, 1000); err != nil {
		return "", err
	}
	if err = s.native(ctx, a, dest, "backup", "/var/lib/travel-diary", "/backups/data"); err != nil {
		return "", err
	}
	if err = s.native(ctx, a, dest, "verify", "/backups/data"); err != nil {
		return "", err
	}
	receipt := BackupReceipt{ID: a.Manifest.ID, Digest: a.Digest, Commit: a.Managed.Commit, Image: a.Managed.Image, Volume: a.Project + "_runtime", Verified: true, CreatedAt: time.Now().UTC()}
	if err = registry.WriteJSON(filepath.Join(dest, "receipt.json"), receipt, 0600); err != nil {
		return "", err
	}
	return dest, nil
}

func (s Service) backupReceipt(id, dir string) (BackupReceipt, error) {
	var receipt BackupReceipt
	base := s.Legacy.Store.Path("backups", id)
	if filepath.Dir(filepath.Clean(dir)) != base || !regexp.MustCompile(`^travel-diary-[0-9T-]+$`).MatchString(filepath.Base(dir)) {
		return receipt, errors.New("备份须位于该应用的受控备份目录")
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil || real != dir {
		return receipt, errors.New("备份路径无效或包含链接")
	}
	b, err := os.ReadFile(filepath.Join(dir, "receipt.json"))
	if err != nil {
		return receipt, err
	}
	if json.Unmarshal(b, &receipt) != nil || receipt.ID != id || !receipt.Verified || !manifests.ValidImage(receipt.Image, false) {
		return receipt, errors.New("备份收据无效")
	}
	return receipt, nil
}

func (s Service) VerifyBackup(ctx context.Context, id, dir string) error {
	r, err := s.backupReceipt(id, dir)
	if err != nil {
		return err
	}
	a, err := s.Legacy.Store.Get(id)
	if err != nil {
		return err
	}
	if a.Managed == nil || r.Volume != a.Project+"_runtime" {
		return errors.New("备份卷与应用不一致")
	}
	copy := *a.Managed
	copy.Image = r.Image
	a.Managed = &copy
	return s.native(ctx, a, dir, "verify", "/backups/data")
}

func (s Service) Restore(ctx context.Context, id, dir string, confirmed bool) error {
	if !confirmed {
		return errors.New("恢复会替换日记及密码，需要二次确认")
	}
	global, err := registry.Lock(s.Legacy.Store.Path("state", "platform.lock"))
	if err != nil {
		return err
	}
	defer global()
	u, err := registry.Lock(s.Legacy.Store.Path("state", "locks", id+".lock"))
	if err != nil {
		return err
	}
	defer u()
	a, err := s.Legacy.Store.Get(id)
	if err != nil {
		return err
	}
	if err = s.checkRecord(a); err != nil {
		return err
	}
	if err = s.VerifyBackup(ctx, id, dir); err != nil {
		return err
	}
	if _, err = s.backupLocked(ctx, a, false); err != nil {
		return err
	}
	if err = s.Legacy.Adapter.Stop(ctx, a); err != nil {
		return err
	}
	if err = s.native(ctx, a, dir, "restore", "/var/lib/travel-diary", "/backups/data", "/backups/pre-restore-"+fmt.Sprint(time.Now().UnixNano()), "--confirm"); err != nil {
		return errors.New("恢复失败，应用保持停止，请检查原生备份及维护记录")
	}
	if !a.Disabled {
		return s.Legacy.Adapter.Start(ctx, a)
	}
	return nil
}

func (s Service) Purge(ctx context.Context, id, backup, confirmation string) error {
	global, err := registry.Lock(s.Legacy.Store.Path("state", "platform.lock"))
	if err != nil {
		return err
	}
	defer global()
	if confirmation != id+"_runtime" && !strings.HasSuffix(confirmation, "_runtime") {
		return errors.New("请确认实际数据卷名")
	}
	a, err := s.Legacy.Store.Get(id)
	if err != nil {
		return err
	}
	if a.Managed == nil || a.Manifest.Kind != "compose" || confirmation != a.Project+"_runtime" {
		return errors.New("确认的卷名与应用不一致")
	}
	if err = s.VerifyBackup(ctx, id, backup); err != nil {
		return err
	}
	if err = s.uninstallLocked(ctx, id); err != nil {
		return err
	}
	_, err = s.Legacy.Adapter.Docker.Run(ctx, "", "volume", "rm", confirmation)
	return err
}
