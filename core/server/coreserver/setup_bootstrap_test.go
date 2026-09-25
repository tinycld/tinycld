package coreserver

import (
	"net/http"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"golang.org/x/crypto/bcrypt"
)

// ensureUsersRoleAndNameFields patches a tests.NewTestApp app's `users`
// collection with the fields createOwnerOperator sets (role, name). A test
// app built from tests.NewTestApp has no JS migrations dir, so the
// PocketBase-system `users` collection it creates lacks these — see
// ensureUsersCollection above for the same gap on a from-scratch app.
func ensureUsersRoleAndNameFields(t *testing.T, app core.App) {
	t.Helper()
	usersCol, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	changed := false
	if usersCol.Fields.GetByName("role") == nil {
		usersCol.Fields.Add(&core.TextField{Name: "role"})
		changed = true
	}
	if usersCol.Fields.GetByName("name") == nil {
		usersCol.Fields.Add(&core.TextField{Name: "name"})
		changed = true
	}
	if changed {
		if err := app.Save(usersCol); err != nil {
			t.Fatal(err)
		}
	}
}

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

	operator, err := createOwnerOperator(app, "operator@example.com", "", "BootstrapPass1234!")
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

	operator, err := createOwnerOperator(app, "operator@example.com", "", "BootstrapPass1234!")
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

// An owner is minted from a bcrypt hash computed elsewhere so the plaintext
// password never crosses a process boundary. The record must authenticate
// with the original password and carry the supplied display name.
func TestCreateOwnerAccountWithHash(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Cleanup() })

	hash, err := bcrypt.GenerateFromPassword([]byte("correct horse battery"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	operator, err := CreateOwnerAccountWithHash(app, "owner@example.com", "Ada Lovelace", string(hash))
	if err != nil {
		t.Fatalf("CreateOwnerAccountWithHash: %v", err)
	}
	if !operator.ValidatePassword("correct horse battery") {
		t.Fatal("the original password must validate against the copied hash")
	}
	if operator.ValidatePassword("wrong") {
		t.Fatal("a wrong password must not validate")
	}
	if got := operator.GetString("name"); got != "Ada Lovelace" {
		t.Fatalf("name = %q, want Ada Lovelace", got)
	}
	if got := operator.GetString("role"); got != "owner" {
		t.Fatalf("role = %q, want owner", got)
	}
	if !operator.Verified() {
		t.Fatal("owner must be verified")
	}
}

// An empty name falls back to the email local-part, the same default the
// plaintext path uses, so both paths mint the same shape of account.
func TestCreateOwnerAccountWithHash_DefaultName(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Cleanup() })
	hash, _ := bcrypt.GenerateFromPassword([]byte("pw-1234567890"), bcrypt.MinCost)
	operator, err := CreateOwnerAccountWithHash(app, "grace@example.com", "", string(hash))
	if err != nil {
		t.Fatal(err)
	}
	if got := operator.GetString("name"); got != "grace" {
		t.Fatalf("name = %q, want grace", got)
	}
}

// A value that is not a bcrypt hash must be refused: SetRaw would store it as
// the hash verbatim and the account could never authenticate.
func TestCreateOwnerAccountWithHash_RejectsPlaintext(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Cleanup() })
	if _, err := CreateOwnerAccountWithHash(app, "x@example.com", "X", "not-a-hash"); err == nil {
		t.Fatal("expected an error for a non-bcrypt value")
	}
}

func TestSetupInitCreatesOwnerAndStartsWizard(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	createSystemSettingsCollection(t, app)
	ensureUsersRoleAndNameFields(t, app)

	var announced []string
	guard := newSetupGuard(time.Now, func(c string) { announced = append(announced, c) })
	if err := guard.Issue(); err != nil {
		t.Fatal(err)
	}

	req := setupInitRequest{
		Code: formatSetupCode(announced[0]), Name: "Dana Reyes",
		Email: "dana@example.com", Password: "OwnerPass1234!", AppURL: "https://cloud.example.com",
	}
	res, status := runSetupInit(app, guard, "1.1.1.1", req)
	if status != http.StatusOK {
		t.Fatalf("status = %d (%v)", status, res)
	}
	owner, err := app.FindAuthRecordByEmail("users", "dana@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if owner.GetString("name") != "Dana Reyes" || owner.GetString("role") != "owner" {
		t.Fatalf("owner = name %q role %q", owner.GetString("name"), owner.GetString("role"))
	}
	if _, err := app.FindFirstRecordByFilter("system_settings", "key = {:k}", map[string]any{"k": setupWizardKey}); err != nil {
		t.Fatal("wizard state row missing")
	}
	if guard.NeedsSetup() {
		t.Fatal("code still active after init")
	}

	_, again := runSetupInit(app, guard, "1.1.1.1", req)
	if again != http.StatusForbidden {
		t.Fatalf("second init status = %d, want 403", again)
	}
}
