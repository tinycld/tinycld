package coreserver

import (
	"strconv"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// setupGuardTestApp builds a TestApp with the users.role / is_demo schema the
// guard relies on. Single-org: there is no orgs / user_org collection — the
// caller's owner/admin status lives on their users.role field.
func setupGuardTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users: %v", err)
	}
	users.Fields.Add(&core.BoolField{Name: "is_demo"})
	users.Fields.Add(&core.SelectField{
		Name: "role", MaxSelect: 1,
		Values: []string{"owner", "admin", "member", "guest"},
	})
	// PB's bundled test fixture ships the default users collection with a
	// 3-char minimum username; our production migration relaxes it to 1.
	// Mirror that here so derived usernames from short email prefixes
	// (e.g. "ma@test.local" → "ma") validate the same way they do in prod.
	relaxUsernameMinLength(users)
	// Single-org relaxed rule: any authed user may attempt an update; the Go
	// guard narrows it to self-edits and owner/admin edits.
	users.UpdateRule = stringPtr(
		`@request.auth.id != "" && (id = @request.auth.id || ` +
			`@request.auth.role = "owner" || @request.auth.role = "admin")`,
	)
	if err := app.Save(users); err != nil {
		t.Fatalf("save users: %v", err)
	}

	registerUsersFieldGuardCore(app)
	return app
}

func stringPtr(s string) *string { return &s }

func makeUser(t *testing.T, app core.App, email string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.SetEmail(email)
	r.Set("username", uniqueDerivedUsername(t, app, email))
	r.Set("name", "Original Name")
	r.SetVerified(true)
	r.SetPassword("Password123!")
	if err := app.Save(r); err != nil {
		t.Fatalf("save user %s: %v", email, err)
	}
	return r
}

// makeUserWithRole creates a user and assigns them a single-org role.
func makeUserWithRole(t *testing.T, app core.App, email, role string) *core.Record {
	t.Helper()
	r := makeUser(t, app, email)
	r.Set("role", role)
	if err := app.Save(r); err != nil {
		t.Fatalf("set role on %s: %v", email, err)
	}
	return r
}

// uniqueDerivedUsername derives a username from email and adds a numeric
// suffix if the base is already taken. Mirrors the production backfill so
// short prefixes like "ma@..." and "mb@..." (both → "user") don't collide.
func uniqueDerivedUsername(t *testing.T, app core.App, email string) string {
	t.Helper()
	base := DeriveUsername(email)
	candidate := base
	for i := 2; ; i++ {
		existing, _ := app.FindFirstRecordByFilter(
			"users", "username = {:u}", map[string]any{"u": candidate})
		if existing == nil {
			return candidate
		}
		candidate = base + strconv.Itoa(i)
	}
}

// updateAsAuthenticated invokes the OnRecordUpdateRequest hook chain the
// way the API would. Reloads the target from the DB first so Record.Original()
// reflects the persisted state (PB's Save doesn't refresh originalData
// in-place, so a record that was Save()'d in test setup still reports its
// initial pre-Save values as Original — the API-side flow always loads
// fresh records from DB before applying writes).
func updateAsAuthenticated(
	t *testing.T,
	app *tests.TestApp,
	caller *core.Record,
	target *core.Record,
	mutate func(*core.Record),
) error {
	t.Helper()
	fresh, err := app.FindRecordById("users", target.Id)
	if err != nil {
		t.Fatalf("reload target: %v", err)
	}
	mutate(fresh)

	usersCol, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}

	e := &core.RecordRequestEvent{
		RequestEvent: &core.RequestEvent{Auth: caller, App: app},
		Record:       fresh,
	}
	// Tags() reads from the embedded baseCollectionEventData.Collection;
	// without it, the tagged hook filter would skip our handler.
	e.Collection = usersCol

	return app.OnRecordUpdateRequest("users").Trigger(e, func(_ *core.RecordRequestEvent) error {
		return app.Save(fresh)
	})
}

