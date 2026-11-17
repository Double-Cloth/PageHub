package presentation

import (
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/double-cloth-studio/PageHub/backend/internal/manifests"
	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
)

func TestOfflineApprovalBoundToCurrentVersion(t *testing.T) {
	store := registry.Store{Root: t.TempDir()}
	app := registry.Record{Manifest: manifests.Manifest{ID: "demo", Visibility: "public"}, Digest: "first"}
	if OfflineReady(store, app) {
		t.Fatal("未审核项目被公开")
	}
	record := OfflineRecord{AppID: "demo", Fingerprint: registry.Fingerprint(app), Target: "Ubuntu 24.04 amd64", Evidence: "出口阻断与核心功能验收报告", Passed: true, VerifiedAt: time.Now()}
	if err := registry.WriteJSON(store.Path("state", "offline", "demo.json"), record, 0640); err != nil {
		t.Fatal(err)
	}
	if !OfflineReady(store, app) {
		t.Fatal("审核记录未被接受")
	}
	app.Digest = "new-version"
	if OfflineReady(store, app) {
		t.Fatal("更新版本继承了旧审核")
	}
}

func TestManagedDisplayAdmission(t *testing.T) {
	store := registry.Store{Root: t.TempDir()}
	app := registry.Record{Manifest: manifests.Manifest{ID: "managed", Visibility: "public"}, Managed: &registry.Managed{}}
	if !DisplayReady(store, app) || OfflineReady(store, app) {
		t.Fatal("托管公开登记的展示准入状态无效")
	}
	app.Disabled = true
	if DisplayReady(store, app) {
		t.Fatal("已停用路由仍进入展示目录")
	}
	app.Disabled = false
	app.Manifest.Visibility = "private"
	if DisplayReady(store, app) {
		t.Fatal("私有托管应用被公开展示")
	}
}
func TestSettingsRevisionConflict(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("沿用平台 Linux flock 互斥")
	}
	store := registry.Store{Root: t.TempDir()}
	first, err := Load(store)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := Save(store, first)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Revision != 1 {
		t.Fatal("版本号未增加")
	}
	if _, err = Save(store, first); !errors.Is(err, ErrConflict) {
		t.Fatalf("未拒绝并发覆盖 %v", err)
	}
	saved.Profile.AvatarPath = "https://remote/avatar.webp"
	if _, err = Save(store, saved); err == nil {
		t.Fatal("接受了外部必要资源")
	}
}
