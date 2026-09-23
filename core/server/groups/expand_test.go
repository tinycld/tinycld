package groups

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// newZooApp builds the two core collections plus a fictional package table,
// zoo_keepers (zoo text, user, group, role), and binds the hooks. It does NOT
// run migrations: the rules are covered by rules_test.go, and hooks fire on
// app.Save regardless of rules.
func newZooApp(t *testing.T) *tests.TestApp {
	t.Helper()
	ResetForTesting()
	a, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(a.Cleanup)

	users, err := a.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}

	groups := core.NewBaseCollection("groups")
	groups.Id = "pbc_groups_01"
	groups.Fields.Add(&core.TextField{Name: "name", Required: true})
	if err := a.Save(groups); err != nil {
		t.Fatalf("save groups: %v", err)
	}

	members := core.NewBaseCollection("group_members")
	members.Id = "pbc_group_members_01"
	members.Fields.Add(&core.RelationField{Name: "group", Required: true, CollectionId: groups.Id, CascadeDelete: true, MaxSelect: 1})
	members.Fields.Add(&core.RelationField{Name: "user", Required: true, CollectionId: users.Id, CascadeDelete: true, MaxSelect: 1})
	members.AddIndex("idx_gm_unique", true, "`group`, `user`", "")
	if err := a.Save(members); err != nil {
		t.Fatalf("save group_members: %v", err)
	}

	keepers := core.NewBaseCollection("zoo_keepers")
	keepers.Fields.Add(&core.TextField{Name: "zoo", Required: true})
	keepers.Fields.Add(&core.RelationField{Name: "user", CollectionId: users.Id, CascadeDelete: true, MaxSelect: 1})
	keepers.Fields.Add(&core.RelationField{Name: "group", CollectionId: groups.Id, CascadeDelete: true, MaxSelect: 1})
	keepers.Fields.Add(&core.SelectField{Name: "role", Required: true, MaxSelect: 1, Values: []string{"owner", "editor", "viewer"}})
	keepers.AddIndex("idx_zk_unique", true, "`zoo`, `user`, `group`", "")
	if err := a.Save(keepers); err != nil {
		t.Fatalf("save zoo_keepers: %v", err)
	}

	RegisterGrantTable(GrantTable{Collection: "zoo_keepers", ResourceField: "zoo"})
	registerCore(a)
	return a
}

