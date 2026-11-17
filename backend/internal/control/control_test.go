package control

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/double-cloth-studio/PageHub/backend/internal/manifests"
	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
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

func TestMaintenanceRequiresTargetAndReauthentication(t *testing.T) {
	store := registry.Store{Root: t.TempDir()}
	if err := store.Save(registry.Registry{SchemaVersion: 1, Apps: []registry.Record{{Manifest: manifests.Manifest{ID: "managed-app"}, Managed: &registry.Managed{}}}}); err != nil {
		t.Fatal(err)
	}
	token := store.Path("token")
	if err := os.WriteFile(token, []byte("test-token"), 0600); err != nil {
		t.Fatal(err)
	}
	m := Manager{Store: store, TokenFile: token, Frontend: store.Root}
	handler := m.Admin()
	for _, test := range []struct {
		body   string
		status int
	}{
		{`{}`, 400},
		{`{"confirm_id":"other","password":"value"}`, 400},
		{`{"confirm_id":"managed-app","password":"value","unknown":true}`, 400},
		{`{"confirm_id":"managed-app","password":"value"}`, 401},
	} {
		req := httptest.NewRequest("POST", "http://localhost/admin/api/apps/managed-app/uninstall", strings.NewReader(test.body))
		req.Header.Set("Authorization", "Bearer test-token")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != test.status {
			t.Fatalf("得到 %d，预期 %d", response.Code, test.status)
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
