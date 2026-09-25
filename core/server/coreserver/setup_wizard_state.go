package coreserver

import (
	"encoding/json"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// setupWizardKey holds the first-run wizard's progress. Only the two paths
// that create a deployment's owner write it, so a deployment set up before
// the wizard existed has no row and never sees the wizard.
const setupWizardKey = "setup.wizard"

// MarkSetupWizardStarted writes the initial wizard state unless a row exists.
func MarkSetupWizardStarted(app core.App) error {
	if _, err := app.FindFirstRecordByFilter(
		"system_settings", "key = {:key}", map[string]any{"key": setupWizardKey},
	); err == nil {
		return nil
	}
	value, err := json.Marshal(map[string]any{
		"startedAt":    time.Now().UTC().Format(time.RFC3339),
		"acknowledged": []string{},
		"skipped":      []string{},
	})
	if err != nil {
		return err
	}
	return upsertSystemSetting(app, setupWizardKey, string(value), false)
}
