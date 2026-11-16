package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/double-cloth-studio/PageHub/backend/internal/control"
	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
)

func main() {
	root := os.Getenv("PAGEHUB_ROOT")
	if root == "" {
		root = "/srv/pagehub"
	}
	m := control.Manager{Store: registry.Store{Root: root}, Runner: control.RunnerClient{Socket: "/run/pagehub/runner.sock"}, TokenFile: filepath.Join(root, "secrets", "admin.token"), Frontend: filepath.Join(root, "current", "frontend", "dist")}
	listener, err := control.UnixListener("/run/pagehub-public/manager.sock", 0666)
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		server := &http.Server{Handler: m.Public(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
		log.Fatal(server.Serve(listener))
	}()
	server := &http.Server{Addr: "127.0.0.1:8091", Handler: m.Admin(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	log.Fatal(server.ListenAndServe())
}
