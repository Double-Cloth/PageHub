package managed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/double-cloth-studio/PageHub/backend/internal/desired"
	"github.com/double-cloth-studio/PageHub/backend/internal/lifecycle"
	"github.com/double-cloth-studio/PageHub/backend/internal/manifests"
	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
)

func configuration(t *testing.T) desired.Config {
	t.Helper()
	c, err := desired.Load("../../../config/pagehub.yaml.example")
	if err != nil {
		t.Fatal(err)
	}
	c.Apps = c.Apps[1:]
	return c
}

func TestPlanIdempotenceAndRouteDiff(t *testing.T) {
	c := configuration(t)
	s := Service{Legacy: lifecycle.Service{Store: registry.Store{Root: t.TempDir()}}}
	c.Platform.Root = s.Legacy.Store.Root
	// 生产配置使用 Linux 路径，Windows 仅运行与绝对目录无关的校验。
	if filepath.Separator == '\\' {
		t.Skip("部署差异测试需要 Linux 路径")
	}
	c.Platform.SourceDir = c.Platform.Root + "/sources/pagehub"
	a := c.Apps[0]
	r := registry.Record{Manifest: manifests.Manifest{ID: a.ID, Path: a.Path}, Project: project(a), Digest: strings.Repeat("a", 64), Managed: &registry.Managed{Config: a, Origin: c.Platform.PublicOrigin, Commit: strings.Repeat("b", 40), ConfigSHA256: configDigest(a, c.Platform.PublicOrigin)}}
	r.Manifest = manifests.Manifest{SchemaVersion: 1, ID: a.ID, Name: a.Name, Version: r.Managed.Commit, Kind: "static", Path: a.Path, Startup: "always", Visibility: "public", RouteMode: "strip_prefix", Source: manifests.Source{StaticDir: "dist", Fallback: "index.html"}, Health: manifests.Health{Path: "/", TimeoutSeconds: 5, IntervalSeconds: 15}}
	r.Manifest.Backup.Mode = "external"
	r.Directory = s.Legacy.Store.Path("apps", a.ID, "releases", r.Digest)
	content := []byte("<!doctype html><title>Test</title>")
	sum := sha256.Sum256(content)
	r.Managed.AssetSHA256 = hex.EncodeToString(sum[:])
	if err := registry.AtomicWrite(filepath.Join(r.Directory, "dist", "index.html"), content, 0644); err != nil {
		t.Fatal(err)
	}
	if err := registry.WriteJSON(filepath.Join(r.Directory, "app.yaml"), r.Manifest, 0644); err != nil {
		t.Fatal(err)
	}
	if err := s.Legacy.Store.Save(registry.Registry{SchemaVersion: 1, Apps: []registry.Record{r}}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(s.Legacy.Store.Path("config", "apps.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Plan(context.Background(), c, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if p.Changes[0].Action != "noop" || p.Changes[0].TargetCommit != r.Managed.Commit {
		t.Fatal(p)
	}
	second, err := s.Plan(context.Background(), c, false, false)
	if err != nil || second.SHA256 != p.SHA256 {
		t.Fatal("计划摘要不稳定", err)
	}
	p.Config.Apps = append([]desired.App(nil), p.Config.Apps...)
	c.Apps[0].Path = "/calculator/"
	changed, err := s.Plan(context.Background(), c, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Changes[0].Action != "configure" || changed.SHA256 == p.SHA256 {
		t.Fatal("未识别配置差异")
	}
	after, err := os.ReadFile(s.Legacy.Store.Path("config", "apps.yaml"))
	if err != nil || string(before) != string(after) {
		t.Fatal("plan 修改了注册表")
	}
	if _, err := os.Stat(s.Legacy.Store.Path("sources")); !os.IsNotExist(err) {
		t.Fatal("plan 写入了源码目录")
	}
	if err = s.Apply(context.Background(), p, "wrong"); err == nil {
		t.Fatal("错误审批摘要被接受")
	}
	s.ConfigPath = s.Legacy.Store.Path("config", "pagehub.yaml")
	if err = s.SaveConfig(c); err != nil {
		t.Fatal(err)
	}
	if err = s.Apply(context.Background(), p, p.SHA256); err == nil || !strings.Contains(err.Error(), "配置已变更") {
		t.Fatal("审批后配置变更未被拒绝", err)
	}
}

func TestCLIRejectsUnknownOptions(t *testing.T) {
	for _, args := range [][]string{{"--unknown"}, {"--config"}, {"one", "two"}} {
		if _, err := parse(args); err == nil {
			t.Fatal(args)
		}
	}
}
