//go:build !windows

package lifecycle

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/double-cloth-studio/PageHub/backend/internal/gateway"
	"github.com/double-cloth-studio/PageHub/backend/internal/manifests"
	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
)

func TestStaticRestoreTransaction(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "success"
		if fail {
			name = "reload-failure"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			store := registry.Store{Root: root}
			manifest := []byte("schema_version: 1\nid: demo\nname: 测试\nversion: 1\nkind: static\npath: /demo/\nstartup: always\nvisibility: public\nroute_mode: strip_prefix\nsource:\n  static_dir: dist\nbackup:\n  mode: external\n")
			m, err := manifests.Decode(manifest, false)
			if err != nil {
				t.Fatal(err)
			}
			a := registry.Record{Manifest: m, Digest: strings.Repeat("a", 64), Project: "pagehub-demo", Directory: store.Path("apps", "demo", "releases", strings.Repeat("a", 64))}
			if err = os.MkdirAll(filepath.Join(a.Directory, "dist"), 0755); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(a.Directory, "app.yaml"), manifest, 0644); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(a.Directory, "dist", "index.html"), []byte("original"), 0644); err != nil {
				t.Fatal(err)
			}
			if err = store.Save(registry.Registry{SchemaVersion: 1, Apps: []registry.Record{a}}); err != nil {
				t.Fatal(err)
			}
			backup := filepath.Join(root, "input")
			if err = os.Mkdir(backup, 0700); err != nil {
				t.Fatal(err)
			}
			f, err := os.Create(filepath.Join(backup, "bundle.zip"))
			if err != nil {
				t.Fatal(err)
			}
			w := zip.NewWriter(f)
			for path, body := range map[string]string{"app.yaml": string(manifest), "dist/index.html": "restored"} {
				entry, err := w.Create(path)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = entry.Write([]byte(body)); err != nil {
					t.Fatal(err)
				}
			}
			if err = w.Close(); err != nil {
				t.Fatal(err)
			}
			if err = f.Close(); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(backup, "bundle.zip"))
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(data)
			if err = registry.WriteJSON(filepath.Join(backup, "receipt.json"), backupReceipt{ID: "demo", Digest: a.Digest, Files: map[string]string{"bundle.zip": hex.EncodeToString(sum[:])}}, 0600); err != nil {
				t.Fatal(err)
			}
			script := "#!/bin/sh\ncase \"$1\" in\nps) echo gateway;;\nexec) if [ -f \"$(dirname \"$0\")/fail\" ]; then rm \"$(dirname \"$0\")/fail\"; exit 1; fi;;\nesac\n"
			if err = os.WriteFile(filepath.Join(root, "docker"), []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			if fail {
				if err = os.WriteFile(filepath.Join(root, "fail"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
			service := Service{Store: store, Gateway: gateway.Gateway{Store: store, Image: "test"}}
			err = service.Restore(context.Background(), "demo", backup, false)
			if (err != nil) != fail {
				t.Fatalf("恢复结果 %v", err)
			}
			current, err := store.Get("demo")
			if err != nil {
				t.Fatal(err)
			}
			if (current.StaticRestore == "") != fail {
				t.Fatalf("登记恢复目录 %q", current.StaticRestore)
			}
			original, err := os.ReadFile(filepath.Join(a.Directory, "dist", "index.html"))
			if err != nil || string(original) != "original" {
				t.Fatalf("Release 被修改 %q %v", original, err)
			}
			routes, err := os.ReadFile(store.Path("gateway", "Caddyfile"))
			if err != nil {
				t.Fatal(err)
			}
			if fail && strings.Contains(string(routes), "/restores/") {
				t.Fatal("失败后路由未回退")
			}
			if !fail {
				if err = store.Validate(current); err != nil {
					t.Fatal(err)
				}
				content, err := os.ReadFile(filepath.Join(current.ContentDirectory(), "dist", "index.html"))
				if err != nil || string(content) != "restored" {
					t.Fatalf("恢复内容错误 %q %v", content, err)
				}
			}
		})
	}
}
