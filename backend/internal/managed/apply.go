package managed

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/double-cloth-studio/PageHub/backend/internal/desired"
	"github.com/double-cloth-studio/PageHub/backend/internal/importer"
	"github.com/double-cloth-studio/PageHub/backend/internal/manifests"
	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
	"gopkg.in/yaml.v3"
)

func (s Service) Apply(ctx context.Context, p Plan, approved string) (err error) {
	if approved == "" || approved != p.SHA256 {
		return errors.New("执行需要与计划精确匹配的 --approved-plan 摘要")
	}
	check := p
	check.SHA256 = ""
	if desired.Digest(check) != p.SHA256 {
		return errors.New("计划摘要不匹配")
	}
	unlock, err := registry.Lock(s.Legacy.Store.Path("state", "platform.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	// 配置及登记在审批后发生变化时，禁止执行旧计划。
	currentConfig, err := s.Config()
	if err != nil {
		return err
	}
	if desired.Digest(currentConfig) != desired.Digest(p.Config) {
		return errors.New("配置已变更，请重新生成计划")
	}
	latest, err := s.Plan(ctx, p.Config, p.Adopt, false)
	if err != nil {
		return err
	}
	for _, change := range p.Changes {
		for _, current := range latest.Changes {
			if change.ID == current.ID && change.Fingerprint != current.Fingerprint {
				return errors.New("登记状态已变更，请重新生成计划")
			}
		}
	}
	completed := []string{}
	operationFile := s.Legacy.Store.Path("state", "operations", fmt.Sprintf("apply-%d.json", time.Now().UnixNano()))
	actor := "root"
	if s.ExpectedFingerprint != "" {
		actor = "admin"
	}
	boot, _ := os.ReadFile("/proc/sys/kernel/random/boot_id")
	operation := map[string]any{"action": "apply", "state": "running", "plan_sha256": p.SHA256, "changes": p.Changes, "completed": completed, "actor": actor, "pid": os.Getpid(), "boot_id": strings.TrimSpace(string(boot)), "at": time.Now().UTC()}
	if err = registry.WriteJSON(operationFile, operation, 0600); err != nil {
		return err
	}
	defer func() {
		state := "succeeded"
		message := ""
		if err != nil {
			state = "error"
			message = err.Error()
		}
		operation["state"] = state
		operation["error"] = message
		operation["completed"] = completed
		components, loadErr := s.auditComponents()
		operation["components"] = components
		auditErr := registry.WriteJSON(operationFile, operation, 0600)
		err = errors.Join(err, loadErr, auditErr)
	}()
	for _, change := range p.Changes {
		if change.Action == "noop" {
			continue
		}
		var app desired.App
		for _, candidate := range p.Config.Apps {
			if candidate.ID == change.ID {
				app = candidate
			}
		}
		if err = s.applyApp(ctx, app, p.Config.Platform.PublicOrigin, change, p.Adopt); err != nil {
			return fmt.Errorf("%s 失败，已完成 %v，后续组件未执行: %w", change.ID, completed, err)
		}
		completed = append(completed, change.ID)
		operation["completed"] = completed
		if err = registry.WriteJSON(operationFile, operation, 0600); err != nil {
			return err
		}
	}
	if err = s.SaveConfig(p.Config); err != nil {
		return err
	}
	if err = s.WriteVersions(ctx); err != nil {
		return err
	}
	return s.retain(p.Config.Update.RetainReleases)
}

func (s Service) applyApp(ctx context.Context, app desired.App, origin string, change Change, adopt bool) error {
	unlock, err := registry.Lock(s.Legacy.Store.Path("state", "locks", app.ID+".lock"))
	if err != nil {
		return err
	}
	defer unlock()
	old, err := loadRecord(s.Legacy.Store, app.ID)
	if err != nil {
		return err
	}
	if old != nil && registry.Fingerprint(*old) != change.Fingerprint {
		return errors.New("应用计划已过期")
	}
	if change.Action == "disable" {
		if old == nil {
			return nil
		}
		candidate := *old
		candidate.Disabled = true
		return s.switchRelease(ctx, candidate, old)
	}
	if old == nil && app.Deployment.Driver == "managed-compose" {
		existing, err := s.inspectAdoption(ctx, app)
		if err != nil {
			return err
		}
		if existing != nil {
			if !adopt || change.AdoptionSHA256 != desired.Digest(*existing) {
				return errors.New("原项目在审批后发生变化，请重新生成接管计划")
			}
			baseline, err := s.captureAdoption(app, origin, *existing)
			if err != nil {
				return err
			}
			old = &baseline
		}
	}
	if old != nil && old.Managed == nil && app.Deployment.Driver == "managed-compose" {
		existing, err := s.inspectAdoption(ctx, app)
		if err != nil {
			return err
		}
		if existing == nil || desired.Digest(existing) != change.AdoptionSHA256 {
			return errors.New("接管目标已变化")
		}
		baseline, err := s.captureAdoption(app, origin, *existing)
		if err != nil {
			return err
		}
		old = &baseline
	}
	candidate, err := s.prepare(ctx, app, origin, change.TargetCommit)
	if err != nil {
		return err
	}
	candidate.UpdatedAt = time.Now().UTC()
	candidate.Disabled = !app.Enabled
	if app.Startup == "manual" {
		candidate.Disabled = old == nil || old.Disabled
	}
	if old != nil && candidate.Digest != old.Digest {
		previous := *old
		previous.Previous = nil
		candidate.Previous = &previous
	}
	if old != nil && candidate.Digest == old.Digest {
		candidate.Previous = old.Previous
	}
	if candidate.Manifest.Kind == "compose" {
		if app.Settings["TRAVEL_WRITE_MODE"] == "remote" {
			request, err := http.NewRequestWithContext(ctx, "GET", origin+"/healthz", nil)
			if err != nil {
				return err
			}
			client := http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			response, err := client.Do(request)
			if err != nil {
				return errors.New("启用远程写入前 HTTPS 入口须通过证书验证")
			}
			response.Body.Close()
			if response.StatusCode != http.StatusOK {
				return errors.New("公开 HTTPS 网关健康检查未通过")
			}
		}
		if old == nil {
			if err = s.initialize(ctx, candidate); err != nil {
				return err
			}
		} else {
			if candidate.BackupReceipt, err = s.backupLocked(ctx, *old, false); err != nil {
				return fmt.Errorf("升级前原生备份失败，应用保持维护状态: %w", err)
			}
		}
	}
	if err = s.switchRelease(ctx, candidate, old); err != nil && candidate.BackupReceipt != "" {
		return fmt.Errorf("%w；升级前已验证备份 %s", err, candidate.BackupReceipt)
	}
	return err
}

func (s Service) switchRelease(ctx context.Context, candidate registry.Record, old *registry.Record) (err error) {
	unlock, err := registry.Lock(s.Legacy.Store.Path("state", "routes.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	r, err := s.Legacy.Store.Load()
	if err != nil {
		return err
	}
	next := registry.Registry{SchemaVersion: 1, Apps: []registry.Record{}}
	for _, entry := range r.Apps {
		if entry.Manifest.ID == candidate.Manifest.ID {
			continue
		}
		if manifests.Conflict(entry.Manifest, candidate.Manifest) || entry.Project == candidate.Project {
			return errors.New("路由或 Compose 项目已被其他应用占用")
		}
		next.Apps = append(next.Apps, entry)
	}
	next.Apps = append(next.Apps, candidate)
	changed := false
	defer func() {
		if err == nil {
			return
		}
		recovery, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		if !changed && old != nil && old.Manifest.Kind == "compose" && !old.Disabled {
			err = errors.Join(err, s.Legacy.Adapter.Start(recovery, *old))
		}
		if changed && candidate.Manifest.Kind == "compose" {
			stopErr := s.Legacy.Adapter.Stop(recovery, candidate)
			if old != nil && old.Managed != nil && candidate.Managed.Image != old.Managed.Image {
				restoreErr := stopErr
				if restoreErr == nil {
					restoreErr = s.VerifyBackup(recovery, old.Manifest.ID, candidate.BackupReceipt)
				}
				if restoreErr == nil {
					restoreErr = s.native(recovery, *old, candidate.BackupReceipt, "restore", "/var/lib/travel-diary", "/backups/data", fmt.Sprintf("/backups/pre-recovery-%d", time.Now().UnixNano()), "--confirm")
				}
				if restoreErr != nil {
					for i := range r.Apps {
						if r.Apps[i].Manifest.ID == old.Manifest.ID {
							r.Apps[i].Disabled = true
						}
					}
					err = errors.Join(err, stopErr, restoreErr, errors.New("自动数据恢复失败，应用保持维护状态，请核对备份后恢复"))
					old = nil
				}
			}
			if old != nil && !old.Disabled {
				err = errors.Join(err, stopErr, s.Legacy.Adapter.Start(recovery, *old))
			} else {
				err = errors.Join(err, stopErr)
			}
		}
		err = errors.Join(err, s.Legacy.Gateway.Commit(recovery, r), s.Legacy.Store.Save(r))
	}()
	if err = s.Legacy.Gateway.Validate(ctx, next); err != nil {
		return err
	}
	if candidate.Manifest.Kind == "compose" {
		changed = true
		if candidate.Disabled {
			err = s.Legacy.Adapter.Stop(ctx, candidate)
		} else {
			err = s.Legacy.Adapter.Start(ctx, candidate)
		}
		if err != nil {
			return err
		}
	}
	if err = s.Legacy.Gateway.Commit(ctx, next); err != nil {
		return err
	}
	if !candidate.Disabled {
		if err = (importer.Importer{Store: s.Legacy.Store, Adapter: s.Legacy.Adapter, Gateway: s.Legacy.Gateway}).WaitHealthy(ctx, candidate); err != nil {
			return err
		}
	}
	return s.Legacy.Store.Save(next)
}

func (s Service) Rollback(ctx context.Context, id string) error {
	u, err := registry.Lock(s.Legacy.Store.Path("state", "platform.lock"))
	if err != nil {
		return err
	}
	defer u()
	u, err = registry.Lock(s.Legacy.Store.Path("state", "locks", id+".lock"))
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
	if a.Managed == nil || a.Previous == nil {
		return errors.New("没有可回退的托管版本")
	}
	previous := *a.Previous
	if err = s.Legacy.Store.Validate(previous); err != nil {
		return err
	}
	if a.Manifest.Kind == "compose" {
		if previous.BackupReceipt, err = s.backupLocked(ctx, a, false); err != nil {
			return err
		}
	}
	a.Previous = nil
	previous.Previous = &a
	previous.Disabled = a.Disabled
	return s.switchRelease(ctx, previous, &a)
}

func (s Service) Uninstall(ctx context.Context, id string) error {
	u, err := registry.Lock(s.Legacy.Store.Path("state", "platform.lock"))
	if err != nil {
		return err
	}
	defer u()
	return s.uninstallLocked(ctx, id)
}

func (s Service) uninstallLocked(ctx context.Context, id string) error {
	a, err := s.Legacy.Store.Get(id)
	if err != nil {
		return err
	}
	if a.Managed == nil {
		return s.Legacy.Remove(ctx, id)
	}
	if err = s.checkRecord(a); err != nil {
		return err
	}
	// 配置保留禁用条目，再次 apply 不会重新安装已卸载应用。
	c, err := s.Config()
	if err != nil {
		return err
	}
	original := c
	c.Apps = append([]desired.App{}, c.Apps...)
	data, err := os.ReadFile(s.ConfigFile())
	if err != nil {
		return err
	}
	var document yaml.Node
	if err = yaml.Unmarshal(data, &document); err != nil {
		return err
	}
	root := document.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "apps" {
			continue
		}
		for _, node := range root.Content[i+1].Content {
			matches := false
			for j := 0; j+1 < len(node.Content); j += 2 {
				if node.Content[j].Value == "id" && node.Content[j+1].Value == id {
					matches = true
				}
			}
			if matches {
				for j := 0; j+1 < len(node.Content); j += 2 {
					if node.Content[j].Value == "enabled" {
						node.Content[j+1].Value = "false"
					}
				}
			}
		}
	}
	disabled, err := yaml.Marshal(&document)
	if err != nil {
		return err
	}
	for i := range c.Apps {
		if c.Apps[i].ID == id {
			c.Apps[i].Enabled = false
		}
	}
	if err = s.Legacy.Remove(ctx, id); err != nil {
		return err
	}
	if err = s.SaveConfig(c); err != nil {
		return errors.Join(err, s.switchRelease(ctx, a, nil))
	}
	if err = registry.AtomicWrite(s.ConfigFile(), disabled, 0600); err != nil {
		return errors.Join(err, s.SaveConfig(original), s.switchRelease(ctx, a, nil))
	}
	if a.Manifest.Kind == "static" {
		return os.RemoveAll(s.Legacy.Store.Path("apps", id, "releases"))
	}
	return nil
}
