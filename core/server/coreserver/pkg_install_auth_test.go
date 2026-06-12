package coreserver

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func TestRejectBaseUninstall(t *testing.T) {
	if err := rejectBaseUninstall("core"); err == nil {
		t.Fatal("expected uninstall of core to be rejected, got nil")
	} else if !strings.Contains(strings.ToLower(err.Error()), "base") {
		t.Fatalf("expected a base-specific rejection message, got: %v", err)
	}
	for _, slug := range []string{"mail", "drive", "calendar", "contacts"} {
		if err := rejectBaseUninstall(slug); err != nil {
			t.Errorf("rejectBaseUninstall(%q) = %v, want nil (features are uninstallable)", slug, err)
		}
	}
}

// These guard the admin authorization paths:
//   - requireAdmin authorizes a PB superuser OR an app user listed in
//     super_admins, and rejects plain users / anonymous requests.
//   - requireSuperuserOrToken adds a ?token= query-param path for the SSE
//     progress stream (EventSource can't send headers). The token's auth-record
//     lookup must use the token TYPE, not a collection id — an earlier version
//     passed the superusers collection id, which matched no valid type and 403'd
//     every install's progress stream. The security inverse (a plain user's
//     token must NOT authorize the endpoint) is guarded too.

// createSuperAdminsCollection mirrors pb_migrations/1910000005_create_super_admins.js
// in-memory, since tests.NewTestApp() ships only PB's default fixture collections.
func createSuperAdminsCollection(t *testing.T, app core.App, usersID string) {
	t.Helper()
	c := core.NewBaseCollection("super_admins")
	c.Id = "pbc_super_admins"
	c.Fields.Add(&core.RelationField{
		Name: "user", Required: true, CollectionId: usersID,
		CascadeDelete: true, MaxSelect: 1,
	})
	c.Fields.Add(&core.RelationField{
		Name: "created_by", CollectionId: usersID, MaxSelect: 1,
	})
	// Mirror the migration's autodate fields so handlers that sort by -created
	// (handleListSuperAdmins) resolve a real column rather than erroring with
	// invalid sort field "created".
	c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
	// Mirror the migration's unique index on user so duplicate grants are
	// rejected at the DB layer too, not only by the handler's isSuperAdmin check.
	c.AddIndex("idx_super_admins_user", true, "user", "")
	if err := app.Save(c); err != nil {
		t.Fatalf("save super_admins collection: %v", err)
	}
}

// newGuardUser creates a regular app user and returns the record.
func newGuardUser(t *testing.T, app core.App, email string) *core.Record {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	user := core.NewRecord(users)
	user.SetEmail(email)
	user.SetPassword("Regular1234!")
	if err := app.Save(user); err != nil {
		t.Fatalf("save user: %v", err)
	}
	return user
}

// grantSuperAdmin inserts a super_admins row for the given user.
func grantSuperAdmin(t *testing.T, app core.App, userID string) {
	t.Helper()
	c, err := app.FindCollectionByNameOrId("super_admins")
	if err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(c)
	rec.Set("user", userID)
	if err := app.Save(rec); err != nil {
		t.Fatalf("grant super admin: %v", err)
	}
}

func newAuthGuardEvent(app core.App, token string) *core.RequestEvent {
	req := httptest.NewRequest("GET", "/api/admin/packages/events/job_1?token="+token, nil)
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = httptest.NewRecorder()
	return re
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

func newGuardSuperuserToken(t *testing.T, app core.App) string {
	t.Helper()
	su, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(su)
	rec.SetEmail("ssetoken@test.local")
	rec.SetPassword("Superuser1234!")
	if err := app.Save(rec); err != nil {
		t.Fatalf("save superuser: %v", err)
	}
	tok, err := rec.NewAuthToken()
	if err != nil {
		t.Fatalf("new auth token: %v", err)
	}
	return tok
}

// ---------- requireAdmin (header-auth path) ----------

func TestRequireAdmin_SuperAdminUser(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	createSuperAdminsCollection(t, app, users.Id)

	user := newGuardUser(t, app, "admin@test.local")
	grantSuperAdmin(t, app, user.Id)

	if err := requireAdmin(app, newHeaderAuthEvent(app, user)); err != nil {
		t.Fatalf("super-admin app user should be authorized, got: %v", err)
	}
}

func TestRequireAdmin_RejectsPlainUser(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	createSuperAdminsCollection(t, app, users.Id)

	user := newGuardUser(t, app, "plain@test.local")
	if err := requireAdmin(app, newHeaderAuthEvent(app, user)); err == nil {
		t.Fatal("a plain user must NOT be authorized for admin endpoints")
	}
}

func TestRequireAdmin_RejectsAnonymous(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	createSuperAdminsCollection(t, app, users.Id)

	if err := requireAdmin(app, newHeaderAuthEvent(app, nil)); err == nil {
		t.Fatal("an anonymous request must NOT be authorized for admin endpoints")
	}
}

// ---------- requireSuperuserOrToken (SSE token path) ----------

func TestRequireSuperuserOrToken_ValidSuperuserToken(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	createSuperAdminsCollection(t, app, users.Id)

	token := newGuardSuperuserToken(t, app)
	if err := requireSuperuserOrToken(app, newAuthGuardEvent(app, token)); err != nil {
		t.Fatalf("valid superuser token should be authorized, got: %v", err)
	}
}

func TestRequireSuperuserOrToken_ValidSuperAdminToken(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	createSuperAdminsCollection(t, app, users.Id)

	user := newGuardUser(t, app, "sse-admin@test.local")
	grantSuperAdmin(t, app, user.Id)
	tok, err := user.NewAuthToken()
	if err != nil {
		t.Fatalf("new auth token: %v", err)
	}

	if err := requireSuperuserOrToken(app, newAuthGuardEvent(app, tok)); err != nil {
		t.Fatalf("super-admin app user's token should be authorized, got: %v", err)
	}
}

func TestRequireSuperuserOrToken_RejectsEmptyAndGarbage(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	createSuperAdminsCollection(t, app, users.Id)

	for _, tok := range []string{"", "not-a-jwt", "a.b.c"} {
		if err := requireSuperuserOrToken(app, newAuthGuardEvent(app, tok)); err == nil {
			t.Fatalf("token %q should be rejected", tok)
		}
	}
}

func TestRequireSuperuserOrToken_RejectsRegularUserToken(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	createSuperAdminsCollection(t, app, users.Id)

	user := newGuardUser(t, app, "regular@test.local")
	tok, err := user.NewAuthToken()
	if err != nil {
		t.Fatalf("new auth token: %v", err)
	}

	if err := requireSuperuserOrToken(app, newAuthGuardEvent(app, tok)); err == nil {
		t.Fatal("a regular user's token must NOT authorize the superuser endpoint")
	}
}
