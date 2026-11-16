package importer

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
)

func PreserveEnvironment(old, next registry.Record) error {
	var previous, current map[string]any
	b, err := os.ReadFile(filepath.Join(old.Directory, "pagehub.runtime.json"))
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, &previous); err != nil {
		return err
	}
	b, err = os.ReadFile(filepath.Join(next.Directory, "pagehub.runtime.json"))
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, &current); err != nil {
		return err
	}
	oldServices := previous["services"].(map[string]any)
	for name, value := range current["services"].(map[string]any) {
		service := value.(map[string]any)
		prior, ok := oldServices[name].(map[string]any)
		if !ok {
			continue
		}
		environment, _ := prior["environment"].(map[string]any)
		if environment == nil {
			environment = map[string]any{}
		}
		declared, _ := service["environment"].(map[string]any)
		for key, item := range declared {
			environment[key] = item
		}
		service["environment"] = environment
	}
	return registry.WriteJSON(filepath.Join(next.Directory, "pagehub.runtime.json"), current, 0600)
}
