package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/double-cloth-studio/PageHub/backend/internal/adapters"
	"github.com/double-cloth-studio/PageHub/backend/internal/gateway"
	"github.com/double-cloth-studio/PageHub/backend/internal/importer"
	"github.com/double-cloth-studio/PageHub/backend/internal/manifests"
	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
)

type Service struct {
	Store   registry.Store
	Adapter adapters.Builtin
	Gateway gateway.Gateway
}

func (s Service) Execute(ctx context.Context, p adapters.ExecutionPlan) (err error) {
	defer func() {
		if !manifests.IDPattern.MatchString(p.ID) {
			return
		}
		state := "succeeded"
		message := ""
		if err != nil {
			state = "error"
			message = err.Error()
		}
		_ = registry.WriteJSON(s.Store.Path("state", "operations", fmt.Sprintf("%d.json", time.Now().UnixNano())), map[string]any{"id": p.ID, "action": p.Action, "state": state, "error": message, "at": time.Now().UTC()}, 0600)
	}()
	if !validAction(p.Action) {
		return errors.New("动作不在白名单")
	}
	a, err := s.Store.Get(p.ID)
	if err != nil {
		return err
	}
	if err = s.Store.Validate(a); err != nil {
		return err
	}
	if p.Fingerprint != registry.Fingerprint(a) {
		return errors.New("执行计划已过期")
	}
	if !a.Manifest.Allows(p.Action) {
		return errors.New("应用未授权此操作")
	}
	unlock, err := registry.Lock(s.Store.Path("state", "locks", p.ID+".lock"))
	if err != nil {
		return err
	}
	defer unlock()
	// 取得锁后再次核验，避免更新与生命周期任务交叉。
	current, err := s.Store.Get(p.ID)
	if err != nil {
		return err
	}
	if registry.Fingerprint(current) != p.Fingerprint {
		return errors.New("执行计划已过期")
	}
	if p.Action == "stop" || p.Action == "restart" {
		if err = s.Adapter.Stop(ctx, a); err != nil {
			return err
		}
	}
	if p.Action == "start" || p.Action == "restart" {
		return s.Adapter.Start(ctx, a)
	}
	return nil
}

func validAction(action string) bool {
	return action == "start" || action == "stop" || action == "restart"
}

func (s Service) Recover(ctx context.Context, force, check bool) error {
	r, err := s.Store.Load()
	if err != nil {
		return err
	}
	failures := []error{}
	for _, a := range r.Apps {
		if err = s.Store.Validate(a); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", a.Manifest.ID, err))
			continue
		}
		status := s.Adapter.Inspect(ctx, a)
		fmt.Printf("%s\t%s\t%s\n", a.Manifest.ID, a.Manifest.Startup, status.State)
		if check {
			if a.Manifest.Startup == "always" && status.State != "healthy" && a.Manifest.Visibility == "public" {
				failures = append(failures, fmt.Errorf("%s 尚未健康", a.Manifest.ID))
			}
			continue
		}
		if a.Manifest.Startup != "always" || a.Manifest.Kind == "static" {
			continue
		}
		unlock, lockErr := registry.Lock(s.Store.Path("state", "locks", a.Manifest.ID+".lock"))
		if lockErr != nil {
			failures = append(failures, lockErr)
			continue
		}
		var operationErr error
		current, getErr := s.Store.Get(a.Manifest.ID)
		if getErr != nil || registry.Fingerprint(current) != registry.Fingerprint(a) {
			unlock()
			failures = append(failures, errors.New("恢复过程中登记版本发生变化，请重试"))
			continue
		}
		if force {
			operationErr = s.Adapter.Stop(ctx, a)
		}
		if operationErr == nil {
			operationErr = s.Adapter.Start(ctx, a)
		}
		if operationErr == nil && a.Manifest.Visibility == "public" {
			operationErr = (importer.Importer{Store: s.Store, Adapter: s.Adapter, Gateway: s.Gateway}).WaitHealthy(ctx, a)
		}
		unlock()
		if operationErr != nil {
			failures = append(failures, fmt.Errorf("%s: %w", a.Manifest.ID, operationErr))
		}
	}
	return errors.Join(failures...)
}

