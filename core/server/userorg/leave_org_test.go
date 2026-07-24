package userorg

import (
	"errors"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// setupTestApp builds an in-memory PocketBase test app with one reassignable
// collection (test_events) whose ownership FK points at the users collection
// directly — the single-org model (the former user_org junction is gone).
//
// test_events.created_by is a required, non-cascade relation to users, which is
// the shape that pins a user record: without OffboardUser's reassign/delete
// pass, offboarding a user who owns an event would leave a dangling reference.
func setupTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	ResetReassignableForTesting()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(app.Cleanup)

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("users collection: %v", err)
	}

	events := core.NewBaseCollection("test_events")
	events.Fields.Add(&core.TextField{Name: "title", Required: true})
	events.Fields.Add(&core.RelationField{
		Name: "created_by", Required: true, CollectionId: users.Id,
		CascadeDelete: false, MaxSelect: 1,
	})
	if err := app.Save(events); err != nil {
		t.Fatalf("save test_events: %v", err)
	}

	RegisterReassignable(ReassignableRef{Collection: "test_events", Field: "created_by"})

	return app
}

func makeUser(t *testing.T, app core.App, email string) *core.Record {
	t.Helper()
	users, _ := app.FindCollectionByNameOrId("users")
	u := core.NewRecord(users)
	u.SetEmail(email)
	u.Set("name", "T")
	u.SetVerified(true)
	u.SetPassword("Password123!")
	if err := app.Save(u); err != nil {
		t.Fatalf("save user %s: %v", email, err)
	}
	return u
}

func makeEvent(t *testing.T, app core.App, title, createdBy string) *core.Record {
	t.Helper()
	col, _ := app.FindCollectionByNameOrId("test_events")
	e := core.NewRecord(col)
	e.Set("title", title)
	e.Set("created_by", createdBy)
	if err := app.Save(e); err != nil {
		t.Fatalf("save event %s: %v", title, err)
	}
	return e
}

// TestOffboardUser_ReassignRewritesOwnedFKs — a user owns test_events.created_by
// (a required non-cascade FK). ModeReassign rewrites the FK to the successor,
// then anonymizes the offboarded user.
func TestOffboardUser_ReassignRewritesOwnedFKs(t *testing.T) {
	app := setupTestApp(t)

	alice := makeUser(t, app, "alice@test.local")
	bob := makeUser(t, app, "bob@test.local")
	event := makeEvent(t, app, "Standup", alice.Id)

	result, err := OffboardUser(app, alice.Id, Plan{
		Mode:            ModeReassign,
		SuccessorUserID: bob.Id,
	}, alice.Id)
	if err != nil {
		t.Fatalf("OffboardUser: %v", err)
	}
	if result.RecordsReassigned != 1 {
		t.Errorf("expected 1 record reassigned, got %d", result.RecordsReassigned)
	}
	if !result.UserAnonymized {
		t.Error("expected user_anonymized=true")
	}

	updated, err := app.FindRecordById("test_events", event.Id)
	if err != nil {
		t.Fatalf("re-find event: %v", err)
	}
	if updated.GetString("created_by") != bob.Id {
		t.Errorf("created_by: got %s, want %s", updated.GetString("created_by"), bob.Id)
	}

	// Alice's PII is scrubbed.
	scrubbed, err := app.FindRecordById("users", alice.Id)
	if err != nil {
		t.Fatalf("re-find alice: %v", err)
	}
	if scrubbed.GetString("name") != "Deleted user" {
		t.Errorf("name = %q, want %q", scrubbed.GetString("name"), "Deleted user")
	}
	if scrubbed.Email() == "alice@test.local" {
		t.Error("expected email to be scrubbed")
	}
}

// TestOffboardUser_DeleteMyData removes the user's owned records instead of
// reassigning them, then anonymizes.
func TestOffboardUser_DeleteMyData(t *testing.T) {
	app := setupTestApp(t)
	alice := makeUser(t, app, "alice@test.local")
	event := makeEvent(t, app, "Standup", alice.Id)

	result, err := OffboardUser(app, alice.Id, Plan{Mode: ModeDeleteMyData}, alice.Id)
	if err != nil {
		t.Fatalf("OffboardUser: %v", err)
	}
	if result.RecordsDeleted != 1 {
		t.Errorf("expected 1 record deleted, got %d", result.RecordsDeleted)
	}
	if !result.UserAnonymized {
		t.Error("expected user_anonymized=true")
	}
	if _, err := app.FindRecordById("test_events", event.Id); err == nil {
		t.Error("expected event to be deleted")
	}
}

// TestOffboardUser_ReassignRequiresSuccessor — ModeReassign with no successor
// is an invalid plan.
func TestOffboardUser_ReassignRequiresSuccessor(t *testing.T) {
	app := setupTestApp(t)
	alice := makeUser(t, app, "alice@test.local")

	_, err := OffboardUser(app, alice.Id, Plan{Mode: ModeReassign}, alice.Id)
	if !errors.Is(err, ErrInvalidPlan) {
		t.Fatalf("expected ErrInvalidPlan, got %v", err)
	}

	// Alice must NOT have been anonymized on a rejected plan.
	still, _ := app.FindRecordById("users", alice.Id)
	if still.GetString("name") == "Deleted user" {
		t.Error("user should not be anonymized when the plan is rejected")
	}
}

