package blankfile

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/filesystem"
)

const testMime = "application/test-blank"

// a couple of non-empty, arbitrary skeleton bytes — content is irrelevant to
// the hook, only that it's attached.
var testSkeleton = []byte("SKELETON-BYTES-1234")

// newAppWithDriveItems builds a test app with a drive_items collection carrying
// the fields the hook reads/writes, and registers the blankfile hook for
// testMime.
func newAppWithDriveItems(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("tests.NewTestApp: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	items := core.NewBaseCollection(driveItemsCollection)
	items.Fields.Add(&core.TextField{Name: "name"})
	items.Fields.Add(&core.TextField{Name: "org"})
	items.Fields.Add(&core.TextField{Name: "mime_type"})
	items.Fields.Add(&core.NumberField{Name: "size"})
	items.Fields.Add(&core.FileField{Name: "file", MaxSelect: 1, MaxSize: 104857600})
	if err := app.Save(items); err != nil {
		t.Fatalf("save drive_items collection: %v", err)
	}

	// Register takes core.App (the OnRecordCreate hook surface), which
	// *tests.TestApp implements — so pass it directly.
	Register(app, testMime, "blank.bin", testSkeleton)
	return app
}

func newDriveItem(t *testing.T, app *tests.TestApp) *core.Record {
	t.Helper()
	c, err := app.FindCollectionByNameOrId(driveItemsCollection)
	if err != nil {
		t.Fatalf("find drive_items: %v", err)
	}
	return core.NewRecord(c)
}

// A blank create (matching mime, no file) gets the skeleton + size attached.
func TestAttachesSkeletonToBlankCreate(t *testing.T) {
	app := newAppWithDriveItems(t)
	rec := newDriveItem(t, app)
	rec.Set("name", "Untitled.bin")
	rec.Set("mime_type", testMime)
	rec.Set("size", 0)

	if err := app.Save(rec); err != nil {
		t.Fatalf("save blank create: %v", err)
	}

	reloaded, err := app.FindRecordById(driveItemsCollection, rec.Id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.GetString("file") == "" {
		t.Fatal("expected file to be attached, got empty")
	}
	if got := reloaded.GetInt("size"); got != len(testSkeleton) {
		t.Fatalf("expected size %d, got %d", len(testSkeleton), got)
	}
}

// A create that already carries a file (a real upload) is left untouched — the
// hook must never overwrite user content.
func TestLeavesExistingFileUntouched(t *testing.T) {
	app := newAppWithDriveItems(t)
	rec := newDriveItem(t, app)
	rec.Set("name", "real-upload.bin")
	rec.Set("mime_type", testMime)

	userBytes := []byte("USER-UPLOADED-CONTENT-that-differs-from-skeleton")
	f, err := filesystem.NewFileFromBytes(userBytes, "real.bin")
	if err != nil {
		t.Fatalf("build user file: %v", err)
	}
	rec.Set("file", f)
	rec.Set("size", len(userBytes))

	if err := app.Save(rec); err != nil {
		t.Fatalf("save real upload: %v", err)
	}

	reloaded, err := app.FindRecordById(driveItemsCollection, rec.Id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	// Size must reflect the user's upload, not the skeleton — proving the hook
	// no-op'd on an already-filed record.
	if got := reloaded.GetInt("size"); got != len(userBytes) {
		t.Fatalf("expected user size %d preserved, got %d (skeleton is %d)",
			len(userBytes), got, len(testSkeleton))
	}
}

// A create with a different mime is ignored entirely (no file attached).
func TestIgnoresOtherMime(t *testing.T) {
	app := newAppWithDriveItems(t)
	rec := newDriveItem(t, app)
	rec.Set("name", "photo.png")
	rec.Set("mime_type", "image/png")
	rec.Set("size", 0)

	if err := app.Save(rec); err != nil {
		t.Fatalf("save other-mime create: %v", err)
	}

	reloaded, err := app.FindRecordById(driveItemsCollection, rec.Id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.GetString("file") != "" {
		t.Fatal("expected no file attached for non-owned mime, got one")
	}
}
