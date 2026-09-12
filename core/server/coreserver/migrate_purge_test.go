package coreserver

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// purgeUnregisteredPackageRows must delete the uninstalled package's stranded
// history rows (Down was skipped because unregistered) so a reinstall isn't
// blocked by stale "already applied" rows — the Bug B reproduction.
func TestPurgeUnregisteredPackageRows_RemovesOwnedStrandedRows(t *testing.T) {
	app := newMigrateTestApp(t)

	createFile := "1800000000_create_calendar-slots.js"
	configFile := "1800000001_calendar-slots-booking-config.js"
	// A row belonging to a DIFFERENT package that also ended up skipped — must be
	// left untouched.
	otherFile := "1715000000_create_calendar_collections.js"

	for _, f := range []string{createFile, configFile, otherFile} {
		if err := insertMigrationRow(app, f); err != nil {
			t.Fatalf("seed _migrations row %s: %v", f, err)
		}
	}

	restore := setMigrationOwnersForTest(map[string]string{
		createFile: "calendar-slots",
		configFile: "calendar-slots",
		otherFile:  "calendar",
	})
	defer restore()

	skipped := []string{createFile, configFile, otherFile}
	purged, err := purgeUnregisteredPackageRows(app, "calendar-slots", skipped)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if len(purged) != 2 {
		t.Fatalf("purged %d rows, want 2 (%v)", len(purged), purged)
	}

	for _, f := range []string{createFile, configFile} {
		if ok, _ := migrationApplied(app, f); ok {
			t.Errorf("row %s should be purged but is still applied", f)
		}
	}
	// The other package's row must survive.
	if ok, _ := migrationApplied(app, otherFile); !ok {
		t.Errorf("row %s (calendar) was wrongly purged", otherFile)
	}
}

// When the owner map is stale/absent, fall back to a slug-substring match on the
// filename so a deeply-stranded row is still recoverable.
func TestPurgeUnregisteredPackageRows_FilenameFallback(t *testing.T) {
	app := newMigrateTestApp(t)

	createFile := "1800000000_create_calendar-slots.js"
	if err := insertMigrationRow(app, createFile); err != nil {
		t.Fatalf("seed row: %v", err)
	}

	// Empty owner map → forces the filename fallback.
	restore := setMigrationOwnersForTest(map[string]string{})
	defer restore()

	purged, err := purgeUnregisteredPackageRows(app, "calendar-slots", []string{createFile})
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if len(purged) != 1 || purged[0] != createFile {
		t.Fatalf("filename fallback purged %v, want [%s]", purged, createFile)
	}
	if ok, _ := migrationApplied(app, createFile); ok {
		t.Errorf("row %s should be purged via filename fallback", createFile)
	}
}

// A slug that owns none of the skipped files (and isn't in any filename) must
// purge nothing — no collateral damage to unrelated history.
func TestPurgeUnregisteredPackageRows_NoMatchPurgesNothing(t *testing.T) {
	app := newMigrateTestApp(t)

	otherFile := "1715000000_create_calendar_collections.js"
	if err := insertMigrationRow(app, otherFile); err != nil {
		t.Fatalf("seed row: %v", err)
	}
	restore := setMigrationOwnersForTest(map[string]string{otherFile: "calendar"})
	defer restore()

	purged, err := purgeUnregisteredPackageRows(app, "calendar-slots", []string{otherFile})
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if len(purged) != 0 {
		t.Fatalf("purged %v, want nothing", purged)
	}
	if ok, _ := migrationApplied(app, otherFile); !ok {
		t.Errorf("unrelated row %s was wrongly purged", otherFile)
	}
}

func TestPurgeUnregisteredPackageRows_EmptyInputs(t *testing.T) {
	app := newMigrateTestApp(t)
	if p, err := purgeUnregisteredPackageRows(app, "", []string{"x.js"}); err != nil || p != nil {
		t.Fatalf("empty slug: got (%v, %v), want (nil, nil)", p, err)
	}
	if p, err := purgeUnregisteredPackageRows(app, "calendar-slots", nil); err != nil || p != nil {
		t.Fatalf("empty skipped: got (%v, %v), want (nil, nil)", p, err)
	}
}

