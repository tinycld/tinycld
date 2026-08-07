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
// The rules under test are NOT restated here. They are applied by running
// core's real pb_migrations (rlstest) — schema and rules alike — so a later
// migration that restates a rule and drops the guest clause turns these tests
// red instead of leaving them validating a stale copy. An earlier version of
// this file kept the rules as constants "mirroring the migration verbatim",
// which is the exact fixture trap that let drive's guest clause regress
// silently.
//
// Each scenario builds a FRESH TestApp: ApiScenario.Test re-triggers OnServe,
// which re-registers PocketBase's built-in routes and panics on the duplicate
// route pattern if a single app is reused across scenarios.

type guestRLSEnv struct {
	app    *tests.TestApp
	member *core.Record
	guest  *core.Record
	// tokens
	memberToken string
	guestToken  string
}

// setupGuestRLSApp applies core's shipped migrations (which create the users
// fields plus labels / settings / audit_logs with their rules), then seeds a
// real member (role 'member') and a guest (role 'guest')
// and returns auth tokens for each.
func setupGuestRLSApp(t *testing.T) *guestRLSEnv {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	// The bundled PB test fixture's users collection already carries a
	// username unique index; 1820000000 adds the same definition and the
	// duplicate is rejected. Drop the fixture's copy so the migration chain
	// applies the way it does on a real DB, where the index does not
	// pre-exist.
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
	// The fixture also lists username in passwordAuth.identityFields, which
	// demands that unique index; reset to email-only the way a pre-1820000000
	// DB looks. The migration re-enables username identity itself.
	users.PasswordAuth.IdentityFields = []string{"email"}
	if err := app.Save(users); err != nil {
		t.Fatalf("drop fixture username index: %v", err)
	}

	rlstest.Apply(t, app, rlstest.MigrationsDir(t, "../pb_migrations"))

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

// The guest clause the deny-tests below depend on must be present in every
// SHIPPED rule they exercise — this names the collection and predicate when a
// future migration restates one without it.
func TestGuestRLS_ShippedRulesCarryGuestClause(t *testing.T) {
	env := setupGuestRLSApp(t)
	for _, c := range []struct{ collection, kind string }{
		{"users", "list"},
		{"users", "view"},
		{"labels", "list"},
		{"labels", "create"},
		{"settings", "list"},
	} {
		rlstest.RequireRuleContains(t, env.app, c.collection, c.kind,
			`@request.auth.role != "guest"`)
	}
	// audit_logs is stricter than non-guest: the trail carries member emails
	// and role changes, and the UI only shows it to admins — the rule must
	// match, so a member's REST client can't read what the screen hides.
	for _, kind := range []string{"list", "view"} {
		rlstest.RequireRuleContains(t, env.app, "audit_logs", kind,
			`@request.auth.role = "owner" || @request.auth.role = "admin"`)
	}
}

// ============================ users ============================

func TestGuestRLS_Users_GuestSeesOnlySelf(t *testing.T) {
	env := setupGuestRLSApp(t)

	// Guest must not enumerate members. With the `|| id = @request.auth.id`
	// carve-out the guest sees their own row (needed for the PB SDK's
	// authRefresh after pb.authStore.save) but no one else's.
	runListScenario(t, env.app, "users", env.guestToken, []string{`"totalItems":1`})
}

func TestGuestRLS_Users_GuestCannotSeeMemberEmail(t *testing.T) {
	env := setupGuestRLSApp(t)

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

// The audit trail records member emails and role changes; the UI shows it
// only to admins (settings/audit-log.tsx gates on isAdmin). A plain member's
// REST client must not read what the screen hides.
func TestGuestRLS_AuditLogs_MemberCannotRead(t *testing.T) {
	env := setupGuestRLSApp(t)
	auditCol, _ := env.app.FindCollectionByNameOrId("audit_logs")
	a := core.NewRecord(auditCol)
	a.Set("action", "created")
	a.Set("resource_type", "drive_items")
	a.Set("resource_id", "abc123")
	if err := env.app.Save(a); err != nil {
		t.Fatal(err)
	}
	runListScenario(t, env.app, "audit_logs", env.memberToken, []string{`"totalItems":0`})
}

func TestGuestRLS_AuditLogs_AdminCanRead(t *testing.T) {
	env := setupGuestRLSApp(t)
	admin := guestRLSUser(t, env.app, "auditadmin@test.local", "admin")
	adminToken, err := admin.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}

	auditCol, _ := env.app.FindCollectionByNameOrId("audit_logs")
	a := core.NewRecord(auditCol)
	a.Set("action", "created")
	a.Set("resource_type", "drive_items")
	a.Set("resource_id", "abc123")
	if err := env.app.Save(a); err != nil {
		t.Fatal(err)
	}
	runListScenario(t, env.app, "audit_logs", adminToken, []string{`"totalItems":1`})
}