func zooUser(t *testing.T, app core.App, email string) *core.Record {
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

func zooGroup(t *testing.T, app core.App, name string) *core.Record {
	t.Helper()
	col, _ := app.FindCollectionByNameOrId("groups")
	g := core.NewRecord(col)
	g.Set("name", name)
	if err := app.Save(g); err != nil {
		t.Fatalf("save group %s: %v", name, err)
	}
	return g
}

func zooMember(t *testing.T, app core.App, group, user *core.Record) *core.Record {
	t.Helper()
	col, _ := app.FindCollectionByNameOrId("group_members")
	m := core.NewRecord(col)
	m.Set("group", group.Id)
	m.Set("user", user.Id)
	if err := app.Save(m); err != nil {
		t.Fatalf("save membership: %v", err)
	}
	return m
}

func zooGrant(t *testing.T, app core.App, zoo string, group *core.Record, role string) *core.Record {
	t.Helper()
	col, _ := app.FindCollectionByNameOrId("zoo_keepers")
	r := core.NewRecord(col)
	r.Set("zoo", zoo)
	r.Set("group", group.Id)
	r.Set("role", role)
	if err := app.Save(r); err != nil {
		t.Fatalf("save grant: %v", err)
	}
	return r
}

// derivedRows lists derived rows (user AND group set) for one zoo, keyed
// "userId:role".
func derivedRows(t *testing.T, app core.App, zoo string) map[string]string {
	t.Helper()
	rows, err := app.FindRecordsByFilter("zoo_keepers", `zoo = {:zoo} && user != "" && group != ""`, "", 0, 0, map[string]any{"zoo": zoo})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, r := range rows {
		out[r.GetString("user")] = r.GetString("role")
	}
	return out
}

func TestGrantCreateExpandsToCurrentMembers(t *testing.T) {
	app := newZooApp(t)
	alice, bob := zooUser(t, app, "alice@x.test"), zooUser(t, app, "bob@x.test")
	g := zooGroup(t, app, "keepers")
	zooMember(t, app, g, alice)
	zooMember(t, app, g, bob)

	zooGrant(t, app, "bronx", g, "editor")

	got := derivedRows(t, app, "bronx")
	if got[alice.Id] != "editor" || got[bob.Id] != "editor" || len(got) != 2 {
		t.Fatalf("derived = %v", got)
	}
}

func TestMemberJoinAndLeaveFollowExistingGrants(t *testing.T) {
	app := newZooApp(t)
	alice := zooUser(t, app, "alice@x.test")
	g := zooGroup(t, app, "keepers")
	zooGrant(t, app, "bronx", g, "viewer")
	zooGrant(t, app, "sd", g, "viewer")

	m := zooMember(t, app, g, alice)
	if derivedRows(t, app, "bronx")[alice.Id] != "viewer" || derivedRows(t, app, "sd")[alice.Id] != "viewer" {
		t.Fatal("join did not derive rows for both grants")
	}

	if err := app.Delete(m); err != nil {
		t.Fatal(err)
	}
	if len(derivedRows(t, app, "bronx")) != 0 || len(derivedRows(t, app, "sd")) != 0 {
		t.Fatal("leave did not remove derived rows")
	}
}

func TestGrantUpdateCopiesRoleAndDeleteRemovesDerived(t *testing.T) {
	app := newZooApp(t)
	alice := zooUser(t, app, "alice@x.test")
	g := zooGroup(t, app, "keepers")
	zooMember(t, app, g, alice)
	grant := zooGrant(t, app, "bronx", g, "viewer")

	grant.Set("role", "editor")
	if err := app.Save(grant); err != nil {
		t.Fatal(err)
	}
	if derivedRows(t, app, "bronx")[alice.Id] != "editor" {
		t.Fatal("role change did not reach the derived row")
	}

	if err := app.Delete(grant); err != nil {
		t.Fatal(err)
	}
	if len(derivedRows(t, app, "bronx")) != 0 {
		t.Fatal("grant delete left derived rows behind")
	}
}

func TestUserInTwoGroupsGrantedOnOneResourceKeepsBothRows(t *testing.T) {
	app := newZooApp(t)
	alice := zooUser(t, app, "alice@x.test")
	g1, g2 := zooGroup(t, app, "keepers"), zooGroup(t, app, "vets")
	zooMember(t, app, g1, alice)
	zooMember(t, app, g2, alice)
	zooGrant(t, app, "bronx", g1, "viewer")
	zooGrant(t, app, "bronx", g2, "editor")

	rows, err := app.FindRecordsByFilter("zoo_keepers", `zoo = "bronx" && user = {:u}`, "", 0, 0, map[string]any{"u": alice.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("want one derived row per group, got %d", len(rows))
	}

	// Leaving one group removes only that group's row.
	m, err := app.FindFirstRecordByFilter("group_members", "group = {:g} && user = {:u}", map[string]any{"g": g1.Id, "u": alice.Id})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Delete(m); err != nil {
		t.Fatal(err)
	}
	if got := derivedRows(t, app, "bronx"); got[alice.Id] != "editor" {
		t.Fatalf("after leaving keepers, alice should keep the vets row: %v", got)
	}
}

func TestGroupDeleteRemovesGrantsAndDerivedRows(t *testing.T) {
	app := newZooApp(t)
	alice := zooUser(t, app, "alice@x.test")
	g := zooGroup(t, app, "keepers")
	zooMember(t, app, g, alice)
	zooGrant(t, app, "bronx", g, "viewer")

	if err := app.Delete(g); err != nil {
		t.Fatal(err)
	}
	n, err := app.CountRecords("zoo_keepers")
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("want zoo_keepers empty after group delete, got %d rows", n)
	}
}

func TestDirectRowsAreLeftAlone(t *testing.T) {
	app := newZooApp(t)
	alice := zooUser(t, app, "alice@x.test")
	col, _ := app.FindCollectionByNameOrId("zoo_keepers")
	direct := core.NewRecord(col)
	direct.Set("zoo", "bronx")
	direct.Set("user", alice.Id)
	direct.Set("role", "owner")
	if err := app.Save(direct); err != nil {
		t.Fatal(err)
	}
	direct.Set("role", "editor")
	if err := app.Save(direct); err != nil {
		t.Fatal(err)
	}
	if err := app.Delete(direct); err != nil {
		t.Fatal(err)
	}
	n, _ := app.CountRecords("zoo_keepers")
	if n != 0 {
		t.Fatalf("direct row lifecycle should not create rows, got %d", n)
	}
}

func TestMembershipListenerFiresAfterCommit(t *testing.T) {
	app := newZooApp(t)
	var events []MembershipEvent
	OnMembershipChange(func(e MembershipEvent) error {
		events = append(events, e)
		return nil
	})
	alice := zooUser(t, app, "alice@x.test")
	g := zooGroup(t, app, "keepers")
	m := zooMember(t, app, g, alice)
	if err := app.Delete(m); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || !events[0].Joined || events[1].Joined || events[0].UserID != alice.Id || events[1].GroupID != g.Id {
		t.Fatalf("events = %+v", events)
	}
}

func TestRemoveUserMemberships(t *testing.T) {
	app := newZooApp(t)
	alice := zooUser(t, app, "alice@x.test")
	g1, g2 := zooGroup(t, app, "keepers"), zooGroup(t, app, "vets")
	zooMember(t, app, g1, alice)
	zooMember(t, app, g2, alice)
	zooGrant(t, app, "bronx", g1, "viewer")

	if err := RemoveUserMemberships(app, alice.Id); err != nil {
		t.Fatal(err)
	}
	n, _ := app.CountRecords("group_members")
	if n != 0 {
		t.Fatalf("memberships left: %d", n)
	}
	if len(derivedRows(t, app, "bronx")) != 0 {
		t.Fatal("derived rows left after membership removal")
	}
}
