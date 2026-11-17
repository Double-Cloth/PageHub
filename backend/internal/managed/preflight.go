package managed

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/double-cloth-studio/PageHub/backend/internal/desired"
)

func (s Service) preflightSelf(ctx context.Context, c desired.Config, commit, configFile string) (string, error) {
	request := SelfRequest{Commit: commit, ConfigSHA256: desired.Digest(c)}
	stage, err := s.PrepareSelf(ctx, c, request)
	if err != nil {
		return "", err
	}
	build := exec.CommandContext(ctx, "bash", filepath.Join(stage, "scripts", "build.sh"))
	build.Env = append(os.Environ(), "PAGEHUB_SOURCE="+stage, "PAGEHUB_ROOT="+s.Legacy.Store.Root)
	build.Stdout = os.Stderr
	build.Stderr = os.Stderr
	if err = build.Run(); err != nil {
		return "", errors.New("平台暂存构建失败，应用更新尚未执行")
	}
	validate := exec.CommandContext(ctx, filepath.Join(stage, "bin", "pagehub-runner"), "desired", "validate", "-c", configFile)
	validate.Env = build.Env
	validate.Stdout = os.Stderr
	validate.Stderr = os.Stderr
	if err = validate.Run(); err != nil {
		return "", errors.New("新平台与目标配置或现有注册表不兼容，应用更新尚未执行")
	}
	return stage, nil
}
