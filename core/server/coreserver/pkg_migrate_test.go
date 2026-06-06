package coreserver

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// Layer-1 tests exercise the named, per-package migration runner against a real
// in-memory PocketBase app. We register synthetic migrations directly into the
// global core.AppMigrations list (the same list the jsvm plugin registers .js
// migrations into) so the runner can resolve them by filename, then assert that
// apply/revert touch ONLY the named files' schema + _migrations rows and leave
// everything else untouched.

// withTestMigrations registers synthetic migrations into the process-global
// core.AppMigrations list so the named runner can resolve them by filename.
//
// core.AppMigrations exposes no public remove/clear, so these registrations
// persist for the rest of the test binary's run. That is safe here because:
//   - every test uses uniquely-prefixed filenames (9xxxxxxxxx_), so they never
//     collide with each other or with the bundled migrations, and
//   - no test invokes a global `migrate up`, so a leaked synthetic Up is never
//     executed against another test's app.
//
// The named runner only ever touches the exact files a test passes it, so a
// leaked registration is inert for every other test.
func withTestMigrations(t *testing.T, migs []*core.Migration) {
	t.Helper()
	for _, m := range migs {
		core.AppMigrations.Register(m.Up, m.Down, m.File)
	}
}

func newMigrateTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })
	return app
}

func collectionExists(t *testing.T, app core.App, name string) bool {
	t.Helper()
	_, err := app.FindCollectionByNameOrId(name)
	return err == nil
}

func TestApplyAndRevertNamedMigrations(t *testing.T) {
	app := newMigrateTestApp(t)

	const colName = "ml_widgets_apply"
	createFile := "9000000001_ml_create_widgets.js"
	addFieldFile := "9000000002_ml_add_color.js"

	withTestMigrations(t, []*core.Migration{
		{
			File: createFile,
			Up: func(txApp core.App) error {
				c := core.NewBaseCollection(colName)
				c.Fields.Add(&core.TextField{Name: "title"})
				return txApp.Save(c)
			},
			Down: func(txApp core.App) error {
				c, err := txApp.FindCollectionByNameOrId(colName)
				if err != nil {
					return nil
				}
				return txApp.Delete(c)
			},
		},
		{
			File: addFieldFile,
			Up: func(txApp core.App) error {
				c, err := txApp.FindCollectionByNameOrId(colName)
				if err != nil {
					return err
				}
				c.Fields.Add(&core.TextField{Name: "color"})
				return txApp.Save(c)
			},
			Down: func(txApp core.App) error {
				c, err := txApp.FindCollectionByNameOrId(colName)
				if err != nil {
					return err
				}
				f := c.Fields.GetByName("color")
				if f != nil {
					c.Fields.RemoveByName("color")
				}
				return txApp.Save(c)
			},
		},
	})

	files := []string{createFile, addFieldFile}

	// Apply both — collection created with both fields, both recorded.
	applied, err := applyNamedMigrations(app, files)
	if err != nil {
		t.Fatalf("applyNamedMigrations: %v", err)
	}
	if len(applied) != 2 {
		t.Fatalf("applied %d migrations, want 2", len(applied))
	}
	if !collectionExists(t, app, colName) {
		t.Fatalf("collection %s not created", colName)
	}
	c, _ := app.FindCollectionByNameOrId(colName)
	if c.Fields.GetByName("color") == nil {
		t.Fatalf("color field not added")
	}
	for _, f := range files {
		ok, _ := migrationApplied(app, f)
		if !ok {
			t.Fatalf("migration %s not recorded as applied", f)
		}
	}

	// Re-apply is a no-op (already applied, no ReapplyCondition).
	reapplied, err := applyNamedMigrations(app, files)
	if err != nil {
		t.Fatalf("re-applyNamedMigrations: %v", err)
	}
	if len(reapplied) != 0 {
		t.Fatalf("re-apply applied %d, want 0 (idempotent)", len(reapplied))
	}

	// Revert both — collection gone, both rows removed.
	reverted, err := revertNamedMigrations(app, files)
	if err != nil {
		t.Fatalf("revertNamedMigrations: %v", err)
	}
	if len(reverted) != 2 {
		t.Fatalf("reverted %d migrations, want 2", len(reverted))
	}
	if collectionExists(t, app, colName) {
		t.Fatalf("collection %s should have been dropped", colName)
	}
	for _, f := range files {
		ok, _ := migrationApplied(app, f)
		if ok {
			t.Fatalf("migration %s still recorded after revert", f)
		}
	}
}

// TestRevertOnlyNamedLeavesOthersUntouched is the core guarantee: reverting one
// package's migrations must not affect another package's schema or history,
// even though both are interleaved in the same _migrations table. This is what
// the count-based `migrate down N` cannot do.
func TestRevertOnlyNamedLeavesOthersUntouched(t *testing.T) {
	app := newMigrateTestApp(t)

	mailCol := "ml_mail_iso"
	driveCol := "ml_drive_iso"
	mailFile := "9100000001_mail_iso.js"
	driveFile := "9100000002_drive_iso.js" // newer timestamp — would be reverted first by `down N`

	withTestMigrations(t, []*core.Migration{
		{
			File: mailFile,
			Up:   func(txApp core.App) error { return txApp.Save(core.NewBaseCollection(mailCol)) },
			Down: func(txApp core.App) error {
				c, err := txApp.FindCollectionByNameOrId(mailCol)
				if err != nil {
					return nil
				}
				return txApp.Delete(c)
			},
		},
		{
			File: driveFile,
			Up:   func(txApp core.App) error { return txApp.Save(core.NewBaseCollection(driveCol)) },
			Down: func(txApp core.App) error {
				c, err := txApp.FindCollectionByNameOrId(driveCol)
				if err != nil {
					return nil
				}
				return txApp.Delete(c)
			},
		},
	})

	if _, err := applyNamedMigrations(app, []string{mailFile, driveFile}); err != nil {
		t.Fatalf("apply both: %v", err)
	}

	// Revert ONLY mail, even though drive's migration is newer (the case where a
	// count-based `down 1` would wrongly hit drive).
	if _, err := revertNamedMigrations(app, []string{mailFile}); err != nil {
		t.Fatalf("revert mail only: %v", err)
	}

	if collectionExists(t, app, mailCol) {
		t.Errorf("mail collection should be dropped")
	}
	if !collectionExists(t, app, driveCol) {
		t.Errorf("drive collection must survive a mail-only revert")
	}
	if ok, _ := migrationApplied(app, driveFile); !ok {
		t.Errorf("drive migration row must survive a mail-only revert")
	}
	if ok, _ := migrationApplied(app, mailFile); ok {
		t.Errorf("mail migration row should be gone")
	}
}

