package coreserver

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestRejectBaseUninstall(t *testing.T) {
	if err := rejectBaseUninstall("core"); err == nil {
		t.Fatal("expected uninstall of core to be rejected, got nil")
	} else if !strings.Contains(strings.ToLower(err.Error()), "base") {
		t.Fatalf("expected a base-specific rejection message, got: %v", err)
	}
	for _, slug := range []string{"gizmos", "cogs", "sprockets", "doodads"} {
		if err := rejectBaseUninstall(slug); err != nil {
			t.Errorf("rejectBaseUninstall(%q) = %v, want nil (features are uninstallable)", slug, err)
		}
	}
}

// These guard the two admin authorization tiers:
//   - requireAdmin authorizes a PB superuser OR an owner/admin app user. It is
//     the /admin console's outer gate.
//   - requireOwner is stricter: PB superuser or role=owner only. Package
//     install/uninstall/version-apply sit behind it, because they rebuild what
//     the whole deployment runs. An ADMIN must be rejected here — that split is
//     the point of the tier, so it's asserted explicitly below.

// newSuperuserRecord creates a PB superuser and returns the record, for tests
// that need to set re.Auth to a superuser identity (whose id lives in the
// _superusers collection, not users).
func newSuperuserRecord(t *testing.T, app core.App, email string) *core.Record {
	t.Helper()
	supers, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		t.Fatal(err)
	}
	su := core.NewRecord(supers)
	su.SetEmail(email)
	su.SetPassword("Superuser1234!")
	if err := app.Save(su); err != nil {
		t.Fatalf("save superuser: %v", err)
	}
	return su
}

// newHeaderAuthEvent builds a request event with re.Auth set, modeling the
// normal Authorization-header path (the install/versions/etc. endpoints).
func newHeaderAuthEvent(app core.App, auth *core.Record) *core.RequestEvent {
	req := httptest.NewRequest("POST", "/api/admin/packages/install", nil)
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = httptest.NewRecorder()
	re.Auth = auth
	return re
}

// ---------- requireAdmin / requireOwner (header-auth path) ----------

// The full role matrix across both tiers, in one table so the owner/admin split
// is visible at a glance rather than spread over separate tests.
func TestAdminAndOwnerGuards_RoleMatrix(t *testing.T) {
	app := setupGuardTestApp(t)

	cases := []struct {
		role      string
		wantAdmin bool
		wantOwner bool
		reason    string
	}{
		{"owner", true, true, "the owner runs the console and manages packages"},
		{"admin", true, false, "an admin reaches the console but must NOT manage packages"},
		{"member", false, false, "a member has no console access at all"},
		{"guest", false, false, "a guest has no console access at all"},
	}

	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			user := makeUserWithRole(t, app, tc.role+"@test.local", tc.role)

			gotAdmin := requireAdmin(newHeaderAuthEvent(app, user)) == nil
			if gotAdmin != tc.wantAdmin {
				t.Errorf("requireAdmin(role=%s) authorized=%v, want %v — %s",
					tc.role, gotAdmin, tc.wantAdmin, tc.reason)
			}

			gotOwner := requireOwner(newHeaderAuthEvent(app, user)) == nil
			if gotOwner != tc.wantOwner {
				t.Errorf("requireOwner(role=%s) authorized=%v, want %v — %s",
					tc.role, gotOwner, tc.wantOwner, tc.reason)
			}
		})
	}
}

func TestAdminAndOwnerGuards_RejectAnonymous(t *testing.T) {
	app := setupGuardTestApp(t)

	if err := requireAdmin(newHeaderAuthEvent(app, nil)); err == nil {
		t.Error("an anonymous request must NOT be authorized for admin endpoints")
	}
	if err := requireOwner(newHeaderAuthEvent(app, nil)); err == nil {
		t.Error("an anonymous request must NOT be authorized for owner endpoints")
	}
}

// A PB superuser bypasses both tiers — it's the recovery identity, and the
// guards check HasSuperuserAuth before looking at any role.
func TestAdminAndOwnerGuards_SuperuserBypasses(t *testing.T) {
	app := setupGuardTestApp(t)

	su := newSuperuserRecord(t, app, "su@test.local")
	if err := requireAdmin(newHeaderAuthEvent(app, su)); err != nil {
		t.Errorf("a PB superuser should pass requireAdmin, got: %v", err)
	}
	if err := requireOwner(newHeaderAuthEvent(app, su)); err != nil {
		t.Errorf("a PB superuser should pass requireOwner, got: %v", err)
	}
}
