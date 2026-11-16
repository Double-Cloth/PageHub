package auth

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestPasswordSessionCSRFAndLogout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "password.json")
	if err := SetPassword(path, "valid-test-password"); err != nil {
		t.Fatal(err)
	}
	var store Store
	if _, _, err := store.Login(path, "wrong", "127.0.0.1:5000"); err == nil {
		t.Fatal("错误密码被接受")
	}
	id, csrf, err := store.Login(path, "valid-test-password", "127.0.0.1:5000")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("PUT", "http://localhost/admin/api/portal", nil)
	req.AddCookie(&http.Cookie{Name: "pagehub_admin", Value: id})
	if store.Authorized(req) {
		t.Fatal("缺少 CSRF 被接受")
	}
	req.Header.Set("X-CSRF-Token", csrf)
	if !store.Authorized(req) {
		t.Fatal("有效会话被拒绝")
	}
	req.Header.Set("Origin", "http://evil")
	if store.Authorized(req) {
		t.Fatal("跨来源会话被接受")
	}
	req.Header.Del("Origin")
	store.Logout(req)
	if store.Authorized(req) {
		t.Fatal("退出后会话仍有效")
	}
}
