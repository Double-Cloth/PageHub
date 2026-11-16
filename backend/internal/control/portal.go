package control

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/double-cloth-studio/PageHub/backend/internal/auth"
	"github.com/double-cloth-studio/PageHub/backend/internal/presentation"
	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
)

func (m *Manager) portalRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /admin/api/session", func(w http.ResponseWriter, r *http.Request) {
		if !auth.SafeOrigin(r) {
			Fail(w, 403, "请求来源不允许")
			return
		}
		var body struct {
			Password string `json:"password"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body) != nil {
			Fail(w, 400, "密码请求无效")
			return
		}
		id, csrf, err := m.Sessions.Login(m.Store.Path("secrets", "admin-password.json"), body.Password, r.RemoteAddr)
		if err != nil {
			Fail(w, 401, err.Error())
			return
		}
		auth.Cookie(w, r, id, 1800)
		JSON(w, 200, map[string]string{"csrf": csrf})
	})
	mux.HandleFunc("POST /admin/api/session/logout", func(w http.ResponseWriter, r *http.Request) {
		if !m.authorized(w, r) {
			return
		}
		m.Sessions.Logout(r)
		auth.Cookie(w, r, "", -1)
		JSON(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /admin/api/portal", func(w http.ResponseWriter, r *http.Request) {
		if !m.authorized(w, r) {
			return
		}
		settings, err := presentation.Load(m.Store)
		if err != nil {
			Fail(w, 503, "读取展示配置失败")
			return
		}
		JSON(w, 200, settings)
	})
	mux.HandleFunc("PUT /admin/api/portal", func(w http.ResponseWriter, r *http.Request) {
		if !m.authorized(w, r) {
			return
		}
		var settings presentation.Settings
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256*1024))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&settings) != nil {
			Fail(w, 400, "展示配置格式无效")
			return
		}
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			Fail(w, 400, "展示配置格式无效")
			return
		}
		result, err := presentation.Save(m.Store, settings)
		if errors.Is(err, presentation.ErrConflict) {
			Fail(w, 409, err.Error())
			return
		}
		if err != nil {
			Fail(w, 400, err.Error())
			return
		}
		m.recordPortalAudit("portal.update", "portal", "succeeded")
		JSON(w, 200, result)
	})
	socialURL, _ := url.Parse("http://social")
	proxy := httputil.NewSingleHostReverseProxy(socialURL)
	proxy.Transport = &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", "/run/pagehub-social/admin.sock")
	}}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		Fail(w, 503, "弹幕管理服务暂不可用")
	}
	proxy.ModifyResponse = func(response *http.Response) error {
		state := "succeeded"
		if response.StatusCode >= 400 {
			state = "error"
		}
		m.recordPortalAudit("social.manage", response.Request.URL.Path, state)
		return nil
	}
	publicProxy := httputil.NewSingleHostReverseProxy(socialURL)
	publicProxy.Transport = &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", "/run/pagehub-social/public.sock")
	}}
	publicProxy.ErrorHandler = proxy.ErrorHandler
	publicProxy.FlushInterval = -1
	mux.HandleFunc("/_pagehub/social/", func(w http.ResponseWriter, r *http.Request) {
		if !auth.SafeOrigin(r) {
			Fail(w, 403, "请求来源不允许")
			return
		}
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		r.Header.Set("X-PageHub-Client", host)
		if r.URL.Path == "/_pagehub/social/events" {
			_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
		}
		publicProxy.ServeHTTP(w, r)
	})
	mux.HandleFunc("/admin/api/social/", func(w http.ResponseWriter, r *http.Request) {
		if !m.authorized(w, r) {
			return
		}
		r.Header.Del("Authorization")
		r.Header.Del("Cookie")
		proxy.ServeHTTP(w, r)
	})
}

func (m *Manager) recordPortalAudit(action, target, state string) {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		log.Printf("生成审计标识失败: %v", err)
		return
	}
	id := hex.EncodeToString(data)
	now := time.Now().UTC()
	entry := Job{ID: id, AppID: target, Action: action, State: state, Actor: "local-admin", CreatedAt: now, UpdatedAt: now}
	if err := registry.WriteJSON(m.Store.Path("state", "manager", "audit", id+".json"), entry, 0600); err != nil {
		log.Printf("持久化管理审计失败: %v", err)
	}
}
