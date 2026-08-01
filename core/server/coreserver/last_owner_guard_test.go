package coreserver

import (
	"errors"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// last_owner_guard_test.go proves an org cannot be left with zero active
// owners. Non-owners cannot assign the owner role (users field guard +
// updateRule), so a zero-owner org is unrecoverable without a superuser —
// the review found the Members drawer computes isLastOwner but only renders
// helper text with it, and no server backstop existed at all.
//
// "Active" matters: a disabled owner cannot log in to promote anyone, so a
// disabled owner must not count as the org's remaining owner.

func setupLastOwnerApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app := setupGuardTestApp(t)
	registerLastOwnerGuardCore(app)
	return app
}

// deleteAsAuthenticated mirrors updateAsAuthenticated for the delete-request
// hook chain.
func deleteAsAuthenticated(
	t *testing.T,
	app *tests.TestApp,
	caller *core.Record,
	target *core.Record,
) error {
	t.Helper()
	fresh, err := app.FindRecordById("users", target.Id)
	if err != nil {
		t.Fatalf("reload target: %v", err)
	}
	usersCol, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	e := &core.RecordRequestEvent{
		RequestEvent: &core.RequestEvent{Auth: caller, App: app},
		Record:       fresh,
	}
	e.Collection = usersCol
	return app.OnRecordDeleteRequest("users").Trigger(e, func(_ *core.RecordRequestEvent) error {
		return app.Delete(fresh)
	})
}

func TestLastOwnerGuard_BlocksDemotingLastOwner(t *testing.T) {
	app := setupLastOwnerApp(t)
	owner := makeUserWithRole(t, app, "solo-owner@test.local", "owner")
	admin := makeUserWithRole(t, app, "helper-admin@test.local", "admin")

	err := updateAsAuthenticated(t, app, admin, owner, func(r *core.Record) {
		r.Set("role", "member")
	})
	if err == nil {
		t.Fatal("demoting the last owner should be rejected")
	}

	fresh, _ := app.FindRecordById("users", owner.Id)
	if fresh.GetString("role") != "owner" {
		t.Errorf("role should still be owner, got %q", fresh.GetString("role"))
	}
}

func TestLastOwnerGuard_AllowsDemotionWithAnotherOwner(t *testing.T) {
	app := setupLastOwnerApp(t)
	ownerA := makeUserWithRole(t, app, "owner-a@test.local", "owner")
	ownerB := makeUserWithRole(t, app, "owner-b@test.local", "owner")

	if err := updateAsAuthenticated(t, app, ownerA, ownerB, func(r *core.Record) {
		r.Set("role", "admin")
	}); err != nil {
		t.Fatalf("demoting one of two owners should be allowed: %v", err)
	}

	fresh, _ := app.FindRecordById("users", ownerB.Id)
	if fresh.GetString("role") != "admin" {
		t.Errorf("role should be admin, got %q", fresh.GetString("role"))
	}
}

func TestLastOwnerGuard_BlocksDisablingLastOwner(t *testing.T) {
	app := setupLastOwnerApp(t)
	owner := makeUserWithRole(t, app, "solo-owner2@test.local", "owner")
	admin := makeUserWithRole(t, app, "helper-admin2@test.local", "admin")

	err := updateAsAuthenticated(t, app, admin, owner, func(r *core.Record) {
		r.Set("disabled", true)
	})
	if err == nil {
		t.Fatal("disabling the last owner should be rejected")
	}

	fresh, _ := app.FindRecordById("users", owner.Id)
	if fresh.GetBool("disabled") {
		t.Error("owner should not be disabled")
	}
}

// A disabled owner cannot promote anyone, so they must not count as the
// remaining owner when deciding whether a demotion is safe.
func TestLastOwnerGuard_DisabledOwnerDoesNotCount(t *testing.T) {
	app := setupLastOwnerApp(t)
	dormant := makeUserWithRole(t, app, "dormant-owner@test.local", "owner")
	dormant.Set("disabled", true)
	if err := app.Save(dormant); err != nil {
		t.Fatal(err)
	}
	active := makeUserWithRole(t, app, "active-owner@test.local", "owner")
	admin := makeUserWithRole(t, app, "helper-admin3@test.local", "admin")

	err := updateAsAuthenticated(t, app, admin, active, func(r *core.Record) {
		r.Set("role", "member")
	})
	if err == nil {
		t.Fatal("demoting the only ACTIVE owner should be rejected even though a disabled owner row exists")
	}
}

func TestLastOwnerGuard_BlocksDeletingLastOwner(t *testing.T) {
	app := setupLastOwnerApp(t)
	owner := makeUserWithRole(t, app, "solo-owner3@test.local", "owner")
	admin := makeUserWithRole(t, app, "helper-admin4@test.local", "admin")

	if err := deleteAsAuthenticated(t, app, admin, owner); err == nil {
		t.Fatal("deleting the last owner should be rejected")
	}
	if _, err := app.FindRecordById("users", owner.Id); err != nil {
		t.Fatalf("owner record should still exist: %v", err)
	}
}

func TestLastOwnerGuard_AllowsDeletingNonOwner(t *testing.T) {
	app := setupLastOwnerApp(t)
	makeUserWithRole(t, app, "the-owner@test.local", "owner")
	member := makeUserWithRole(t, app, "just-member@test.local", "member")
	admin := makeUserWithRole(t, app, "helper-admin5@test.local", "admin")

	if err := deleteAsAuthenticated(t, app, admin, member); err != nil {
		t.Fatalf("deleting a member should be allowed: %v", err)
	}
}

// Operators can always re-promote someone from the superuser console, so the
// guard steps aside for them — consistent with the field-guard bypass, and it
// keeps decommissioning flows possible.
func TestLastOwnerGuard_SuperuserBypasses(t *testing.T) {
	app := setupLastOwnerApp(t)
	owner := makeUserWithRole(t, app, "su-demote-owner@test.local", "owner")

	superuser, err := app.FindFirstRecordByFilter(core.CollectionNameSuperusers, "id != ''")
	if err != nil {
		t.Fatalf("find superuser: %v", err)
	}

	if err := updateAsAuthenticated(t, app, superuser, owner, func(r *core.Record) {
		r.Set("role", "member")
	}); err != nil {
		t.Fatalf("superuser demotion should bypass the guard: %v", err)
	}
}

// The self-service endpoints (/api/account/disable, /api/account/delete) and
// the admin offboard endpoint mutate through model-level saves that never
// pass the record-request hooks, so they share this check instead.
func TestEnsureNotLastActiveOwner(t *testing.T) {
	app := setupLastOwnerApp(t)
	owner := makeUserWithRole(t, app, "endpoint-owner@test.local", "owner")
	member := makeUserWithRole(t, app, "endpoint-member@test.local", "member")

	if err := ensureNotLastActiveOwner(app, owner); !errors.Is(err, errLastOwner) {
		t.Fatalf("sole owner should be reported as last owner, got %v", err)
	}
	if err := ensureNotLastActiveOwner(app, member); err != nil {
		t.Fatalf("member should pass the check: %v", err)
	}

	makeUserWithRole(t, app, "endpoint-owner2@test.local", "owner")
	if err := ensureNotLastActiveOwner(app, owner); err != nil {
		t.Fatalf("with a second owner the check should pass: %v", err)
	}
}
