package managed

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/double-cloth-studio/PageHub/backend/internal/adapters"
	"github.com/double-cloth-studio/PageHub/backend/internal/desired"
)

func (s Service) SelfPlan(ctx context.Context) (Plan, error) {
	c, err := s.Config()
	if err != nil {
		return Plan{}, err
	}
	if err = s.checkPlatformConfig(c); err != nil {
		return Plan{}, err
	}
	commit, err := remoteCommit(ctx, c.Platform.Repository, c.Platform.Branch)
	if err != nil {
		return Plan{}, err
	}
	current := s.SourceVersion()
	action := "update"
	if current.Commit == commit && !current.Dirty {
		action = "noop"
	}
	p := Plan{Config: c, Changes: []Change{{ID: "self", Action: action, CurrentCommit: current.Commit, TargetCommit: commit, ConfigSHA256: desired.Digest(c), Fingerprint: desired.Digest(current), Path: "/"}}}
	p.SHA256 = desired.Digest(p)
	return p, nil
}

func (s Service) executePlatform(ctx context.Context, p adapters.ExecutionPlan) (map[string]string, error) {
	plan, err := s.SelfPlan(ctx)
	if err != nil {
		return nil, err
	}
	if p.ApprovedPlan == "" || p.ApprovedPlan != plan.SHA256 {
		return nil, errors.New("平台计划已变化，请重新审核目标 commit")
	}
	change := plan.Changes[0]
	if change.Action == "noop" {
		return map[string]string{"state": "succeeded"}, nil
	}
	stage, err := s.preflightSelf(ctx, plan.Config, change.TargetCommit, s.ConfigFile())
	if err != nil {
		return nil, err
	}
	request, err := s.ScheduleSelf(ctx, plan.Config, change.TargetCommit, stage)
	if err != nil {
		return nil, err
	}
	return map[string]string{"state": "running", "request_id": request.ID, "commit": request.Commit}, nil
}

func (s Service) SelfResult(ctx context.Context, id string) (map[string]string, error) {
	if !regexp.MustCompile(`^[0-9]{1,32}$`).MatchString(id) {
		return nil, errors.New("独立更新请求 ID 无效")
	}
	b, err := os.ReadFile(s.Legacy.Store.Path("state", "self-results", id+".json"))
	if os.IsNotExist(err) {
		var request SelfRequest
		b, err = os.ReadFile(s.Legacy.Store.Path("state", "self-request.json"))
		if err != nil {
			return nil, err
		}
		if json.Unmarshal(b, &request) != nil || request.ID != id {
			return nil, errors.New("独立更新请求不存在")
		}
		if time.Since(request.RequestedAt) > 5*time.Second {
			out, err := exec.CommandContext(ctx, "systemctl", "show", "--property=ActiveState", "--value", "pagehub-updater.service").Output()
			if err != nil || strings.TrimSpace(string(out)) == "failed" || strings.TrimSpace(string(out)) == "inactive" {
				return map[string]string{"state": "error", "error": "独立更新服务异常退出，请检查 updater 日志"}, nil
			}
		}
		return map[string]string{"state": "running"}, nil
	}
	if err != nil {
		return nil, err
	}
	var result struct {
		State  string `json:"state"`
		Error  string `json:"error"`
		PID    int    `json:"pid"`
		BootID string `json:"boot_id"`
		Commit string `json:"commit"`
	}
	if err = json.Unmarshal(b, &result); err != nil {
		return nil, err
	}
	if result.State == "running" && operationInterrupted(result.PID, result.BootID) {
		result.State = "error"
		result.Error = "独立更新进程已中断，请检查 updater 日志"
	}
	return map[string]string{"state": result.State, "error": result.Error, "commit": result.Commit}, nil
}
