package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/double-cloth-studio/PageHub/backend/internal/auth"
	"github.com/double-cloth-studio/PageHub/backend/internal/control"
	"github.com/double-cloth-studio/PageHub/backend/internal/presentation"
	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
)

func main() {
	root := os.Getenv("PAGEHUB_ROOT")
	if root == "" {
		root = "/srv/pagehub"
	}
	if len(os.Args) > 1 {
		store := registry.Store{Root: root}
		switch os.Args[1] {
		case "set-password":
			password, err := bufio.NewReader(os.Stdin).ReadString('\n')
			if err != nil {
				log.Fatal("请从标准输入提供密码")
			}
			if err = auth.SetPassword(store.Path("secrets", "admin-password.json"), strings.TrimSpace(password)); err != nil {
				log.Fatal(err)
			}
			fmt.Println("管理密码已更新，重启 Manager 将撤销现有会话")
		case "approve-offline":
			if len(os.Args) != 4 {
				log.Fatal("用法: pagehub-manager approve-offline <app-id> <验收记录.json>")
			}
			app, err := store.Get(os.Args[2])
			if err != nil {
				log.Fatal(err)
			}
			data, err := os.ReadFile(os.Args[3])
			if err != nil {
				log.Fatal(err)
			}
			var record presentation.OfflineRecord
			if json.Unmarshal(data, &record) != nil || !record.Passed || record.AppID != app.Manifest.ID || record.Fingerprint != registry.Fingerprint(app) || record.Target == "" || record.Evidence == "" || record.VerifiedAt.IsZero() {
				log.Fatal("离线验收记录须通过测试、匹配当前登记指纹并包含目标系统、证据及时间")
			}
			if err = registry.WriteJSON(store.Path("state", "offline", app.Manifest.ID+".json"), record, 0640); err != nil {
				log.Fatal(err)
			}
			fmt.Println("离线审核记录已保存，请重新加载网关以更新静态兜底")
		case "offline-fingerprint":
			if len(os.Args) != 3 {
				log.Fatal("需要应用 ID")
			}
			app, err := store.Get(os.Args[2])
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println(registry.Fingerprint(app))
		default:
			log.Fatal("未知命令")
		}
		return
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
