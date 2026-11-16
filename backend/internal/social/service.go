package social

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	_ "modernc.org/sqlite"
)

type Message struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}
type event struct {
	Kind string
	ID   string
	Data any
}
type Service struct {
	db      *sql.DB
	mu      sync.Mutex
	clients map[chan event]string
	rates   map[string][]time.Time
	enabled bool
	Origin  string
}

func Open(path, origin string) (*Service, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
 CREATE TABLE IF NOT EXISTS messages (id TEXT PRIMARY KEY, content TEXT NOT NULL, created_at TEXT NOT NULL, sender_hash TEXT NOT NULL, visible INTEGER NOT NULL DEFAULT 1);
 CREATE INDEX IF NOT EXISTS messages_time ON messages(created_at);
 CREATE TABLE IF NOT EXISTS settings (id INTEGER PRIMARY KEY, enabled INTEGER NOT NULL);
 INSERT OR IGNORE INTO settings(id,enabled) VALUES(1,1);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	s := &Service{db: db, clients: map[chan event]string{}, rates: map[string][]time.Time{}, Origin: origin, enabled: true}
	if err = db.QueryRow("SELECT enabled FROM settings WHERE id=1").Scan(&s.enabled); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}
func (s *Service) Close() error { return s.db.Close() }

// SQLite 自身生成一致性快照，运行中的 WAL 写入不会造成半份备份。
func (s *Service) Backup(path string) error {
	_, err := s.db.Exec("VACUUM INTO '" + strings.ReplaceAll(path, "'", "''") + "'")
	return err
}
func respond(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
func fail(w http.ResponseWriter, status int, message string) {
	respond(w, status, map[string]string{"error": message})
}
func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func digest(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
func client(r *http.Request) string {
	// 生产仅通过 Unix Socket 接收 Caddy 覆盖写入的客户端地址。
	if ip := r.Header.Get("X-PageHub-Client"); ip != "" {
		return digest(ip)
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return digest(host)
}
func (s *Service) broadcast(value event) {
	for ch := range s.clients {
		select {
		case ch <- value:
		default:
		}
	}
}
func (s *Service) permit(key string, limit int) bool {
	now := time.Now()
	times := s.rates[key]
	fresh := times[:0]
	for _, value := range times {
		if now.Sub(value) < time.Minute {
			fresh = append(fresh, value)
		}
	}
	if len(fresh) >= limit {
		s.rates[key] = fresh
		return false
	}
	s.rates[key] = append(fresh, now)
	if len(s.rates) > 4096 {
		for id, values := range s.rates {
			if len(values) == 0 || now.Sub(values[len(values)-1]) > time.Minute {
				delete(s.rates, id)
			}
		}
	}
	return len(s.rates) <= 8192
}
func (s *Service) Public() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /_pagehub/social/health", func(w http.ResponseWriter, r *http.Request) {
		if s.db.Ping() != nil {
			fail(w, 503, "数据库不可用")
			return
		}
		respond(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /_pagehub/social/messages", s.history)
	mux.HandleFunc("POST /_pagehub/social/messages", s.send)
	mux.HandleFunc("GET /_pagehub/social/events", s.events)
	return mux
}
func (s *Service) history(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	permitted := s.permit("history:"+client(r), 60)
	enabled := s.enabled
	s.mu.Unlock()
	if !permitted {
		fail(w, 429, "读取过于频繁，请稍后重试")
		return
	}
	before := r.URL.Query().Get("before")
	if before == "" {
		before = time.Now().Add(time.Second).UTC().Format(time.RFC3339Nano)
	}
	rows, err := s.db.QueryContext(r.Context(), "SELECT id,content,created_at FROM messages WHERE visible=1 AND created_at < ? AND created_at > ? ORDER BY created_at DESC LIMIT 50", before, time.Now().Add(-30*24*time.Hour).UTC().Format(time.RFC3339Nano))
	if err != nil {
		fail(w, 503, "弹幕历史暂不可用")
		return
	}
	defer rows.Close()
	messages := []Message{}
	for rows.Next() {
		var m Message
		if err = rows.Scan(&m.ID, &m.Content, &m.CreatedAt); err != nil {
			fail(w, 503, "读取历史失败")
			return
		}
		messages = append(messages, m)
	}
	if rows.Err() != nil {
		fail(w, 503, "读取历史失败")
		return
	}
	respond(w, 200, map[string]any{"messages": messages, "enabled": enabled})
}
func (s *Service) send(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	localAdmin := r.Host == "127.0.0.1:8091" && origin == "http://127.0.0.1:8091"
	if (origin == "" || (origin != s.Origin && !localAdmin)) || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		fail(w, 403, "请求来源不允许")
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		fail(w, 415, "需要 JSON 格式")
		return
	}
	var body struct {
		Content string `json:"content"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&body) != nil {
		fail(w, 400, "弹幕格式无效")
		return
	}
	body.Content = strings.TrimSpace(strings.Map(func(char rune) rune {
		if unicode.IsControl(char) {
			return -1
		}
		return char
	}, body.Content))
	if !utf8.ValidString(body.Content) || utf8.RuneCountInString(body.Content) < 1 || utf8.RuneCountInString(body.Content) > 100 {
		fail(w, 400, "弹幕长度须为 1–100 个字符")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.enabled {
		fail(w, 403, "弹幕发布已暂停")
		return
	}
	if !s.permit("send:"+client(r), 5) || !s.permit("global", 120) {
		w.Header().Set("Retry-After", "15")
		fail(w, 429, "发送过于频繁，请稍后重试")
		return
	}
	cookie, err := r.Cookie("pagehub_visitor")
	visitor := ""
	if err == nil && len(cookie.Value) == 32 {
		visitor = cookie.Value
	}
	if visitor == "" {
		visitor = randomID()
		http.SetCookie(w, &http.Cookie{Name: "pagehub_visitor", Value: visitor, Path: "/_pagehub/social", HttpOnly: true, Secure: strings.HasPrefix(s.Origin, "https://"), SameSite: http.SameSiteStrictMode, MaxAge: 86400 * 30})
	}
	sender := digest(visitor)
	var last, duplicate int
	cutoff := time.Now().Add(-15 * time.Second).UTC().Format(time.RFC3339Nano)
	if err = s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM messages WHERE sender_hash=? AND created_at>?", sender, cutoff).Scan(&last); err != nil {
		fail(w, 503, "弹幕服务暂不可用")
		return
	}
	if last > 0 {
		w.Header().Set("Retry-After", "15")
		fail(w, 429, "每 15 秒只能发送一条弹幕")
		return
	}
	_ = s.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM messages WHERE content=? AND created_at>?", body.Content, time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)).Scan(&duplicate)
	if duplicate > 0 {
		fail(w, 429, "相同内容已发送，请稍后再试")
		return
	}
	message := Message{ID: randomID(), Content: body.Content, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, 503, "无法保存弹幕")
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(r.Context(), "INSERT INTO messages(id,content,created_at,sender_hash) VALUES(?,?,?,?)", message.ID, message.Content, message.CreatedAt, sender); err == nil {
		_, err = tx.ExecContext(r.Context(), "DELETE FROM messages WHERE created_at < ? OR id IN (SELECT id FROM messages ORDER BY created_at DESC LIMIT -1 OFFSET 5000)", time.Now().Add(-30*24*time.Hour).UTC().Format(time.RFC3339Nano))
	}
	if err != nil {
		fail(w, 503, "无法保存弹幕")
		return
	}
	if err = tx.Commit(); err != nil {
		fail(w, 503, "无法提交弹幕")
		return
	}
	s.broadcast(event{Kind: "message", ID: message.ID, Data: message})
	respond(w, 201, message)
}
func (s *Service) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		fail(w, 503, "流式连接不可用")
		return
	}
	s.mu.Lock()
	ip := client(r)
	count := 0
	for _, value := range s.clients {
		if value == ip {
			count++
		}
	}
	if len(s.clients) >= 256 || count >= 6 {
		s.mu.Unlock()
		fail(w, 429, "实时连接已达上限")
		return
	}
	ch := make(chan event, 32)
	s.clients[ch] = ip
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.clients, ch); s.mu.Unlock() }()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	_, _ = fmt.Fprint(w, "retry: 5000\n: connected\n\n")
	flusher.Flush()
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(30 * time.Second))
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case value := <-ch:
			data, _ := json.Marshal(value.Data)
			if value.Kind == "delete" {
				data = []byte(value.ID)
			}
			if _, err := fmt.Fprintf(w, "event: %s\nid: %s\ndata: %s\n\n", value.Kind, value.ID, data); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// Admin 仅绑定受文件系统权限保护的独立 Unix Socket。
func (s *Service) Admin() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /admin/api/social/messages/{id}/delete", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		result, err := s.db.ExecContext(r.Context(), "UPDATE messages SET visible=0 WHERE id=?", r.PathValue("id"))
		if err != nil {
			fail(w, 503, "删除失败")
			return
		}
		count, _ := result.RowsAffected()
		if count == 0 {
			fail(w, 404, "弹幕不存在")
			return
		}
		s.broadcast(event{Kind: "delete", ID: r.PathValue("id")})
		respond(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("PUT /admin/api/social/settings", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body) != nil {
			fail(w, 400, "设置无效")
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, err := s.db.ExecContext(r.Context(), "UPDATE settings SET enabled=? WHERE id=1", body.Enabled); err != nil {
			fail(w, 503, "更新失败")
			return
		}
		s.enabled = body.Enabled
		s.broadcast(event{Kind: "settings", Data: body})
		respond(w, 200, body)
	})
	return mux
}
