package coreserver

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"
	"tinycld.org/core/rlstest"
)

// org_pkg_access carries per-user package grants. The UI that manages it,
// PackageAccessPanel, renders inside Settings > Members — gated on isAdmin —
// but the collection shipped superuser-only for writes and self-only for
// reads, so an admin saw an always-empty list and every toggle 403'd silently
// (the panel's mutations have no onError). Guests were the worst case: they
// receive packages only through a row here, so with no writable path they
// could never be granted anything at all.
//
// 1950000000 opens it to owners/admins while keeping a
// member's read of their own row, which use-pkg-access depends on.
//
// As in guest_rls_test.go the rules are NOT restated here — the real
// pb_migrations are applied, so a later migration that restates these rules
// and drops a clause turns these tests red rather than leaving them checking a
// stale copy.

type pkgAccessRLSEnv struct {
	app *tests.TestApp

	admin  *core.Record
	member *core.Record
	other  *core.Record

	adminToken  string
	memberToken string
}

func setupPkgAccessRLSApp(t *testing.T) *pkgAccessRLSEnv {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	// Same fixture reconciliation as guest_rls_test: drop the bundled username
	// index so 1820000000 applies the way it does against a real DB.
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	var kept types.JSONArray[string]
	for _, idx := range users.Indexes {
		if !strings.Contains(idx, "username") {
			kept = append(kept, idx)
		}
	}
	users.Indexes = kept
	users.PasswordAuth.IdentityFields = []string{"email"}
	if err := app.Save(users); err != nil {
		t.Fatalf("drop fixture username index: %v", err)
	}

	rlstest.Apply(t, app, rlstest.MigrationsDir(t, "../pb_migrations"))

	admin := guestRLSUser(t, app, "admin@test.local", "admin")
	member := guestRLSUser(t, app, "member@test.local", "member")
	other := guestRLSUser(t, app, "other@test.local", "member")

	adminToken, err := admin.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	memberToken, err := member.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}

	return &pkgAccessRLSEnv{
		app: app, admin: admin, member: member, other: other,
		adminToken: adminToken, memberToken: memberToken,
	}
}

func pkgAccessRow(t *testing.T, app core.App, userID, pkg, access string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("org_pkg_access")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.Set("user", userID)
	r.Set("pkg", pkg)
	r.Set("access", access)
	if err := app.Save(r); err != nil {
		t.Fatalf("seed org_pkg_access: %v", err)
	}
	return r
}

// The defect: an admin could not create a grant, so no member's package access
// could ever be changed and no guest could be granted anything.
func TestOrgPkgAccess_AdminCanCreate(t *testing.T) {
	env := setupPkgAccessRLSApp(t)

	scenario := &tests.ApiScenario{
		Method:  http.MethodPost,
		URL:     "/api/collections/org_pkg_access/records",
		Headers: map[string]string{"Authorization": env.adminToken},
		Body: strings.NewReader(
			`{"user":"` + env.member.Id + `","pkg":"mail","access":"readonly"}`),
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{`"pkg":"mail"`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return env.app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

// An admin must see other users' rows, or the panel renders empty for every
// member they open.
func TestOrgPkgAccess_AdminSeesOtherUsersRows(t *testing.T) {
	env := setupPkgAccessRLSApp(t)
	pkgAccessRow(t, env.app, env.member.Id, "mail", "readonly")

	runListScenario(t, env.app, "org_pkg_access", env.adminToken, []string{`"pkg":"mail"`})
}

// A member keeps reading their OWN row — use-pkg-access resolves the current
// user's access from it, so losing this read would fail every gate closed.
func TestOrgPkgAccess_MemberReadsOwnRow(t *testing.T) {
	env := setupPkgAccessRLSApp(t)
	pkgAccessRow(t, env.app, env.member.Id, "drive", "readonly")

	runListScenario(t, env.app, "org_pkg_access", env.memberToken, []string{`"pkg":"drive"`})
}

// ...and only their own. Opening reads to admins must not leak another user's
// grants to an ordinary member.
func TestOrgPkgAccess_MemberCannotSeeOtherUsersRows(t *testing.T) {
	env := setupPkgAccessRLSApp(t)
	pkgAccessRow(t, env.app, env.other.Id, "calendar", "readonly")

	runListScenario(t, env.app, "org_pkg_access", env.memberToken, []string{`"totalItems":0`})
}

// A member must not be able to grant themselves access to a package an admin
// restricted — the whole point of the collection.
func TestOrgPkgAccess_MemberCannotCreate(t *testing.T) {
	env := setupPkgAccessRLSApp(t)

	scenario := &tests.ApiScenario{
		Method:  http.MethodPost,
		URL:     "/api/collections/org_pkg_access/records",
		Headers: map[string]string{"Authorization": env.memberToken},
		Body: strings.NewReader(
			`{"user":"` + env.member.Id + `","pkg":"mail","access":"full"}`),
		ExpectedStatus:        http.StatusBadRequest,
		ExpectedContent:       []string{`"message":"Failed to create record."`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return env.app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

// Anonymous callers get nothing.
func TestOrgPkgAccess_AnonDenied(t *testing.T) {
	env := setupPkgAccessRLSApp(t)
	pkgAccessRow(t, env.app, env.member.Id, "mail", "readonly")

	runListScenario(t, env.app, "org_pkg_access", "", []string{`"totalItems":0`})
}
