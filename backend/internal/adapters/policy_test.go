package adapters

import (
	"testing"
)

func TestDangerousCompose(t *testing.T) {
	for _, field := range []string{"privileged", "network_mode", "pid", "ipc", "devices", "cap_add", "ports", "entrypoint", "extends", "env_file", "post_start", "pre_stop", "profiles"} {
		t.Run(field, func(t *testing.T) {
			config := map[string]any{"services": map[string]any{"web": map[string]any{"image": "test:1", field: true}}}
			if err := CheckRaw(config); err == nil {
				t.Fatal("危险字段未被拒绝")
			}
		})
	}
}

func TestMountPolicy(t *testing.T) {
	for _, mount := range []map[string]any{
		{"type": "bind", "source": "/var/run/docker.sock", "target": "/var/run/docker.sock", "read_only": true},
		{"type": "bind", "source": "/", "target": "/host", "read_only": true},
		{"type": "bind", "source": "/app/file", "target": "/config", "read_only": false},
		{"type": "volume", "target": "/data"},
	} {
		config := map[string]any{"services": map[string]any{"web": map[string]any{"image": "test:1", "volumes": []any{mount}}}}
		if err := CheckResolved(config, "/app", "pagehub-demo", true); err == nil {
			t.Fatal("危险挂载未被拒绝")
		}
	}
}

func TestNormalizedEntrypointDefault(t *testing.T) {
	config := map[string]any{"services": map[string]any{"web": map[string]any{"image": "test:1", "entrypoint": nil}}}
	if err := CheckRaw(config); err != nil {
		t.Fatal(err)
	}
}
