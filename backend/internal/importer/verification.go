package importer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// 卷更新必须引用程序生成的演练结果，并重新核对归档内容。
func CheckBackupVerification(maintenance, id, digest string) error {
	data, err := os.ReadFile(maintenance)
	if err != nil {
		return err
	}
	var approval struct {
		Backup string `json:"backup"`
	}
	if err = json.Unmarshal(data, &approval); err != nil {
		return err
	}
	directory := filepath.Dir(approval.Backup)
	data, err = os.ReadFile(filepath.Join(directory, "verification.json"))
	if err != nil {
		return err
	}
	var result struct {
		ID       string            `json:"id"`
		Digest   string            `json:"digest"`
		Verified bool              `json:"restore_verified"`
		Files    map[string]string `json:"files"`
	}
	if json.Unmarshal(data, &result) != nil || result.ID != id || result.Digest != digest || !result.Verified || len(result.Files) == 0 {
		return errors.New("缺少与当前版本匹配的恢复演练结果")
	}
	for name, expected := range result.Files {
		if filepath.Base(name) != name || strings.ContainsAny(name, "/\\") {
			return errors.New("演练清单路径无效")
		}
		f, err := os.Open(filepath.Join(directory, name))
		if err != nil {
			return err
		}
		hash := sha256.New()
		_, err = io.Copy(hash, f)
		f.Close()
		if err != nil {
			return err
		}
		if hex.EncodeToString(hash.Sum(nil)) != expected {
			return errors.New("演练后备份内容已变化")
		}
	}
	return nil
}
