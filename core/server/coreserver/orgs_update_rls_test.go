package coreserver

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// orgs_update_rls_test.go proves the orgs.updateRule tightened by migration
// 1920000001 against PocketBase's REAL rule engine.
//
// Background: 1870000000 let ANY non-guest member PATCH the org (name/slug/logo).
// The slug drives every /a/<slug>/… URL, so a plain member could break every
// deep link by re-slugging. 1920000001 restricts the write to owner/admin — plus
// a super_admins carve-out, because the in-shell admin console edits orgs via
// the regular app client while running as a super-admin APP USER that holds NO
// user_org membership in the edited org.
//
// These tests assert the three-way outcome of the tightened rule:
//   - a plain member is DENIED,
//   - an owner/admin member is ALLOWED,
//   - a super-admin app user (no membership) is ALLOWED.

// orgsAdminWriteRule mirrors 1920000001 byte-for-byte — it is the source of
// truth this test validates. Keep it identical to the migration string.
const orgsAdminWriteRule = `(@request.auth.id != "" && ` +
	`user_org_via_org.user ?= @request.auth.id && ` +
	`user_org_via_org.role ?!= "guest" && ` +
	`user_org_via_org.role ?!= "member") || ` +
	`@collection.super_admins.user ?= @request.auth.id`

// setOrgsUpdateRule applies the candidate update rule to the orgs collection.
func setOrgsUpdateRule(t *testing.T, app core.App) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("orgs")
	if err != nil {
		t.Fatal(err)
	}
	rule := orgsAdminWriteRule
	col.UpdateRule = &rule
	// A view rule is needed too so a denied write is a clean 404 (record-level
	// filter miss) rather than an unrelated read failure.
	col.ViewRule = &rule
	if err := app.Save(col); err != nil {
		t.Fatalf("set orgs update rule: %v", err)
	}
}

// ensureSuperAdminsCollection creates the super_admins junction (user relation)
// if the shared setup didn't. Mirrors 1910000005's shape (only the `user`
// relation matters for the rule).
func ensureSuperAdminsCollection(t *testing.T, app core.App, usersID string) *core.Collection {
	t.Helper()
	if col, err := app.FindCollectionByNameOrId("super_admins"); err == nil {
		return col
	}
	sa := core.NewBaseCollection("super_admins")
	sa.Fields.Add(&core.RelationField{
		Name: "user", Required: true, CollectionId: usersID,
		CascadeDelete: true, MaxSelect: 1,
	})
	if err := app.Save(sa); err != nil {
		t.Fatalf("create super_admins: %v", err)
	}
	return sa
}

// addOrgMember seeds a user with the given role in env.org and returns an auth
// token for them.
func addOrgMember(t *testing.T, env *guestRLSEnv, email, role string) string {
	t.Helper()
	u := guestRLSUser(t, env.app, email)
	guestRLSMembership(t, env.app, u, env.org, role)
	token, err := u.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func patchOrgName(t *testing.T, app *tests.TestApp, orgID, token, newName string, wantStatus int, wantContent []string) {
	t.Helper()
	scenario := &tests.ApiScenario{
		Method:                http.MethodPatch,
		URL:                   "/api/collections/orgs/records/" + orgID,
		Body:                  strings.NewReader(`{"name":"` + newName + `"}`),
		Headers:               map[string]string{"Authorization": token, "Content-Type": "application/json"},
		ExpectedStatus:        wantStatus,
		ExpectedContent:       wantContent,
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

func TestOrgsUpdateRLS_PlainMemberDenied(t *testing.T) {
	env := setupGuestRLSApp(t)
	ensureSuperAdminsCollection(t, env.app, env.member.Collection().Id)
	setOrgsUpdateRule(t, env.app)

	// env.member holds role='member' in the org. Under the tightened rule a plain
	// member must NOT be able to rename the org. PB returns 404 when the record
	// fails the update rule's record-level filter.
	patchOrgName(t, env.app, env.org.Id, env.memberToken, "MemberHijack", http.StatusNotFound, []string{`"message"`})
}

func TestOrgsUpdateRLS_OwnerAllowed(t *testing.T) {
	env := setupGuestRLSApp(t)
	ensureSuperAdminsCollection(t, env.app, env.member.Collection().Id)
	setOrgsUpdateRule(t, env.app)

	ownerToken := addOrgMember(t, env, "owner@test.local", "owner")
	patchOrgName(t, env.app, env.org.Id, ownerToken, "OwnerRenamed", http.StatusOK, []string{`"name":"OwnerRenamed"`})
}

func TestOrgsUpdateRLS_AdminAllowed(t *testing.T) {
	env := setupGuestRLSApp(t)
	ensureSuperAdminsCollection(t, env.app, env.member.Collection().Id)
	setOrgsUpdateRule(t, env.app)

	adminToken := addOrgMember(t, env, "admin@test.local", "admin")
	patchOrgName(t, env.app, env.org.Id, adminToken, "AdminRenamed", http.StatusOK, []string{`"name":"AdminRenamed"`})
}

func TestOrgsUpdateRLS_SuperAdminAllowedWithoutMembership(t *testing.T) {
	env := setupGuestRLSApp(t)
	sa := ensureSuperAdminsCollection(t, env.app, env.member.Collection().Id)
	setOrgsUpdateRule(t, env.app)

	// A super-admin APP USER with NO user_org membership in env.org — exactly the
	// admin-console edit path. The super_admins carve-out must let them rename.
	saUser := guestRLSUser(t, env.app, "superadmin@test.local")
	saRow := core.NewRecord(sa)
	saRow.Set("user", saUser.Id)
	if err := env.app.Save(saRow); err != nil {
		t.Fatal(err)
	}
	saToken, err := saUser.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}

	patchOrgName(t, env.app, env.org.Id, saToken, "AdminConsoleRenamed", http.StatusOK, []string{`"name":"AdminConsoleRenamed"`})
}

func TestOrgsUpdateRLS_NonSuperAdminNonMemberDenied(t *testing.T) {
	env := setupGuestRLSApp(t)
	ensureSuperAdminsCollection(t, env.app, env.member.Collection().Id)
	setOrgsUpdateRule(t, env.app)

	// A stranger with neither a membership nor a super_admins row must be denied.
	stranger := guestRLSUser(t, env.app, "stranger@test.local")
	strangerToken, err := stranger.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	patchOrgName(t, env.app, env.org.Id, strangerToken, "StrangerHijack", http.StatusNotFound, []string{`"message"`})
}
