package managed

import (
	"context"
	"errors"

	"github.com/double-cloth-studio/PageHub/backend/internal/adapters"
)

func (s Service) Execute(ctx context.Context, p adapters.ExecutionPlan) (map[string]string, error) {
	if p.ID == "self" && p.Action == "update-self" {
		return s.executePlatform(ctx, p)
	}
	a, err := s.Legacy.Store.Get(p.ID)
	if err != nil {
		return nil, err
	}
	if a.Managed == nil {
		return nil, errors.New("操作需要托管应用")
	}
	s.ExpectedFingerprint = p.Fingerprint
	if err = s.checkRecord(a); err != nil {
		return nil, err
	}
	result := map[string]string{"state": "succeeded"}
	switch p.Action {
	case "backup":
		result["backup"], err = s.Backup(ctx, p.ID)
	case "restore":
		err = s.Restore(ctx, p.ID, p.Backup, true)
	case "rollback":
		err = s.Rollback(ctx, p.ID)
	case "uninstall":
		err = s.Uninstall(ctx, p.ID)
	case "update":
		c, e := s.Config()
		if e != nil {
			return nil, e
		}
		plan, e := s.TargetPlan(ctx, c, p.ID)
		if e != nil {
			return nil, e
		}
		err = s.Apply(ctx, plan, p.ApprovedPlan)
	default:
		return nil, errors.New("托管操作无效")
	}
	return result, err
}
