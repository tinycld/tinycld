package useraccount

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// setupUsersApp returns a test app with the standard "users" collection
// extended with a `disabled` bool field, plus one enabled user record.
func setupUsersApp(t *testing.T) (*tests.TestApp, *core.Record) {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	users.Fields.Add(&core.BoolField{Name: "disabled"})
	if err := app.Save(users); err != nil {
		t.Fatal(err)
	}

	user := core.NewRecord(users)
	user.SetEmail("enabled@test.local")
	user.SetPassword("Password123!")
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}

	return app, user
}

func TestIsSuspended_EnabledUserReturnsFalse(t *testing.T) {
	app, user := setupUsersApp(t)

	if IsSuspended(app, user.Id) {
		t.Error("IsSuspended(enabled user) = true, want false")
	}
}

func TestIsSuspended_DisabledUserReturnsTrue(t *testing.T) {
	app, user := setupUsersApp(t)

	fresh, err := app.FindRecordById("users", user.Id)
	if err != nil {
		t.Fatal(err)
	}
	fresh.Set("disabled", true)
	if err := app.Save(fresh); err != nil {
		t.Fatal(err)
	}

	if !IsSuspended(app, user.Id) {
		t.Error("IsSuspended(disabled user) = false, want true")
	}
}

func TestIsSuspended_NonexistentUserFailsClosed(t *testing.T) {
	app, _ := setupUsersApp(t)

	if !IsSuspended(app, "does-not-exist") {
		t.Error("IsSuspended(nonexistent user) = false, want true — a token for a deleted user must not keep reading")
	}
}

func TestIsSuspended_EmptyUserIDFailsClosed(t *testing.T) {
	app, _ := setupUsersApp(t)

	if !IsSuspended(app, "") {
		t.Error(`IsSuspended("") = false, want true`)
	}
}

// A user disabled AFTER their record was first loaded must still read as
// suspended. This is the case an inline auth.GetBool("disabled") check gets
// wrong: that reads a token-issuance-time snapshot, so a suspension applied
// mid-session would go unnoticed until the token expired.
func TestIsSuspended_RefetchesRatherThanTrustingASnapshot(t *testing.T) {
	app, user := setupUsersApp(t)

	// A stale in-memory copy, as a caller holding re.Auth would have.
	stale, err := app.FindRecordById("users", user.Id)
	if err != nil {
		t.Fatal(err)
	}

	fresh, err := app.FindRecordById("users", user.Id)
	if err != nil {
		t.Fatal(err)
	}
	fresh.Set("disabled", true)
	if err := app.Save(fresh); err != nil {
		t.Fatal(err)
	}

	if stale.GetBool("disabled") {
		t.Fatal("precondition: the stale copy should still read enabled")
	}
	if !IsSuspended(app, stale.Id) {
		t.Error("IsSuspended must re-fetch, not trust a record loaded before the suspension")
	}
}
