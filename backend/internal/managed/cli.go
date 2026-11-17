package managed

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/double-cloth-studio/PageHub/backend/internal/desired"
	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
)

type arguments struct {
	Config   string
	Approved string
	Adopt    bool
	Check    bool
	All      bool
	Purge    bool
	Backup   string
	Target   string
}

func parse(args []string) (arguments, error) {
	var result arguments
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-c", "--config", "--approved-plan", "--backup":
			if i+1 == len(args) {
				return result, errors.New("选项缺少值")
			}
			key := args[i]
			i++
			switch key {
			case "-c", "--config":
				result.Config = args[i]
			case "--approved-plan":
				result.Approved = args[i]
			case "--backup":
				result.Backup = args[i]
			}
		case "--adopt":
			result.Adopt = true
		case "--check":
			result.Check = true
		case "--all":
			result.All = true
		case "--purge-data":
			result.Purge = true
		default:
			if strings.HasPrefix(args[i], "-") || result.Target != "" {
				return result, errors.New("未知选项或多余参数")
			}
			result.Target = args[i]
		}
	}
	return result, nil
}

func confirm(digest, approved string) error {
	if approved != "" {
		if approved != digest {
			return errors.New("审批摘要与当前计划不匹配")
		}
		return nil
	}
	fmt.Fprintln(os.Stderr, "请审核计划中的 Git commit、容器、数据卷与维护窗口。输入计划 SHA-256 确认执行")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != digest {
		return errors.New("计划未确认，非交互执行请传入 --approved-plan <精确摘要>")
	}
	return nil
}

