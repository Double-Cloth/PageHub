package managed

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
)

func (s Service) Operations() ([]map[string]any, error) {
	dir := s.Legacy.Store.Path("state", "operations")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	files := []os.FileInfo{}
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".json") {
			info, err := entry.Info()
			if err != nil {
				return nil, err
			}
			files = append(files, info)
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].ModTime().After(files[j].ModTime()) })
	result := []map[string]any{}
	for _, file := range files[:min(200, len(files))] {
		b, err := os.ReadFile(filepath.Join(dir, file.Name()))
		if err != nil {
			return nil, err
		}
		var record map[string]any
		if json.Unmarshal(b, &record) != nil {
			continue
		}
		if record["state"] == "running" {
			pid, _ := record["pid"].(float64)
			boot, _ := record["boot_id"].(string)
			if operationInterrupted(int(pid), boot) {
				record["state"] = "error"
				record["error"] = "部署进程已中断，请核对实际状态与备份"
			}
		}
		item := map[string]any{"id": strings.TrimSuffix(file.Name(), ".json"), "app_id": record["id"], "created_at": record["at"]}
		for _, key := range []string{"action", "state", "error", "actor", "changes", "completed", "components", "commit", "backup_receipt"} {
			if value, ok := record[key]; ok {
				item[key] = value
			}
		}
		if item["app_id"] == nil {
			item["app_id"] = "self"
		}
		if record["action"] == "update-self" {
			item["app_id"] = "self"
		}
		result = append(result, item)
	}
	return result, nil
}

func operationInterrupted(pid int, bootID string) bool {
	if runtime.GOOS != "linux" || pid <= 0 {
		return false
	}
	boot, _ := os.ReadFile("/proc/sys/kernel/random/boot_id")
	process, err := os.FindProcess(pid)
	return err != nil || process.Signal(syscall.Signal(0)) != nil || bootID != strings.TrimSpace(string(boot))
}

func (s Service) auditComponents() ([]map[string]any, error) {
	r, err := s.Legacy.Store.Load()
	if err != nil {
		return nil, err
	}
	components := []map[string]any{}
	for _, a := range r.Apps {
		if a.Managed != nil {
			components = append(components, map[string]any{"id": a.Manifest.ID, "commit": a.Managed.Commit, "image": a.Managed.Image, "asset_sha256": a.Managed.AssetSHA256, "digest": a.Digest, "backup_receipt": a.BackupReceipt, "disabled": a.Disabled})
		}
	}
	return components, nil
}
