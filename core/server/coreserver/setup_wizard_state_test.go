package coreserver

import (
	"encoding/json"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

func TestMarkSetupWizardStartedWritesOnce(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	createSystemSettingsCollection(t, app)

	if err := MarkSetupWizardStarted(app); err != nil {
		t.Fatal(err)
	}
	rec, err := app.FindFirstRecordByFilter("system_settings", "key = {:k}", map[string]any{"k": setupWizardKey})
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		StartedAt    string   `json:"startedAt"`
		Acknowledged []string `json:"acknowledged"`
		Skipped      []string `json:"skipped"`
	}
	if err := json.Unmarshal([]byte(rec.GetString("value")), &state); err != nil {
		t.Fatal(err)
	}
	if state.StartedAt == "" || state.Acknowledged == nil || state.Skipped == nil {
		t.Fatalf("incomplete state: %+v", state)
	}

	// A second call (create-owner re-run) must not reset progress.
	rec.Set("value", `{"startedAt":"x","acknowledged":["core:apps"],"skipped":[]}`)
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
	if err := MarkSetupWizardStarted(app); err != nil {
		t.Fatal(err)
	}
	again, _ := app.FindRecordById("system_settings", rec.Id)
	if again.GetString("value") != `{"startedAt":"x","acknowledged":["core:apps"],"skipped":[]}` {
		t.Fatalf("progress was reset: %s", again.GetString("value"))
	}
}

// create-owner is how every non-wizard deployment gets its owner, so it must
// start the wizard too.
func TestCreateOperatorIdentitiesStartsWizard(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	createSystemSettingsCollection(t, app)

	if _, err := createOperatorIdentities(app, "owner@example.com", "", "OwnerPass1234!", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := app.FindFirstRecordByFilter("system_settings", "key = {:k}", map[string]any{"k": setupWizardKey}); err != nil {
		t.Fatalf("wizard state row missing: %v", err)
	}
}
