package adapters

import (
	"github.com/double-cloth-studio/PageHub/backend/internal/manifests"
	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
	"testing"
)

func TestContainerStatus(t *testing.T) {
	tests := []struct {
		name    string
		items   []map[string]any
		state   string
		running bool
	}{
		{"empty", nil, "stopped", false},
		{"exited", []map[string]any{{"State": "exited"}}, "stopped", false},
		{"unhealthy", []map[string]any{{"State": "running", "Health": "unhealthy"}}, "unhealthy", true},
		{"health-starting", []map[string]any{{"State": "running", "Health": "starting"}}, "starting", true},
		{"restarting", []map[string]any{{"State": "restarting"}}, "starting", true},
		{"running", []map[string]any{{"State": "running"}}, "running", true},
		{"mixed", []map[string]any{{"State": "running", "Health": "unhealthy"}, {"State": "exited"}}, "stopped", true},
		{"mixed-reverse", []map[string]any{{"State": "exited"}, {"State": "running", "Health": "unhealthy"}}, "stopped", true},
		{"invalid", []map[string]any{{"Status": "Up"}}, "unknown", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containerStatus("demo", tt.items)
			if got.State != tt.state || got.Running != tt.running {
				t.Fatalf("%+v", got)
			}
		})
	}
}

func TestExternalSubpath(t *testing.T) {
	a := registry.Record{Manifest: manifests.Manifest{Path: "/demo/", RouteMode: "strip_prefix", Health: manifests.Health{Path: "/"}}}
	for _, tt := range []struct{ response, state string }{
		{"HTTP/1.1 200 OK\n<html></html>", "ok"},
		{"HTTP/1.1 404 Not Found", "warning"},
		{"wget: bad address", "warning"},
		{"HTTP/1.1 200 OK\n<script src=\"/assets/app.js\">", "warning"},
		{"HTTP/1.1 200 OK\n<link href='/demo/style.css'>", "ok"},
		{"HTTP/1.1 200 OK\n<script src=/assets/app.js>", "warning"},
		{"HTTP/1.1 200 OK\n<a href=/>", "warning"},
		{"HTTP/1.1 302 Found\n Location: /login", "warning"},
		{"HTTP/1.1 302 Found\n Location: https://example.org/login", "warning"},
		{"HTTP/1.1 302 Found\n Location: /demo/login", "ok"},
	} {
		if got := checkExternalSubpath(a, tt.response); got.State != tt.state {
			t.Fatalf("%q: %+v", tt.response, got)
		}
	}
	a.Manifest.RouteMode = "preserve_prefix"
	if got := externalEntryPath(a); got != "/demo/" {
		t.Fatal(got)
	}
	a.Manifest.RouteMode = "strip_prefix"
	if got := externalEntryPath(a); got != "/" {
		t.Fatal(got)
	}
}
