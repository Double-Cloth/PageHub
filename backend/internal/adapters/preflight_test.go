//go:build !windows

package adapters

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/double-cloth-studio/PageHub/backend/internal/manifests"
	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
)

func TestPreflightDNS(t *testing.T) {
	root := t.TempDir()
	// 模拟网关命令，实际执行 DNS 预检使用的 shell 表达式。
	scripts := map[string]string{
		"docker": `#!/bin/sh
case "$1" in
network) exit 0;;
ps) echo gateway;;
inspect) echo '{"pagehub-ingress":{}}';;
exec) shift 2; exec "$@";;
*) exit 1;;
esac
`,
		"nslookup": `#!/bin/sh
case "$1:$2" in
-type=A:ipv4.test|-type=AAAA:ipv6.test) exit 0;;
*) exit 1;;
esac
`,
		"nc":   "#!/bin/sh\nexit 0\n",
		"wget": "#!/bin/sh\nprintf 'HTTP/1.1 200 OK\\n<html></html>\\n'\n",
	}
	for name, script := range scripts {
		if err := os.WriteFile(filepath.Join(root, name), []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, tt := range []struct{ host, state string }{
		{"ipv4.test", "ok"},
		{"ipv6.test", "ok"},
		{"missing.test", "warning"},
	} {
		t.Run(tt.host, func(t *testing.T) {
			a := registry.Record{Manifest: manifests.Manifest{Kind: "external", Path: "/demo/", Source: manifests.Source{Upstream: "http://" + tt.host}}}
			checks := (Builtin{}).PreflightExternal(context.Background(), a)
			for _, check := range checks {
				if check.Name == "dns" {
					if check.State != tt.state {
						t.Fatalf("DNS 预检结果为 %+v，期望 %s", check, tt.state)
					}
					return
				}
			}
			t.Fatal("缺少 DNS 预检结果")
		})
	}
}
