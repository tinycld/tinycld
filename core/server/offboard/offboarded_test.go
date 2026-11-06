package offboard

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestIsOffboarded_NilIsFalse(t *testing.T) {
	if IsOffboarded(nil) {
		t.Error("IsOffboarded(nil) = true, want false")
	}
}

func TestIsOffboarded_FreshUserIsFalse(t *testing.T) {
	app := setupTestApp(t)
	alice := makeUser(t, app, "alice@test.local")

	if IsOffboarded(alice) {
		t.Error("fresh user reported as offboarded")
	}
}

func TestIsOffboarded_TrueAfterOffboardUser(t *testing.T) {
	app := setupTestApp(t)
	alice := makeUser(t, app, "alice@test.local")

	if _, err := OffboardUser(app, alice.Id, Plan{Mode: ModeKeep}, alice.Id); err != nil {
		t.Fatalf("OffboardUser: %v", err)
	}
	reread, err := app.FindRecordById("users", alice.Id)
	if err != nil {
		t.Fatalf("re-find alice: %v", err)
	}
	if !IsOffboarded(reread) {
		t.Errorf("offboarded user not reported as offboarded (email %q)", reread.Email())
	}
}

func TestIsOffboarded_TrueAfterAnonymizeUser(t *testing.T) {
	app := setupTestApp(t)
	alice := makeUser(t, app, "alice@test.local")

	if err := AnonymizeUser(app, alice.Id); err != nil {
		t.Fatalf("AnonymizeUser: %v", err)
	}
	reread, err := app.FindRecordById("users", alice.Id)
	if err != nil {
		t.Fatalf("re-find alice: %v", err)
	}
	if !IsOffboarded(reread) {
		t.Errorf("anonymized user not reported as offboarded (email %q)", reread.Email())
	}
}

// A copied sentinel marks nobody: it is bound to the id of the user it was
// written for.
func TestIsOffboarded_SentinelOfAnotherUserIsFalse(t *testing.T) {
	app := setupTestApp(t)
	alice := makeUser(t, app, "alice@test.local")
	bob := makeUser(t, app, "bob@test.local")

	bob.SetEmail(sentinelEmail(alice.Id))
	if IsOffboarded(bob) {
		t.Error("user holding another user's sentinel email reported as offboarded")
	}
}

// bindFictionalGrantHook stands in for a package hook that grants rows from a
// users update. It counts grants per user id.
func bindFictionalGrantHook(app core.App) map[string]int {
	grants := map[string]int{}
	app.OnRecordAfterUpdateSuccess("users").BindFunc(func(e *core.RecordEvent) error {
		if IsOffboarded(e.Record) {
			return e.Next()
		}
		grants[e.Record.Id]++
		return e.Next()
	})
	return grants
}

func TestIsOffboarded_FictionalPackageHookSkipsOffboardedUser(t *testing.T) {
	app := setupTestApp(t)
	alice := makeUser(t, app, "alice@test.local")
	grants := bindFictionalGrantHook(app)

	alice.Set("name", "Alice B")
	if err := app.Save(alice); err != nil {
		t.Fatalf("ordinary update: %v", err)
	}
	if grants[alice.Id] != 1 {
		t.Fatalf("ordinary update grants = %d, want 1", grants[alice.Id])
	}

	if _, err := OffboardUser(app, alice.Id, Plan{Mode: ModeKeep}, alice.Id); err != nil {
		t.Fatalf("OffboardUser: %v", err)
	}
	if grants[alice.Id] != 1 {
		t.Errorf("grants after offboard = %d, want 1 (hook must skip the offboarded user)", grants[alice.Id])
	}

	reread, err := app.FindRecordById("users", alice.Id)
	if err != nil {
		t.Fatalf("re-find alice: %v", err)
	}
	reread.Set("name", "Edited by admin")
	if err := app.Save(reread); err != nil {
		t.Fatalf("later admin save: %v", err)
	}
	if grants[alice.Id] != 1 {
		t.Errorf("grants after later admin save = %d, want 1 (marker must persist)", grants[alice.Id])
	}
}
