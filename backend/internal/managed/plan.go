package managed

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/double-cloth-studio/PageHub/backend/internal/desired"
	"github.com/double-cloth-studio/PageHub/backend/internal/lifecycle"
	"github.com/double-cloth-studio/PageHub/backend/internal/manifests"
	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
)

type Service struct {
	Legacy              lifecycle.Service
	ExpectedFingerprint string
	ConfigPath          string
}

func (s Service) checkRecord(a registry.Record) error {
	if s.ExpectedFingerprint != "" && registry.Fingerprint(a) != s.ExpectedFingerprint {
		return errors.New("执行计划已过期")
	}
	return s.Legacy.Store.Validate(a)
}

func (s Service) TargetPlan(ctx context.Context, c desired.Config, id string) (Plan, error) {
	p, err := s.Plan(ctx, c, false, false)
	if err != nil {
		return p, err
	}
	found := false
	for i := range p.Changes {
		change := &p.Changes[i]
		if change.ID != id {
			change.Action = "noop"
			continue
		}
		found = true
		for _, a := range c.Apps {
			if a.ID == id && a.Enabled {
				change.TargetCommit, err = remoteCommit(ctx, a.Source.Repository, a.Source.Branch)
				if err != nil {
					return p, err
				}
				if change.TargetCommit != change.CurrentCommit {
					change.Action = "update"
				}
			}
		}
	}
	if !found {
		return p, errors.New("更新目标未出现在配置中")
	}
	p.SHA256 = ""
	p.SHA256 = desired.Digest(p)
	return p, nil
}

type Change struct {
	ID             string `json:"id"`
	Action         string `json:"action"`
	CurrentCommit  string `json:"current_commit,omitempty"`
	TargetCommit   string `json:"target_commit,omitempty"`
	ConfigSHA256   string `json:"config_sha256"`
	Project        string `json:"project,omitempty"`
	Volume         string `json:"volume,omitempty"`
	Path           string `json:"path"`
	Fingerprint    string `json:"fingerprint,omitempty"`
	AdoptionSHA256 string `json:"adoption_sha256,omitempty"`
	CurrentImage   string `json:"current_image,omitempty"`
}

type Plan struct {
	Config  desired.Config `json:"config"`
	Changes []Change       `json:"changes"`
	Adopt   bool           `json:"adopt"`
	SHA256  string         `json:"sha256"`
}

func configDigest(a desired.App, origin string) string {
	return desired.Digest(struct {
		App    desired.App
		Origin string
	}{a, origin})
}

