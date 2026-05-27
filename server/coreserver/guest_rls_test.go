package coreserver

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// guest_rls_test.go proves the collection access rules tightened by the
// 187* migrations against PocketBase's REAL rule engine.
//
// Background: a "guest" share-link visitor gets a real users record plus a
// user_org row with role='guest' in the owner's org. ~11 collection rules
// granted access to ANY org member regardless of role, so a guest membership
// row would leak the member roster, emails, audit log, org settings and let
// the guest create files/calendars/mailboxes. These tests assert that a
// role='guest' member is DENIED while a real (member/owner/admin) member is
// still ALLOWED.
//
// Each scenario builds a FRESH TestApp: ApiScenario.Test re-triggers OnServe,
// which re-registers PocketBase's built-in routes and panics on the duplicate
// `GET /_/extensions.js` pattern under PB v0.38.1 if a single app is reused
// across scenarios (see invite_link_test.go for the same note).
//
// The TestApp runs only PocketBase's bundled system migrations, not this
// repo's JS migrations, so the schema + the candidate rule strings are built
// programmatically here. The rule strings MUST stay byte-for-byte identical to
// what the 187* migrations set — they are the source of truth this test
// validates.

// ----- candidate rule predicates (mirror the 187* migrations verbatim) -----

// guestRLSUserOrgRule is user_org's tightened list/view rule.
// A non-guest member of the org may list the roster; a guest may see ONLY
// their own membership row. The role pin lives on the same back-relation path
// prefix as the user pin so PB applies both to the SAME joined user_org row.
const guestRLSUserOrgRule = `@request.auth.id != "" && (` +
	`(org.user_org_via_org.user ?= @request.auth.id && org.user_org_via_org.role ?!= "guest")` +
	` || user = @request.auth.id)`

// guestRLSUsersRule is users' tightened list/view rule. A user U is visible
// to a caller who has a NON-GUEST membership in an org U also belongs to.
const guestRLSUsersRule = `@request.auth.id != "" && ` +
	`user_org_via_user.org.user_org_via_org.user ?= @request.auth.id && ` +
	`user_org_via_user.org.user_org_via_org.role ?!= "guest"`

// guestRLSOrgsMemberRule is the non-guest-member predicate for orgs.
const guestRLSOrgsMemberRule = `user_org_via_org.user ?= @request.auth.id && ` +
	`user_org_via_org.role ?!= "guest"`

// guestRLSOrgsReadRule is orgs' tightened list/view rule: a non-guest member
// sees the org; a guest may VIEW (only) the org(s) they hold a membership in.
const guestRLSOrgsReadRule = `@request.auth.id != "" && (` +
	`(` + guestRLSOrgsMemberRule + `)` +
	` || user_org_via_org.user ?= @request.auth.id)`

// guestRLSOrgsWriteRule is orgs' tightened update rule (guests never update).
const guestRLSOrgsWriteRule = `@request.auth.id != "" && (` + guestRLSOrgsMemberRule + `)`

// guestRLSOrgScopedRule is the non-guest-member predicate for org-scoped
// collections (labels, settings, org_pkg_enabled) whose org relation is a
// direct field `org`.
const guestRLSOrgScopedRule = `org.user_org_via_org.user ?= @request.auth.id && ` +
	`org.user_org_via_org.role ?!= "guest"`

// guestRLSAuditRule is audit_logs' tightened list/view rule.
const guestRLSAuditRule = `@request.auth.id != "" && (` + guestRLSOrgScopedRule + `)`

// ---------------------------------------------------------------------------

type guestRLSEnv struct {
	app    *tests.TestApp
	org    *core.Record
	member *core.Record
	guest  *core.Record
	// tokens
	memberToken string
	guestToken  string
}

