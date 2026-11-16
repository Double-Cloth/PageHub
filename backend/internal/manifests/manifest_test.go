package manifests

import (
	"strings"
	"testing"
)

const valid = `schema_version: 1
id: static-demo
name: 测试
version: 1.0.0
kind: static
path: /demo/
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

func TestManifestValidation(t *testing.T) {
	if _, err := Decode([]byte(valid), false); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ name, from, to string }{
		{"保留路径", "/demo/", "/_pagehub/"}, {"管理路径", "/demo/", "/admin/"}, {"根路径", "/demo/", "/"},
		{"非法ID", "id: static-demo", "id: ../../root"}, {"未知字段", "name: 测试", "name: 测试\nprivileged: true"},
		{"目录逃逸", "static_dir: dist", "static_dir: ../secret"}, {"路由注入", "static_dir: dist", "static_dir: 'dist { respond 200 }'"},
		{"非法策略", "startup: always", "startup: manual"}, {"非法路由模式", "strip_prefix", "preserve_prefix"},
		{"健康路径越界", "source:", "health:\n  path: /../_pagehub/api/health\nsource:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Decode([]byte(strings.Replace(valid, tc.from, tc.to, 1)), false); err == nil {
				t.Fatal("应拒绝非法清单")
			}
		})
	}
}

func TestConflicts(t *testing.T) {
	a := Manifest{ID: "one", Path: "/notes/"}
	for _, b := range []Manifest{{ID: "one", Path: "/different/"}, {ID: "two", Path: "/notes/child/"}, {ID: "two", Path: "/notes/"}} {
		if !Conflict(a, b) {
			t.Fatalf("未识别冲突 %#v", b)
		}
	}
	if Conflict(a, Manifest{ID: "two", Path: "/notes-other/"}) {
		t.Fatal("相邻路径误报")
	}
}

func TestChineseCharacterLimits(t *testing.T) {
	data := strings.Replace(valid, "name: 测试", "name: "+strings.Repeat("名", 80)+"\ndescription: "+strings.Repeat("说", 400), 1)
	if _, err := Decode([]byte(data), false); err != nil {
		t.Fatal(err)
	}
}
