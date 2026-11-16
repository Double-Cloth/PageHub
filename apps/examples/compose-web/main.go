package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func redis(args ...string) (string, error) {
	c, err := net.DialTimeout("tcp", "db:6379", 2*time.Second)
	if err != nil {
		return "", err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(2 * time.Second))
	fmt.Fprintf(c, "*%d\r\n", len(args))
	for _, arg := range args {
		fmt.Fprintf(c, "$%d\r\n%s\r\n", len(arg), arg)
	}
	r := bufio.NewReader(c)
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(line, "$") {
		length, err := strconv.Atoi(strings.TrimSpace(line[1:]))
		if err != nil {
			return "", err
		}
		if length < 0 {
			return "", nil
		}
		data := make([]byte, length+2)
		_, err = io.ReadFull(r, data)
		return string(data[:length]), err
	}
	if strings.HasPrefix(line, "-") {
		return "", fmt.Errorf("数据库错误")
	}
	return strings.TrimSpace(line[1:]), nil
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if _, err := redis("PING"); err != nil {
			http.Error(w, "数据库未就绪", 503)
			return
		}
		fmt.Fprint(w, "ok")
	})
	mux.HandleFunc("GET /api/value", func(w http.ResponseWriter, r *http.Request) {
		value, err := redis("GET", "pagehub-example")
		if err != nil {
			http.Error(w, "读取失败", 503)
			return
		}
		fmt.Fprint(w, value)
	})
	mux.HandleFunc("POST /api/value", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024))
		if err != nil {
			http.Error(w, "数据过长", 400)
			return
		}
		if _, err = redis("SET", "pagehub-example", string(body)); err != nil {
			http.Error(w, "写入失败", 503)
			return
		}
		fmt.Fprint(w, "saved")
	})
	mux.HandleFunc("GET /media", func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "sample.txt", time.Unix(0, 0), strings.NewReader("0123456789abcdefghijklmnopqrstuvwxyz"))
	})
	mux.HandleFunc("GET /socket", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "websocket" {
			http.Error(w, "需要 WebSocket", 400)
			return
		}
		key := r.Header.Get("Sec-WebSocket-Key")
		hash := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
		c, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(5 * time.Second))
		fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(hash[:]))
		rw.Flush()
		var header [2]byte
		if _, err = io.ReadFull(rw, header[:]); err != nil {
			return
		}
		size := int(header[1] & 127)
		if size > 125 {
			return
		}
		var mask [4]byte
		if _, err = io.ReadFull(rw, mask[:]); err != nil {
			return
		}
		body := make([]byte, size)
		if _, err = io.ReadFull(rw, body); err != nil {
			return
		}
		for i := range body {
			body[i] ^= mask[i%4]
		}
		rw.Write([]byte{0x81, byte(size)})
		rw.Write(body)
		rw.Flush()
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<!doctype html><html lang=zh-CN><meta charset=utf-8><title>Compose 示例</title><h1>Compose 应用已接入</h1><p>私有数据库通过应用接口访问。</p><a href=api/value>读取持久数据</a><br><a href=/>返回 PageHub</a></html>")
	})
	log.Fatal(http.ListenAndServe(":3000", mux))
}
