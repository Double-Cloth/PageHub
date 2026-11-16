package gateway

import (
	"github.com/Double-Cloth/PageHub/backend/internal/manifests"
	"github.com/Double-Cloth/PageHub/backend/internal/registry"
	"strings"
	"testing"
)

func TestRoutesAndPrivacy(t *testing.T) {
	r := registry.Registry{Apps: []registry.Record{
		{Manifest: manifests.Manifest{ID: "demo", Path: "/demo/", Kind: "image", Visibility: "public", RouteMode: "strip_prefix", Source: manifests.Source{ContainerPort: 80}}},
		{Manifest: manifests.Manifest{ID: "secret", Path: "/secret/", Kind: "image", Visibility: "private", RouteMode: "preserve_prefix"}},
	}}
	s := Render(r)
	for _, expected := range []string{"handle_path /demo/*", "handle /secret/*", "Private application", "503", "/admin/*"} {
		if !strings.Contains(s, expected) {
			t.Fatalf("路由缺少 %s", expected)
		}
	}
	if len(Catalog(r)) != 1 || Catalog(r)[0].ID != "demo" {
		t.Fatal("私有应用泄漏到公开清单")
	}
}
