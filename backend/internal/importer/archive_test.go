package importer

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchive(t *testing.T) {
	for _, name := range []string{"../escape", "/absolute", "C:\\escape", "dist/../../escape", "dist\\escape", ".env", "secret.pem", "dist/app.yaml", "pagehub.runtime.json"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "bad.zip")
			f, _ := os.Create(file)
			z := zip.NewWriter(f)
			entry, _ := z.Create("app.yaml")
			entry.Write([]byte("test"))
			entry, _ = z.Create(name)
			entry.Write([]byte("test"))
			z.Close()
			f.Close()
			dest := t.TempDir()
			if _, err := Extract(context.Background(), file, dest); err == nil {
				t.Fatal("恶意归档被接受")
			}
		})
	}
}

func TestArchiveDuplicateAndSymlink(t *testing.T) {
	for _, mode := range []string{"duplicate", "case", "symlink", "bomb"} {
		t.Run(mode, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "bad.zip")
			f, _ := os.Create(file)
			z := zip.NewWriter(f)
			entry, _ := z.Create("app.yaml")
			entry.Write([]byte("test"))
			switch mode {
			case "duplicate":
				entry, _ = z.Create("app.yaml")
			case "case":
				entry, _ = z.Create("APP.yaml")
			case "symlink":
				h := &zip.FileHeader{Name: "link"}
				h.SetMode(os.ModeSymlink | 0777)
				entry, _ = z.CreateHeader(h)
			case "bomb":
				entry, _ = z.Create("huge")
				entry.Write([]byte(strings.Repeat("0", 1<<20)))
			}
			if mode != "bomb" {
				entry.Write([]byte("test"))
			}
			z.Close()
			f.Close()
			if _, err := Extract(context.Background(), file, t.TempDir()); err == nil {
				t.Fatal("应拒绝归档")
			}
		})
	}
}

func TestTarLinks(t *testing.T) {
	file := filepath.Join(t.TempDir(), "bad.tar.gz")
	f, _ := os.Create(file)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "escape", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"})
	tw.Close()
	gz.Close()
	f.Close()
	if _, err := Extract(context.Background(), file, t.TempDir()); err == nil {
		t.Fatal("TAR 链接被接受")
	}
}

func TestArchiveValid(t *testing.T) {
	file := filepath.Join(t.TempDir(), "valid.zip")
	f, _ := os.Create(file)
	z := zip.NewWriter(f)
	for name, body := range map[string]string{"app.yaml": "schema_version: 1", "dist/index.html": "<h1>hello</h1>"} {
		entry, _ := z.Create(name)
		entry.Write([]byte(body))
	}
	z.Close()
	f.Close()
	if digest, err := Extract(context.Background(), file, t.TempDir()); err != nil || len(digest) != 64 {
		t.Fatalf("%s %v", digest, err)
	}
}

func TestArchiveDirectoryCaseConflict(t *testing.T) {
	file := filepath.Join(t.TempDir(), "bad.zip")
	f, _ := os.Create(file)
	z := zip.NewWriter(f)
	for _, name := range []string{"app.yaml", "Dist/one.txt", "dist/two.txt"} {
		entry, _ := z.Create(name)
		entry.Write([]byte("test"))
	}
	z.Close()
	f.Close()
	if _, err := Extract(context.Background(), file, t.TempDir()); err == nil {
		t.Fatal("目录大小写冲突被接受")
	}
}
