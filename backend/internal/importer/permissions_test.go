//go:build !windows

package importer

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestArchivePermissionsUnderRestrictedUmask(t *testing.T) {
	previous := syscall.Umask(0027)
	defer syscall.Umask(previous)
	file := filepath.Join(t.TempDir(), "valid.zip")
	f, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(f)
	for name, mode := range map[string]os.FileMode{"app.yaml": 0644, "src/build.sh": 0755} {
		header := &zip.FileHeader{Name: name}
		header.SetMode(mode)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		entry.Write([]byte("test"))
	}
	writer.Close()
	f.Close()
	dest := t.TempDir()
	if _, err := Extract(context.Background(), file, dest); err != nil {
		t.Fatal(err)
	}
	for name, expected := range map[string]os.FileMode{"app.yaml": 0644, "src": 0755, "src/build.sh": 0755} {
		st, err := os.Stat(filepath.Join(dest, name))
		if err != nil || st.Mode().Perm() != expected {
			t.Fatalf("%s 权限错误: %v %v", name, st, err)
		}
	}
}
