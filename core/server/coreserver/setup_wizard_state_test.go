package coreserver

import (
	"encoding/json"
	"testing"

	"github.com/pocketbase/pocketbase/core"
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

// When an existing deployment (one with an owner but no wizard row) re-runs
// create-owner, the wizard should start. created is false because the owner
// already existed.
func TestCreateOperatorIdentitiesStartsWizardOnRerun(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	createSystemSettingsCollection(t, app)

	// Ensure users collection has the fields we need.
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	usersCol, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	if usersCol.Fields.GetByName("role") == nil {
		usersCol.Fields.Add(&core.TextField{Name: "role"})
		if err := app.Save(usersCol); err != nil {
			t.Fatal(err)
		}
	}

	// Create the owner without the wizard row (simulate a pre-wizard deployment).
	if _, err := createOperatorIdentities(app, "owner@example.com", "", "OwnerPass1234!", ""); err != nil {
		t.Fatal(err)
	}
	// Delete the wizard row to simulate a deployment that never had one.
	wizardRec, err := app.FindFirstRecordByFilter("system_settings", "key = {:k}", map[string]any{"k": setupWizardKey})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Delete(wizardRec); err != nil {
		t.Fatal(err)
	}

	// Re-run create-owner (the idempotent path — owner already exists).
	created, err := createOperatorIdentities(app, "owner@example.com", "", "OwnerPass1234!", "")
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("created should be false when owner already existed")
	}

	// Wizard row should now exist.
	if _, err := app.FindFirstRecordByFilter("system_settings", "key = {:k}", map[string]any{"k": setupWizardKey}); err != nil {
		t.Fatalf("wizard state row missing after re-run: %v", err)
	}
}
