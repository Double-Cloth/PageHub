//go:build !windows

package managed

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/double-cloth-studio/PageHub/backend/internal/adapters"
	"github.com/double-cloth-studio/PageHub/backend/internal/lifecycle"
	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
)

func TestIndependentUpdaterReceipts(t *testing.T) {
	s := Service{Legacy: lifecycle.Service{Store: registry.Store{Root: t.TempDir()}}}
	commands := t.TempDir()
	if err := os.WriteFile(filepath.Join(commands, "systemctl"), []byte("#!/bin/sh\nif [ \"$1\" = show ]; then echo inactive; fi\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", commands+":"+os.Getenv("PATH"))
	c := configuration(t)
	request, err := s.ScheduleSelf(context.Background(), c, strings.Repeat("a", 40), "stage")
	if err != nil {
		t.Fatal(err)
	}
	if request.ID == "" {
		t.Fatal("缺少独立任务 ID")
	}
	file := s.Legacy.Store.Path("state", "self-request.json")
	info, err := os.Stat(file)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("更新请求权限无效", err)
	}
	result := s.Legacy.Store.Path("state", "self-result.json")
	if err = registry.WriteJSON(result, map[string]string{"id": "old-request", "state": "succeeded"}, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = s.WaitSelf(ctx, request); err == nil {
		t.Fatal("旧任务收据被误认为本次更新成功")
	}
	if err = registry.WriteJSON(result, map[string]string{"id": request.ID, "state": "error"}, 0600); err != nil {
		t.Fatal(err)
	}
	if err = s.WaitSelf(context.Background(), request); err == nil {
		t.Fatal("失败任务被报告为成功")
	}
	if err = registry.WriteJSON(result, map[string]string{"id": request.ID, "state": "succeeded"}, 0600); err != nil {
		t.Fatal(err)
	}
	if err = s.WaitSelf(context.Background(), request); err != nil {
		t.Fatal(err)
	}
}

func TestPlatformPlanApprovalAndNoop(t *testing.T) {
	s := Service{Legacy: lifecycle.Service{Store: registry.Store{Root: t.TempDir()}}}
	c := configuration(t)
	c.Platform.Root = s.Legacy.Store.Root
	c.Platform.SourceDir = c.Platform.Root + "/sources/pagehub"
	t.Setenv("PAGEHUB_CONFIG_FILE", s.Legacy.Store.Path("config", "pagehub.yaml"))
	if err := s.SaveConfig(c); err != nil {
		t.Fatal(err)
	}
	commit := strings.Repeat("a", 40)
	if err := registry.WriteJSON(s.Legacy.Store.Path("current", "source-version.json"), SourceVersion{Commit: commit}, 0600); err != nil {
		t.Fatal(err)
	}
	commands := t.TempDir()
	if err := os.WriteFile(filepath.Join(commands, "git"), []byte("#!/bin/sh\nprintf '"+commit+"\\trefs/heads/main\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", commands+":"+os.Getenv("PATH"))
	plan, err := s.SelfPlan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if plan.Changes[0].Action != "noop" {
		t.Fatal("相同 commit 未形成幂等计划")
	}
	if _, err = s.Execute(context.Background(), adapters.ExecutionPlan{ID: "self", Action: "update-self", ApprovedPlan: "wrong"}); err == nil {
		t.Fatal("无效平台审批被接受")
	}
	result, err := s.Execute(context.Background(), adapters.ExecutionPlan{ID: "self", Action: "update-self", ApprovedPlan: plan.SHA256})
	if err != nil || result["state"] != "succeeded" {
		t.Fatal("平台无变更操作失败", err)
	}
	if _, err = os.Stat(s.Legacy.Store.Path("state", "self-request.json")); !os.IsNotExist(err) {
		t.Fatal("无变更操作启动了更新事务")
	}
}
