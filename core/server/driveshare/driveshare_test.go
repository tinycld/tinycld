package driveshare

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// setupApp builds the minimal collection graph driveshare touches:
// drive_items (with created_by → users) + drive_shares (item, user, role).
// users ships with NewTestApp().
func setupApp(t *testing.T) *tests.TestApp {
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
	users.Fields.Add(&core.BoolField{Name: "disabled"})
	if err := app.Save(users); err != nil {
		t.Fatalf("add users.disabled: %v", err)
	}

	driveItems := core.NewBaseCollection(driveItemsCollection)
	driveItems.Fields.Add(&core.TextField{Name: "name", Required: true})
	driveItems.Fields.Add(&core.RelationField{
		Name: "created_by", CollectionId: users.Id, MaxSelect: 1,
	})
	if err := app.Save(driveItems); err != nil {
		t.Fatalf("save drive_items: %v", err)
	}

	shares := core.NewBaseCollection(sharesCollection)
	shares.Fields.Add(&core.RelationField{
		Name: "item", Required: true, CollectionId: driveItems.Id, MaxSelect: 1,
	})
	shares.Fields.Add(&core.RelationField{
		Name: "user", Required: true, CollectionId: users.Id, MaxSelect: 1,
	})
	shares.Fields.Add(&core.TextField{Name: "role", Required: true})
	if err := app.Save(shares); err != nil {
		t.Fatalf("save drive_shares: %v", err)
	}
	return app
}

func mkUser(t *testing.T, app *tests.TestApp, email string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.SetEmail(email)
	r.Set("name", strings.Split(email, "@")[0])
	r.SetVerified(true)
	r.SetPassword("Password123!")
	if err := app.Save(r); err != nil {
		t.Fatalf("save user %s: %v", email, err)
	}
	return r
}

// mkItem creates a drive_item. Pass a nil creator for an item with no
// created_by, which isolates the share-row path from the creator branch.
func mkItem(t *testing.T, app *tests.TestApp, creator *core.Record, name string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(driveItemsCollection)
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.Set("name", name)
	if creator != nil {
		r.Set("created_by", creator.Id)
	}
	if err := app.Save(r); err != nil {
		t.Fatalf("save item %s: %v", name, err)
	}
	return r
}

func grant(t *testing.T, app *tests.TestApp, item, user *core.Record, role Role) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(sharesCollection)
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.Set("item", item.Id)
	r.Set("user", user.Id)
	r.Set("role", string(role))
	if err := app.Save(r); err != nil {
		t.Fatalf("save share %s: %v", role, err)
	}
}

// TestResolveRole_Creator pins the `created_by ?= @request.auth.id`
// disjunct that every drive_items rule carries. The creator gets owner
// with no drive_shares row at all — drive's owner-share hook can be
// bypassed by a direct SDK write, so a creator-without-share item is a
// state the system actually reaches.
func TestResolveRole_Creator(t *testing.T) {
	app := setupApp(t)
	creator := mkUser(t, app, "creator@test.local")
	item := mkItem(t, app, creator, "doc")

	role, err := ResolveRole(app, creator.Id, item.Id)
	if err != nil {
		t.Fatalf("creator: got %v, want nil", err)
	}
	if role != RoleOwner {
		t.Errorf("creator role: got %q, want %q", role, RoleOwner)
	}
}

// TestResolveRole_PicksHighestPrivilege defends against duplicate share
// rows for one (item, user) — e.g. an editor row added on promotion
// without cleaning up the original viewer row. The UNIQUE (item, user)
// index blocks this today; the rows here predate it.
func TestResolveRole_PicksHighestPrivilege(t *testing.T) {
	app := setupApp(t)
	owner := mkUser(t, app, "owner@test.local")
	grantee := mkUser(t, app, "grantee@test.local")
	item := mkItem(t, app, owner, "doc")

	grant(t, app, item, grantee, RoleViewer)
	grant(t, app, item, grantee, RoleEditor)

	role, err := ResolveRole(app, grantee.Id, item.Id)
	if err != nil {
		t.Fatalf("grantee: got %v, want nil", err)
	}
	if role != RoleEditor {
		t.Errorf("role: got %q, want %q", role, RoleEditor)
	}
}

