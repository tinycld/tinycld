package jsvm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

// transform_module_test.go pins the error surface for a .pb.ts hook that uses
// import/export. Hook and migration files run as plain scripts (RunScript /
// script-mode compile), but esbuild's transform preserves module syntax — so
// an author's habitual `export const …` produced ESM that the script compile
// rejected with a syntax error pointing at the GENERATED wrapper, not the
// author's file, and never saying what to change.

func writeHook(t *testing.T, dir, name, src string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestHookWithExport_ErrorNamesTheFileAndTheFix(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	hooksDir := filepath.Join(t.TempDir(), "pb_hooks")
	writeHook(t, hooksDir, "exports.pb.ts",
		"export const answer = 42\nrouterAdd('GET', '/answer', (e) => e.string(200, String(answer)))\n")

	err = Register(app, Config{HooksDir: hooksDir, Sandboxed: true})
	if err == nil {
		t.Fatal("a module-syntax hook file loaded without error")
	}
	if !strings.Contains(err.Error(), "exports.pb.ts") {
		t.Fatalf("error %q does not name the author's file", err)
	}
	if !strings.Contains(err.Error(), "import/export") {
		t.Fatalf("error %q does not say what to change", err)
	}
}

func TestMigrationWithExport_ErrorNamesTheFileAndTheFix(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	migDir := filepath.Join(t.TempDir(), "pb_migrations")
	writeHook(t, migDir, "1_exported.ts",
		"export default migrate(() => {})\n")

	err = Register(app, Config{MigrationsDir: migDir, Sandboxed: true})
	if err == nil {
		t.Fatal("a module-syntax migration file loaded without error")
	}
	if !strings.Contains(err.Error(), "1_exported.ts") {
		t.Fatalf("error %q does not name the author's file", err)
	}
	if !strings.Contains(err.Error(), "import/export") {
		t.Fatalf("error %q does not say what to change", err)
	}
}

// A string that merely CONTAINS export-looking text is not module syntax —
// detection must parse, not pattern-match, or working hooks start failing.
func TestHookWithExportInsideString_StillLoads(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	hooksDir := filepath.Join(t.TempDir(), "pb_hooks")
	writeHook(t, hooksDir, "strings.pb.ts",
		"const doc = `\nexport const looksLikeESM = true\nimport nothing from 'nowhere'\n`\nrouterAdd('GET', '/doc', (e) => e.string(200, doc))\n")

	if err := Register(app, Config{HooksDir: hooksDir, Sandboxed: true}); err != nil {
		t.Fatalf("a script whose string mentions export failed to load: %v", err)
	}
}

// A garden-variety TS syntax error must also blame the author's file, not the
// anonymous compile target.
func TestHookSyntaxError_NamesTheFile(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	hooksDir := filepath.Join(t.TempDir(), "pb_hooks")
	writeHook(t, hooksDir, "broken.pb.ts", "const x = {\n")

	err = Register(app, Config{HooksDir: hooksDir, Sandboxed: true})
	if err == nil {
		t.Fatal("a syntactically broken hook file loaded without error")
	}
	if !strings.Contains(err.Error(), "broken.pb.ts") {
		t.Fatalf("error %q does not name the author's file", err)
	}
}
