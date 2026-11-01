package coreserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/plugins/jsvm"
)

// Two apps in one process each load their own JS migrations. A package
// install or uninstall on one app resolves only the files that app loaded.
func TestPkgMigrationByFile_ResolvesOnlyTheAppsOwnJSMigrations(t *testing.T) {
	appA, appB := newMigrateTestApp(t), newMigrateTestApp(t)

	const file = "9700000020_only_a.js"
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(`migrate((app) => {}, (app) => {})`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := jsvm.Register(appA, jsvm.Config{MigrationsDir: dir, HooksDir: filepath.Join(dir, ".no-hooks")}); err != nil {
		t.Fatal(err)
	}

	if _, ok := pkgMigrationByFile(appA, file); !ok {
		t.Fatal("app A does not resolve the JS migration it loaded")
	}
	if _, ok := pkgMigrationByFile(appB, file); ok {
		t.Fatal("app B resolves a JS migration only app A loaded")
	}
	if got := RegisteredOnly(appB, []string{file}); len(got) != 0 {
		t.Fatalf("RegisteredOnly(appB) = %v, want none", got)
	}
}

// A package install and uninstall call a JS migration's Up and Down long after
// jsvm registered it, from the app's own list. The functions still see the
// file's top-level declarations, the Down undoes the Up, and a file that passed
// no down is refused rather than recorded as reverted.
func TestNamedMigrations_RunTheAppsJSMigrationsAfterRegister(t *testing.T) {
	app := newMigrateTestApp(t)

	const withDown = "9700000030_js_pkg_cards.js"
	const upOnly = "9700000031_js_pkg_notes.js"
	dir := t.TempDir()
	for name, src := range map[string]string{
		withDown: `
const name = "js_pkg_cards"
migrate((app) => {
	app.save(new Collection({ type: "base", name: name }))
}, (app) => {
	app.delete(app.findCollectionByNameOrId(name))
})`,
		upOnly: `
const name = "js_pkg_notes"
migrate((app) => {
	app.save(new Collection({ type: "base", name: name }))
})`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := jsvm.Register(app, jsvm.Config{
		MigrationsDir: dir,
		HooksDir:      filepath.Join(dir, ".no-hooks"),
		Sandboxed:     true,
	}); err != nil {
		t.Fatal(err)
	}

	applied, err := applyNamedMigrations(app, []string{withDown, upOnly})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(applied) != 2 {
		t.Fatalf("applied %v, want both files", applied)
	}
	for _, col := range []string{"js_pkg_cards", "js_pkg_notes"} {
		if !collectionExists(t, app, col) {
			t.Fatalf("apply did not create %s", col)
		}
	}

	reverted, err := revertNamedMigrations(app, []string{withDown}, nil)
	if err != nil {
		t.Fatalf("revert: %v", err)
	}
	if len(reverted) != 1 || reverted[0] != withDown {
		t.Fatalf("reverted %v, want [%s]", reverted, withDown)
	}
	if collectionExists(t, app, "js_pkg_cards") {
		t.Fatal("the JS down did not run: js_pkg_cards is still there")
	}

	_, err = revertNamedMigrations(app, []string{upOnly}, nil)
	if err == nil || !strings.Contains(err.Error(), "no down function") {
		t.Fatalf("revert of a file with no down: err = %v, want a refusal naming the missing down", err)
	}
	if !collectionExists(t, app, "js_pkg_notes") {
		t.Fatal("a refused revert changed the schema")
	}
}