// TestResolveRole_UnknownRoleFailsClosed covers a share row carrying a
// role this build has never heard of. Admitting it would let a future
// migration silently widen access to old binaries.
func TestResolveRole_UnknownRoleFailsClosed(t *testing.T) {
	app := setupApp(t)
	owner := mkUser(t, app, "owner@test.local")
	grantee := mkUser(t, app, "grantee@test.local")
	item := mkItem(t, app, owner, "doc")
	grant(t, app, item, grantee, Role("archivist"))

	role, err := ResolveRole(app, grantee.Id, item.Id)
	if !errors.Is(err, ErrNoAccess) {
		t.Errorf("err: got %v, want ErrNoAccess", err)
	}
	if role != RoleNone {
		t.Errorf("role: got %q, want %q", role, RoleNone)
	}
}

func TestResolveRole_NoShare(t *testing.T) {
	app := setupApp(t)
	owner := mkUser(t, app, "owner@test.local")
	stranger := mkUser(t, app, "stranger@test.local")
	item := mkItem(t, app, owner, "doc")

	if _, err := ResolveRole(app, stranger.Id, item.Id); !errors.Is(err, ErrNoAccess) {
		t.Errorf("stranger: got %v, want ErrNoAccess", err)
	}
}

// TestResolveRole_NonExistentItem asserts a missing item denies rather
// than surfacing a raw DB error — callers branch on ErrNoAccess, and a
// distinguishable error would confirm the item's absence to a prober.
func TestResolveRole_NonExistentItem(t *testing.T) {
	app := setupApp(t)
	user := mkUser(t, app, "user@test.local")

	if _, err := ResolveRole(app, user.Id, "nonexistent000000"); !errors.Is(err, ErrNoAccess) {
		t.Errorf("missing item: got %v, want ErrNoAccess", err)
	}
}

// TestResolveRole_EmptyIDs covers the unauthenticated caller. Each of the
// three call sites open-codes this guard today; core owning it is the
// point of the package.
func TestResolveRole_EmptyIDs(t *testing.T) {
	app := setupApp(t)
	owner := mkUser(t, app, "owner@test.local")
	item := mkItem(t, app, owner, "doc")

	if _, err := ResolveRole(app, "", item.Id); !errors.Is(err, ErrNoAccess) {
		t.Errorf("empty userID: got %v, want ErrNoAccess", err)
	}
	if _, err := ResolveRole(app, owner.Id, ""); !errors.Is(err, ErrNoAccess) {
		t.Errorf("empty itemID: got %v, want ErrNoAccess", err)
	}
	if _, err := ResolveRoleForItem(app, owner.Id, nil); !errors.Is(err, ErrNoAccess) {
		t.Errorf("nil item: got %v, want ErrNoAccess", err)
	}
}

// TestCheckMatrix pins the whole role ladder in one table. This is the
// authorization contract; a change here is a change to what every drive,
// text and calc access path permits.
func TestCheckMatrix(t *testing.T) {
	app := setupApp(t)
	creator := mkUser(t, app, "creator@test.local")
	ownerShare := mkUser(t, app, "ownershare@test.local")
	editor := mkUser(t, app, "editor@test.local")
	viewer := mkUser(t, app, "viewer@test.local")
	stranger := mkUser(t, app, "stranger@test.local")

	item := mkItem(t, app, creator, "doc")
	grant(t, app, item, ownerShare, RoleOwner)
	grant(t, app, item, editor, RoleEditor)
	grant(t, app, item, viewer, RoleViewer)

	cases := []struct {
		name                   string
		userID                 string
		read, write, canDelete bool
	}{
		{"creator (no share row)", creator.Id, true, true, true},
		{"owner share", ownerShare.Id, true, true, true},
		{"editor share", editor.Id, true, true, false},
		{"viewer share", viewer.Id, true, false, false},
		{"no share", stranger.Id, false, false, false},
		{"empty user id", "", false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CheckRead(app, tc.userID, item.Id) == nil; got != tc.read {
				t.Errorf("read: got %v, want %v", got, tc.read)
			}
			if got := CheckWrite(app, tc.userID, item.Id) == nil; got != tc.write {
				t.Errorf("write: got %v, want %v", got, tc.write)
			}
			if got := CanWrite(app, tc.userID, item.Id); got != tc.write {
				t.Errorf("CanWrite: got %v, want %v", got, tc.write)
			}
			if got := CheckDelete(app, tc.userID, item.Id) == nil; got != tc.canDelete {
				t.Errorf("delete: got %v, want %v", got, tc.canDelete)
			}
			if got := IsOwner(app, tc.userID, item.Id); got != tc.canDelete {
				t.Errorf("IsOwner: got %v, want %v", got, tc.canDelete)
			}
		})
	}
}

