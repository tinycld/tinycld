package jsvm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// Config.MigrationsList exists so a caller can read a DIFFERENT build's
// migrations without disturbing the running one. core.AppMigrations is
// process-global and has no remove, so a load that leaked into it would shadow
// the migrations this process booted with for the rest of its life.
func TestMigrationsList_LoadsPrivatelyWithoutTouchingGlobal(t *testing.T) {
	dir := t.TempDir()
	const name = "1700000001_private.js"
	src := `migrate((app) => { throw new Error("up-ran") }, (app) => { throw new Error("down-ran") })`
	if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	globalBefore := len(core.AppMigrations.Items())

	list := &core.MigrationsList{}
	if err := Register(app, Config{
		MigrationsDir:  dir,
		MigrationsList: list,
		HooksDir:       filepath.Join(dir, ".no-hooks"),
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if got := len(list.Items()); got != 1 {
		t.Fatalf("private list holds %d migrations, want 1", got)
	}
	if got := len(core.AppMigrations.Items()); got != globalBefore {
		t.Errorf("core.AppMigrations grew from %d to %d — the load leaked into the global registry",
			globalBefore, got)
	}

	m := list.Item(0)
	if m.File != name {
		t.Errorf("File = %q, want %q", m.File, name)
	}
	// The closures must be the file's own, not placeholders.
	if err := m.Up(nil); err == nil || !strings.Contains(err.Error(), "up-ran") {
		t.Errorf("Up did not come from the file, got %v", err)
	}
	if err := m.Down(nil); err == nil || !strings.Contains(err.Error(), "down-ran") {
		t.Errorf("Down did not come from the file, got %v", err)
	}
}

// A nil MigrationsList must keep the stock behavior: register into
// core.AppMigrations.
func TestMigrationsList_NilRegistersIntoGlobal(t *testing.T) {
	dir := t.TempDir()
	const name = "1700000002_global.js"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(`migrate((app) => {})`), 0o644); err != nil {
		t.Fatal(err)
	}

	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	before := len(core.AppMigrations.Items())
	if err := Register(app, Config{
		MigrationsDir: dir,
		HooksDir:      filepath.Join(dir, ".no-hooks"),
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if got := len(core.AppMigrations.Items()); got != before+1 {
		t.Errorf("core.AppMigrations went %d -> %d, want +1 (nil list must use the global)", before, got)
	}
}
