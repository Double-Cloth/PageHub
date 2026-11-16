package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"time"

	"github.com/double-cloth-studio/PageHub/backend/internal/manifests"
)

type Record struct {
	Manifest      manifests.Manifest `json:"manifest"`
	Digest        string             `json:"digest"`
	Directory     string             `json:"directory"`
	Project       string             `json:"project"`
	UpdatedAt     time.Time          `json:"updated_at"`
	Previous      *Record            `json:"previous,omitempty"`
	Adopted       bool               `json:"adopted,omitempty"`
	StaticRestore string             `json:"static_restore,omitempty"`
}

// ContentDirectory 返回当前静态内容目录，旧登记继续使用原版本产物。
func (a Record) ContentDirectory() string {
	if a.StaticRestore != "" {
		return filepath.Join(filepath.Dir(filepath.Dir(a.Directory)), "restores", a.StaticRestore)
	}
	return a.Directory
}

type Registry struct {
	SchemaVersion int      `json:"schema_version"`
	Apps          []Record `json:"apps"`
}

type Store struct{ Root string }

func (s Store) Path(parts ...string) string {
	return filepath.Join(append([]string{s.Root}, parts...)...)
}

func (s Store) Load() (Registry, error) {
	var r Registry
	b, err := os.ReadFile(s.Path("config", "apps.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return Registry{SchemaVersion: 1, Apps: []Record{}}, nil
	}
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(b, &r)
	if err == nil && r.SchemaVersion != 1 {
		err = errors.New("注册表版本无效")
	}
	return r, err
}

func (s Store) Save(r Registry) error {
	sort.Slice(r.Apps, func(i, j int) bool { return r.Apps[i].Manifest.ID < r.Apps[j].Manifest.ID })
	return WriteJSON(s.Path("config", "apps.yaml"), r, 0640)
}

func (s Store) Get(id string) (Record, error) {
	r, err := s.Load()
	if err != nil {
		return Record{}, err
	}
	for _, a := range r.Apps {
		if a.Manifest.ID == id {
			return a, nil
		}
	}
	return Record{}, os.ErrNotExist
}

func (s Store) Validate(a Record) error {
	if err := a.Manifest.Validate(os.Getenv("PAGEHUB_DEV_IMAGES") == "true"); err != nil {
		return err
	}
	validProject := a.Project == "pagehub-"+a.Manifest.ID
	if a.Adopted {
		validProject = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`).MatchString(a.Project) && a.Project != "pagehub-gateway" && a.Project != "pagehub-portal"
	}
	if !validProject || len(a.Digest) != 64 {
		return errors.New("注册目标或摘要无效")
	}
	expected := s.Path("apps", a.Manifest.ID, "releases", a.Digest)
	if filepath.Clean(a.Directory) != expected {
		return errors.New("应用目录超出登记边界")
	}
	real, err := filepath.EvalSymlinks(a.Directory)
	if err != nil || real != expected {
		return errors.New("应用目录含软链接或不存在")
	}
	if a.StaticRestore != "" {
		if a.Manifest.Kind != "static" || !regexp.MustCompile(`^restore-[0-9]+$`).MatchString(a.StaticRestore) {
			return errors.New("静态恢复目录无效")
		}
		content := a.ContentDirectory()
		real, err := filepath.EvalSymlinks(content)
		if err != nil || real != content {
			return errors.New("静态恢复目录含软链接或不存在")
		}
	}
	b, err := os.ReadFile(filepath.Join(a.Directory, "app.yaml"))
	if err != nil {
		return err
	}
	m, err := manifests.Decode(b, os.Getenv("PAGEHUB_DEV_IMAGES") == "true")
	if err != nil {
		return err
	}
	left, _ := json.Marshal(m)
	right, _ := json.Marshal(a.Manifest)
	if string(left) != string(right) {
		return errors.New("应用清单与登记版本不一致")
	}
	return nil
}

func Fingerprint(a Record) string {
	a.Previous = nil
	b, _ := json.Marshal(a)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func AtomicWrite(name string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(name), 0750); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(name), ".pending-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(tmp, name); err != nil {
		return fmt.Errorf("原子提交失败: %w", err)
	}
	d, err := os.Open(filepath.Dir(name))
	if err != nil {
		return err
	}
	defer d.Close()
	if runtime.GOOS == "windows" {
		return nil
	}
	return d.Sync()
}

func WriteJSON(name string, v any, mode os.FileMode) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWrite(name, append(b, '\n'), mode)
}
