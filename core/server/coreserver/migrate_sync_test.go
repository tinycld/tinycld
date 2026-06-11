package coreserver

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestMigrationsToApply(t *testing.T) {
	applied := []string{"100_a.js", "200_b.js"}
	newSet := []string{"100_a.js", "200_b.js", "300_c.js", "400_d.js"}
	got := migrationsToApply(applied, newSet)
	want := []string{"300_c.js", "400_d.js"} // oldest-first
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("migrationsToApply = %v, want %v", got, want)
	}
}

func TestMigrationsToRevert(t *testing.T) {
	applied := []string{"100_a.js", "200_b.js", "300_c.js"}
	newSet := []string{"100_a.js"}
	got := migrationsToRevert(applied, newSet)
	want := []string{"300_c.js", "200_b.js"} // newest-first (reverse)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("migrationsToRevert = %v, want %v", got, want)
	}
}

func TestMigrationDiff_NoChange(t *testing.T) {
	set := []string{"100_a.js", "200_b.js"}
	if got := migrationsToApply(set, set); len(got) != 0 {
		t.Fatalf("expected no applies, got %v", got)
	}
	if got := migrationsToRevert(set, set); len(got) != 0 {
		t.Fatalf("expected no reverts, got %v", got)
	}
}

func TestSyncMigrations_RevertsDroppedSet(t *testing.T) {
	app := newMigrateTestApp(t)

	const colName = "ms_sync_widgets"
	createFile := "9100000001_ms_create_widgets.js"
	addFieldFile := "9100000002_ms_add_color.js"

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
				if c.Fields.GetByName("color") != nil {
					c.Fields.RemoveByName("color")
				}
				return txApp.Save(c)
			},
		},
	})

	applied := []string{createFile, addFieldFile}
	if _, err := applyNamedMigrations(app, applied); err != nil {
		t.Fatalf("applyNamedMigrations: %v", err)
	}

	// The new build drops addFieldFile — sync must revert exactly that one.
	newSet := []string{createFile}
	res, err := syncMigrations(app, applied, newSet)
	if err != nil {
		t.Fatalf("syncMigrations: %v", err)
	}
	if len(res.Reverted) != 1 || res.Reverted[0] != addFieldFile {
		t.Fatalf("Reverted = %v, want [%s]", res.Reverted, addFieldFile)
	}
	if stillApplied, _ := migrationApplied(app, addFieldFile); stillApplied {
		t.Fatalf("%s should have been reverted", addFieldFile)
	}
	if stillApplied, _ := migrationApplied(app, createFile); !stillApplied {
		t.Fatalf("%s should remain applied", createFile)
	}
}

func TestSyncMigrations_NoChange(t *testing.T) {
	app := newMigrateTestApp(t)
	set := []string{"100_a.js", "200_b.js"}
	res, err := syncMigrations(app, set, set)
	if err != nil {
		t.Fatalf("syncMigrations: %v", err)
	}
	if len(res.Reverted) != 0 || len(res.Pending) != 0 {
		t.Fatalf("expected no-op sync, got %+v", res)
	}
}

func TestSyncMigrations_SkipsUnregisteredDrop(t *testing.T) {
	app := newMigrateTestApp(t)
	// A .go migration is in the applied set but absent from the new build's
	// (file-based) set and NOT registered in this test binary. It must be
	// SKIPPED, not error — mirroring core's compiled-in Go migrations.
	applied := []string{"100_core.go", "200_pkg.js"}
	newSet := []string{"200_pkg.js"} // 100_core.go "dropped" but unregistered
	res, err := syncMigrations(app, applied, newSet)
	if err != nil {
		t.Fatalf("syncMigrations should skip unregistered drops, got: %v", err)
	}
	if len(res.Reverted) != 0 {
		t.Fatalf("expected nothing reverted (unregistered skipped), got %v", res.Reverted)
	}
}

func TestSyncMigrations_RefusesEmptyNewSet(t *testing.T) {
	app := newMigrateTestApp(t)
	applied := []string{"100_a.js", "200_b.js"}
	if _, err := syncMigrations(app, applied, nil); err == nil {
		t.Fatal("expected error: empty newSet with applied migrations must not revert everything")
	}
}

func TestBuildMigrationFiles(t *testing.T) {
	dir := t.TempDir()
	mig := filepath.Join(dir, "tinycld", "server", "pb_migrations")
	if err := os.MkdirAll(mig, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"200_b.js", "100_a.js", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(mig, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := buildMigrationFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"100_a.js", "200_b.js"} // sorted, .js only
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildMigrationFiles = %v, want %v", got, want)
	}
}
