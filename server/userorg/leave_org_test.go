package userorg

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// setupTestApp builds an in-memory PocketBase test app with the minimal
// orgs + user_org schema plus one reassignable collection (calendar_events)
// to exercise the registry path. The collection is named generically so the
// test doesn't depend on calendar/server.
func setupTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	ResetReassignableForTesting()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(app.Cleanup)

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("users collection: %v", err)
	}

	orgs := core.NewBaseCollection("orgs")
	orgs.Fields.Add(&core.TextField{Name: "name", Required: true})
	orgs.Fields.Add(&core.TextField{Name: "slug", Required: true})
	orgs.Fields.Add(&core.RelationField{
		Name: "users", CollectionId: users.Id, MaxSelect: 999,
	})
	if err := app.Save(orgs); err != nil {
		t.Fatalf("save orgs: %v", err)
	}

	userOrg := core.NewBaseCollection("user_org")
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
	userOrg.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	if err := app.Save(userOrg); err != nil {
		t.Fatalf("save user_org: %v", err)
	}

	// Reproducer collection: a required FK to user_org with cascadeDelete:false.
	// This is the shape that blocked the production bug
	// (calendar_events.created_by). Without LeaveOrg's reassign/delete pass,
	// deleting the user_org would fail with the PB referential-integrity error.
	//
	// We also add a cascade-delete FK to `orgs` so the schema mimics how
	// calendar_events lives in production (events belong to a calendar that
	// belongs to an org; deleting the org cascade-deletes events before the
	// user_org cascade runs). This makes ModeDeleteOrg work in tests the
	// same way it works in real schemas.
	events := core.NewBaseCollection("test_events")
	events.Fields.Add(&core.TextField{Name: "title", Required: true})
	events.Fields.Add(&core.RelationField{
		Name: "created_by", Required: true, CollectionId: userOrg.Id,
		CascadeDelete: false, MaxSelect: 1,
	})
	events.Fields.Add(&core.RelationField{
		Name: "org", Required: true, CollectionId: orgs.Id,
		CascadeDelete: true, MaxSelect: 1,
	})
	if err := app.Save(events); err != nil {
		t.Fatalf("save test_events: %v", err)
	}

	RegisterReassignable(ReassignableRef{Collection: "test_events", Field: "created_by"})

	return app
}

func makeUser(t *testing.T, app core.App, email string) *core.Record {
	t.Helper()
	users, _ := app.FindCollectionByNameOrId("users")
	u := core.NewRecord(users)
	u.SetEmail(email)
	u.Set("name", "T")
	u.SetVerified(true)
	u.SetPassword("Password123!")
	if err := app.Save(u); err != nil {
		t.Fatalf("save user %s: %v", email, err)
	}
	return u
}

func makeOrg(t *testing.T, app core.App, name string) *core.Record {
	t.Helper()
	orgs, _ := app.FindCollectionByNameOrId("orgs")
	o := core.NewRecord(orgs)
	o.Set("name", name)
	o.Set("slug", strings.ReplaceAll(strings.ToLower(name), " ", "-"))
	if err := app.Save(o); err != nil {
		t.Fatalf("save org %s: %v", name, err)
	}
	return o
}

func makeUserOrg(t *testing.T, app core.App, user, org *core.Record, role string) *core.Record {
	t.Helper()
	uo, _ := app.FindCollectionByNameOrId("user_org")
	r := core.NewRecord(uo)
	r.Set("user", user.Id)
	r.Set("org", org.Id)
	r.Set("role", role)
	if err := app.Save(r); err != nil {
		t.Fatalf("save user_org: %v", err)
	}
	return r
}

func makeEvent(t *testing.T, app core.App, title, createdBy, orgID string) *core.Record {
	t.Helper()
	col, _ := app.FindCollectionByNameOrId("test_events")
	e := core.NewRecord(col)
	e.Set("title", title)
	e.Set("created_by", createdBy)
	e.Set("org", orgID)
	if err := app.Save(e); err != nil {
		t.Fatalf("save event %s: %v", title, err)
	}
	return e
}

