package social

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func post(service *Service, content string, cookie *http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest("POST", "http://pagehub/_pagehub/social/messages", strings.NewReader(`{"content":`+strconvQuote(content)+`}`))
	request.Header.Set("Origin", "http://pagehub")
	request.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	result := httptest.NewRecorder()
	service.Public().ServeHTTP(result, request)
	return result
}
func strconvQuote(value string) string { data, _ := json.Marshal(value); return string(data) }
func TestMessagesPersistAndEnforceCooldown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "messages.db")
	service, err := Open(path, "http://pagehub")
	if err != nil {
		t.Fatal(err)
	}
	first := post(service, "持久化测试 <script>alert(1)</script>", nil)
	if first.Code != 201 {
		t.Fatalf("发送失败 %d %s", first.Code, first.Body.String())
	}
	cookie := first.Result().Cookies()[0]
	second := post(service, "另一条弹幕", cookie)
	if second.Code != 429 {
		t.Fatalf("冷却未生效 %d", second.Code)
	}
	service.Close()
	service, err = Open(path, "http://pagehub")
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	third := post(service, "重启不能绕过冷却", cookie)
	if third.Code != 429 {
		t.Fatalf("重启后冷却未生效 %d", third.Code)
	}
	history := httptest.NewRecorder()
	service.Public().ServeHTTP(history, httptest.NewRequest("GET", "http://pagehub/_pagehub/social/messages", nil))
	var payload struct {
		Messages []Message `json:"messages"`
	}
	if json.Unmarshal(history.Body.Bytes(), &payload) != nil || len(payload.Messages) != 1 || !strings.Contains(payload.Messages[0].Content, "<script>") {
		t.Fatal("持久化历史不一致")
	}
	id := payload.Messages[0].ID
	deleted := httptest.NewRecorder()
	service.Admin().ServeHTTP(deleted, httptest.NewRequest("POST", "http://social/admin/api/social/messages/"+id+"/delete", nil))
	if deleted.Code != 200 {
		t.Fatal("管理删除失败")
	}
	history = httptest.NewRecorder()
	service.Public().ServeHTTP(history, httptest.NewRequest("GET", "http://pagehub/_pagehub/social/messages", nil))
	_ = json.Unmarshal(history.Body.Bytes(), &payload)
	if len(payload.Messages) != 0 {
		t.Fatal("删除的弹幕仍公开可见")
	}
}
func TestPublicRejectsCrossOriginAndOversizedContent(t *testing.T) {
	service, err := Open(filepath.Join(t.TempDir(), "messages.db"), "http://pagehub")
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	request := httptest.NewRequest("POST", "http://pagehub/_pagehub/social/messages", strings.NewReader(`{"content":"test"}`))
	request.Header.Set("Origin", "http://evil")
	request.Header.Set("Content-Type", "application/json")
	result := httptest.NewRecorder()
	service.Public().ServeHTTP(result, request)
	if result.Code != 403 {
		t.Fatal("跨来源发布未被拒绝")
	}
	if result = post(service, strings.Repeat("中", 101), nil); result.Code != 400 {
		t.Fatal("未限制 Unicode 字符长度")
	}
	result = httptest.NewRecorder()
	service.Public().ServeHTTP(result, httptest.NewRequest("POST", "http://pagehub/admin/api/social/messages/id/delete", nil))
	if result.Code != 404 {
		t.Fatal("公共接口暴露管理删除")
	}
}
