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