func TestUsersGuard_SelfCanEditNameAndAvatar(t *testing.T) {
	app := setupGuardTestApp(t)
	user := makeUser(t, app, "self@test.local")

	err := updateAsAuthenticated(t, app, user, user, func(r *core.Record) {
		r.Set("name", "New Name")
		r.Set("avatar", "")
	})
	if err != nil {
		t.Fatalf("self-edit of name/avatar should be allowed: %v", err)
	}

	// Reload and verify.
	fresh, _ := app.FindRecordById("users", user.Id)
	if fresh.GetString("name") != "New Name" {
		t.Errorf("name not saved, got %q", fresh.GetString("name"))
	}
}

func TestUsersGuard_SelfCannotEditIsDemo(t *testing.T) {
	app := setupGuardTestApp(t)
	user := makeUser(t, app, "self2@test.local")
	user.Set("is_demo", true)
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}

	err := updateAsAuthenticated(t, app, user, user, func(r *core.Record) {
		r.Set("is_demo", false)
	})
	if err == nil {
		t.Fatal("self-edit of is_demo should have been rejected")
	}

	fresh, _ := app.FindRecordById("users", user.Id)
	if !fresh.GetBool("is_demo") {
		t.Error("is_demo should still be true after rejected edit")
	}
}

func TestUsersGuard_DemoUserCannotSelfEditAnything(t *testing.T) {
	app := setupGuardTestApp(t)
	user := makeUser(t, app, "demo@test.local")
	user.Set("is_demo", true)
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}

	// Even fields that would normally be self-editable (name, avatar) must
	// be rejected — the demo account is shared across anonymous visitors,
	// so any persisted edit leaks to the next session.
	err := updateAsAuthenticated(t, app, user, user, func(r *core.Record) {
		r.Set("name", "Visitor Vandal")
	})
	if err == nil {
		t.Fatal("demo user self-edit of name should have been rejected")
	}

	fresh, _ := app.FindRecordById("users", user.Id)
	if fresh.GetString("name") != "Original Name" {
		t.Errorf("name should be unchanged, got %q", fresh.GetString("name"))
	}
}

func TestUsersGuard_AdminCanStillEditDemoUser(t *testing.T) {
	app := setupGuardTestApp(t)
	admin := makeUserWithRole(t, app, "demoadmin@test.local", "owner")
	target := makeUserWithRole(t, app, "demotarget@test.local", "member")
	target.Set("is_demo", true)
	if err := app.Save(target); err != nil {
		t.Fatal(err)
	}

	// The demo lockout only applies to self-edits; an org admin must still
	// be able to flip is_demo back off (e.g. operator reclaiming an account).
	err := updateAsAuthenticated(t, app, admin, target, func(r *core.Record) {
		r.Set("is_demo", false)
	})
	if err != nil {
		t.Fatalf("admin should still be able to clear is_demo on a demo user: %v", err)
	}

	fresh, _ := app.FindRecordById("users", target.Id)
	if fresh.GetBool("is_demo") {
		t.Error("is_demo should be false after admin clear")
	}
}

func TestUsersGuard_SelfCannotEditVerified(t *testing.T) {
	app := setupGuardTestApp(t)
	user := makeUser(t, app, "self3@test.local")
	user.SetVerified(false)
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}

	err := updateAsAuthenticated(t, app, user, user, func(r *core.Record) {
		r.SetVerified(true)
	})
	if err == nil {
		t.Fatal("self-edit of verified should have been rejected")
	}

	fresh, _ := app.FindRecordById("users", user.Id)
	if fresh.Verified() {
		t.Error("verified should still be false after rejected edit")
	}
}

