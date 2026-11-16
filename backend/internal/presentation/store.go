package presentation

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
)

type Profile struct {
	Name       string `json:"name"`
	Signature  string `json:"signature"`
	AvatarPath string `json:"avatarPath"`
	GithubURL  string `json:"githubUrl"`
}
type Project struct {
	ID                  string   `json:"id"`
	AppID               string   `json:"appId"`
	SourceType          string   `json:"sourceType,omitempty"`
	DescriptionOverride string   `json:"descriptionOverride,omitempty"`
	IconPath            string   `json:"iconPath,omitempty"`
	DisplayOrder        int      `json:"displayOrder"`
	DisplayEnabled      bool     `json:"displayEnabled"`
	TitleOverride       string   `json:"titleOverride"`
	Keywords            []string `json:"keywords"`
}
type Settings struct {
	SchemaVersion int       `json:"schemaVersion"`
	Revision      int       `json:"revision"`
	Profile       Profile   `json:"profile"`
	Projects      []Project `json:"projects"`
}
type OfflineRecord struct {
	AppID       string    `json:"appId"`
	Fingerprint string    `json:"fingerprint"`
	Target      string    `json:"target"`
	Evidence    string    `json:"evidence"`
	Passed      bool      `json:"passed"`
	VerifiedAt  time.Time `json:"verifiedAt"`
}

var ErrConflict = errors.New("展示配置已被修改，请重新加载最新版本")

func OfflineReady(store registry.Store, app registry.Record) bool {
	var record OfflineRecord
	data, err := os.ReadFile(store.Path("state", "offline", app.Manifest.ID+".json"))
	return err == nil && json.Unmarshal(data, &record) == nil && record.Passed && record.AppID == app.Manifest.ID && record.Fingerprint == registry.Fingerprint(app) && record.Target != "" && record.Evidence != "" && !record.VerifiedAt.IsZero()
}
func Load(store registry.Store) (Settings, error) {
	var settings Settings
	data, err := os.ReadFile(store.Path("state", "portal", "ui-settings.json"))
	if errors.Is(err, os.ErrNotExist) {
		settings = Settings{SchemaVersion: 1, Revision: 0, Profile: Profile{Name: "Double Cloth", Signature: "Build. Explore. Create.", AvatarPath: "/assets/avatar.svg", GithubURL: "https://github.com/double-cloth-studio"}, Projects: []Project{}}
		apps, loadErr := store.Load()
		if loadErr != nil {
			return settings, loadErr
		}
		for _, app := range apps.Apps {
			if app.Manifest.Visibility == "public" && OfflineReady(store, app) {
				settings.Projects = append(settings.Projects, Project{ID: app.Manifest.ID, AppID: app.Manifest.ID, DisplayOrder: len(settings.Projects), DisplayEnabled: true, Keywords: []string{}})
			}
		}
		return settings, nil
	}
	if err != nil {
		return settings, err
	}
	if err = json.Unmarshal(data, &settings); err != nil {
		return settings, err
	}
	if settings.SchemaVersion != 1 {
		return settings, errors.New("展示配置版本不支持")
	}
	return settings, nil
}
func Public(store registry.Store) (Settings, error) {
	settings, err := Load(store)
	if err != nil {
		return settings, err
	}
	apps, err := store.Load()
	if err != nil {
		return settings, err
	}
	allowed := map[string]bool{}
	for _, app := range apps.Apps {
		allowed[app.Manifest.ID] = app.Manifest.Visibility == "public" && OfflineReady(store, app)
	}
	visible := []Project{}
	for _, project := range settings.Projects {
		if project.DisplayEnabled && allowed[project.AppID] {
			visible = append(visible, project)
		}
	}
	settings.Projects = visible
	return settings, nil
}
func Save(store registry.Store, next Settings) (Settings, error) {
	unlock, err := registry.Lock(store.Path("state", "portal", "settings.lock"))
	if err != nil {
		return next, err
	}
	defer unlock()
	current, err := Load(store)
	if err != nil {
		return next, err
	}
	if current.Revision != next.Revision {
		return next, ErrConflict
	}
	if err = validate(store, next); err != nil {
		return next, err
	}
	next.Revision++
	sort.Slice(next.Projects, func(i, j int) bool { return next.Projects[i].DisplayOrder < next.Projects[j].DisplayOrder })
	if err = registry.WriteJSON(store.Path("state", "portal", "ui-settings.previous.json"), current, 0600); err != nil {
		return next, err
	}
	err = registry.WriteJSON(store.Path("state", "portal", "ui-settings.json"), next, 0600)
	return next, err
}
func localAsset(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "" && parsed.Host == "" && strings.HasPrefix(parsed.Path, "/assets/") && !strings.Contains(value, "..") && !strings.Contains(value, "\\") && filepath.Ext(parsed.Path) != ""
}
func validate(store registry.Store, settings Settings) error {
	if settings.SchemaVersion != 1 || len(settings.Projects) > 500 || utf8.RuneCountInString(strings.TrimSpace(settings.Profile.Name)) < 1 || utf8.RuneCountInString(settings.Profile.Name) > 60 || utf8.RuneCountInString(settings.Profile.Signature) > 140 || !localAsset(settings.Profile.AvatarPath) {
		return errors.New("资料或本地头像路径无效")
	}
	github, err := url.Parse(settings.Profile.GithubURL)
	if err != nil || github.Scheme != "https" || github.Host != "github.com" || github.User != nil {
		return errors.New("GitHub 地址无效")
	}
	ids, appIDs, orders := map[string]bool{}, map[string]bool{}, map[int]bool{}
	for _, project := range settings.Projects {
		if (project.SourceType != "" && project.SourceType != "app") || (project.IconPath != "" && !localAsset(project.IconPath)) || utf8.RuneCountInString(project.DescriptionOverride) > 200 {
			return errors.New("项目来源、图标或描述无效")
		}
		if !validID(project.ID) {
			return errors.New("项目标识无效")
		}
		if ids[project.ID] || appIDs[project.AppID] || orders[project.DisplayOrder] || project.DisplayOrder < 0 || project.DisplayOrder >= len(settings.Projects) || utf8.RuneCountInString(project.TitleOverride) > 80 || len(project.Keywords) > 20 {
			return errors.New("项目排序、标题或标识无效")
		}
		for _, keyword := range project.Keywords {
			if utf8.RuneCountInString(keyword) > 60 {
				return errors.New("关键词过长")
			}
		}
		app, err := store.Get(project.AppID)
		if err != nil || app.Manifest.Visibility != "public" || !OfflineReady(store, app) {
			return errors.New("项目未登记、不可公开或当前版本尚未通过离线验证")
		}
		ids[project.ID], appIDs[project.AppID], orders[project.DisplayOrder] = true, true, true
	}
	return nil
}
func validID(id string) bool {
	if len(id) < 1 || len(id) > 80 {
		return false
	}
	for _, char := range id {
		if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-' || char == '_') {
			return false
		}
	}
	return true
}
