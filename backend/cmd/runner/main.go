package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Double-Cloth/PageHub/backend/internal/adapters"
	"github.com/Double-Cloth/PageHub/backend/internal/control"
	"github.com/Double-Cloth/PageHub/backend/internal/docker"
	"github.com/Double-Cloth/PageHub/backend/internal/gateway"
	"github.com/Double-Cloth/PageHub/backend/internal/importer"
	"github.com/Double-Cloth/PageHub/backend/internal/lifecycle"
	"github.com/Double-Cloth/PageHub/backend/internal/registry"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
func run() error {
	root := os.Getenv("PAGEHUB_ROOT")
	if root == "" {
		root = "/srv/pagehub"
	}
	if !filepath.IsAbs(root) {
		return errors.New("PAGEHUB_ROOT 必须为绝对路径")
	}
	store := registry.Store{Root: filepath.Clean(root)}
	port := os.Getenv("PAGEHUB_GATEWAY_PORT")
	if port == "" {
		port = "8080"
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1024 || n > 65535 {
		return errors.New("网关端口无效")
	}
	image := os.Getenv("PAGEHUB_CADDY_IMAGE")
	if image == "" {
		image = "caddy:2.10.2-alpine"
	}
	d := docker.Client{}
	adapter := adapters.Builtin{Docker: d, GatewayURL: "http://127.0.0.1:" + port, DevImages: os.Getenv("PAGEHUB_DEV_IMAGES") == "true"}
	g := gateway.Gateway{Store: store, Docker: d, Image: image}
	service := lifecycle.Service{Store: store, Adapter: adapter, Gateway: g}
	args := os.Args[1:]
	if len(args) == 0 {
		return errors.New("需要命令: serve/status/app/add-app/recover/gateway/backup/update/rollback/remove-app/doctor")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	switch args[0] {
	case "serve":
		listener, err := control.UnixListener("/run/pagehub/runner.sock", 0660)
		if err != nil {
			return err
		}
		server := &http.Server{Handler: control.RunnerHandler(service), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 200 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16384}
		return server.Serve(listener)
	case "status":
		r, err := store.Load()
		if err != nil {
			return err
		}
		statuses := []adapters.Status{}
		for _, a := range r.Apps {
			statuses = append(statuses, adapter.Inspect(ctx, a))
		}
		return json.NewEncoder(os.Stdout).Encode(statuses)
	case "doctor":
		return service.Doctor(ctx)
	case "gateway":
		if len(args) != 2 || args[1] != "reload" {
			return errors.New("用法: gateway reload")
		}
		return g.Reload(ctx)
	case "app":
		if len(args) != 3 {
			return errors.New("用法: app start|stop|restart <id>")
		}
		a, err := store.Get(args[2])
		if err != nil {
			return err
		}
		plan, err := adapter.Plan(a, args[1])
		if err != nil {
			return err
		}
		return service.Execute(ctx, plan)
	case "recover":
		f := flag.NewFlagSet("recover", flag.ContinueOnError)
		force := f.Bool("force", false, "")
		check := f.Bool("check", false, "")
		_ = f.Bool("boot", false, "")
		if err = f.Parse(args[1:]); err != nil {
			return err
		}
		return service.Recover(ctx, *force, *check)
	case "add-app", "update", "adopt-app":
		update := args[0] == "update"
		offset := 1
		id := ""
		if update {
			if len(args) < 3 {
				return errors.New("用法: update <id> <package> [选项]")
			}
			id = args[1]
			offset = 2
		}
		if len(args) <= offset {
			return errors.New("缺少应用包路径")
		}
		archive := args[offset]
		f := flag.NewFlagSet("import", flag.ContinueOnError)
		dry := f.Bool("dry-run", false, "")
		sha := f.String("approved-sha256", "", "")
		receipt := f.String("maintenance-receipt", "", "")
		project := f.String("project-name", "", "")
		expectedID := f.String("expected-id", "", "")
		if err = f.Parse(args[offset+1:]); err != nil {
			return err
		}
		imp := importer.Importer{Store: store, Adapter: adapter, Gateway: g}
		if args[0] == "adopt-app" && *project == "" {
			return errors.New("接管需要 --project-name <现有项目名>")
		}
		if args[0] != "adopt-app" && *project != "" {
			return errors.New("--project-name 仅用于显式 adopt-app")
		}
		if update {
			if *expectedID != "" && *expectedID != id {
				return errors.New("指定目标 ID 冲突")
			}
			*expectedID = id
			preview, e := imp.Import(ctx, archive, importer.Options{DryRun: true, Update: true})
			if e != nil {
				return e
			}
			if preview.Manifest.ID != id {
				return errors.New("应用包 ID 与更新目标不一致")
			}
		}
		plan, err := imp.Import(ctx, archive, importer.Options{DryRun: *dry, ApprovedSHA256: *sha, Update: update, MaintenanceReceipt: *receipt, AdoptProject: *project, ExpectedID: *expectedID})
		_ = json.NewEncoder(os.Stdout).Encode(plan)
		return err
	case "backup":
		if len(args) != 2 {
			return errors.New("用法: backup <id>")
		}
		dest, err := service.Backup(ctx, args[1])
		if err == nil {
			fmt.Println(dest)
		}
		return err
	case "rollback":
		if len(args) < 2 {
			return errors.New("用法: rollback <id> [--maintenance-receipt <file>]")
		}
		f := flag.NewFlagSet("rollback", flag.ContinueOnError)
		receipt := f.String("maintenance-receipt", "", "")
		if err = f.Parse(args[2:]); err != nil {
			return err
		}
		return service.Rollback(ctx, args[1], *receipt)
	case "remove-app":
		if len(args) != 2 {
			return errors.New("用法: remove-app <id>")
		}
		return service.Remove(ctx, args[1])
	case "logs":
		if len(args) != 2 {
			return errors.New("用法: logs <id>")
		}
		a, err := store.Get(args[1])
		if err != nil {
			return err
		}
		out, err := d.Compose(ctx, a, "logs", "--tail", "100", "--no-color")
		if err == nil {
			fmt.Print(string(out))
		}
		return err
	default:
		return errors.New("未知命令")
	}
}
