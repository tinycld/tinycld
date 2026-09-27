package coreserver

import (
	"encoding/json"
	"fmt"
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

// markOrgNameSeeded records in the wizard state that the workspace name was
// chosen by whoever provisioned the deployment. Until then the wizard reads
// the saved name as PocketBase's default ("Acme") and does not show it.
func markOrgNameSeeded(app core.App) error {
	if err := MarkSetupWizardStarted(app); err != nil {
		return err
	}
	rec, err := app.FindFirstRecordByFilter(
		"system_settings", "key = {:key}", map[string]any{"key": setupWizardKey},
	)
	if err != nil {
		return err
	}
	var state map[string]any
	if err := json.Unmarshal([]byte(rec.GetString("value")), &state); err != nil {
		return fmt.Errorf("read wizard state: %w", err)
	}
	state["orgNameSeeded"] = true
	value, err := json.Marshal(state)
	if err != nil {
		return err
	}
	rec.Set("value", string(value))
	return app.Save(rec)
}
