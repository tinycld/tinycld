package coreserver

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/pkgaccess"
	"tinycld.org/core/rlstest"
)

// pkg_access_guard_test.go proves org_pkg_access levels are enforced
// SERVER-SIDE for writes. Before this guard the level was UI-advisory:
// usePkgAccess gated screens, but a readonly (or none) user's direct REST
// write to a package's collections sailed through on the collection rules,
// which know nothing about package access.
//
// Ownership is by naming convention — a collection belongs to the installed
// package (pkg_registry) whose slug it carries as `<slug>` or `<slug>_*` —
// so enforcement covers TS-only packages with no Go of their own.

type pkgAccessEnv struct {
	app    *tests.TestApp
	member *core.Record
	guest  *core.Record
	admin  *core.Record

	memberToken string
	guestToken  string
	adminToken  string
}

func setupPkgAccessApp(t *testing.T) *pkgAccessEnv {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	// Same fixture normalization as guest_rls_test.go: the bundled users
	// collection pre-carries the username index 1820000000 adds.
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	kept := users.Indexes[:0]
	for _, idx := range users.Indexes {
		if !strings.Contains(idx, "username") {
			kept = append(kept, idx)
		}
	}
	users.Indexes = kept
	users.PasswordAuth.IdentityFields = []string{"email"}
	if err := app.Save(users); err != nil {
		t.Fatal(err)
	}

	rlstest.Apply(t, app, rlstest.MigrationsDir(t, "../pb_migrations"))

	// A fake installed package "demo" owning demo_notes by naming convention,
	// with rules any authenticated user passes — so a refused write below is
	// the package-access guard, never the rule.
	registry, err := app.FindCollectionByNameOrId("pkg_registry")
	if err != nil {
		t.Fatal(err)
	}
	pkg := core.NewRecord(registry)
	pkg.Set("name", "Demo")
	pkg.Set("slug", "demo")
	pkg.Set("npm_package", "@tinycld/demo")
	pkg.Set("version", "1.0.0")
	pkg.Set("status", "installed")
	if err := app.Save(pkg); err != nil {
		t.Fatal(err)
	}

	authed := "@request.auth.id != ''"
	notes := core.NewBaseCollection("demo_notes")
	notes.Fields.Add(&core.TextField{Name: "body"})
	notes.ListRule = &authed
	notes.ViewRule = &authed
	notes.CreateRule = &authed
	notes.UpdateRule = &authed
	notes.DeleteRule = &authed
	if err := app.Save(notes); err != nil {
		t.Fatal(err)
	}

	// A collection owned by NO installed package: the guard must not touch it.
	scratch := core.NewBaseCollection("freeform_scratch")
	scratch.Fields.Add(&core.TextField{Name: "body"})
	scratch.ListRule = &authed
	scratch.ViewRule = &authed
	scratch.CreateRule = &authed
	scratch.UpdateRule = &authed
	scratch.DeleteRule = &authed
	if err := app.Save(scratch); err != nil {
		t.Fatal(err)
	}

	// The binding under test — registerSharedCore ships the same call.
	pkgaccess.Register(app)

	member := guestRLSUser(t, app, "pkg-member@test.local", "member")
	guest := guestRLSUser(t, app, "pkg-guest@test.local", "guest")
	admin := guestRLSUser(t, app, "pkg-admin@test.local", "admin")

	env := &pkgAccessEnv{app: app, member: member, guest: guest, admin: admin}
	if env.memberToken, err = member.NewAuthToken(); err != nil {
		t.Fatal(err)
	}
	if env.guestToken, err = guest.NewAuthToken(); err != nil {
		t.Fatal(err)
	}
	if env.adminToken, err = admin.NewAuthToken(); err != nil {
		t.Fatal(err)
	}
	return env
}

// setAccess writes an org_pkg_access override row for user × pkg.
func setAccess(t *testing.T, app core.App, user *core.Record, pkg, level string) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("org_pkg_access")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.Set("user", user.Id)
	r.Set("pkg", pkg)
	r.Set("access", level)
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
}

// seedNote inserts a demo_notes row directly (system write; the guard only
// binds request hooks) and returns its id.
func seedNote(t *testing.T, app core.App, collection string) string {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(collection)
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.Set("body", "seeded")
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return r.Id
}

