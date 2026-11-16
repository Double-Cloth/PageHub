package importer

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"strings"
	"time"
)

const MaxArchive = 64 << 20
const MaxExpanded = 256 << 20
const MaxFiles = 10000

type extractor struct {
	root  *os.Root
	seen  map[string]bool
	names map[string]string
	bytes int64
	count int
}

func (e *extractor) add(ctx context.Context, name string, size int64, directory bool, executable bool, reader io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	name = strings.TrimSuffix(name, "/")
	if name == "" || strings.ContainsAny(name, "\\:\x00\r\n") || strings.HasPrefix(name, "/") || path.Clean(name) != name || name == ".." || strings.HasPrefix(name, "../") || strings.Count(name, "/") > 20 {
		return errors.New("归档路径非法")
	}
	key := strings.ToLower(name)
	parts := strings.Split(name, "/")
	for index := range parts {
		prefix := strings.Join(parts[:index+1], "/")
		folded := strings.ToLower(prefix)
		if previous, exists := e.names[folded]; exists && previous != prefix {
			return errors.New("归档目录存在大小写冲突")
		}
		e.names[folded] = prefix
	}
	if e.seen[key] {
		return errors.New("归档包含重复路径或大小写冲突")
	}
	e.seen[key] = true
	e.count++
	if e.count > MaxFiles || size < 0 || size > MaxExpanded-e.bytes {
		return errors.New("归档超过文件数或解压大小限制")
	}
	e.bytes += size
	base := strings.ToLower(path.Base(name))
	if base == ".env" || strings.HasPrefix(base, ".env.") && base != ".env.example" || strings.HasSuffix(base, ".pem") || strings.HasSuffix(base, ".key") || base == "id_rsa" || base == ".git" || strings.Contains("/"+key, "/.git/") || strings.HasPrefix(base, "pagehub.") {
		return errors.New("应用包包含秘密文件或平台保留文件")
	}
	if directory {
		return e.mkdir(name)
	}
	if err := e.mkdir(path.Dir(name)); err != nil {
		return err
	}
	mode := os.FileMode(0644)
	if executable {
		mode = 0755
	}
	f, err := e.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if err = f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	n, copyErr := io.CopyN(f, contextReader{ctx, reader}, size)
	closeErr := f.Close()
	if copyErr != nil || n != size {
		return errors.New("归档文件长度不一致")
	}
	return closeErr
}

func (e *extractor) mkdir(name string) error {
	if name == "." {
		return nil
	}
	if err := e.root.MkdirAll(name, 0755); err != nil {
		return err
	}
	parts := strings.Split(name, "/")
	for index := range parts {
		if err := e.root.Chmod(strings.Join(parts[:index+1], "/"), 0755); err != nil {
			return err
		}
	}
	return nil
}

func Extract(ctx context.Context, archive, destination string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	f, err := os.Open(archive)
	if err != nil {
		return "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() > MaxArchive || st.Size() == 0 {
		return "", errors.New("归档大小或类型无效，压缩包最大 64 MiB")
	}
	h := sha256.New()
	if _, err = io.Copy(h, io.LimitReader(f, MaxArchive+1)); err != nil {
		return "", err
	}
	digest := hex.EncodeToString(h.Sum(nil))
	if _, err = f.Seek(0, 0); err != nil {
		return "", err
	}
	root, err := os.OpenRoot(destination)
	if err != nil {
		return "", err
	}
	defer root.Close()
	e := extractor{root: root, seen: map[string]bool{}, names: map[string]string{}}
	if strings.HasSuffix(strings.ToLower(archive), ".zip") {
		z, err := zip.NewReader(f, st.Size())
		if err != nil {
			return "", err
		}
		if len(z.File) > MaxFiles {
			return "", errors.New("归档文件过多")
		}
		for _, file := range z.File {
			if file.Mode()&os.ModeType != 0 && !file.FileInfo().IsDir() {
				return "", errors.New("归档禁止链接与特殊文件")
			}
			if file.UncompressedSize64 > uint64(MaxExpanded) || file.UncompressedSize64 > max(file.CompressedSize64, 1)*200 {
				return "", errors.New("归档压缩比过高")
			}
			r, err := file.Open()
			if err != nil {
				return "", err
			}
			err = e.add(ctx, file.Name, int64(file.UncompressedSize64), file.FileInfo().IsDir(), file.Mode().Perm()&0111 != 0, r)
			if err == nil {
				var extra [1]byte
				n, readErr := r.Read(extra[:])
				if n != 0 || readErr != io.EOF {
					err = errors.New("ZIP 完整性校验失败")
				}
			}
			r.Close()
			if err != nil {
				return "", err
			}
		}
	} else if strings.HasSuffix(strings.ToLower(archive), ".tar.gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return "", err
		}
		defer gz.Close()
		tr := tar.NewReader(io.LimitReader(gz, MaxExpanded+MaxFiles*1024+1))
		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", err
			}
			if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeDir {
				return "", errors.New("TAR 禁止链接与特殊文件")
			}
			if err = e.add(ctx, header.Name, header.Size, header.Typeflag == tar.TypeDir, header.Mode&0111 != 0, tr); err != nil {
				return "", err
			}
			if e.bytes > st.Size()*200 {
				return "", errors.New("归档压缩比过高")
			}
		}
		var remaining []byte
		if remaining, err = io.ReadAll(io.LimitReader(contextReader{ctx, gz}, (1<<20)+1)); err != nil {
			return "", err
		}
		if len(remaining) > 1<<20 || strings.Trim(string(remaining), "\x00") != "" {
			return "", errors.New("TAR 尾部数据无效或超过限制")
		}
	} else {
		return "", errors.New("仅接受 .zip 或 .tar.gz")
	}
	if !e.seen["app.yaml"] {
		return "", errors.New("归档根目录缺少 app.yaml")
	}
	for name := range e.seen {
		if strings.HasSuffix(name, "/app.yaml") {
			return "", errors.New("包内仅允许根目录 app.yaml")
		}
	}
	return digest, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
