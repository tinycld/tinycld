package coreserver

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"golang.org/x/crypto/bcrypt"
)

// ensureUsersCollection makes sure the test app has a `users` auth collection
// with the fields createOwnerOperator/CreateOwnerAccountWithHash need. A test
// app built from tests.NewTestApp has no JS migrations dir, so RunAppMigrations
// is a no-op and the collection PocketBase's system migration creates may lack
// username/role. app.Bootstrap() is called here (before the command's own
// Bootstrap) because the command constructs its App from scratch; PocketBase's
// Bootstrap returns early once already bootstrapped, so calling it again inside
// the command is safe.
func ensureUsersCollection(t *testing.T, app *pocketbase.PocketBase) {
	t.Helper()
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	if existing, err := app.FindCollectionByNameOrId("users"); err == nil {
		changed := false
		if existing.Fields.GetByName("username") == nil {
			existing.Fields.Add(&core.TextField{Name: "username"})
			changed = true
		}
		if existing.Fields.GetByName("role") == nil {
			existing.Fields.Add(&core.TextField{Name: "role"})
			changed = true
		}
		if changed {
			if err := app.Save(existing); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	users := core.NewAuthCollection("users")
	users.Fields.Add(&core.TextField{Name: "name"})
	users.Fields.Add(&core.TextField{Name: "username"})
	users.Fields.Add(&core.TextField{Name: "role"})
	if err := app.Save(users); err != nil {
		t.Fatal(err)
	}
}

// The command must accept a pre-computed hash and a display name, and must
// refuse both --password and --password-hash together (two secrets for one
// account is an operator mistake, not a choice).
func TestCreateOwnerCommand_PasswordHash(t *testing.T) {
	dir := t.TempDir()
	hash, _ := bcrypt.GenerateFromPassword([]byte("pw-1234567890"), bcrypt.MinCost)

	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: dir, HideStartBanner: true})
	ensureUsersCollection(t, app)
	createSystemSettingsCollection(t, app)
	cmd := NewCreateOwnerCommand(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"ada@example.com", "--password-hash", string(hash), "--name", "Ada"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "owner: ada@example.com") {
		t.Fatalf("missing confirmation line: %s", out.String())
	}
	if strings.Contains(out.String(), "password:") {
		t.Fatalf("a hash run must never print a password: %s", out.String())
	}
	user, err := app.FindAuthRecordByEmail("users", "ada@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !user.ValidatePassword("pw-1234567890") || user.GetString("name") != "Ada" {
		t.Fatal("owner not minted from the hash with the given name")
	}
	su, err := app.FindAuthRecordByEmail("_superusers", "ada@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !su.ValidatePassword("pw-1234567890") {
		t.Fatal("superuser must share the same hash")
	}
	// Wizard state is created on owner creation.
	if _, err := app.FindFirstRecordByFilter("system_settings", "key = {:k}", map[string]any{"k": setupWizardKey}); err != nil {
		t.Fatalf("wizard state row missing after owner creation: %v", err)
	}
}

func TestCreateOwnerCommand_RefusesBothSecrets(t *testing.T) {
	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir(), HideStartBanner: true})
	cmd := NewCreateOwnerCommand(app)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"ada@example.com", "--password", "pw-1234567890", "--password-hash", "$2a$10$x"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected an error when both --password and --password-hash are given")
	}
}

// --name must reach the plaintext path too, not just the hash path — the
// command documents --name unconditionally.
func TestCreateOwnerCommand_PasswordWithName(t *testing.T) {
	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir(), HideStartBanner: true})
	ensureUsersCollection(t, app)
	createSystemSettingsCollection(t, app)
	cmd := NewCreateOwnerCommand(app)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"ada@example.com", "--password", "pw-1234567890", "--name", "Ada"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	user, err := app.FindAuthRecordByEmail("users", "ada@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got := user.GetString("name"); got != "Ada" {
		t.Fatalf("name = %q, want Ada: --name must be honored on the plaintext path too", got)
	}
}

// A non-bcrypt --password-hash must be refused before either record is
// touched. Validating only on the users side (after the superuser is already
// written) would leave a permanently corrupt superuser: the idempotency check
// finds that record on retry and skips it, so it never gets a valid password.
func TestCreateOwnerCommand_RejectsBadHash_NoSuperuserLeftBehind(t *testing.T) {
	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir(), HideStartBanner: true})
	ensureUsersCollection(t, app)
	cmd := NewCreateOwnerCommand(app)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"ada@example.com", "--password-hash", "not-a-hash"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected an error for a non-bcrypt --password-hash")
	}
	if su, _ := app.FindAuthRecordByEmail("_superusers", "ada@example.com"); su != nil {
		t.Fatal("a rejected hash must not leave a corrupt superuser record behind")
	}
	if u, _ := app.FindAuthRecordByEmail("users", "ada@example.com"); u != nil {
		t.Fatal("a rejected hash must not leave a users record behind")
	}
}
