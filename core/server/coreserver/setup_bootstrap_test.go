package coreserver

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

// The first operator must end up as a regular `users` record with role=owner —
// that is the identity the /admin console runs as, and the one whose token
// authorizes managed-field writes (e.g. setting `verified` on a new user). A
// raw _superusers token on a throwaway client was the original org-create 400.
// This locks the bootstrap's user creation at the server layer; the full
// first-boot flow is covered by the setup-and-packages install spec.
func TestCreateOwnerOperator(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Cleanup() })

	operator, err := createOwnerOperator(app, "operator@example.com", "BootstrapPass1234!")
	if err != nil {
		t.Fatalf("createOwnerOperator returned error: %v", err)
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

	// The minted auth token must carry the users identity, not _superusers — that
	// is what lets the console's shared pb client satisfy the users manageRule.
	if _, err := operator.NewAuthToken(); err != nil {
		t.Fatalf("operator auth token mint failed: %v", err)
	}
	if operator.Collection().Name != "users" {
		t.Errorf("operator should be a users record, got %q", operator.Collection().Name)
	}
}

// The person who runs the setup wizard is the deployment's owner, and role is
// now the ONLY thing that grants authority — there is no separate grant table
// to fall back on. owner (not admin) because package management is owner-only:
// an `admin` operator would finish the wizard unable to install anything.
//
// This is the standalone path only: RegisterSetupBootstrap is bound in the host
// composition (server.go), never in a tenant, so nothing about hosted orgs
// depends on this value.
func TestCreateOwnerOperator_IsOwner(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Cleanup() })

	operator, err := createOwnerOperator(app, "operator@example.com", "BootstrapPass1234!")
	if err != nil {
		t.Fatalf("createOwnerOperator returned error: %v", err)
	}

	if got := operator.GetString("role"); got != "owner" {
		t.Errorf("operator role = %q, want owner: a non-owner operator cannot "+
			"install packages, and a non-admin one cannot invite anyone, so the "+
			"deployment is stuck", got)
	}

	// isOrgAdmin is the exact predicate /api/invite-member gates on; isOwner is
	// what the package endpoints gate on. The wizard-runner needs both.
	if !isOrgAdmin(operator) {
		t.Error("operator must satisfy isOrgAdmin, or /api/invite-member returns 403 " +
			"to the only account that exists")
	}
	if !isOwner(operator) {
		t.Error("operator must satisfy isOwner, or /api/admin/packages/* returns 403 " +
			"to the only account that exists")
	}
}
