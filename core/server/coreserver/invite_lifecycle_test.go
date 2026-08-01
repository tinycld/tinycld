package coreserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// These tests cover the demo-gated invite-email flow. The mailer's LogSender
// (the default fallback when no Postmark token is configured) writes a JSON
// line per send to the file at TINYCLD_EMAIL_LOG. We point that env var at a
// per-test temp file and assert on what does (or doesn't) get written.

func setupInviteTestApp(t *testing.T) *tests.TestApp {
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
		Name:      "role",
		Values:    []string{"owner", "admin", "member", "guest"},
		MaxSelect: 1,
	})
	relaxUsernameMinLength(users)
	if err := app.Save(users); err != nil {
		t.Fatal(err)
	}

	// invite_tokens collection — minimal shape used by mintInviteToken. Single
	// org: no `org` relation.
	tokens := core.NewBaseCollection("invite_tokens")
	tokens.Fields.Add(&core.TextField{Name: "token", Required: true})
	tokens.Fields.Add(&core.RelationField{
		Name: "user", Required: true, CollectionId: users.Id, MaxSelect: 1,
	})
	tokens.Fields.Add(&core.TextField{Name: "role", Required: true})
	tokens.Fields.Add(&core.TextField{Name: "expires_at"})
	tokens.Fields.Add(&core.TextField{Name: "used_at"})
	if err := app.Save(tokens); err != nil {
		t.Fatal(err)
	}

	return app
}

// setUserRole sets the single-org role field on a user record and saves it.
func setUserRole(t *testing.T, app core.App, user *core.Record, role string) *core.Record {
	t.Helper()
	user.Set("role", role)
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}
	return user
}

// captureMailerOutput configures the LogSender to append every send to a
// per-test temp file via TINYCLD_EMAIL_LOG, returning a reader function.
// Calls before this setup run won't be captured; calls after t.Cleanup are
// still appended but no longer read.
func captureMailerOutput(t *testing.T) func() []map[string]any {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mail.log")
	prev := os.Getenv("TINYCLD_EMAIL_LOG")
	t.Setenv("TINYCLD_EMAIL_LOG", path)
	t.Cleanup(func() { _ = os.Setenv("TINYCLD_EMAIL_LOG", prev) })

	return func() []map[string]any {
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			t.Fatal(err)
		}
		var out []map[string]any
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if line == "" {
				continue
			}
			var entry map[string]any
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				t.Fatalf("decode mail log line %q: %v", line, err)
			}
			out = append(out, entry)
		}
		return out
	}
}

// sendExistingMemberEmail is the "you've been added" notice sent to an
// already-verified user. These tests exercise it directly (it's a plain
// function now, no user_org hook).

func TestExistingMemberEmail_SendsAddedSubject(t *testing.T) {
	app := setupInviteTestApp(t)
	read := captureMailerOutput(t)

	target := mustCreateUser(t, app, "existing2@test.local", false)
	target.SetVerified(true)
	if err := app.Save(target); err != nil {
		t.Fatal(err)
	}

	sendExistingMemberEmail(app, target, "member")

	sends := read()
	if len(sends) != 1 {
		t.Fatalf("expected 1 email, got %d: %v", len(sends), sends)
	}
	subject, _ := sends[0]["subject"].(string)
	if !strings.Contains(subject, "added") {
		t.Errorf("expected 'added' subject for verified-existing user, got %q", subject)
	}
}
