package coreserver

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// guest_rls_test.go proves the collection access rules tightened by
// 1870000000_exclude_guests_from_org_rls.js against PocketBase's REAL rule
// engine.
//
// Single-org: a "guest" share-link visitor gets a real users record with
// role='guest'. The member-scoped collection rules granted access to any
// authenticated user, so a guest would leak the member roster, emails, audit
// log, settings and the package toggles. These tests assert that a
// role='guest' user is DENIED while a real (member/owner/admin) user is still
// ALLOWED. Role now lives on the users auth record, so the rules key on
// `@request.auth.role`.
//
// Each scenario builds a FRESH TestApp: ApiScenario.Test re-triggers OnServe,
// which re-registers PocketBase's built-in routes and panics on the duplicate
// route pattern if a single app is reused across scenarios.
//
// The rule strings MUST stay byte-for-byte identical to what the migration
// sets — they are the source of truth this test validates.

// ----- candidate rule predicates (mirror the migration verbatim) -----

// guestRLSNonGuest is the shared "authenticated non-guest" predicate.
const guestRLSNonGuest = `@request.auth.id != "" && @request.auth.role != "guest"`

// guestRLSUsersRule is users' tightened list/view rule: a non-guest sees
// everyone; a guest sees ONLY their own row (auth-refresh + self-fetch).
const guestRLSUsersRule = `(` + guestRLSNonGuest + `) || id = @request.auth.id`

// ---------------------------------------------------------------------------

type guestRLSEnv struct {
	app    *tests.TestApp
	member *core.Record
	guest  *core.Record
	// tokens
	memberToken string
	guestToken  string
}

// setupGuestRLSApp builds the users(role, is_demo) / labels / settings /
// audit_logs / org_pkg_enabled schema (single-org: no org FK), seeds a real
// member (role 'member') and a guest (role 'guest'), and returns auth tokens
// for each. Collection rules are NOT set here — each sub-test applies the
// candidate rule(s) for the collection under test, then exercises the API.
func setupGuestRLSApp(t *testing.T) *guestRLSEnv {
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
	users.Fields.Add(&core.BoolField{Name: "is_demo"})
	users.Fields.Add(&core.SelectField{
		Name: "role", MaxSelect: 1,
		Values: []string{"owner", "admin", "member", "guest"},
	})
	relaxUsernameMinLength(users)
	if err := app.Save(users); err != nil {
		t.Fatal(err)
	}

	labels := core.NewBaseCollection("labels")
	labels.Fields.Add(&core.TextField{Name: "name", Required: true})
	labels.Fields.Add(&core.TextField{Name: "color", Required: true})
	if err := app.Save(labels); err != nil {
		t.Fatal(err)
	}

	settings := core.NewBaseCollection("settings")
	settings.Fields.Add(&core.TextField{Name: "app", Required: true})
	settings.Fields.Add(&core.TextField{Name: "key", Required: true})
	settings.Fields.Add(&core.JSONField{Name: "value"})
	if err := app.Save(settings); err != nil {
		t.Fatal(err)
	}

	auditLogs := core.NewBaseCollection("audit_logs")
	auditLogs.Fields.Add(&core.TextField{Name: "action", Required: true})
	auditLogs.Fields.Add(&core.TextField{Name: "resource_type", Required: true})
	auditLogs.Fields.Add(&core.TextField{Name: "resource_id", Required: true})
	if err := app.Save(auditLogs); err != nil {
		t.Fatal(err)
	}

	orgPkgEnabled := core.NewBaseCollection("org_pkg_enabled")
	orgPkgEnabled.Fields.Add(&core.TextField{Name: "pkg", Required: true})
	orgPkgEnabled.Fields.Add(&core.BoolField{Name: "enabled"})
	if err := app.Save(orgPkgEnabled); err != nil {
		t.Fatal(err)
	}

	member := guestRLSUser(t, app, "member@test.local", "member")
	guest := guestRLSUser(t, app, "guest@test.local", "guest")

	memberToken, err := member.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	guestToken, err := guest.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}

	return &guestRLSEnv{
		app:         app,
		member:      member,
		guest:       guest,
		memberToken: memberToken,
		guestToken:  guestToken,
	}
}

func guestRLSUser(t *testing.T, app core.App, email, role string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.SetEmail(email)
	r.Set("username", DeriveUsername(email))
	r.Set("name", "Test")
	r.Set("role", role)
	r.SetVerified(true)
	r.SetPassword("Password123!")
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return r
}

