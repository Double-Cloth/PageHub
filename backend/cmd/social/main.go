package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/double-cloth-studio/PageHub/backend/internal/control"
	"github.com/double-cloth-studio/PageHub/backend/internal/social"
)

func main() {
	root := os.Getenv("PAGEHUB_ROOT")
	if root == "" {
		root = "/srv/pagehub"
	}
	origin := os.Getenv("PAGEHUB_PUBLIC_ORIGIN")
	service, err := social.Open(filepath.Join(root, "state", "social", "messages.db"), origin)
	if err != nil {
		log.Fatal(err)
	}
	defer service.Close()
	if len(os.Args) == 3 && os.Args[1] == "backup" {
		if err = service.Backup(os.Args[2]); err != nil {
			log.Fatal(err)
		}
		return
	}
	public, err := control.UnixListener("/run/pagehub-social/public.sock", 0666)
	if err != nil {
		log.Fatal(err)
	}
	admin, err := control.UnixListener("/run/pagehub-social/admin.sock", 0660)
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		server := &http.Server{Handler: service.Admin(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
		log.Fatal(server.Serve(admin))
	}()
	server := &http.Server{Handler: service.Public(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 8192}
	log.Fatal(server.Serve(public))
}