func (s Service) Plan(ctx context.Context, c desired.Config, adopt bool, refresh bool) (Plan, error) {
	p := Plan{Config: c, Adopt: adopt, Changes: []Change{}}
	if err := c.Validate(); err != nil {
		return p, err
	}
	if c.Platform.Root != s.Legacy.Store.Root {
		return p, errors.New("配置 root 与当前平台不一致")
	}
	r, err := s.Legacy.Store.Load()
	if err != nil {
		return p, err
	}
	seen := map[string]bool{}
	for _, a := range c.Apps {
		seen[a.ID] = true
		change := Change{ID: a.ID, Action: "install", ConfigSHA256: configDigest(a, c.Platform.PublicOrigin), Project: a.Deployment.Project, Path: a.Path}
		if a.Deployment.Driver == "managed-compose" {
			change.Volume = a.Deployment.Project + "_" + a.Persistence.Volume
		}
		for _, old := range r.Apps {
			if old.Manifest.ID != a.ID {
				if manifests.Conflict(old.Manifest, manifests.Manifest{ID: a.ID, Path: a.Path}) || (change.Project != "" && old.Project == change.Project) {
					return p, fmt.Errorf("%s 与现有登记路径或项目冲突", a.ID)
				}
				continue
			}
			change.Fingerprint = registry.Fingerprint(old)
			if err = s.Legacy.Store.Validate(old); err != nil {
				return p, fmt.Errorf("%s 当前登记或发行文件校验失败: %w", a.ID, err)
			}
			if old.Managed == nil {
				if !adopt || old.Project != project(a) || (old.Manifest.Kind != "compose" && old.Manifest.Kind != "static") {
					return p, fmt.Errorf("%s 需要 --adopt 并保持原驱动和项目名", a.ID)
				}
				change.Action = "adopt"
				continue
			}
			if old.Managed.Config.Deployment.Driver != a.Deployment.Driver || old.Project != project(a) || old.Managed.Config.Source.Repository != a.Source.Repository {
				return p, errors.New("驱动、项目名或仓库变更需要独立迁移")
			}
			change.CurrentCommit = old.Managed.Commit
			change.Fingerprint = registry.Fingerprint(old)
			change.TargetCommit = change.CurrentCommit
			change.Action = "noop"
			if change.ConfigSHA256 != old.Managed.ConfigSHA256 {
				change.Action = "configure"
			}
			if a.Startup == "always" && old.Disabled == a.Enabled {
				change.Action = "configure"
			}
		}
		if a.Deployment.Driver == "managed-compose" && change.CurrentCommit == "" && (change.Fingerprint == "" || change.Action == "adopt") {
			existing, err := s.inspectAdoption(ctx, a)
			if err != nil {
				return p, err
			}
			if existing != nil {
				if !adopt {
					return p, errors.New("发现现有日记项目，必须用 --adopt 审核原镜像和真实 runtime 卷")
				}
				change.AdoptionSHA256 = desired.Digest(existing)
				change.CurrentImage = existing.Image
				change.Action = "adopt"
			}
		}
		if !a.Enabled {
			change.Action = "disable"
			for _, old := range r.Apps {
				if old.Manifest.ID == a.ID && old.Disabled {
					change.Action = "noop"
				}
			}
			if change.Fingerprint == "" {
				change.Action = "noop"
			}
		} else if change.CurrentCommit == "" || refresh {
			change.TargetCommit, err = remoteCommit(ctx, a.Source.Repository, a.Source.Branch)
			if err != nil {
				return p, fmt.Errorf("%s: %w", a.ID, err)
			}
			if change.CurrentCommit != "" && change.CurrentCommit != change.TargetCommit {
				change.Action = "update"
			}
		}
		p.Changes = append(p.Changes, change)
	}
	for _, old := range r.Apps {
		if old.Managed != nil && !seen[old.Manifest.ID] {
			return p, fmt.Errorf("配置缺少已托管应用 %s，请保留条目并设置 enabled: false，或显式卸载", old.Manifest.ID)
		}
	}
	sort.SliceStable(p.Changes, func(i, j int) bool { return p.Changes[i].Project == "" && p.Changes[j].Project != "" })
	p.SHA256 = desired.Digest(p)
	return p, nil
}

func project(a desired.App) string {
	if a.Deployment.Driver == "managed-compose" {
		return a.Deployment.Project
	}
	return "pagehub-" + a.ID
}

func (s Service) Config() (desired.Config, error) {
	return desired.Load(s.ConfigFile())
}

func (s Service) ConfigFile() string {
	if s.ConfigPath != "" {
		return s.ConfigPath
	}
	if file := os.Getenv("PAGEHUB_CONFIG_FILE"); file != "" {
		return file
	}
	return s.Legacy.Store.Path("config", "pagehub.yaml")
}

func (s Service) IsManaged(id string) bool {
	a, err := s.Legacy.Store.Get(id)
	return err == nil && a.Managed != nil
}

func (s Service) SaveConfig(c desired.Config) error {
	if err := registry.WriteJSON(s.Legacy.Store.Path("config", "pagehub.yaml"), c, 0600); err != nil {
		return err
	}
	return registry.AtomicWrite(s.Legacy.Store.Path("state", "desired-config.sha256"), []byte(desired.Digest(c)+"\n"), 0600)
}

func loadRecord(s registry.Store, id string) (*registry.Record, error) {
	a, err := s.Get(id)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = s.Validate(a); err != nil {
		return nil, err
	}
	return &a, nil
}