// setupGuestRLSApp builds the orgs / user_org / users(is_demo) / labels /
// settings / audit_logs / org_pkg_enabled schema, seeds one org with a real
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
	relaxUsernameMinLength(users)
	if err := app.Save(users); err != nil {
		t.Fatal(err)
	}

	orgs := core.NewBaseCollection("orgs")
	orgs.Id = "pbc_orgs_00001"
	orgs.Fields.Add(&core.TextField{Name: "name", Required: true})
	orgs.Fields.Add(&core.TextField{Name: "slug", Required: true})
	if err := app.Save(orgs); err != nil {
		t.Fatal(err)
	}

	userOrg := core.NewBaseCollection("user_org")
	userOrg.Id = "pbc_user_org_01"
	userOrg.Fields.Add(&core.RelationField{
		Name: "org", Required: true, CollectionId: orgs.Id,
		CascadeDelete: true, MaxSelect: 1,
	})
	userOrg.Fields.Add(&core.RelationField{
		Name: "user", Required: true, CollectionId: users.Id,
		CascadeDelete: true, MaxSelect: 1,
	})
	userOrg.Fields.Add(&core.SelectField{
		Name: "role", Required: true, MaxSelect: 1,
		Values: []string{"owner", "admin", "member", "guest"},
	})
	if err := app.Save(userOrg); err != nil {
		t.Fatal(err)
	}

	labels := core.NewBaseCollection("labels")
	labels.Fields.Add(&core.RelationField{
		Name: "org", Required: true, CollectionId: orgs.Id,
		CascadeDelete: true, MaxSelect: 1,
	})
	labels.Fields.Add(&core.TextField{Name: "name", Required: true})
	labels.Fields.Add(&core.TextField{Name: "color", Required: true})
	if err := app.Save(labels); err != nil {
		t.Fatal(err)
	}

	settings := core.NewBaseCollection("settings")
	settings.Fields.Add(&core.TextField{Name: "app", Required: true})
	settings.Fields.Add(&core.TextField{Name: "key", Required: true})
	settings.Fields.Add(&core.JSONField{Name: "value"})
	settings.Fields.Add(&core.RelationField{
		Name: "org", Required: true, CollectionId: orgs.Id,
		CascadeDelete: true, MaxSelect: 1,
	})
	if err := app.Save(settings); err != nil {
		t.Fatal(err)
	}

	auditLogs := core.NewBaseCollection("audit_logs")
	auditLogs.Fields.Add(&core.RelationField{
		Name: "org", Required: true, CollectionId: orgs.Id,
		CascadeDelete: true, MaxSelect: 1,
	})
	auditLogs.Fields.Add(&core.TextField{Name: "action", Required: true})
	auditLogs.Fields.Add(&core.TextField{Name: "resource_type", Required: true})
	auditLogs.Fields.Add(&core.TextField{Name: "resource_id", Required: true})
	if err := app.Save(auditLogs); err != nil {
		t.Fatal(err)
	}

	orgPkgEnabled := core.NewBaseCollection("org_pkg_enabled")
	orgPkgEnabled.Fields.Add(&core.RelationField{
		Name: "org", Required: true, CollectionId: orgs.Id,
		CascadeDelete: true, MaxSelect: 1,
	})
	orgPkgEnabled.Fields.Add(&core.TextField{Name: "pkg", Required: true})
	orgPkgEnabled.Fields.Add(&core.BoolField{Name: "enabled"})
	if err := app.Save(orgPkgEnabled); err != nil {
		t.Fatal(err)
	}

	// Seed org + a real member + a guest.
	org := core.NewRecord(orgs)
	org.Set("name", "Acme")
	org.Set("slug", "acme")
	if err := app.Save(org); err != nil {
		t.Fatal(err)
	}

	member := guestRLSUser(t, app, "member@test.local")
	guest := guestRLSUser(t, app, "guest@test.local")

	guestRLSMembership(t, app, member, org, "member")
	guestRLSMembership(t, app, guest, org, "guest")

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
		org:         org,
		member:      member,
		guest:       guest,
		memberToken: memberToken,
		guestToken:  guestToken,
	}
}