// setListView applies a list+view rule to a collection and re-saves.
func setListView(t *testing.T, app core.App, name, rule string) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		t.Fatal(err)
	}
	col.ListRule = &rule
	col.ViewRule = &rule
	if err := app.Save(col); err != nil {
		t.Fatalf("set rule on %s: %v", name, err)
	}
}

// runListScenario hits the collection's list endpoint with the given token and
// asserts the response status + body content. A fresh app must be passed; the
// scenario keeps it alive (DisableTestAppCleanup) — the env's t.Cleanup frees it.
func runListScenario(t *testing.T, app *tests.TestApp, name, token string, wantContent []string) {
	t.Helper()
	scenario := &tests.ApiScenario{
		Method:                http.MethodGet,
		URL:                   "/api/collections/" + name + "/records",
		Headers:               map[string]string{"Authorization": token},
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       wantContent,
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

// ============================ users ============================

func TestGuestRLS_Users_GuestSeesOnlySelf(t *testing.T) {
	env := setupGuestRLSApp(t)
	setListView(t, env.app, "users", guestRLSUsersRule)

	// Guest must not enumerate members. With the `|| id = @request.auth.id`
	// carve-out the guest sees their own row (needed for the PB SDK's
	// authRefresh after pb.authStore.save) but no one else's.
	runListScenario(t, env.app, "users", env.guestToken, []string{`"totalItems":1`})
}

func TestGuestRLS_Users_GuestCannotSeeMemberEmail(t *testing.T) {
	env := setupGuestRLSApp(t)
	setListView(t, env.app, "users", guestRLSUsersRule)

	// The roster-leak property: the guest sees themselves (totalItems:1) but
	// no other member's email.
	scenario := &tests.ApiScenario{
		Method:                http.MethodGet,
		URL:                   "/api/collections/users/records",
		Headers:               map[string]string{"Authorization": env.guestToken},
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{`"totalItems":1`, "guest@test.local"},
		NotExpectedContent:    []string{"member@test.local"},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return env.app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

func TestGuestRLS_Users_MemberSeesOtherMembers(t *testing.T) {
	env := setupGuestRLSApp(t)
	setListView(t, env.app, "users", guestRLSUsersRule)

	// A non-guest member sees other users' records — including the seeded
	// guest and the member's own row. (The bundled PB test fixture also ships
	// a few extra users, so assert on visible content rather than a brittle
	// exact count.)
	// Emails are hidden unless emailVisibility is set, so assert on usernames
	// (always visible) — the guest's row being visible is the key property.
	runListScenario(t, env.app, "users", env.memberToken, []string{
		`"username":"member"`, `"username":"guest"`,
	})
}

// ============================ labels ============================

func TestGuestRLS_Labels_GuestDeniedMemberAllowed(t *testing.T) {
	env := setupGuestRLSApp(t)
	setAllCRUD(t, env.app, "labels", guestRLSNonGuest)

	// Seed a label so list has something for a member to see.
	labelsCol, _ := env.app.FindCollectionByNameOrId("labels")
	lbl := core.NewRecord(labelsCol)
	lbl.Set("name", "Important")
	lbl.Set("color", "#f00")
	if err := env.app.Save(lbl); err != nil {
		t.Fatal(err)
	}

	t.Run("guest list empty", func(t *testing.T) {
		runListScenario(t, env.app, "labels", env.guestToken, []string{`"totalItems":0`})
	})
}

func TestGuestRLS_Labels_MemberSeesLabels(t *testing.T) {
	env := setupGuestRLSApp(t)
	setAllCRUD(t, env.app, "labels", guestRLSNonGuest)
	labelsCol, _ := env.app.FindCollectionByNameOrId("labels")
	lbl := core.NewRecord(labelsCol)
	lbl.Set("name", "Important")
	lbl.Set("color", "#f00")
	if err := env.app.Save(lbl); err != nil {
		t.Fatal(err)
	}
	runListScenario(t, env.app, "labels", env.memberToken, []string{`"totalItems":1`, "Important"})
}

func TestGuestRLS_Labels_GuestCannotCreate(t *testing.T) {
	env := setupGuestRLSApp(t)
	setAllCRUD(t, env.app, "labels", guestRLSNonGuest)

	scenario := &tests.ApiScenario{
		Method:                http.MethodPost,
		URL:                   "/api/collections/labels/records",
		Body:                  strings.NewReader(`{"name":"X","color":"#000"}`),
		Headers:               map[string]string{"Authorization": env.guestToken, "Content-Type": "application/json"},
		ExpectedStatus:        http.StatusBadRequest,
		ExpectedContent:       []string{`"message"`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return env.app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

// ============================ settings ============================

func TestGuestRLS_Settings_GuestDenied(t *testing.T) {
	env := setupGuestRLSApp(t)
	setListViewCreateUpdate(t, env.app, "settings", guestRLSNonGuest)

	settingsCol, _ := env.app.FindCollectionByNameOrId("settings")
	s := core.NewRecord(settingsCol)
	s.Set("app", "core")
	s.Set("key", "theme")
	if err := env.app.Save(s); err != nil {
		t.Fatal(err)
	}

	runListScenario(t, env.app, "settings", env.guestToken, []string{`"totalItems":0`})
}

func TestGuestRLS_Settings_MemberAllowed(t *testing.T) {
	env := setupGuestRLSApp(t)
	setListViewCreateUpdate(t, env.app, "settings", guestRLSNonGuest)
	settingsCol, _ := env.app.FindCollectionByNameOrId("settings")
	s := core.NewRecord(settingsCol)
	s.Set("app", "core")
	s.Set("key", "theme")
	if err := env.app.Save(s); err != nil {
		t.Fatal(err)
	}
	runListScenario(t, env.app, "settings", env.memberToken, []string{`"totalItems":1`})
}

// ============================ audit_logs ============================

func TestGuestRLS_AuditLogs_GuestCannotRead(t *testing.T) {
	env := setupGuestRLSApp(t)
	setListView(t, env.app, "audit_logs", guestRLSNonGuest)

	auditCol, _ := env.app.FindCollectionByNameOrId("audit_logs")
	a := core.NewRecord(auditCol)
	a.Set("action", "created")
	a.Set("resource_type", "drive_items")
	a.Set("resource_id", "abc123")
	if err := env.app.Save(a); err != nil {
		t.Fatal(err)
	}

	runListScenario(t, env.app, "audit_logs", env.guestToken, []string{`"totalItems":0`})
}

func TestGuestRLS_AuditLogs_MemberCanRead(t *testing.T) {
	env := setupGuestRLSApp(t)
	setListView(t, env.app, "audit_logs", guestRLSNonGuest)
	auditCol, _ := env.app.FindCollectionByNameOrId("audit_logs")
	a := core.NewRecord(auditCol)
	a.Set("action", "created")
	a.Set("resource_type", "drive_items")
	a.Set("resource_id", "abc123")
	if err := env.app.Save(a); err != nil {
		t.Fatal(err)
	}
	runListScenario(t, env.app, "audit_logs", env.memberToken, []string{`"totalItems":1`})
}

// ============================ org_pkg_enabled ============================

func TestGuestRLS_OrgPkgEnabled_GuestDenied(t *testing.T) {
	env := setupGuestRLSApp(t)
	setAllCRUD(t, env.app, "org_pkg_enabled", guestRLSNonGuest)

	opeCol, _ := env.app.FindCollectionByNameOrId("org_pkg_enabled")
	o := core.NewRecord(opeCol)
	o.Set("pkg", "drive")
	o.Set("enabled", true)
	if err := env.app.Save(o); err != nil {
		t.Fatal(err)
	}

	runListScenario(t, env.app, "org_pkg_enabled", env.guestToken, []string{`"totalItems":0`})
}

func TestGuestRLS_OrgPkgEnabled_MemberAllowed(t *testing.T) {
	env := setupGuestRLSApp(t)
	setAllCRUD(t, env.app, "org_pkg_enabled", guestRLSNonGuest)
	opeCol, _ := env.app.FindCollectionByNameOrId("org_pkg_enabled")
	o := core.NewRecord(opeCol)
	o.Set("pkg", "drive")
	o.Set("enabled", true)
	if err := env.app.Save(o); err != nil {
		t.Fatal(err)
	}
	runListScenario(t, env.app, "org_pkg_enabled", env.memberToken, []string{`"totalItems":1`})
}

// ----- small helpers -----

func setAllCRUD(t *testing.T, app core.App, name, rule string) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		t.Fatal(err)
	}
	col.ListRule = &rule
	col.ViewRule = &rule
	col.CreateRule = &rule
	col.UpdateRule = &rule
	col.DeleteRule = &rule
	if err := app.Save(col); err != nil {
		t.Fatalf("set all-CRUD rule on %s: %v", name, err)
	}
}

func setListViewCreateUpdate(t *testing.T, app core.App, name, rule string) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		t.Fatal(err)
	}
	col.ListRule = &rule
	col.ViewRule = &rule
	col.CreateRule = &rule
	col.UpdateRule = &rule
	if err := app.Save(col); err != nil {
		t.Fatalf("set list/view/create/update rule on %s: %v", name, err)
	}
}