// The guard no longer pre-rejects a self password change. Old-password
// correctness is enforced separately by PocketBase's own auth-record form
// validation on the real request path (see the change-password e2e); this
// test only asserts the field-allowlist guard lets the change through.
func TestUsersGuard_SelfCanChangePassword(t *testing.T) {
	app := setupGuardTestApp(t)
	user := makeUser(t, app, "self-pw@test.local")

	err := updateAsAuthenticated(t, app, user, user, func(r *core.Record) {
		r.SetPassword("RotatedSelf1!")
	})
	if err != nil {
		t.Fatalf("self password change should be allowed by the guard: %v", err)
	}

	fresh, _ := app.FindRecordById("users", user.Id)
	if !fresh.ValidatePassword("RotatedSelf1!") {
		t.Error("new password should validate after self change")
	}
}

func TestUsersGuard_AdminCanFlipIsDemo(t *testing.T) {
	app := setupGuardTestApp(t)
	admin := makeUserWithRole(t, app, "admin@test.local", "admin")
	target := makeUserWithRole(t, app, "target@test.local", "member")

	err := updateAsAuthenticated(t, app, admin, target, func(r *core.Record) {
		r.Set("is_demo", true)
	})
	if err != nil {
		t.Fatalf("admin flipping is_demo on a member should be allowed: %v", err)
	}

	fresh, _ := app.FindRecordById("users", target.Id)
	if !fresh.GetBool("is_demo") {
		t.Error("is_demo not persisted")
	}
}

func TestUsersGuard_AdminCannotEditNonAllowlistedField(t *testing.T) {
	app := setupGuardTestApp(t)
	admin := makeUserWithRole(t, app, "admin2@test.local", "owner")
	target := makeUserWithRole(t, app, "target2@test.local", "member")

	err := updateAsAuthenticated(t, app, admin, target, func(r *core.Record) {
		r.SetVerified(false) // not in adminEditableUserFields
	})
	if err == nil {
		t.Fatal("admin editing verified on another user should be rejected")
	}
}

func TestUsersGuard_AdminCannotEditPasswordOnAnotherUser(t *testing.T) {
	app := setupGuardTestApp(t)
	admin := makeUserWithRole(t, app, "admin3@test.local", "admin")
	target := makeUserWithRole(t, app, "target3@test.local", "member")

	err := updateAsAuthenticated(t, app, admin, target, func(r *core.Record) {
		r.SetPassword("HackedPassword!")
	})
	if err == nil {
		t.Fatal("admin setting another user's password should be rejected")
	}

	// Verify password didn't change by re-validating the original.
	fresh, _ := app.FindRecordById("users", target.Id)
	if !fresh.ValidatePassword("Password123!") {
		t.Error("original password should still validate")
	}
}

func TestUsersGuard_PlainMemberCannotEditAnotherUser(t *testing.T) {
	app := setupGuardTestApp(t)
	memberA := makeUserWithRole(t, app, "ma@test.local", "member")
	memberB := makeUserWithRole(t, app, "mb@test.local", "member")

	err := updateAsAuthenticated(t, app, memberA, memberB, func(r *core.Record) {
		r.Set("name", "tampered")
	})
	if err == nil {
		t.Fatal("non-admin member should not be able to edit another member")
	}
}

// Superusers (e.g. the seed/reset-demo CLI scripts authed as
// _superusers) need to overwrite fields outside the admin allowlist —
// notably email and password on the singleton demo account. Without a
// bypass the guard rejects the write with "Only the record owner can
// change this field" and the script falls back to a doomed create.
func TestUsersGuard_SuperuserCanEditAnyField(t *testing.T) {
	app := setupGuardTestApp(t)
	target := makeUser(t, app, "supertarget@test.local")

	su, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		t.Fatal(err)
	}
	superuser := core.NewRecord(su)
	superuser.SetEmail("super@test.local")
	superuser.SetPassword("Superuser1234!")
	if err := app.Save(superuser); err != nil {
		t.Fatalf("save superuser: %v", err)
	}

	err = updateAsAuthenticated(t, app, superuser, target, func(r *core.Record) {
		r.SetEmail("renamed@test.local")
		r.SetPassword("Rotated1234!")
		r.Set("name", "Operator Rewrite")
	})
	if err != nil {
		t.Fatalf("superuser write should bypass field allowlist: %v", err)
	}

	fresh, _ := app.FindRecordById("users", target.Id)
	if fresh.Email() != "renamed@test.local" {
		t.Errorf("email not saved, got %q", fresh.Email())
	}
	if !fresh.ValidatePassword("Rotated1234!") {
		t.Error("rotated password should validate")
	}
}