// TestDeleteOrgCascade_MultiLevelChain — regression guard for the cascade
// chain ModeDeleteOrg relies on. Sets up a three-level chain that mirrors
// production (events → folders → orgs, with the events ALSO pinning the
// user_org via a required cascadeDelete:false FK), then triggers
// ModeDeleteOrg. If any link in the chain breaks, the user_org delete will
// fail with a referential-integrity error and the test catches it.
//
// This is what calc_comments and text_comments look like in production:
// they reference drive_items (cascade), drive_items references orgs
// (cascade), and a separate `author` FK pins user_org without cascade.
func TestDeleteOrgCascade_MultiLevelChain(t *testing.T) {
	ResetReassignableForTesting()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(app.Cleanup)

	users, _ := app.FindCollectionByNameOrId("users")

	orgs := core.NewBaseCollection("orgs")
	orgs.Fields.Add(&core.TextField{Name: "name", Required: true})
	orgs.Fields.Add(&core.TextField{Name: "slug", Required: true})
	if err := app.Save(orgs); err != nil {
		t.Fatalf("save orgs: %v", err)
	}

	userOrg := core.NewBaseCollection("user_org")
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
	userOrg.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	if err := app.Save(userOrg); err != nil {
		t.Fatalf("save user_org: %v", err)
	}

	// Mid-level parent that cascade-deletes when org goes. Mirrors
	// drive_items / calendar_calendars in production.
	folders := core.NewBaseCollection("test_folders")
	folders.Fields.Add(&core.TextField{Name: "name", Required: true})
	folders.Fields.Add(&core.RelationField{
		Name: "org", Required: true, CollectionId: orgs.Id,
		CascadeDelete: true, MaxSelect: 1,
	})
	if err := app.Save(folders); err != nil {
		t.Fatalf("save test_folders: %v", err)
	}

	// Leaf collection: cascade-delete via folder, BUT also pins user_org
	// with a required non-cascade FK. This is the production shape that
	// broke account-delete.
	events := core.NewBaseCollection("test_chain_events")
	events.Fields.Add(&core.TextField{Name: "title", Required: true})
	events.Fields.Add(&core.RelationField{
		Name: "folder", Required: true, CollectionId: folders.Id,
		CascadeDelete: true, MaxSelect: 1,
	})
	events.Fields.Add(&core.RelationField{
		Name: "created_by", Required: true, CollectionId: userOrg.Id,
		CascadeDelete: false, MaxSelect: 1,
	})
	if err := app.Save(events); err != nil {
		t.Fatalf("save test_chain_events: %v", err)
	}

	RegisterReassignable(ReassignableRef{Collection: "test_chain_events", Field: "created_by"})

	alice := makeUser(t, app, "alice@test.local")
	org := makeOrg(t, app, "SoloChain")
	aliceUO := makeUserOrg(t, app, alice, org, "owner")

	folder := core.NewRecord(folders)
	folder.Set("name", "F")
	folder.Set("org", org.Id)
	if err := app.Save(folder); err != nil {
		t.Fatalf("save folder: %v", err)
	}
	evt := core.NewRecord(events)
	evt.Set("title", "E")
	evt.Set("folder", folder.Id)
	evt.Set("created_by", aliceUO.Id)
	if err := app.Save(evt); err != nil {
		t.Fatalf("save event: %v", err)
	}

	// Sole-member triggers ModeDeleteOrg. The cascade must walk:
	//   org → test_folders(cascade) → test_chain_events(cascade)
	// and only THEN delete user_org via its org cascade — otherwise the
	// created_by FK on test_chain_events pins it.
	result, err := LeaveOrg(app, aliceUO.Id, Plan{Mode: ModeReassign}, true)
	if err != nil {
		t.Fatalf("LeaveOrg: %v", err)
	}
	if !result.OrgDeleted {
		t.Error("expected org_deleted=true")
	}
	if _, err := app.FindRecordById("test_chain_events", evt.Id); err == nil {
		t.Error("expected event to be cascade-deleted")
	}
	if _, err := app.FindRecordById("test_folders", folder.Id); err == nil {
		t.Error("expected folder to be cascade-deleted")
	}
	if _, err := app.FindRecordById("user_org", aliceUO.Id); err == nil {
		t.Error("expected user_org to be cascade-deleted")
	}
}