func TestDryRevertReportsDropsWithoutCommitting(t *testing.T) {
	app := newMigrateTestApp(t)

	keepCol := "ml_dry_keep" // a field gets dropped from this one
	dropCol := "ml_dry_drop" // this whole collection gets dropped
	keepFieldFile := "9200000001_dry_keep_create.js"
	addColorFile := "9200000002_dry_add_color.js"
	dropColFile := "9200000003_dry_drop_create.js"

	withTestMigrations(t, []*core.Migration{
		{
			File: keepFieldFile,
			Up: func(txApp core.App) error {
				c := core.NewBaseCollection(keepCol)
				c.Fields.Add(&core.TextField{Name: "title"})
				return txApp.Save(c)
			},
			Down: func(txApp core.App) error {
				c, err := txApp.FindCollectionByNameOrId(keepCol)
				if err != nil {
					return nil
				}
				return txApp.Delete(c)
			},
		},
		{
			File: addColorFile,
			Up: func(txApp core.App) error {
				c, err := txApp.FindCollectionByNameOrId(keepCol)
				if err != nil {
					return err
				}
				c.Fields.Add(&core.TextField{Name: "color"})
				return txApp.Save(c)
			},
			Down: func(txApp core.App) error {
				c, err := txApp.FindCollectionByNameOrId(keepCol)
				if err != nil {
					return err
				}
				c.Fields.RemoveByName("color")
				return txApp.Save(c)
			},
		},
		{
			File: dropColFile,
			Up:   func(txApp core.App) error { return txApp.Save(core.NewBaseCollection(dropCol)) },
			Down: func(txApp core.App) error {
				c, err := txApp.FindCollectionByNameOrId(dropCol)
				if err != nil {
					return nil
				}
				return txApp.Delete(c)
			},
		},
	})

	all := []string{keepFieldFile, addColorFile, dropColFile}
	if _, err := applyNamedMigrations(app, all); err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Dry-revert the color field + the whole dropCol (keep keepCol itself).
	report, err := dryRevertNamedMigrations(app, []string{addColorFile, dropColFile})
	if err != nil {
		t.Fatalf("dryRevertNamedMigrations: %v", err)
	}

	// Report must list the dropped collection and the dropped field.
	if !containsStr(report.DroppedCollections, dropCol) {
		t.Errorf("DroppedCollections = %v, want to contain %s", report.DroppedCollections, dropCol)
	}
	foundColor := false
	for _, df := range report.DroppedFields {
		if df.Collection == keepCol && df.Field == "color" {
			foundColor = true
		}
	}
	if !foundColor {
		t.Errorf("DroppedFields = %v, want {%s, color}", report.DroppedFields, keepCol)
	}

	// Crucially: nothing was committed. Both collections + color field still live.
	if !collectionExists(t, app, dropCol) {
		t.Errorf("dry run dropped %s for real — must roll back", dropCol)
	}
	c, err := app.FindCollectionByNameOrId(keepCol)
	if err != nil {
		t.Fatalf("keepCol gone after dry run: %v", err)
	}
	if c.Fields.GetByName("color") == nil {
		t.Errorf("dry run dropped color field for real — must roll back")
	}
	if ok, _ := migrationApplied(app, dropColFile); !ok {
		t.Errorf("dry run removed %s history row for real — must roll back", dropColFile)
	}
}

// TestMigrationAppliedSurfacesRealErrors is the H2 regression guard: a real DB
// error (here, querying a _migrations table that doesn't exist) must surface as
// an error, NOT be swallowed as "not applied" — swallowing it makes a revert
// silently skip an applied migration.
func TestMigrationAppliedSurfacesRealErrors(t *testing.T) {
	app := newMigrateTestApp(t)

	// Drop the _migrations table so the query fails for a reason OTHER than
	// "no rows". The fix must return an error here, not (false, nil).
	if _, err := app.DB().NewQuery("DROP TABLE IF EXISTS _migrations").Execute(); err != nil {
		t.Fatalf("drop _migrations: %v", err)
	}

	_, err := migrationApplied(app, "9999_nonexistent.js")
	if err == nil {
		t.Fatal("migrationApplied swallowed a real DB error as (false, nil) — a revert would silently skip applied migrations")
	}
}

// TestMigrationAppliedNoRowsIsNotAnError confirms the normal "not applied" path
// (table present, row absent) still returns (false, nil) and not an error.
func TestMigrationAppliedNoRowsIsNotAnError(t *testing.T) {
	app := newMigrateTestApp(t)
	applied, err := migrationApplied(app, "9999_definitely_not_applied.js")
	if err != nil {
		t.Fatalf("expected (false, nil) for an absent migration, got err: %v", err)
	}
	if applied {
		t.Fatal("absent migration reported as applied")
	}
}

func containsStr(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