// seedAuditLogs adds the audit_logs collection so the demo-audit hook has
// somewhere to write. Mirrors the migration's shape minimally — only the
// fields the hook actually sets.
func seedAuditLogs(t *testing.T, app *tests.TestApp) {
	t.Helper()
	col := core.NewBaseCollection("audit_logs")
	col.Fields.Add(&core.TextField{Name: "action"})
	col.Fields.Add(&core.TextField{Name: "resource_type"})
	col.Fields.Add(&core.TextField{Name: "resource_id"})
	col.Fields.Add(&core.TextField{Name: "resource_label"})
	col.Fields.Add(&core.JSONField{Name: "metadata"})
	col.Fields.Add(&core.TextField{Name: "actor"})
	col.Fields.Add(&core.TextField{Name: "ip_address"})
	col.Fields.Add(&core.TextField{Name: "user_agent"})
	if err := app.Save(col); err != nil {
		t.Fatalf("seed audit_logs: %v", err)
	}
}

func TestUsersDemoAuditHook_LogsOnFlip(t *testing.T) {
	app := setupGuardTestApp(t)
	seedAuditLogs(t, app)
	registerUsersDemoAuditHookCore(app)

	admin := makeUserWithRole(t, app, "auditadmin@test.local", "admin")
	target := makeUserWithRole(t, app, "auditmember@test.local", "member")

	if err := updateAsAuthenticated(t, app, admin, target, func(r *core.Record) {
		r.Set("is_demo", true)
	}); err != nil {
		t.Fatalf("flip should succeed: %v", err)
	}

	logs, err := app.FindRecordsByFilter(
		"audit_logs",
		"action = 'users.demo_changed' && resource_id = {:rid}",
		"", 0, 0,
		map[string]any{"rid": target.Id},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(logs))
	}
	if logs[0].GetString("actor") != admin.Id {
		t.Errorf("actor mismatch: got %q want %q", logs[0].GetString("actor"), admin.Id)
	}
}

func TestUsersDemoAuditHook_NoLogWhenFlagUnchanged(t *testing.T) {
	app := setupGuardTestApp(t)
	seedAuditLogs(t, app)
	registerUsersDemoAuditHookCore(app)

	user := makeUser(t, app, "noflip@test.local")

	// A non-demo-flag self-edit (changing name) shouldn't write to audit_logs.
	if err := updateAsAuthenticated(t, app, user, user, func(r *core.Record) {
		r.Set("name", "Changed Name")
	}); err != nil {
		t.Fatal(err)
	}

	logs, err := app.FindAllRecords("audit_logs")
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 0 {
		t.Errorf("expected 0 audit entries for non-demo-flag change, got %d", len(logs))
	}
}

func TestUsersGuard_UnauthRejected(t *testing.T) {
	app := setupGuardTestApp(t)
	target := makeUser(t, app, "anyone@test.local")
	usersCol, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}

	target.Set("name", "tampered")
	e := &core.RecordRequestEvent{
		RequestEvent: &core.RequestEvent{Auth: nil, App: app},
		Record:       target,
	}
	e.Collection = usersCol

	err = app.OnRecordUpdateRequest("users").Trigger(e, func(_ *core.RecordRequestEvent) error {
		return app.Save(target)
	})
	if err == nil {
		t.Fatal("update without auth must be rejected")
	}
}
