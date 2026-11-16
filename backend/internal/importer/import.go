package importer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/double-cloth-studio/PageHub/backend/internal/adapters"
	"github.com/double-cloth-studio/PageHub/backend/internal/gateway"
	"github.com/double-cloth-studio/PageHub/backend/internal/manifests"
	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
)

type Importer struct {
	Store   registry.Store
	Adapter adapters.Builtin
	Gateway gateway.Gateway
}
type Options struct {
	DryRun             bool
	ApprovedSHA256     string
	Update             bool
	MaintenanceReceipt string
	AdoptProject       string
	ExpectedID         string
}
type Plan struct {
	Preflight []adapters.PreflightCheck `json:"preflight,omitempty"`
	Manifest  manifests.Manifest        `json:"manifest"`
	SHA256    string                    `json:"sha256"`
	Project   string                    `json:"project"`
	Effects   []string                  `json:"effects"`
}

func (i Importer) Import(ctx context.Context, archive string, opts Options) (plan Plan, err error) {
	tmp, err := os.MkdirTemp("", "pagehub-import-")
	if err != nil {
		return plan, err
	}
	defer os.RemoveAll(tmp)
	input, err := os.Open(archive)
	if err != nil {
		return plan, err
	}
	st, err := input.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() > MaxArchive {
		input.Close()
		return plan, errors.New("归档类型或大小无效")
	}
	suffix := ".zip"
	if strings.HasSuffix(strings.ToLower(archive), ".tar.gz") {
		suffix = ".tar.gz"
	} else if !strings.HasSuffix(strings.ToLower(archive), ".zip") {
		input.Close()
		return plan, errors.New("仅接受 .zip 或 .tar.gz")
	}
	immutable := filepath.Join(tmp, "archive"+suffix)
	output, err := os.OpenFile(immutable, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		input.Close()
		return plan, err
	}
	copied, copyErr := io.Copy(output, io.LimitReader(input, MaxArchive+1))
	input.Close()
	closeErr := output.Close()
	if copyErr != nil {
		return plan, copyErr
	}
	if closeErr != nil {
		return plan, closeErr
	}
	if copied > MaxArchive {
		return plan, errors.New("归档超过大小限制")
	}
	content := filepath.Join(tmp, "content")
	if err = os.Mkdir(content, 0700); err != nil {
		return plan, err
	}
	archive = immutable
	digest, err := Extract(ctx, archive, content)
	if err != nil {
		return plan, err
	}
	data, err := os.ReadFile(filepath.Join(content, "app.yaml"))
	if err != nil {
		return plan, err
	}
	m, err := manifests.Decode(data, i.Adapter.DevImages)
	if err != nil {
		return plan, err
	}
	if opts.ExpectedID != "" && m.ID != opts.ExpectedID {
		return plan, errors.New("应用包 ID 与指定目标不一致")
	}
	plan = Plan{Manifest: m, SHA256: digest, Project: "pagehub-" + m.ID, Effects: []string{"可信应用代码将在当前服务器执行", "公开应用与主页共享 Origin", "将创建独立 Compose 项目和持久化卷（如声明）", "首次准备可能访问镜像仓库或执行包内 Dockerfile", "private 路由默认返回 403"}}
	r, err := i.Store.Load()
	if err != nil {
		return plan, err
	}
	var old *registry.Record
	for _, a := range r.Apps {
		if a.Manifest.ID == m.ID {
			copy := a
			old = &copy
			if !opts.Update {
				return plan, errors.New("应用 ID 已登记，请使用 update")
			}
			continue
		}
		if manifests.Conflict(m, a.Manifest) {
			return plan, errors.New("应用 URL 路径冲突")
		}
	}
	if opts.Update && old == nil {
		return plan, errors.New("更新目标不存在")
	}
	if old != nil && old.Manifest.Kind != m.Kind {
		return plan, errors.New("应用类型变更需要独立迁移，禁止在原工程上隐式切换")
	}
	if old != nil {
		plan.Project = old.Project
	}
	if opts.AdoptProject != "" {
		if old != nil || m.Kind != "compose" || !regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`).MatchString(opts.AdoptProject) || opts.AdoptProject == "pagehub-gateway" || opts.AdoptProject == "pagehub-portal" {
			return plan, errors.New("接管目标或应用类型无效")
		}
		plan.Project = opts.AdoptProject
		plan.Effects = append(plan.Effects, "显式接管现有 Compose 工程，保留真实镜像、环境变量和具名卷，将 Web 服务连接共享入口")
	}
	for _, entry := range r.Apps {
		if entry.Project == plan.Project && entry.Manifest.ID != m.ID {
			return plan, errors.New("该 Compose 工程已被其他应用登记")
		}
	}
	a := registry.Record{Manifest: m, Digest: digest, Directory: content, Project: plan.Project, UpdatedAt: time.Now().UTC()}
	if m.Kind == "external" || m.Kind == "external-compose" {
		plan.Effects = []string{"登记应用目录与网关路由", "生命周期操作仅作用于已有容器，备份使用原部署工具"}
		plan.Preflight = i.Adapter.PreflightExternal(ctx, a)
	}
	a.Adopted = opts.AdoptProject != "" || (old != nil && old.Adopted)
	if opts.DryRun {
		// 仅在系统临时目录生成解析产物，正式目录、容器、路由均不变。
		err = i.Adapter.Validate(ctx, a)
		if err == nil && opts.AdoptProject != "" {
			_, err = i.CaptureAdoption(ctx, a)
		}
		return plan, err
	}
	if !strings.EqualFold(opts.ApprovedSHA256, digest) {
		return plan, errors.New("需要核对来源并提供与此归档精确匹配的 --approved-sha256")
	}
	unlock, err := registry.Lock(i.Store.Path("state", "locks", m.ID+".lock"))
	if err != nil {
		return plan, err
	}
	defer unlock()
	if old != nil && m.Kind != "static" && m.Kind != "external" && m.Kind != "external-compose" {
		if err = CheckMaintenance(opts.MaintenanceReceipt, m.ID, old.Digest); err != nil {
			return plan, err
		}
	}
	if old != nil && old.Manifest.Backup.Mode == "volume" {
		if err = CheckBackupVerification(opts.MaintenanceReceipt, m.ID, old.Digest); err != nil {
			return plan, err
		}
	}
	a.Directory = i.Store.Path("apps", m.ID, "releases", digest)
	if _, err = os.Stat(a.Directory); err == nil {
		return plan, errors.New("该版本目录已存在，请检查失败记录或执行 rollback")
	}
	if err = os.MkdirAll(filepath.Dir(a.Directory), 0750); err != nil {
		return plan, err
	}
	// 跨文件系统安全复制，仍通过相同的受限解包流程。
	if err = os.Mkdir(a.Directory, 0755); err != nil {
		return plan, err
	}
	var copiedDigest string
	if copiedDigest, err = Extract(ctx, archive, a.Directory); err != nil || copiedDigest != digest {
		_ = os.RemoveAll(a.Directory)
		if err == nil {
			err = errors.New("归档在审批后发生变化")
		}
		return plan, err
	}
	stage := "STAGED"
	defer func() {
		if err != nil {
			_ = registry.WriteJSON(i.Store.Path("state", "failures", fmt.Sprintf("%s-%d.json", m.ID, time.Now().UnixNano())), map[string]any{"id": m.ID, "stage": stage, "error": err.Error(), "directory": a.Directory, "recovery": "检查 doctor 与登记版本，保留所有持久卷"}, 0600)
		}
	}()
	if err = i.Adapter.Validate(ctx, a); err != nil {
		return plan, err
	}
	if old != nil && old.Adopted {
		if err = PreserveEnvironment(*old, a); err != nil {
			return plan, err
		}
	}
	var adoptionOriginal *registry.Record
	if opts.AdoptProject != "" {
		original, adoptErr := i.CaptureAdoption(ctx, a)
		if adoptErr != nil {
			return plan, adoptErr
		}
		adoptionOriginal = &original
	} else if m.Kind != "static" && m.Kind != "external" && m.Kind != "external-compose" {
		if os.Getenv("PAGEHUB_OFFLINE") != "true" {
			if err = i.PrepareImages(ctx, a); err != nil {
				return plan, err
			}
		}
		if err = i.PinImages(ctx, a); err != nil {
			return plan, err
		}
	}
	stage = "PREPARED"
	shared, err := registry.Lock(i.Store.Path("state", "routes.lock"))
	if err != nil {
		return plan, err
	}
	defer shared()
	r, err = i.Store.Load()
	if err != nil {
		return plan, err
	}
	for _, entry := range r.Apps {
		if entry.Manifest.ID == m.ID {
			if old == nil || entry.Digest != old.Digest {
				return plan, errors.New("登记版本已变更")
			}
		} else if manifests.Conflict(m, entry.Manifest) {
			return plan, errors.New("路径已被其他导入占用")
		}
	}
	next := registry.Registry{SchemaVersion: 1, Apps: []registry.Record{}}
	for _, entry := range r.Apps {
		if entry.Manifest.ID != m.ID {
			next.Apps = append(next.Apps, entry)
		}
	}
	if old != nil {
		previous := *old
		previous.Previous = nil
		a.Previous = &previous
	}
	next.Apps = append(next.Apps, a)
	if err = i.Gateway.Validate(ctx, next); err != nil {
		return plan, err
	}
	started := false
	oldRunning := false
	if old != nil {
		oldRunning = i.Adapter.Inspect(ctx, *old).Running
	}
	defer func() {
		if err == nil {
			return
		}
		recoveryCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if started {
			if adoptionOriginal != nil {
				_ = i.Adapter.Stop(recoveryCtx, a)
				_ = i.Adapter.Start(recoveryCtx, *adoptionOriginal)
			} else if old == nil {
				_, _ = i.Adapter.Docker.Compose(recoveryCtx, a, "down", "--timeout", "30")
			} else {
				_ = i.Adapter.Stop(recoveryCtx, a)
				if oldRunning {
					_ = i.Adapter.Start(recoveryCtx, *old)
				}
			}
		}
		if restoreErr := i.Gateway.Commit(recoveryCtx, r); restoreErr != nil {
			err = fmt.Errorf("%w；旧网关恢复失败，请执行 gateway reload", err)
		}
		if saveErr := i.Store.Save(r); saveErr != nil {
			err = fmt.Errorf("%w；旧注册表恢复失败", err)
		}
	}()
	if old != nil && old.Manifest.Kind != "static" {
		started = true
		if err = i.Adapter.Stop(ctx, *old); err != nil {
			return plan, err
		}
	}
	if (m.Startup == "always" || adoptionOriginal != nil) && m.Kind != "static" && m.Kind != "external" && m.Kind != "external-compose" {
		started = true
		if err = i.Adapter.Start(ctx, a); err != nil {
			return plan, err
		}
	}
	stage = "STARTED"
	if err = i.Gateway.Commit(ctx, next); err != nil {
		return plan, err
	}
	if (m.Startup == "always" || adoptionOriginal != nil) && m.Visibility == "public" {
		if err = i.WaitHealthy(ctx, a); err != nil {
			return plan, err
		}
	}
	if err = i.Store.Save(next); err != nil {
		return plan, err
	}
	stage = "COMMITTED"
	_ = registry.WriteJSON(i.Store.Path("state", "imports", digest+".json"), map[string]any{"stage": stage, "app": a, "sha256": digest}, 0600)
	return plan, nil
}

func (i Importer) WaitHealthy(ctx context.Context, a registry.Record) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	timeout := time.NewTimer(60 * time.Second)
	defer timeout.Stop()
	for {
		if s := i.Adapter.Inspect(ctx, a); s.State == "healthy" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout.C:
			return errors.New("应用健康检查超时，已启动回退流程")
		case <-ticker.C:
		}
	}
}

func CheckMaintenance(file, id, digest string) error {
	if file == "" {
		return errors.New("容器更新需 --maintenance-receipt，记录外部备份、可恢复性和维护窗口")
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	var receipt struct {
		ID              string    `json:"id"`
		Digest          string    `json:"digest"`
		Backup          string    `json:"backup"`
		RestoreVerified bool      `json:"restore_verified"`
		Expires         time.Time `json:"expires_at"`
	}
	if json.Unmarshal(b, &receipt) != nil || receipt.ID != id || receipt.Digest != digest || receipt.Backup == "" || !receipt.RestoreVerified || time.Now().After(receipt.Expires) {
		return errors.New("维护收据无效或已过期")
	}
	return nil
}
