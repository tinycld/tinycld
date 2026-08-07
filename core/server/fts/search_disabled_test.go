package fts

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// search_disabled_test.go covers the two seams Search's security checks pass
// through: the disabled/missing-user gate (isDisabled) and the nil-Scope
// fail-closed guard. It does not attempt an end-to-end proof against a real
// FTS-backed collection with rows and a membership table — that harness
// (tests.NewTestApp() plus a seeded collection, à la drive's
// search_disabled_test.go) belongs to the cards package task that actually
// owns such a collection. Here we only have the "users" collection PocketBase
// ships in every test app, which is exactly what isDisabled reads.

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

func TestIsDisabled_EnabledUserReturnsFalse(t *testing.T) {
	app, user := setupUsersApp(t)

	if isDisabled(app, user.Id) {
		t.Error("isDisabled(enabled user) = true, want false")
	}
}

func TestIsDisabled_DisabledUserReturnsTrue(t *testing.T) {
	app, user := setupUsersApp(t)

	fresh, err := app.FindRecordById("users", user.Id)
	if err != nil {
		t.Fatal(err)
	}
	fresh.Set("disabled", true)
	if err := app.Save(fresh); err != nil {
		t.Fatal(err)
	}

	if !isDisabled(app, user.Id) {
		t.Error("isDisabled(disabled user) = false, want true")
	}
}

func TestIsDisabled_NonexistentUserFailsClosed(t *testing.T) {
	app, _ := setupUsersApp(t)

	if !isDisabled(app, "does-not-exist") {
		t.Error("isDisabled(nonexistent user) = false, want true — a token for a deleted user must not keep reading")
	}
}

func TestIsDisabled_EmptyUserIDFailsClosed(t *testing.T) {
	app, _ := setupUsersApp(t)

	if !isDisabled(app, "") {
		t.Error("isDisabled(\"\") = false, want true")
	}
}

// TestSearch_NilScopeReturnsNoRowsNotPanic proves the fail-closed guard: a
// Config that omits Scope must return zero rows, never an unscoped query
// (which would hand every row in the table to any authenticated caller) and
// never a panic (the zero value of the Scope interface is nil).
func TestSearch_NilScopeReturnsNoRowsNotPanic(t *testing.T) {
	app, user := setupUsersApp(t)

	cfg := Config{
		Slug:       "test",
		Collection: "users",
		Table:      "fts_users_missing", // never reached if the guard fires first
	}

	results, total, err := Search(app, cfg, user.Id, SearchOpts{Query: "anything", Limit: 25})
	if err != nil {
		t.Fatalf("Search with nil Scope returned an error instead of failing closed: %v", err)
	}
	if results != nil || total != 0 {
		t.Fatalf("Search with nil Scope = (%v, %d), want (nil, 0)", results, total)
	}
}