// TestOffboardUser_ReassignRejectsMissingSuccessor — a successor id that isn't a
// real users record is rejected.
func TestOffboardUser_ReassignRejectsMissingSuccessor(t *testing.T) {
	app := setupTestApp(t)
	alice := makeUser(t, app, "alice@test.local")

	_, err := OffboardUser(app, alice.Id, Plan{
		Mode:            ModeReassign,
		SuccessorUserID: "does-not-exist",
	}, alice.Id)
	if !errors.Is(err, ErrInvalidPlan) {
		t.Fatalf("expected ErrInvalidPlan, got %v", err)
	}
}

// TestOffboardUser_ReassignRejectsSelfSuccessor — the successor can't be the
// offboarded user (their records would be reassigned to a user about to be
// anonymized).
func TestOffboardUser_ReassignRejectsSelfSuccessor(t *testing.T) {
	app := setupTestApp(t)
	alice := makeUser(t, app, "alice@test.local")

	_, err := OffboardUser(app, alice.Id, Plan{
		Mode:            ModeReassign,
		SuccessorUserID: alice.Id,
	}, alice.Id)
	if !errors.Is(err, ErrInvalidPlan) {
		t.Fatalf("expected ErrInvalidPlan, got %v", err)
	}
}

// TestOffboardUser_RejectsUnknownMode — a bogus mode is an invalid plan.
func TestOffboardUser_RejectsUnknownMode(t *testing.T) {
	app := setupTestApp(t)
	alice := makeUser(t, app, "alice@test.local")

	_, err := OffboardUser(app, alice.Id, Plan{Mode: Mode("nonsense")}, alice.Id)
	if !errors.Is(err, ErrInvalidPlan) {
		t.Fatalf("expected ErrInvalidPlan, got %v", err)
	}
}

// TestOffboardUser_RejectsEmptyUser — an empty user id is an invalid plan.
func TestOffboardUser_RejectsEmptyUser(t *testing.T) {
	app := setupTestApp(t)

	_, err := OffboardUser(app, "", Plan{Mode: ModeDeleteMyData}, "")
	if !errors.Is(err, ErrInvalidPlan) {
		t.Fatalf("expected ErrInvalidPlan, got %v", err)
	}
}

// TestOffboardUser_EmptyRegistry — a user who owns nothing (empty registry)
// still anonymizes cleanly.
func TestOffboardUser_EmptyRegistry(t *testing.T) {
	ResetReassignableForTesting()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(app.Cleanup)

	alice := makeUser(t, app, "alice@test.local")
	result, err := OffboardUser(app, alice.Id, Plan{Mode: ModeDeleteMyData}, "")
	if err != nil {
		t.Fatalf("OffboardUser: %v", err)
	}
	if result.RecordsDeleted != 0 {
		t.Errorf("expected 0 records deleted, got %d", result.RecordsDeleted)
	}
	if !result.UserAnonymized {
		t.Error("expected user_anonymized=true")
	}
}

// TestOffboardUser_WritesAuditRow — with an actor set and an audit_logs
// collection present, a single summary audit row is written.
func TestOffboardUser_WritesAuditRow(t *testing.T) {
	app := setupTestApp(t)

	// Seed a minimal audit_logs collection (the de-orged shape: no org field).
	auditLogs := core.NewBaseCollection("audit_logs")
	auditLogs.Fields.Add(&core.TextField{Name: "action"})
	auditLogs.Fields.Add(&core.TextField{Name: "resource_type"})
	auditLogs.Fields.Add(&core.TextField{Name: "resource_id"})
	auditLogs.Fields.Add(&core.TextField{Name: "actor"})
	auditLogs.Fields.Add(&core.JSONField{Name: "metadata"})
	if err := app.Save(auditLogs); err != nil {
		t.Fatalf("save audit_logs: %v", err)
	}

	alice := makeUser(t, app, "alice@test.local")
	bob := makeUser(t, app, "bob@test.local")
	makeEvent(t, app, "Standup", alice.Id)

	if _, err := OffboardUser(app, alice.Id, Plan{
		Mode:            ModeReassign,
		SuccessorUserID: bob.Id,
	}, bob.Id); err != nil {
		t.Fatalf("OffboardUser: %v", err)
	}

	rows, err := app.FindRecordsByFilter("audit_logs",
		"resource_id = {:uid}", "", 0, 0, map[string]any{"uid": alice.Id})
	if err != nil {
		t.Fatalf("find audit rows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 audit row, got %d", len(rows))
	}
	if got := rows[0].GetString("action"); got != "account_offboard.reassign" {
		t.Errorf("action = %q, want account_offboard.reassign", got)
	}
	if got := rows[0].GetString("actor"); got != bob.Id {
		t.Errorf("actor = %q, want %q", got, bob.Id)
	}
}

// TestAnonymizeUser scrubs PII directly, without touching owned records.
func TestAnonymizeUser(t *testing.T) {
	app := setupTestApp(t)
	alice := makeUser(t, app, "alice@test.local")

	if err := AnonymizeUser(app, alice.Id); err != nil {
		t.Fatalf("AnonymizeUser: %v", err)
	}
	scrubbed, _ := app.FindRecordById("users", alice.Id)
	if scrubbed.GetString("name") != "Deleted user" {
		t.Errorf("name = %q, want %q", scrubbed.GetString("name"), "Deleted user")
	}
	if scrubbed.Verified() {
		t.Error("expected verified=false after anonymize")
	}
}
