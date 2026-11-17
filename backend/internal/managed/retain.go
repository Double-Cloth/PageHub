package managed

import (
	"os"
	"path/filepath"
	"sort"
)

// 版本清理始终保留当前登记和回滚引用。
func (s Service) retain(count int) error {
	r, err := s.Legacy.Store.Load()
	if err != nil {
		return err
	}
	for _, a := range r.Apps {
		if a.Managed == nil {
			continue
		}
		root := s.Legacy.Store.Path("apps", a.Manifest.ID, "releases")
		entries, err := os.ReadDir(root)
		if err != nil {
			return err
		}
		versions := []os.FileInfo{}
		for _, entry := range entries {
			if !entry.IsDir() || len(entry.Name()) != 64 {
				continue
			}
			if _, err = os.Stat(filepath.Join(root, entry.Name(), "managed.json")); err != nil {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			versions = append(versions, info)
		}
		sort.Slice(versions, func(i, j int) bool { return versions[i].ModTime().After(versions[j].ModTime()) })
		for _, version := range versions[min(count, len(versions)):] {
			if version.Name() == a.Digest || a.Previous != nil && version.Name() == a.Previous.Digest {
				continue
			}
			target := filepath.Join(root, version.Name())
			real, err := filepath.EvalSymlinks(target)
			if err != nil {
				return err
			}
			if real != target {
				continue
			}
			if err = os.RemoveAll(target); err != nil {
				return err
			}
		}
	}
	return nil
}
