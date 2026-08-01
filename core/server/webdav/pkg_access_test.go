package webdav

import (
	"errors"
	"os"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// pkg_access_test.go pins that a readonly org_pkg_access grant binds WebDAV
// writes too. DAV bypasses the REST layer (where the request-hook guard
// lives), so without its own check a readonly user's Finder mount could
// still upload, rename, and delete — the level would be enforced everywhere
// except the one protocol built for writing files.

// setupReadonlyTree gives alice a readonly override for the source's owning
// package ("testdrive" — the test Source's slug).
func setupReadonlyTree(t *testing.T) (*tests.TestApp, *core.Record, *core.Record) {
	t.Helper()
	app, alice, bob := setupTree(t)

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	access := core.NewBaseCollection("org_pkg_access")
	access.Fields.Add(&core.RelationField{
		Name: "user", Required: true, CollectionId: users.Id, MaxSelect: 1,
	})
	access.Fields.Add(&core.TextField{Name: "pkg", Required: true})
	access.Fields.Add(&core.SelectField{
		Name: "access", Required: true, Values: []string{"full", "readonly", "none"}, MaxSelect: 1,
	})
	if err := app.Save(access); err != nil {
		t.Fatal(err)
	}

	row := core.NewRecord(access)
	row.Set("user", alice.Id)
	row.Set("pkg", "testdrive")
	row.Set("access", "readonly")
	if err := app.Save(row); err != nil {
		t.Fatal(err)
	}

	return app, alice, bob
}

func TestWebDAV_ReadonlyGrantBlocksWrites(t *testing.T) {
	app, alice, _ := setupReadonlyTree(t)
	allowAuthenticated(t, app)
	mkFile(t, app, alice, "readable.txt", "", "still readable")

	fs := newFS(t, app, testSource())

	if _, err := fs.OpenFile(ctxAs(alice), "/files/new.txt", os.O_WRONLY|os.O_CREATE, 0); err == nil {
		t.Fatal("readonly user opened a file for writing")
	}
	if err := fs.Mkdir(ctxAs(alice), "/files/newdir", 0); err == nil {
		t.Fatal("readonly user created a directory")
	}
	if err := fs.RemoveAll(ctxAs(alice), "/files/readable.txt"); err == nil {
		t.Fatal("readonly user deleted an entry")
	}
	if err := fs.Rename(ctxAs(alice), "/files/readable.txt", "/files/renamed.txt"); err == nil {
		t.Fatal("readonly user renamed an entry")
	}

	// Reading stays available — readonly means read.
	if _, err := fs.Stat(ctxAs(alice), "/files/readable.txt"); err != nil {
		t.Fatalf("readonly user should still stat: %v", err)
	}
	f, err := fs.OpenFile(ctxAs(alice), "/files/readable.txt", os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("readonly user should still read: %v", err)
	}
	_ = f.Close()
}

// The refusal must be permission-shaped, not not-found: the user can SEE the
// tree, they just may not change it.
func TestWebDAV_ReadonlyRefusalIsPermission(t *testing.T) {
	app, alice, _ := setupReadonlyTree(t)
	allowAuthenticated(t, app)

	err := fsErr(newFS(t, app, testSource()).Mkdir(ctxAs(alice), "/files/newdir", 0))
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("refusal = %v, want os.ErrPermission", err)
	}
}

func fsErr(err error) error { return err }

// Another user with no override keeps full access on the same tree.
func TestWebDAV_ReadonlyGrantIsPerUser(t *testing.T) {
	app, _, bob := setupReadonlyTree(t)
	allowAuthenticated(t, app)

	if err := newFS(t, app, testSource()).Mkdir(ctxAs(bob), "/files/bobs-dir", 0); err != nil {
		t.Fatalf("unrestricted user's write refused: %v", err)
	}
}
