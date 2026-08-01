package webdav

import (
	"errors"
	"os"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// trash_test.go pins DELETE-goes-to-trash. Drive's own UI never destroys on
// delete — it stamps a per-user state row (drive_item_state.trashed_at) and
// the trash screen restores from it. A DAV DELETE that calls app.Delete
// instead makes Finder's "Move to Trash" permanently destroy the record and
// its blob, with no restore path — the one client where users most expect
// trash semantics.

// trashSource is testSource plus the trash state collection binding, the
// shape drive ships.
func trashSource() Source {
	src := testSource()
	src.Trash = &TrashConfig{
		Collection:     "tree_item_state",
		ItemField:      "item",
		UserField:      "user",
		TrashedAtField: "trashed_at",
	}
	return src
}

// setupTrashTree is setupTree plus the per-user state collection.
func setupTrashTree(t *testing.T) (*tests.TestApp, *core.Record, *core.Record) {
	t.Helper()
	app, alice, bob := setupTree(t)

	items, err := app.FindCollectionByNameOrId("tree_items")
	if err != nil {
		t.Fatal(err)
	}
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}

	state := core.NewBaseCollection("tree_item_state")
	state.Fields.Add(&core.RelationField{
		Name: "item", Required: true, CollectionId: items.Id, MaxSelect: 1, CascadeDelete: true,
	})
	state.Fields.Add(&core.RelationField{
		Name: "user", Required: true, CollectionId: users.Id, MaxSelect: 1, CascadeDelete: true,
	})
	state.Fields.Add(&core.TextField{Name: "trashed_at"})
	if err := app.Save(state); err != nil {
		t.Fatal(err)
	}

	return app, alice, bob
}

func trashStateFor(t *testing.T, app *tests.TestApp, itemID, userID string) *core.Record {
	t.Helper()
	states, err := app.FindRecordsByFilter("tree_item_state",
		"item = {:item} && user = {:user}", "", 1, 0,
		map[string]any{"item": itemID, "user": userID})
	if err != nil || len(states) == 0 {
		return nil
	}
	return states[0]
}

func TestRemoveAll_TrashConfigured_SoftDeletes(t *testing.T) {
	app, alice, _ := setupTrashTree(t)
	allowAuthenticated(t, app)
	item := mkFile(t, app, alice, "keepsake.txt", "", "precious bytes")

	fs := newFS(t, app, trashSource())
	if err := fs.RemoveAll(ctxAs(alice), "/files/keepsake.txt"); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	// The record survives — DELETE moved it to trash, not oblivion.
	if _, err := app.FindRecordById("tree_items", item.Id); err != nil {
		t.Fatal("DELETE destroyed the record; it must move to trash instead")
	}
	state := trashStateFor(t, app, item.Id, alice.Id)
	if state == nil || state.GetString("trashed_at") == "" {
		t.Fatal("DELETE left no trashed_at stamp; the trash screen cannot restore it")
	}
}

// After a DELETE the entry must be gone from the client's view: absent from
// listings, Stat answering not-found, and a second DELETE finding nothing.
func TestTrashedEntry_InvisibleOverDAV(t *testing.T) {
	app, alice, _ := setupTrashTree(t)
	allowAuthenticated(t, app)
	item := mkFile(t, app, alice, "gone.txt", "", "bye")
	mkFile(t, app, alice, "stays.txt", "", "hello")

	fs := newFS(t, app, trashSource())
	if err := fs.RemoveAll(ctxAs(alice), "/files/gone.txt"); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	if _, err := fs.Stat(ctxAs(alice), "/files/gone.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat on trashed entry = %v, want ErrNotExist", err)
	}

	root, err := fs.OpenFile(ctxAs(alice), "/files", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	infos, err := root.Readdir(-1)
	if err != nil {
		t.Fatal(err)
	}
	for _, info := range infos {
		if info.Name() == "gone.txt" {
			t.Fatal("trashed entry still listed; DELETE appears to do nothing")
		}
	}

	if err := fs.RemoveAll(ctxAs(alice), "/files/gone.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("second DELETE = %v, want ErrNotExist", err)
	}

	// Restore path intact: clearing the stamp brings the entry back.
	state := trashStateFor(t, app, item.Id, alice.Id)
	state.Set("trashed_at", "")
	if err := app.Save(state); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(ctxAs(alice), "/files/gone.txt"); err != nil {
		t.Fatalf("restored entry should stat again, got %v", err)
	}
}

// Trash state is per-user (drive_item_state has a user column): one sharer
// trashing their view must not hide the item from everyone else.
func TestTrash_IsPerUser(t *testing.T) {
	app, alice, bob := setupTrashTree(t)
	allowAuthenticated(t, app)
	mkFile(t, app, alice, "shared.txt", "", "ours")

	fs := newFS(t, app, trashSource())
	if err := fs.RemoveAll(ctxAs(alice), "/files/shared.txt"); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	if _, err := fs.Stat(ctxAs(bob), "/files/shared.txt"); err != nil {
		t.Fatalf("bob should still see the file alice trashed, got %v", err)
	}
}

// Without a Trash binding the old semantics stand: DELETE destroys.
func TestRemoveAll_NoTrashConfig_HardDeletes(t *testing.T) {
	app, alice, _ := setupTrashTree(t)
	allowAuthenticated(t, app)
	item := mkFile(t, app, alice, "doomed.txt", "", "x")

	fs := newFS(t, app, testSource())
	if err := fs.RemoveAll(ctxAs(alice), "/files/doomed.txt"); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	if _, err := app.FindRecordById("tree_items", item.Id); err == nil {
		t.Fatal("hard delete expected without a Trash binding")
	}
}
