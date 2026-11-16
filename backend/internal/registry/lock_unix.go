//go:build !windows

package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func Lock(name string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(name), 0750); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0640)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("已有任务正在执行: %w", err)
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
