package gateway

import (
	"github.com/double-cloth-studio/PageHub/backend/internal/manifests"
	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
	"strings"
	"testing"
)

func TestStaticRestoreRoute(t *testing.T) {
	a := registry.Record{Digest: "release-digest", Manifest: manifests.Manifest{ID: "demo", Kind: "static", Path: "/demo/", Visibility: "public", Source: manifests.Source{StaticDir: "dist"}}}
	if got := Render(registry.Registry{Apps: []registry.Record{a}}); !strings.Contains(got, "/apps/demo/releases/release-digest/dist") {
		t.Fatal(got)
	}
	a.StaticRestore = "restore-123"
	got := Render(registry.Registry{Apps: []registry.Record{a}})
	if !strings.Contains(got, "/apps/demo/restores/restore-123/dist") || strings.Contains(got, "/releases/") {
		t.Fatal(got)
	}
}
