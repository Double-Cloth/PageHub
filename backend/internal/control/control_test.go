package control

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Double-Cloth/PageHub/backend/internal/manifests"
	"github.com/Double-Cloth/PageHub/backend/internal/registry"
)

func TestAdminAuthentication(t *testing.T) {
	root := t.TempDir()
	tokenFile := filepath.Join(root, "token")
	os.WriteFile(tokenFile, []byte("test-only-token"), 0600)
	m := Manager{Store: registry.Store{Root: root}, TokenFile: tokenFile, Frontend: root}
	handler := m.Admin()
	for _, tc := range []struct {
		token, origin string
		status        int
	}{{"", "", 401}, {"wrong", "", 401}, {"test-only-token", "http://evil.example", 403}, {"test-only-token", "", 200}} {
		r := httptest.NewRequest("GET", "http://localhost/admin/api/apps", nil)
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("得到 %d，预期 %d", w.Code, tc.status)
		}
	}
}

func TestPublicPrivacy(t *testing.T) {
	s := registry.Store{Root: t.TempDir()}
	if err := s.Save(registry.Registry{SchemaVersion: 1, Apps: []registry.Record{{Manifest: manifests.Manifest{ID: "secret", Visibility: "private"}}}}); err != nil {
		t.Fatal(err)
	}
	m := Manager{Store: s}
	r := httptest.NewRequest(http.MethodGet, "http://localhost/_pagehub/api/apps/secret/status", nil)
	w := httptest.NewRecorder()
	m.Public().ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatal("私有应用状态泄漏")
	}
}
