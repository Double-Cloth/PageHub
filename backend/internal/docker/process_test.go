//go:build !windows

package docker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTimeoutTerminatesComposeChildren(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\n(sleep 1; echo done > child-completed) &\nwait\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := (Client{}).Run(ctx, dir, "compose", "up"); err == nil {
		t.Fatal("超时命令未失败")
	}
	time.Sleep(1200 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(dir, "child-completed")); !os.IsNotExist(err) {
		t.Fatal("超时后 Compose 子进程仍继续执行")
	}
}
