package managed

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/double-cloth-studio/PageHub/backend/internal/importer"
)

var commitPattern = regexp.MustCompile(`^[a-f0-9]{40}$`)

func git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	// 禁止继承 Git 配置注入、凭据助手、代理命令和交互凭据提示。
	for _, key := range []string{"PATH", "SYSTEMROOT", "TMPDIR", "TEMP"} {
		if value, ok := os.LookupEnv(key); ok {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	if err != nil {
		return nil, errors.New("Git 操作失败，请检查审核仓库、分支和网络")
	}
	return out, nil
}

func remoteCommit(ctx context.Context, repository, branch string) (string, error) {
	out, err := git(ctx, "", "ls-remote", "--exit-code", repository, "refs/heads/"+branch)
	if err != nil {
		return "", err
	}
	parts := strings.Fields(string(out))
	if len(parts) != 2 || !commitPattern.MatchString(parts[0]) || parts[1] != "refs/heads/"+branch {
		return "", errors.New("远程分支未返回唯一 commit")
	}
	return parts[0], nil
}

func checkout(ctx context.Context, cache, repository, branch, commit, destination string, sparse []string) error {
	if !commitPattern.MatchString(commit) {
		return errors.New("必须冻结完整 commit SHA")
	}
	if err := os.MkdirAll(filepath.Dir(cache), 0700); err != nil {
		return err
	}
	if _, err := os.Stat(cache); errors.Is(err, os.ErrNotExist) {
		if _, err = git(ctx, "", "init", "--bare", cache); err != nil {
			return err
		}
	}
	// 直接使用审核过的 URL，忽略缓存中的 remote 设置。
	if _, err := git(ctx, cache, "fetch", "--no-tags", "--depth=1", repository, "refs/heads/"+branch); err != nil {
		return err
	}
	if _, err := git(ctx, cache, "cat-file", "-e", commit+"^{commit}"); err != nil {
		return errors.New("冻结 commit 已不可获取，请重新生成计划")
	}
	if err := os.MkdirAll(destination, 0750); err != nil {
		return err
	}
	archive := filepath.Join(filepath.Dir(destination), fmt.Sprintf("source-%s.zip", commit))
	defer os.Remove(archive)
	args := []string{"archive", "--format=zip", "--output=" + archive, commit}
	if len(sparse) > 0 {
		args = append(args, "--")
		args = append(args, sparse...)
	}
	if _, err := git(ctx, cache, args...); err != nil {
		return err
	}
	_, err := importer.ExtractSource(ctx, archive, destination)
	return err
}