// TestLeaveOrg_ReproducesProductionBug — the exact shape that broke prod:
// user owns calendar_events.created_by, raw user_org delete fails. With the
// new flow, ModeReassign rewrites the FK and the user_org deletes cleanly.
func TestLeaveOrg_ReproducesProductionBug(t *testing.T) {
	app := setupTestApp(t)

	alice := makeUser(t, app, "alice@test.local")
	bob := makeUser(t, app, "bob@test.local")
	org := makeOrg(t, app, "Acme")
	aliceUO := makeUserOrg(t, app, alice, org, "owner")
	bobUO := makeUserOrg(t, app, bob, org, "owner")

	event := makeEvent(t, app, "Standup", aliceUO.Id, org.Id)

	// Bare user_org delete fails (this is the production bug).
	if err := app.Delete(aliceUO); err == nil {
		t.Fatal("expected raw delete to fail with FK error, got nil")
	}

	// LeaveOrg with reassign succeeds.
	result, err := LeaveOrg(app, aliceUO.Id, Plan{
		Mode:               ModeReassign,
		SuccessorUserOrgID: bobUO.Id,
	}, true)
	if err != nil {
		t.Fatalf("LeaveOrg: %v", err)
	}
	if result.RecordsReassigned != 1 {
		t.Errorf("expected 1 record reassigned, got %d", result.RecordsReassigned)
	}

	// Event now belongs to bob.
	updated, err := app.FindRecordById("test_events", event.Id)
	if err != nil {
		t.Fatalf("re-find event: %v", err)
	}
	if updated.GetString("created_by") != bobUO.Id {
		t.Errorf("created_by: got %s, want %s", updated.GetString("created_by"), bobUO.Id)
	}

	// Alice's user_org is gone.
	if _, err := app.FindRecordById("user_org", aliceUO.Id); err == nil {
		t.Error("expected aliceUO to be deleted")
	}
}

// TestLeaveOrg_DeleteMyData removes the user's content instead of reassigning.
func TestLeaveOrg_DeleteMyData(t *testing.T) {
	app := setupTestApp(t)
	alice := makeUser(t, app, "alice@test.local")
	bob := makeUser(t, app, "bob@test.local")
	org := makeOrg(t, app, "Acme")
	aliceUO := makeUserOrg(t, app, alice, org, "owner")
	_ = makeUserOrg(t, app, bob, org, "member")

	event := makeEvent(t, app, "Standup", aliceUO.Id, org.Id)

	result, err := LeaveOrg(app, aliceUO.Id, Plan{Mode: ModeDeleteMyData}, true)
	if err != nil {
		t.Fatalf("LeaveOrg: %v", err)
	}
	if result.RecordsDeleted != 1 {
		t.Errorf("expected 1 record deleted, got %d", result.RecordsDeleted)
	}
	if _, err := app.FindRecordById("test_events", event.Id); err == nil {
		t.Error("expected event to be deleted")
	}
}

// TestLeaveOrg_SoleMemberForcesDeleteOrg — even if the client asks for
// reassign, the server overrides to delete_org because there's no one to
// reassign to and the org would be left empty otherwise.
func TestLeaveOrg_SoleMemberForcesDeleteOrg(t *testing.T) {
	app := setupTestApp(t)
	alice := makeUser(t, app, "alice@test.local")
	org := makeOrg(t, app, "Solo")
	aliceUO := makeUserOrg(t, app, alice, org, "owner")
	_ = makeEvent(t, app, "Lonely Event", aliceUO.Id, org.Id)

	// Client passes ModeReassign with a bogus successor — server should
	// ignore both and force delete_org since alice is the only member.
	result, err := LeaveOrg(app, aliceUO.Id, Plan{
		Mode:               ModeReassign,
		SuccessorUserOrgID: "doesnt-exist",
	}, true)
	if err != nil {
		t.Fatalf("LeaveOrg: %v", err)
	}
	if !result.OrgDeleted {
		t.Error("expected org_deleted=true for sole member")
	}
	if !result.UserAnonymized {
		t.Error("expected user_anonymized=true after last org gone")
	}

	if _, err := app.FindRecordById("orgs", org.Id); err == nil {
		t.Error("expected org to be deleted")
	}
}

// TestLeaveOrg_SoleOwnerPromotesSuccessor — leaver was the only owner.
// LeaveOrg must promote the chosen successor to owner before reassigning,
// otherwise the org ends up ownerless.
func TestLeaveOrg_SoleOwnerPromotesSuccessor(t *testing.T) {
	app := setupTestApp(t)
	alice := makeUser(t, app, "alice@test.local")
	bob := makeUser(t, app, "bob@test.local")
	org := makeOrg(t, app, "Acme")
	aliceUO := makeUserOrg(t, app, alice, org, "owner")
	bobUO := makeUserOrg(t, app, bob, org, "member")

	_, err := LeaveOrg(app, aliceUO.Id, Plan{
		Mode:               ModeReassign,
		SuccessorUserOrgID: bobUO.Id,
	}, true)
	if err != nil {
		t.Fatalf("LeaveOrg: %v", err)
	}

	bobAfter, err := app.FindRecordById("user_org", bobUO.Id)
	if err != nil {
		t.Fatalf("re-find bob's user_org: %v", err)
	}
	if bobAfter.GetString("role") != "owner" {
		t.Errorf("bob role: got %q, want %q (sole-owner leaver should promote successor)",
			bobAfter.GetString("role"), "owner")
	}
}

