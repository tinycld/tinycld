package coreserver

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

// The first operator must end up as a regular `users` record promoted via a
// super_admins row — that is the identity the /admin console runs as, and the
// one whose token authorizes managed-field writes (e.g. setting `verified` on a
// new org owner). A raw _superusers token on a throwaway client was the original
// org-create 400. This locks the bootstrap's user/grant creation at the server
// layer; the full first-boot → create-org flow is covered by the
// setup-and-packages install spec.
func TestCreateSuperAdminOperator(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Cleanup() })

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	createSuperAdminsCollection(t, app, users.Id)

	operator, err := createSuperAdminOperator(app, "operator@example.com", "BootstrapPass1234!")
	if err != nil {
		t.Fatalf("createSuperAdminOperator returned error: %v", err)
	}

	if !operator.Verified() {
		t.Error("operator should be pre-verified so they can sign in immediately")
	}
	if operator.GetString("username") == "" {
		t.Error("operator must have a username (the field is required)")
	}
	if operator.GetString("name") == "" {
		t.Error("operator must have a name (the field is required)")
	}
	if !isSuperAdmin(app, operator.Id) {
		t.Error("operator must be a super admin (super_admins row should exist)")
	}

	// The minted auth token must carry the users identity, not _superusers — that
	// is what lets the console's shared pb client satisfy the users manageRule.
	if _, err := operator.NewAuthToken(); err != nil {
		t.Fatalf("operator auth token mint failed: %v", err)
	}
	if operator.Collection().Name != "users" {
		t.Errorf("operator should be a users record, got %q", operator.Collection().Name)
	}
}

// The person who runs the setup wizard is the deployment's owner, and the UI
// must let them act like one.
//
// The operator was previously minted as `member` on the reasoning that their
// real authority is the super_admins row. That holds for collection rules, but
// not for the app: Settings > Members gates on useCurrentRole().isAdmin, and
// /api/invite-member rejects a non-admin caller outright. The /admin console
// has no role management either — so a fresh self-hoster finished the wizard,
// was told "Only admins and owners can manage organization members", and had
// no path to inviting anyone or promoting themselves. The deployment was
// permanently single-user.
//
// This is the standalone path only: RegisterSetupBootstrap is bound in the host
// composition (server.go), never in a tenant, so nothing about hosted orgs
// depends on this value.
func TestCreateSuperAdminOperator_IsOwner(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Cleanup() })

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	createSuperAdminsCollection(t, app, users.Id)

	operator, err := createSuperAdminOperator(app, "operator@example.com", "BootstrapPass1234!")
	if err != nil {
		t.Fatalf("createSuperAdminOperator returned error: %v", err)
	}

	if got := operator.GetString("role"); got != "owner" {
		t.Errorf("operator role = %q, want owner: a non-admin operator cannot "+
			"invite anyone and cannot promote themselves, so the deployment is "+
			"stuck with exactly one user", got)
	}

	// isOrgAdmin is the exact predicate /api/invite-member gates on.
	if !isOrgAdmin(operator) {
		t.Error("operator must satisfy isOrgAdmin, or /api/invite-member returns 403 " +
			"to the only account that exists")
	}
}