// TestCheckReadItem_MatchesCheckRead guards the pre-loaded-record entry
// point from drifting away from the id-based one.
func TestCheckReadItem_MatchesCheckRead(t *testing.T) {
	app := setupApp(t)
	creator := mkUser(t, app, "creator@test.local")
	grantee := mkUser(t, app, "grantee@test.local")
	stranger := mkUser(t, app, "stranger@test.local")
	item := mkItem(t, app, creator, "doc")
	grant(t, app, item, grantee, RoleViewer)

	for _, userID := range []string{creator.Id, grantee.Id, stranger.Id} {
		byID := CheckRead(app, userID, item.Id) == nil
		byItem := CheckReadItem(app, userID, item) == nil
		if byID != byItem {
			t.Errorf("user %s: CheckRead=%v but CheckReadItem=%v", userID, byID, byItem)
		}
	}
}

func TestRoleCanWrite(t *testing.T) {
	cases := []struct {
		role Role
		want bool
	}{
		{RoleOwner, true},
		{RoleEditor, true},
		{RoleViewer, false},
		{RoleNone, false},
		{Role("archivist"), false},
	}
	for _, tc := range cases {
		if got := tc.role.CanWrite(); got != tc.want {
			t.Errorf("Role(%q).CanWrite(): got %v, want %v", tc.role, got, tc.want)
		}
	}
}

// TestErrNoAccessIsPermission pins the wrapping drive's WebDAV layer
// depends on: it branches on os.ErrPermission to map a denial onto a 403.
func TestErrNoAccessIsPermission(t *testing.T) {
	if !errors.Is(ErrNoAccess, os.ErrPermission) {
		t.Error("ErrNoAccess must wrap os.ErrPermission")
	}
}

// setDisabled flips the suspension flag on an existing user.
func setDisabled(t *testing.T, app *tests.TestApp, user *core.Record) {
	t.Helper()
	user.Set("disabled", true)
	if err := app.Save(user); err != nil {
		t.Fatalf("set disabled: %v", err)
	}
}

// TestResolveRole_DisabledUserDenied pins the suspension gate. Both subjects
// would otherwise pass: the creator via the created_by branch, the sharee via
// a live owner-role row. Suspension has to outrank both, or disabling an
// account would leave its own files (and everything shared with it) reachable
// through WebDAV, the render endpoints and realtime.
func TestResolveRole_DisabledUserDenied(t *testing.T) {
	app := setupApp(t)
	creator := mkUser(t, app, "creator@test.local")
	sharee := mkUser(t, app, "sharee@test.local")
	item := mkItem(t, app, creator, "doc")
	grant(t, app, item, sharee, RoleOwner)

	// Sanity: both reach the item while enabled, so a later denial is the
	// flag doing the work rather than a broken fixture.
	if _, err := ResolveRole(app, creator.Id, item.Id); err != nil {
		t.Fatalf("creator before disable: %v", err)
	}
	if _, err := ResolveRole(app, sharee.Id, item.Id); err != nil {
		t.Fatalf("sharee before disable: %v", err)
	}

	setDisabled(t, app, creator)
	setDisabled(t, app, sharee)

	if _, err := ResolveRole(app, creator.Id, item.Id); !errors.Is(err, ErrNoAccess) {
		t.Errorf("disabled creator: got %v, want ErrNoAccess", err)
	}
	if _, err := ResolveRole(app, sharee.Id, item.Id); !errors.Is(err, ErrNoAccess) {
		t.Errorf("disabled sharee: got %v, want ErrNoAccess", err)
	}
	if CanWrite(app, sharee.Id, item.Id) {
		t.Error("disabled owner-share still reports write access")
	}
	if err := CheckDelete(app, creator.Id, item.Id); !errors.Is(err, ErrNoAccess) {
		t.Errorf("disabled creator delete: got %v, want ErrNoAccess", err)
	}
}

// TestResolveRole_UnknownUserFailsClosed covers the isDisabled lookup error
// path: an id that resolves to no users row must deny rather than be treated
// as "not disabled, therefore fine".
func TestResolveRole_UnknownUserFailsClosed(t *testing.T) {
	app := setupApp(t)
	creator := mkUser(t, app, "creator@test.local")
	item := mkItem(t, app, creator, "doc")

	if _, err := ResolveRole(app, "ghost00000000000", item.Id); !errors.Is(err, ErrNoAccess) {
		t.Errorf("unknown user: got %v, want ErrNoAccess", err)
	}
}