// TestPurgeUnregisteredPackageRows_KeepsRowWhenSchemaSurvives is the regression
// guard for the tinycld.org boards outage.
//
// The purge exists for the case where a skipped Down left ONLY a stale history
// row behind. But "Down was skipped" does not imply "the schema is gone" — it
// usually means the opposite, because the Down is exactly what would have
// dropped the collections. Purging unconditionally therefore produced a DB that
// contradicted itself: the boards_* collections were still present while their
// _migrations rows were deleted.
//
// On the next install PocketBase saw no history row, re-ran the Up, and hit the
// already-existing collection:
//
//	id: The model id is invalid or already exists.;
//	name: Collection name must be unique (case insensitive).
//
// That error is fatal in the migration runner, so the server exited before
// binding a port — a boot crash loop that took the whole deployment down, and
// which the rollback could not clear because the armed DB backup had been taken
// AFTER the purge and so carried the same contradictory state.
//
// A migration whose collections still exist must therefore KEEP its history
// row: history and schema stay consistent, and the re-Up is correctly skipped.
func TestPurgeUnregisteredPackageRows_KeepsRowWhenSchemaSurvives(t *testing.T) {
	app := newMigrateTestApp(t)

	const survivingCol = "ml_boards_projects"
	// Its Down was skipped, so this collection is still in the DB.
	strandedSchema := "1800000000_create_ml_boards_collections.js"
	// Nothing of this one's schema remains — a pure stale row, the case the
	// purge was actually written for.
	pureStaleRow := "1800000001_ml_boards_tweak.js"

	// Both rows are "applied". The collection for strandedSchema is still in the
	// DB — its Down never ran, which is what "unregistered" means in production.
	// No migration is registered here, matching the real case: the active build
	// no longer carries this package's JS migrations.
	for _, f := range []string{strandedSchema, pureStaleRow} {
		if err := insertMigrationRow(app, f); err != nil {
			t.Fatalf("seed _migrations row %s: %v", f, err)
		}
	}
	col := core.NewBaseCollection(survivingCol)
	col.Fields.Add(&core.TextField{Name: "title"})
	if err := app.Save(col); err != nil {
		t.Fatalf("seed surviving collection: %v", err)
	}

	restore := setMigrationOwnersForTest(map[string]string{
		strandedSchema: "ml-boards",
		pureStaleRow:   "ml-boards",
	})
	defer restore()

	if !collectionExists(t, app, survivingCol) {
		t.Fatalf("precondition: %s should exist before the purge", survivingCol)
	}

	purged, err := purgeUnregisteredPackageRows(app, "ml-boards", []string{strandedSchema, pureStaleRow})
	if err == nil {
		t.Fatalf("purge succeeded (purged %v) while collection %s still exists; want a refusal", purged, survivingCol)
	}
	if len(purged) != 0 {
		t.Errorf("purged = %v, want none when the package's schema survives", purged)
	}

	// Neither row may be dropped: with the schema still present, deleting ANY of
	// its history is what makes the reinstall re-run an Up and crash the server.
	for _, f := range []string{strandedSchema, pureStaleRow} {
		if ok, _ := migrationApplied(app, f); !ok {
			t.Errorf("row %s was purged while collection %s still exists — a reinstall will re-run its Up and die on \"already exists\"", f, survivingCol)
		}
	}
}

// With the package's collections genuinely gone, the purge still does its
// original job: clearing stale history so a reinstall can re-run the Up. This is
// the case purgeUnregisteredPackageRows was written for, and the schema guard
// must not break it.
func TestPurgeUnregisteredPackageRows_PurgesWhenSchemaIsGone(t *testing.T) {
	app := newMigrateTestApp(t)

	staleRow := "1800000000_create_ml_gone_collections.js"
	if err := insertMigrationRow(app, staleRow); err != nil {
		t.Fatalf("seed _migrations row: %v", err)
	}
	restore := setMigrationOwnersForTest(map[string]string{staleRow: "ml-gone"})
	defer restore()

	purged, err := purgeUnregisteredPackageRows(app, "ml-gone", []string{staleRow})
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if len(purged) != 1 || purged[0] != staleRow {
		t.Fatalf("purged = %v, want [%s]", purged, staleRow)
	}
	if ok, _ := migrationApplied(app, staleRow); ok {
		t.Errorf("stale row %s should have been purged", staleRow)
	}
}