// TestLeaveOrg_RejectsCrossOrgSuccessor — passing a user_org ID from a
// different org must fail loudly. Tested both via the validator and as a
// safety check against malicious / buggy clients.
func TestLeaveOrg_RejectsCrossOrgSuccessor(t *testing.T) {
	app := setupTestApp(t)
	alice := makeUser(t, app, "alice@test.local")
	bob := makeUser(t, app, "bob@test.local")
	carol := makeUser(t, app, "carol@test.local")
	orgA := makeOrg(t, app, "A")
	orgB := makeOrg(t, app, "B")
	aliceUO := makeUserOrg(t, app, alice, orgA, "owner")
	_ = makeUserOrg(t, app, bob, orgA, "member")
	carolUO := makeUserOrg(t, app, carol, orgB, "owner") // wrong org

	_, err := LeaveOrg(app, aliceUO.Id, Plan{
		Mode:               ModeReassign,
		SuccessorUserOrgID: carolUO.Id,
	}, true)
	if err == nil {
		t.Fatal("expected ErrInvalidPlan for cross-org successor")
	}
}

// TestLeaveOrg_DeleteOrgRejectedForMultiMember — the client must not be able
// to force-delete an org by passing ModeDeleteOrg when it has other members.
// (Only the server's sole-member detection should ever choose delete_org.)
func TestLeaveOrg_DeleteOrgRejectedForMultiMember(t *testing.T) {
	app := setupTestApp(t)
	alice := makeUser(t, app, "alice@test.local")
	bob := makeUser(t, app, "bob@test.local")
	org := makeOrg(t, app, "Acme")
	aliceUO := makeUserOrg(t, app, alice, org, "owner")
	_ = makeUserOrg(t, app, bob, org, "member")

	_, err := LeaveOrg(app, aliceUO.Id, Plan{Mode: ModeDeleteOrg}, true)
	if err == nil {
		t.Fatal("expected ErrInvalidPlan when client requests delete_org with peers present")
	}
}

// TestLeaveOrg_AdminDoesNotAnonymize — when an admin removes a member, that
// member's users record must not be anonymized (the user might still be in
// other orgs, or even be active on this server independently). Anonymize
// only fires when the leaver themselves is the caller.
func TestLeaveOrg_AdminDoesNotAnonymize(t *testing.T) {
	app := setupTestApp(t)
	alice := makeUser(t, app, "alice@test.local")
	bob := makeUser(t, app, "bob@test.local")
	org := makeOrg(t, app, "Acme")
	aliceUO := makeUserOrg(t, app, alice, org, "owner")
	bobUO := makeUserOrg(t, app, bob, org, "member")

	// Bob is being removed by alice (the admin). actorIsLeaver=false.
	result, err := LeaveOrg(app, bobUO.Id, Plan{
		Mode:               ModeReassign,
		SuccessorUserOrgID: aliceUO.Id,
	}, false)
	if err != nil {
		t.Fatalf("LeaveOrg: %v", err)
	}
	if result.UserAnonymized {
		t.Error("admin-driven removal must not anonymize the removed user")
	}

	// Bob's users record is untouched.
	bobUser, err := app.FindRecordById("users", bob.Id)
	if err != nil {
		t.Fatalf("re-find bob user: %v", err)
	}
	if bobUser.GetString("email") != "bob@test.local" {
		t.Errorf("bob email changed: got %q", bobUser.GetString("email"))
	}
}

// TestLeaveOrg_AutoPicksOldestOwner — when the client doesn't specify a
// successor, the server auto-picks the oldest owner (by user_org.created).
// This is the default for delete-account, where the client may not have
// per-org context to pick.
func TestLeaveOrg_AutoPicksOldestOwner(t *testing.T) {
	app := setupTestApp(t)
	alice := makeUser(t, app, "alice@test.local")
	bob := makeUser(t, app, "bob@test.local")
	carol := makeUser(t, app, "carol@test.local")
	org := makeOrg(t, app, "Acme")

	bobUO := makeUserOrg(t, app, bob, org, "owner")    // oldest owner
	_ = makeUserOrg(t, app, carol, org, "owner")        // newer owner
	aliceUO := makeUserOrg(t, app, alice, org, "owner") // even newer; she's leaving

	event := makeEvent(t, app, "Pick me", aliceUO.Id, org.Id)

	_, err := LeaveOrg(app, aliceUO.Id, Plan{Mode: ModeReassign}, true)
	if err != nil {
		t.Fatalf("LeaveOrg: %v", err)
	}

	updated, _ := app.FindRecordById("test_events", event.Id)
	if updated.GetString("created_by") != bobUO.Id {
		t.Errorf("auto-pick: created_by=%s, want oldest owner %s", updated.GetString("created_by"), bobUO.Id)
	}
}