func (s Service) CLI(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少托管命令")
	}
	command := args[0]
	opts, err := parse(args[1:])
	if err != nil {
		return err
	}
	if opts.Config == "" {
		opts.Config = s.ConfigFile()
	}
	s.ConfigPath = opts.Config
	if command == "status" {
		result, err := s.Status(ctx)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	}
	if command == "uninstall" {
		if !s.IsManaged(opts.Target) {
			return errors.New("应用未托管，请使用已有 remove-app 命令")
		}
		if opts.Purge {
			a, err := s.Legacy.Store.Get(opts.Target)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "将永久删除数据卷 %s_runtime。输入应用 ID，再输入实际卷名确认\n", a.Project)
			reader := bufio.NewReader(os.Stdin)
			id, err := reader.ReadString('\n')
			if err != nil || strings.TrimSpace(id) != opts.Target {
				return errors.New("应用 ID 未确认")
			}
			volume, err := reader.ReadString('\n')
			if err != nil {
				return err
			}
			return s.Purge(ctx, opts.Target, opts.Backup, strings.TrimSpace(volume))
		}
		return s.Uninstall(ctx, opts.Target)
	}
	if command == "rollback" {
		return s.Rollback(ctx, opts.Target)
	}
	if command == "backup" {
		dest, err := s.Backup(ctx, opts.Target)
		if err == nil {
			fmt.Println(dest)
		}
		return err
	}
	if command == "restore" {
		fmt.Fprintf(os.Stderr, "将恢复 %s 的数据和密码。输入应用 ID 与 RESTORE 确认\n", opts.Target)
		reader := bufio.NewReader(os.Stdin)
		id, err := reader.ReadString('\n')
		if err != nil || strings.TrimSpace(id) != opts.Target {
			return errors.New("应用 ID 未确认")
		}
		answer, err := reader.ReadString('\n')
		if err != nil || strings.TrimSpace(answer) != "RESTORE" {
			return errors.New("恢复未确认")
		}
		return s.Restore(ctx, opts.Target, opts.Backup, true)
	}
	c, err := desired.Load(opts.Config)
	if err != nil {
		return err
	}
	if command == "validate" {
		if err = s.checkPlatformConfig(c); err != nil {
			return err
		}
		r, err := s.Legacy.Store.Load()
		if err != nil {
			return err
		}
		for _, a := range r.Apps {
			if err = s.Legacy.Store.Validate(a); err != nil {
				return err
			}
		}
		return nil
	}
	if command == "retention" {
		fmt.Println(c.Update.RetainReleases)
		return nil
	}
	if command == "bootstrap-env" {
		p := c.Platform
		fmt.Printf("PAGEHUB_ROOT=%s\nPAGEHUB_PUBLIC_ORIGIN=%s\nPAGEHUB_GATEWAY_PORT=%d\nPAGEHUB_ADMIN_PORT=%d\nPAGEHUB_IMPORT_BUNDLED=false\nPAGEHUB_CONFIG_FILE=/etc/pagehub/pagehub.yaml\n", p.Root, p.PublicOrigin, p.Gateway.Port, p.Admin.Port)
		return nil
	}
	if command == "prepare-self" {
		var request SelfRequest
		b, err := os.ReadFile(s.Legacy.Store.Path("state", "self-request.json"))
		if err != nil {
			return err
		}
		if err = json.Unmarshal(b, &request); err != nil {
			return err
		}
		dir, err := s.PrepareSelf(ctx, c, request)
		if err == nil {
			fmt.Println(dir)
		}
		return err
	}
	if command != "plan" && command != "apply" && command != "update" {
		return errors.New("未知托管命令")
	}
	if err = s.checkPlatformConfig(c); err != nil {
		return err
	}
	refresh := command == "update"
	p, err := s.Plan(ctx, c, opts.Adopt, refresh)
	if err != nil {
		return err
	}
	selfCommit := ""
	if command == "update" {
		if opts.Target == "" && !opts.Check && !opts.All {
			return errors.New("update 需要 self、应用 ID、--check 或 --all")
		}
		if opts.All || opts.Check && opts.Target == "" || opts.Target == "self" {
			selfCommit, err = remoteCommit(ctx, c.Platform.Repository, c.Platform.Branch)
			if err != nil {
				return err
			}
		}
		if opts.Target != "" {
			found := opts.Target == "self"
			for i := range p.Changes {
				if p.Changes[i].ID != opts.Target {
					p.Changes[i].Action = "noop"
				} else {
					found = true
				}
			}
			if !found {
				return errors.New("更新目标未出现在配置中")
			}
			p.SHA256 = ""
			p.SHA256 = desired.Digest(p)
		}
	}
	review := struct {
		SelfCurrent SourceVersion `json:"self_current"`
		Plan        Plan          `json:"plan"`
		SelfCommit  string        `json:"self_commit,omitempty"`
		SHA256      string        `json:"sha256"`
	}{Plan: p, SelfCommit: selfCommit, SelfCurrent: s.SourceVersion()}
	review.SHA256 = desired.Digest(review)
	if err = json.NewEncoder(os.Stdout).Encode(review); err != nil {
		return err
	}
	if command == "plan" || opts.Check {
		return nil
	}
	if err = confirm(review.SHA256, opts.Approved); err != nil {
		return err
	}
	selfStage := ""
	if current := s.SourceVersion(); current.Commit == selfCommit && !current.Dirty {
		selfCommit = ""
	}
	if selfCommit != "" {
		selfStage, err = s.preflightSelf(ctx, c, selfCommit, opts.Config)
		if err != nil {
			return err
		}
	}
	if err = s.Apply(ctx, p, p.SHA256); err != nil {
		return err
	}
	if canonical := os.Getenv("PAGEHUB_CONFIG_FILE"); canonical != "" && canonical != opts.Config {
		data, err := os.ReadFile(opts.Config)
		if err != nil {
			return err
		}
		if err = registry.AtomicWrite(canonical, data, 0600); err != nil {
			return err
		}
	}
	if selfCommit != "" {
		request, err := s.ScheduleSelf(ctx, c, selfCommit, selfStage)
		if err != nil {
			return err
		}
		return s.WaitSelf(ctx, request)
	}
	return nil
}

func (s Service) checkPlatformConfig(c desired.Config) error {
	if c.Platform.Root != s.Legacy.Store.Root {
		return errors.New("配置 root 与安装根目录不一致")
	}
	// 已安装平台改变端口和 Origin 时，需要先同步平台环境再重启所有服务。
	if _, err := os.Stat(s.Legacy.Store.Path("state", "install-state.json")); err == nil {
		for key, wanted := range map[string]string{"PAGEHUB_PUBLIC_ORIGIN": c.Platform.PublicOrigin, "PAGEHUB_GATEWAY_PORT": fmt.Sprint(c.Platform.Gateway.Port), "PAGEHUB_ADMIN_PORT": fmt.Sprint(c.Platform.Admin.Port)} {
			actual := os.Getenv(key)
			if key == "PAGEHUB_ADMIN_PORT" && actual == "" {
				actual = "8091"
			}
			if actual != "" && actual != wanted {
				return errors.New("平台 Origin 或端口变更需要平台迁移，当前环境与声明式配置不匹配")
			}
		}
	}
	return nil
}

func (s Service) WriteVersions(ctx context.Context) error {
	status, err := s.Status(ctx)
	if err != nil {
		return err
	}
	return registry.WriteJSON(filepath.Join(s.Legacy.Store.Root, "state", "versions.json"), status, 0600)
}
