package importer

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/double-cloth-studio/PageHub/backend/internal/adapters"
	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
)

func TestDryRunDoesNotCreateProductionFiles(t *testing.T) {
	manifest := `schema_version: 1
id: static-test
name: 测试
version: 1.0.0
kind: static
path: /test/
startup: always
visibility: public
route_mode: strip_prefix
source:
  static_dir: dist
lifecycle:
  allow: []
backup:
  mode: none
`
	archive := filepath.Join(t.TempDir(), "app.zip")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(f)
	for name, body := range map[string]string{"app.yaml": manifest, "dist/index.html": "hello"} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		entry.Write([]byte(body))
	}
	writer.Close()
	f.Close()
	root := filepath.Join(t.TempDir(), "absent-production")
	i := Importer{Store: registry.Store{Root: root}, Adapter: adapters.Builtin{}}
	plan, err := i.Import(context.Background(), archive, Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Manifest.ID != "static-test" || len(plan.SHA256) != 64 {
		t.Fatal("导入计划无效")
	}
	if _, err = i.Import(context.Background(), archive, Options{DryRun: true, ExpectedID: "another-app"}); err == nil {
		t.Fatal("目标 ID 不匹配时应拒绝")
	}
	if _, err = os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("dry-run 创建了正式目录")
	}
	if _, err = i.Import(context.Background(), archive, Options{ApprovedSHA256: "wrong"}); err == nil {
		t.Fatal("未经匹配摘要审批的包被接受")
	}
	if _, err = os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("审批失败创建了正式目录")
	}
}