// TestLeaveOrg_AutoPickPromotesNonOwnerWhenNoOwnersLeft — exotic case: org
// has no other owners (just members). Auto-pick falls back to oldest member,
// who gets promoted to owner. Verifies the auto-pick + promote handshake
// works end-to-end.
func TestLeaveOrg_AutoPickPromotesNonOwnerWhenNoOwnersLeft(t *testing.T) {
	app := setupTestApp(t)
	alice := makeUser(t, app, "alice@test.local")
	bob := makeUser(t, app, "bob@test.local")
	org := makeOrg(t, app, "Acme")

	bobUO := makeUserOrg(t, app, bob, org, "member")    // sole peer, not owner
	aliceUO := makeUserOrg(t, app, alice, org, "owner") // leaving

	_, err := LeaveOrg(app, aliceUO.Id, Plan{Mode: ModeReassign}, true)
	if err != nil {
		t.Fatalf("LeaveOrg: %v", err)
	}

	bobAfter, _ := app.FindRecordById("user_org", bobUO.Id)
	if bobAfter.GetString("role") != "owner" {
		t.Errorf("bob role: got %q, want owner", bobAfter.GetString("role"))
	}
}

// TestLeaveOrg_AutoPickSkipsGuests — guests must never be silently promoted
// to owner. When the only remaining peer is a guest and the leaver is sole
// owner, auto-pick must refuse (caller must either pick the guest explicitly
// or accept the rejection).
func TestLeaveOrg_AutoPickSkipsGuests(t *testing.T) {
	app := setupTestApp(t)
	alice := makeUser(t, app, "alice@test.local")
	bob := makeUser(t, app, "bob@test.local")
	org := makeOrg(t, app, "Acme")
	_ = makeUserOrg(t, app, bob, org, "guest") // only peer; guest
	aliceUO := makeUserOrg(t, app, alice, org, "owner")

	_, err := LeaveOrg(app, aliceUO.Id, Plan{Mode: ModeReassign}, true)
	if err == nil {
		t.Fatal("expected ErrInvalidPlan when only peer is a guest")
	}
}

// TestLeaveOrg_DeleteMyDataPromotesSoleOwnerSuccessor — sole owner deletes
// her own data and leaves. The org needs someone promoted to owner so it
// isn't ownerless; pick the oldest non-guest peer.
func TestLeaveOrg_DeleteMyDataPromotesSoleOwnerSuccessor(t *testing.T) {
	app := setupTestApp(t)
	alice := makeUser(t, app, "alice@test.local")
	bob := makeUser(t, app, "bob@test.local")
	org := makeOrg(t, app, "Acme")
	bobUO := makeUserOrg(t, app, bob, org, "member")
	aliceUO := makeUserOrg(t, app, alice, org, "owner")

	_, err := LeaveOrg(app, aliceUO.Id, Plan{Mode: ModeDeleteMyData}, true)
	if err != nil {
		t.Fatalf("LeaveOrg: %v", err)
	}
	bobAfter, _ := app.FindRecordById("user_org", bobUO.Id)
	if bobAfter.GetString("role") != "owner" {
		t.Errorf("delete_my_data on sole-owner leave should promote peer; got %q", bobAfter.GetString("role"))
	}
}

// TestLeaveOrg_EmptyRegistry — installs that haven't registered any
// reassignable refs (lean-shell, fresh setup) should succeed with zero
// records reassigned and no failures.
func TestLeaveOrg_EmptyRegistry(t *testing.T) {
	app := setupTestApp(t)
	ResetReassignableForTesting() // clear what setupTestApp registered

	alice := makeUser(t, app, "alice@test.local")
	bob := makeUser(t, app, "bob@test.local")
	org := makeOrg(t, app, "Acme")
	aliceUO := makeUserOrg(t, app, alice, org, "owner")
	bobUO := makeUserOrg(t, app, bob, org, "owner")

	result, err := LeaveOrg(app, aliceUO.Id, Plan{
		Mode:               ModeReassign,
		SuccessorUserOrgID: bobUO.Id,
	}, true)
	if err != nil {
		t.Fatalf("LeaveOrg: %v", err)
	}
	if result.RecordsReassigned != 0 {
		t.Errorf("empty registry: records_reassigned = %d, want 0", result.RecordsReassigned)
	}
	if _, err := app.FindRecordById("user_org", aliceUO.Id); err == nil {
		t.Error("expected aliceUO to be deleted")
	}
}
