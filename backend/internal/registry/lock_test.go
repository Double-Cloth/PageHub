//go:build !windows

package registry

import (
	"path/filepath"
	"testing"
)

func TestMutationLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.lock")
	unlock, err := Lock(path)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := Lock(path); err == nil {
		second()
		t.Fatal("同一应用允许并发变更")
	}
	unlock()
	third, err := Lock(path)
	if err != nil {
		t.Fatal(err)
	}
	third()
}