func guestRLSUser(t *testing.T, app core.App, email string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.SetEmail(email)
	r.Set("username", DeriveUsername(email))
	r.Set("name", "Test")
	r.SetVerified(true)
	r.SetPassword("Password123!")
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func guestRLSMembership(t *testing.T, app core.App, user, org *core.Record, role string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("user_org")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.Set("user", user.Id)
	r.Set("org", org.Id)
	r.Set("role", role)
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return r
}

// setRule applies a list+view rule to a collection (most rules) and re-saves.
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

// ============================ user_org ============================

func TestGuestRLS_UserOrg_GuestSeesOnlyOwnRow(t *testing.T) {
	env := setupGuestRLSApp(t)
	setListView(t, env.app, "user_org", guestRLSUserOrgRule)

	// Guest: must see exactly ONE row (their own), never the member's row.
	t.Run("guest sees only own row", func(t *testing.T) {
		runListScenario(t, env.app, "user_org", env.guestToken, []string{
			`"totalItems":1`,
			`"role":"guest"`,
		})
	})
}

func TestGuestRLS_UserOrg_GuestCannotSeeMemberRow(t *testing.T) {
	env := setupGuestRLSApp(t)
	setListView(t, env.app, "user_org", guestRLSUserOrgRule)

	// The guest's list must NOT contain a member-role row.
	scenario := &tests.ApiScenario{
		Method:                http.MethodGet,
		URL:                   "/api/collections/user_org/records",
		Headers:               map[string]string{"Authorization": env.guestToken},
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{`"totalItems":1`},
		NotExpectedContent:    []string{`"role":"member"`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return env.app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

func TestGuestRLS_UserOrg_MemberSeesRoster(t *testing.T) {
	env := setupGuestRLSApp(t)
	setListView(t, env.app, "user_org", guestRLSUserOrgRule)

	// Member: must see BOTH rows (member + guest) — the full roster.
	runListScenario(t, env.app, "user_org", env.memberToken, []string{
		`"totalItems":2`,
		`"role":"guest"`,
		`"role":"member"`,
	})
}

// ============================ users ============================

func TestGuestRLS_Users_GuestSeesNoOtherMembers(t *testing.T) {
	env := setupGuestRLSApp(t)
	setListView(t, env.app, "users", guestRLSUsersRule)

	// Guest must not enumerate members. The guest is not a non-guest member of
	// any org, so the rule matches nothing for them -> totalItems:0.
	runListScenario(t, env.app, "users", env.guestToken, []string{`"totalItems":0`})
}

func TestGuestRLS_Users_GuestCannotSeeMemberEmail(t *testing.T) {
	env := setupGuestRLSApp(t)
	setListView(t, env.app, "users", guestRLSUsersRule)

	scenario := &tests.ApiScenario{
		Method:                http.MethodGet,
		URL:                   "/api/collections/users/records",
		Headers:               map[string]string{"Authorization": env.guestToken},
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{`"totalItems":0`},
		NotExpectedContent:    []string{"member@test.local"},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return env.app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

func TestGuestRLS_Users_MemberSeesOtherMembers(t *testing.T) {
	env := setupGuestRLSApp(t)
	setListView(t, env.app, "users", guestRLSUsersRule)

	// Member shares the org and is non-guest, so sees both user records
	// (their own + the guest's). The key assertion is they still see >=2.
	runListScenario(t, env.app, "users", env.memberToken, []string{`"totalItems":2`})
}

// ===================== same-row semantics + cross-org isolation =====================
//
// These guard the SUBTLE part of the predicate: the role pin must apply to the
// CALLER's OWN membership row, not to "some non-guest row in the org." With a
// second real member added, the org contains multiple non-guest rows, so an
// "any-row" misreading of `... role ?!= "guest"` would leak the roster to the
// guest. We assert the guest still sees only their own row — proving PB applies
// both legs of the same relation-path prefix to the same joined row.

// addSecondMember adds another role='member' user to env.org. The org then has
// two non-guest rows + one guest row — the trap configuration.
func addSecondMember(t *testing.T, env *guestRLSEnv) {
	t.Helper()
	m2 := guestRLSUser(t, env.app, "member2@test.local")
	guestRLSMembership(t, env.app, m2, env.org, "member")
}

func TestGuestRLS_UserOrg_GuestStillIsolatedWithMultipleMembers(t *testing.T) {
	env := setupGuestRLSApp(t)
	addSecondMember(t, env)
	setListView(t, env.app, "user_org", guestRLSUserOrgRule)

	scenario := &tests.ApiScenario{
		Method:                http.MethodGet,
		URL:                   "/api/collections/user_org/records",
		Headers:               map[string]string{"Authorization": env.guestToken},
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{`"totalItems":1`, `"role":"guest"`},
		NotExpectedContent:    []string{`"role":"member"`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return env.app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

func TestGuestRLS_Users_GuestStillBlockedWithMultipleMembers(t *testing.T) {
	env := setupGuestRLSApp(t)
	addSecondMember(t, env)
	setListView(t, env.app, "users", guestRLSUsersRule)

	// Even with two non-guest members in the shared org, the guest sees no one.
	runListScenario(t, env.app, "users", env.guestToken, []string{`"totalItems":0`})
}

func TestGuestRLS_UserOrg_CrossOrgIsolation(t *testing.T) {
	env := setupGuestRLSApp(t)
	setListView(t, env.app, "user_org", guestRLSUserOrgRule)

	// A separate org with its own member; env.member belongs only to org A and
	// must never see org B's membership rows.
	orgsCol, err := env.app.FindCollectionByNameOrId("orgs")
	if err != nil {
		t.Fatal(err)
	}
	orgB := core.NewRecord(orgsCol)
	orgB.Set("name", "OrgB")
	orgB.Set("slug", "orgb")
	if err := env.app.Save(orgB); err != nil {
		t.Fatal(err)
	}
	stranger := guestRLSUser(t, env.app, "stranger@test.local")
	guestRLSMembership(t, env.app, stranger, orgB, "member")

	scenario := &tests.ApiScenario{
		Method:                http.MethodGet,
		URL:                   "/api/collections/user_org/records",
		Headers:               map[string]string{"Authorization": env.memberToken},
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{`"totalItems":2`}, // only org A's two rows
		NotExpectedContent:    []string{stranger.Id},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return env.app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

// ============================ orgs ============================

func TestGuestRLS_Orgs_GuestCanViewOwnOrgOnly(t *testing.T) {
	env := setupGuestRLSApp(t)
	setListView(t, env.app, "orgs", guestRLSOrgsReadRule)

	// A guest legitimately needs to read the org row they're a guest in (for
	// the editor to show the org name). They see exactly that one org.
	runListScenario(t, env.app, "orgs", env.guestToken, []string{
		`"totalItems":1`,
		`"Acme"`,
	})
}

func TestGuestRLS_Orgs_GuestCannotSeeOtherOrg(t *testing.T) {
	env := setupGuestRLSApp(t)
	setListView(t, env.app, "orgs", guestRLSOrgsReadRule)

	// Seed a SECOND org the guest has no membership in. The guest must not see it.
	orgsCol, err := env.app.FindCollectionByNameOrId("orgs")
	if err != nil {
		t.Fatal(err)
	}
	other := core.NewRecord(orgsCol)
	other.Set("name", "Secret Org")
	other.Set("slug", "secret")
	if err := env.app.Save(other); err != nil {
		t.Fatal(err)
	}

	scenario := &tests.ApiScenario{
		Method:                http.MethodGet,
		URL:                   "/api/collections/orgs/records",
		Headers:               map[string]string{"Authorization": env.guestToken},
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{`"totalItems":1`},
		NotExpectedContent:    []string{"Secret Org"},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return env.app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

func TestGuestRLS_Orgs_MemberCanView(t *testing.T) {
	env := setupGuestRLSApp(t)
	setListView(t, env.app, "orgs", guestRLSOrgsReadRule)

	runListScenario(t, env.app, "orgs", env.memberToken, []string{
		`"totalItems":1`,
		`"Acme"`,
	})
}

func TestGuestRLS_Orgs_GuestCannotUpdate(t *testing.T) {
	env := setupGuestRLSApp(t)
	// Need a read rule too so a 404-vs-403 distinction is meaningful; set both.
	setListView(t, env.app, "orgs", guestRLSOrgsReadRule)
	orgsCol, err := env.app.FindCollectionByNameOrId("orgs")
	if err != nil {
		t.Fatal(err)
	}
	orgsCol.UpdateRule = strPtrGuest(guestRLSOrgsWriteRule)
	if err := env.app.Save(orgsCol); err != nil {
		t.Fatal(err)
	}

	// Guest PATCH must be denied (the update rule excludes guests). PB returns
	// 404 when the record fails the update rule's record-level filter.
	scenario := &tests.ApiScenario{
		Method:                http.MethodPatch,
		URL:                   "/api/collections/orgs/records/" + env.org.Id,
		Body:                  strings.NewReader(`{"name":"Hijacked"}`),
		Headers:               map[string]string{"Authorization": env.guestToken, "Content-Type": "application/json"},
		ExpectedStatus:        http.StatusNotFound,
		ExpectedContent:       []string{`"message"`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return env.app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

func TestGuestRLS_Orgs_MemberCanUpdate(t *testing.T) {
	env := setupGuestRLSApp(t)
	setListView(t, env.app, "orgs", guestRLSOrgsReadRule)
	orgsCol, err := env.app.FindCollectionByNameOrId("orgs")
	if err != nil {
		t.Fatal(err)
	}
	orgsCol.UpdateRule = strPtrGuest(guestRLSOrgsWriteRule)
	if err := env.app.Save(orgsCol); err != nil {
		t.Fatal(err)
	}

	scenario := &tests.ApiScenario{
		Method:                http.MethodPatch,
		URL:                   "/api/collections/orgs/records/" + env.org.Id,
		Body:                  strings.NewReader(`{"name":"Renamed"}`),
		Headers:               map[string]string{"Authorization": env.memberToken, "Content-Type": "application/json"},
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{`"name":"Renamed"`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return env.app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

// ============================ labels (org-scoped, all CRUD) ============================

func TestGuestRLS_Labels_GuestDeniedMemberAllowed(t *testing.T) {
	env := setupGuestRLSApp(t)
	setAllCRUD(t, env.app, "labels", guestRLSOrgScopedRule)

	// Seed a label so list has something for a member to see.
	labelsCol, _ := env.app.FindCollectionByNameOrId("labels")
	lbl := core.NewRecord(labelsCol)
	lbl.Set("org", env.org.Id)
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
	setAllCRUD(t, env.app, "labels", guestRLSOrgScopedRule)
	labelsCol, _ := env.app.FindCollectionByNameOrId("labels")
	lbl := core.NewRecord(labelsCol)
	lbl.Set("org", env.org.Id)
	lbl.Set("name", "Important")
	lbl.Set("color", "#f00")
	if err := env.app.Save(lbl); err != nil {
		t.Fatal(err)
	}
	runListScenario(t, env.app, "labels", env.memberToken, []string{`"totalItems":1`, "Important"})
}

func TestGuestRLS_Labels_GuestCannotCreate(t *testing.T) {
	env := setupGuestRLSApp(t)
	setAllCRUD(t, env.app, "labels", guestRLSOrgScopedRule)

	scenario := &tests.ApiScenario{
		Method:                http.MethodPost,
		URL:                   "/api/collections/labels/records",
		Body:                  strings.NewReader(`{"org":"` + env.org.Id + `","name":"X","color":"#000"}`),
		Headers:               map[string]string{"Authorization": env.guestToken, "Content-Type": "application/json"},
		ExpectedStatus:        http.StatusBadRequest,
		ExpectedContent:       []string{`"message"`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return env.app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

// ============================ settings (org-scoped) ============================

func TestGuestRLS_Settings_GuestDenied(t *testing.T) {
	env := setupGuestRLSApp(t)
	setListViewCreateUpdate(t, env.app, "settings", guestRLSOrgScopedRule)

	settingsCol, _ := env.app.FindCollectionByNameOrId("settings")
	s := core.NewRecord(settingsCol)
	s.Set("app", "core")
	s.Set("key", "theme")
	s.Set("org", env.org.Id)
	if err := env.app.Save(s); err != nil {
		t.Fatal(err)
	}

	runListScenario(t, env.app, "settings", env.guestToken, []string{`"totalItems":0`})
}

func TestGuestRLS_Settings_MemberAllowed(t *testing.T) {
	env := setupGuestRLSApp(t)
	setListViewCreateUpdate(t, env.app, "settings", guestRLSOrgScopedRule)
	settingsCol, _ := env.app.FindCollectionByNameOrId("settings")
	s := core.NewRecord(settingsCol)
	s.Set("app", "core")
	s.Set("key", "theme")
	s.Set("org", env.org.Id)
	if err := env.app.Save(s); err != nil {
		t.Fatal(err)
	}
	runListScenario(t, env.app, "settings", env.memberToken, []string{`"totalItems":1`})
}

// ============================ audit_logs ============================

func TestGuestRLS_AuditLogs_GuestCannotRead(t *testing.T) {
	env := setupGuestRLSApp(t)
	setListView(t, env.app, "audit_logs", guestRLSAuditRule)

	auditCol, _ := env.app.FindCollectionByNameOrId("audit_logs")
	a := core.NewRecord(auditCol)
	a.Set("org", env.org.Id)
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
	setListView(t, env.app, "audit_logs", guestRLSAuditRule)
	auditCol, _ := env.app.FindCollectionByNameOrId("audit_logs")
	a := core.NewRecord(auditCol)
	a.Set("org", env.org.Id)
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
	setAllCRUD(t, env.app, "org_pkg_enabled", guestRLSOrgScopedRule)

	opeCol, _ := env.app.FindCollectionByNameOrId("org_pkg_enabled")
	o := core.NewRecord(opeCol)
	o.Set("org", env.org.Id)
	o.Set("pkg", "drive")
	o.Set("enabled", true)
	if err := env.app.Save(o); err != nil {
		t.Fatal(err)
	}

	runListScenario(t, env.app, "org_pkg_enabled", env.guestToken, []string{`"totalItems":0`})
}

func TestGuestRLS_OrgPkgEnabled_MemberAllowed(t *testing.T) {
	env := setupGuestRLSApp(t)
	setAllCRUD(t, env.app, "org_pkg_enabled", guestRLSOrgScopedRule)
	opeCol, _ := env.app.FindCollectionByNameOrId("org_pkg_enabled")
	o := core.NewRecord(opeCol)
	o.Set("org", env.org.Id)
	o.Set("pkg", "drive")
	o.Set("enabled", true)
	if err := env.app.Save(o); err != nil {
		t.Fatal(err)
	}
	runListScenario(t, env.app, "org_pkg_enabled", env.memberToken, []string{`"totalItems":1`})
}

// ----- small helpers -----

func strPtrGuest(s string) *string { return &s }

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