func (s Service) Remove(ctx context.Context, id string) error {
	a, err := s.Store.Get(id)
	if err != nil {
		return err
	}
	if err = s.Store.Validate(a); err != nil {
		return err
	}
	u, err := registry.Lock(s.Store.Path("state", "locks", id+".lock"))
	if err != nil {
		return err
	}
	defer u()
	g, err := registry.Lock(s.Store.Path("state", "routes.lock"))
	if err != nil {
		return err
	}
	defer g()
	r, err := s.Store.Load()
	if err != nil {
		return err
	}
	next := registry.Registry{SchemaVersion: 1, Apps: []registry.Record{}}
	for _, entry := range r.Apps {
		if entry.Manifest.ID != id {
			next.Apps = append(next.Apps, entry)
		}
	}
	if err = s.Gateway.Validate(ctx, next); err != nil {
		return err
	}
	if err = s.Gateway.Commit(ctx, next); err != nil {
		_ = s.Gateway.Commit(ctx, r)
		return err
	}
	if a.Manifest.Kind != "static" && a.Manifest.Kind != "external" && a.Manifest.Kind != "external-compose" {
		if _, err = s.Adapter.Docker.Compose(ctx, a, "down", "--timeout", "30"); err != nil {
			_ = s.Gateway.Commit(ctx, r)
			return err
		}
	}
	return s.Store.Save(next)
}

func (s Service) Rollback(ctx context.Context, id, receipt string) (err error) {
	a, err := s.Store.Get(id)
	if err != nil {
		return err
	}
	if err = s.Store.Validate(a); err != nil {
		return err
	}
	if a.Previous == nil {
		return errors.New("没有可回退的版本")
	}
	if a.Manifest.Kind != "static" && a.Manifest.Kind != "external" && a.Manifest.Kind != "external-compose" {
		if err = importer.CheckMaintenance(receipt, id, a.Digest); err != nil {
			return err
		}
	}
	u, err := registry.Lock(s.Store.Path("state", "locks", id+".lock"))
	if err != nil {
		return err
	}
	defer u()
	g, err := registry.Lock(s.Store.Path("state", "routes.lock"))
	if err != nil {
		return err
	}
	defer g()
	r, err := s.Store.Load()
	if err != nil {
		return err
	}
	oldRegistry := registry.Registry{SchemaVersion: r.SchemaVersion, Apps: append([]registry.Record{}, r.Apps...)}
	previous := *a.Previous
	if err = s.Store.Validate(previous); err != nil {
		return err
	}
	previous.UpdatedAt = time.Now().UTC()
	a.Previous = nil
	previous.Previous = &a
	for index := range r.Apps {
		if r.Apps[index].Manifest.ID == id {
			r.Apps[index] = previous
		}
	}
	if err = s.Gateway.Validate(ctx, r); err != nil {
		return err
	}
	changed := false
	defer func() {
		if err == nil {
			return
		}
		restoreCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if changed {
			_ = s.Adapter.Stop(restoreCtx, previous)
			_ = s.Adapter.Start(restoreCtx, a)
		}
		_ = s.Gateway.Commit(restoreCtx, oldRegistry)
		_ = s.Store.Save(oldRegistry)
	}()
	if a.Manifest.Kind != "static" && a.Manifest.Kind != "external" && a.Manifest.Kind != "external-compose" {
		changed = true
		if err = s.Adapter.Stop(ctx, a); err != nil {
			return err
		}
		if previous.Manifest.Startup == "always" {
			if err = s.Adapter.Start(ctx, previous); err != nil {
				return err
			}
		}
	}
	if err = s.Gateway.Commit(ctx, r); err != nil {
		return err
	}
	if previous.Manifest.Startup == "always" && previous.Manifest.Visibility == "public" {
		if err = (importer.Importer{Store: s.Store, Adapter: s.Adapter, Gateway: s.Gateway}).WaitHealthy(ctx, previous); err != nil {
			return err
		}
	}
	return s.Store.Save(r)
}

func (s Service) Doctor(ctx context.Context) error {
	if _, err := os.Stat(s.Store.Path("state", "install-state.json")); err != nil {
		return errors.New("安装状态缺失，请先执行 install.sh")
	}
	if _, err := s.Adapter.Docker.Run(ctx, "", "info"); err != nil {
		return err
	}
	return s.Recover(ctx, false, true)
}
