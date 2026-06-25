package coreserver

import "testing"

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