func runWrite(t *testing.T, app *tests.TestApp, method, url, token, body string, wantStatus int, wantContent []string) {
	t.Helper()
	scenario := &tests.ApiScenario{
		Method:                method,
		URL:                   url,
		Headers:               map[string]string{"Authorization": token, "Content-Type": "application/json"},
		ExpectedStatus:        wantStatus,
		ExpectedContent:       wantContent,
		TestAppFactory:        func(t testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	if body != "" {
		scenario.Body = strings.NewReader(body)
	}
	if wantStatus == http.StatusNoContent {
		scenario.ExpectedContent = nil
	}
	scenario.Test(t)
}

// One fresh env per scenario, each inside its own subtest: ApiScenario.Test
// re-triggers OnServe (reusing an app panics on duplicate routes — see
// guest_rls), and rlstest.Apply's global-migrations snapshot restores on the
// OWNING t's cleanup, so consecutive setups need distinct t scopes.
func TestPkgAccess_ReadonlyMemberWritesRefused(t *testing.T) {
	t.Run("create refused", func(t *testing.T) {
		env := setupPkgAccessApp(t)
		setAccess(t, env.app, env.member, "demo", "readonly")
		runWrite(t, env.app, http.MethodPost, "/api/collections/demo_notes/records",
			env.memberToken, `{"body":"nope"}`, http.StatusForbidden, []string{"read-only"})
	})

	t.Run("update refused", func(t *testing.T) {
		env := setupPkgAccessApp(t)
		setAccess(t, env.app, env.member, "demo", "readonly")
		noteID := seedNote(t, env.app, "demo_notes")
		runWrite(t, env.app, http.MethodPatch, "/api/collections/demo_notes/records/"+noteID,
			env.memberToken, `{"body":"nope"}`, http.StatusForbidden, []string{"read-only"})
	})

	t.Run("delete refused", func(t *testing.T) {
		env := setupPkgAccessApp(t)
		setAccess(t, env.app, env.member, "demo", "readonly")
		noteID := seedNote(t, env.app, "demo_notes")
		runWrite(t, env.app, http.MethodDelete, "/api/collections/demo_notes/records/"+noteID,
			env.memberToken, "", http.StatusForbidden, []string{"read-only"})
	})

	// Reads stay rule-governed: readonly means read, not locked out.
	t.Run("reads still allowed", func(t *testing.T) {
		env := setupPkgAccessApp(t)
		setAccess(t, env.app, env.member, "demo", "readonly")
		seedNote(t, env.app, "demo_notes")
		runWrite(t, env.app, http.MethodGet, "/api/collections/demo_notes/records",
			env.memberToken, "", http.StatusOK, []string{"seeded"})
	})
}

func TestPkgAccess_MemberDefaultIsFull(t *testing.T) {
	env := setupPkgAccessApp(t)
	runWrite(t, env.app, http.MethodPost, "/api/collections/demo_notes/records",
		env.memberToken, `{"body":"fine"}`, http.StatusOK, []string{"fine"})
}

func TestPkgAccess_GuestDefaultIsNone(t *testing.T) {
	env := setupPkgAccessApp(t)
	runWrite(t, env.app, http.MethodPost, "/api/collections/demo_notes/records",
		env.guestToken, `{"body":"nope"}`, http.StatusForbidden, []string{"access"})
}

func TestPkgAccess_GuestGrantedFullCanWrite(t *testing.T) {
	env := setupPkgAccessApp(t)
	setAccess(t, env.app, env.guest, "demo", "full")
	runWrite(t, env.app, http.MethodPost, "/api/collections/demo_notes/records",
		env.guestToken, `{"body":"granted"}`, http.StatusOK, []string{"granted"})
}

// Admins (and owners) are always full — a stray override row must not lock
// out the people who manage the overrides.
func TestPkgAccess_AdminIgnoresOverrideRows(t *testing.T) {
	env := setupPkgAccessApp(t)
	setAccess(t, env.app, env.admin, "demo", "none")
	runWrite(t, env.app, http.MethodPost, "/api/collections/demo_notes/records",
		env.adminToken, `{"body":"admin"}`, http.StatusOK, []string{"admin"})
}

// Collections owned by no installed package are outside package access
// entirely — the guard must not invent restrictions for core data.
func TestPkgAccess_UnownedCollectionsUntouched(t *testing.T) {
	t.Run("readonly member", func(t *testing.T) {
		env := setupPkgAccessApp(t)
		setAccess(t, env.app, env.member, "demo", "readonly")
		runWrite(t, env.app, http.MethodPost, "/api/collections/freeform_scratch/records",
			env.memberToken, `{"body":"ok"}`, http.StatusOK, []string{"ok"})
	})

	t.Run("readonly guest", func(t *testing.T) {
		env := setupPkgAccessApp(t)
		setAccess(t, env.app, env.guest, "demo", "readonly")
		runWrite(t, env.app, http.MethodPost, "/api/collections/freeform_scratch/records",
			env.guestToken, `{"body":"ok"}`, http.StatusOK, []string{"ok"})
	})
}
