package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
	"golang.org/x/crypto/argon2"
)

type Password struct {
	Salt    string `json:"salt"`
	Hash    string `json:"hash"`
	Version int    `json:"version"`
}
type Session struct {
	CSRF    string
	Expires time.Time
}
type attempt struct {
	Count int
	Since time.Time
}
type Store struct {
	mu       sync.Mutex
	sessions map[string]Session
	attempts map[string]attempt
}

func random() string {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(data)
}
func SetPassword(path, password string) error {
	if len(password) < 12 || len(password) > 256 {
		return errors.New("密码长度须为 12–256 字节")
	}
	salt := random()
	hash := argon2.IDKey([]byte(password), []byte(salt), 3, 64*1024, 2, 32)
	return registry.WriteJSON(path, Password{Salt: salt, Hash: base64.RawStdEncoding.EncodeToString(hash), Version: 1}, 0640)
}
func (s *Store) Login(file string, password string, remote string) (string, string, error) {
	// 串行执行密码验证并限制请求，防止并行 Argon2 耗尽内存。
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions == nil {
		s.sessions = map[string]Session{}
		s.attempts = map[string]attempt{}
	}
	now := time.Now()
	host, _, _ := net.SplitHostPort(remote)
	for key, value := range s.attempts {
		if now.Sub(value.Since) > 10*time.Minute {
			delete(s.attempts, key)
		}
	}
	a := s.attempts[host]
	if a.Count >= 5 || len(s.attempts) > 1024 {
		return "", "", errors.New("登录尝试过多，请在十分钟后重试")
	}
	if a.Count == 0 {
		a.Since = now
	}
	a.Count++
	s.attempts[host] = a
	data, err := os.ReadFile(file)
	if err != nil {
		return "", "", errors.New("尚未设置管理密码，请使用本机命令设置或选择兼容 Token")
	}
	var stored Password
	if json.Unmarshal(data, &stored) != nil || stored.Version != 1 {
		return "", "", errors.New("密码配置无效")
	}
	expected, err := base64.RawStdEncoding.DecodeString(stored.Hash)
	if err != nil || len(password) > 256 {
		return "", "", errors.New("密码验证失败")
	}
	hash := argon2.IDKey([]byte(password), []byte(stored.Salt), 3, 64*1024, 2, 32)
	if subtle.ConstantTimeCompare(hash, expected) != 1 {
		return "", "", errors.New("密码验证失败")
	}
	delete(s.attempts, host)
	for key, value := range s.sessions {
		if now.After(value.Expires) {
			delete(s.sessions, key)
		}
	}
	if len(s.sessions) >= 100 {
		return "", "", errors.New("管理会话已达上限")
	}
	id, csrf := random(), random()
	s.sessions[id] = Session{CSRF: csrf, Expires: now.Add(30 * time.Minute)}
	return id, csrf, nil
}
func SafeOrigin(r *http.Request) bool {
	host := r.Host
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	}
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	origin := r.Header.Get("Origin")
	return (origin == "" || origin == scheme+"://"+r.Host) && r.Header.Get("Sec-Fetch-Site") != "cross-site"
}
func (s *Store) Authorized(r *http.Request) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	cookie, err := r.Cookie("pagehub_admin")
	if err != nil {
		return false
	}
	value, ok := s.sessions[cookie.Value]
	if !ok || time.Now().After(value.Expires) {
		delete(s.sessions, cookie.Value)
		return false
	}
	return SafeOrigin(r) && subtle.ConstantTimeCompare([]byte(value.CSRF), []byte(r.Header.Get("X-CSRF-Token"))) == 1
}
func (s *Store) Logout(r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cookie, err := r.Cookie("pagehub_admin"); err == nil {
		delete(s.sessions, cookie.Value)
	}
}
func Cookie(w http.ResponseWriter, r *http.Request, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: "pagehub_admin", Value: strings.TrimSpace(value), Path: "/", HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: maxAge})
}
