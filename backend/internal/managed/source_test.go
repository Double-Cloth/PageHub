package managed

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/double-cloth-studio/PageHub/backend/internal/desired"
)

func TestGitSourceFreeze(t *testing.T) {
	if os.Getenv("PAGEHUB_TEST_GIT_NETWORK") != "true" {
		t.Skip("显式启用后验证真实 GitHub 源码获取")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	commit, err := remoteCommit(ctx, desired.StaticRepository, "main")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err = checkout(ctx, filepath.Join(root, "cache"), desired.StaticRepository, "main", commit, source, []string{"dist/Whalgebra.html"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(source, "dist", "Whalgebra.html"))
	if err != nil || !info.Mode().IsRegular() || info.Size() < 100 {
		t.Fatal("冻结源码未生成有效静态产物", err)
	}
	if _, err = os.Stat(filepath.Join(source, ".git")); !os.IsNotExist(err) {
		t.Fatal("发布产物包含 Git 元数据")
	}
	t.Log("已冻结真实 Whalgebra main commit", commit)
}
