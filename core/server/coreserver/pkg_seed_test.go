package coreserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

// TestSyncBundledPackagesPersistsManifestJSON is the regression guard for the
// compatibility-solver no-op bug: SyncBundledPackages must write manifest_json
// (carrying peerVersions) onto bundled rows, so the solver has constraints to
// check. Without it, every bundled package has empty peerVersions and the
// solver silently never blocks anything.
func TestSyncBundledPackagesPersistsManifestJSON(t *testing.T) {
	app := newRegistryOnlyApp(t)

	// Write a bundled-packages.json in cwd (findBundledPackagesJSON checks it
	// first) and restore cwd after.
	dir := t.TempDir()
	rows := []bundledPackage{
		{
			Name:    "Mail",
			Slug:    "mail",
			Version: "1.2.0",
			ManifestJSON: mustJSON(t, map[string]any{
				"slug":         "mail",
				"version":      "1.2.0",
				"peerVersions": map[string]string{"@tinycld/core": ">=2.1 <3"},
			}),
		},
	}
	writeBundledJSON(t, dir, rows)
	withCwd(t, dir)

	SyncBundledPackages(app)

	rec, err := app.FindFirstRecordByFilter("pkg_registry", "slug = 'mail'", nil)
	if err != nil {
		t.Fatalf("mail row not seeded: %v", err)
	}
	got := rec.GetString("manifest_json")
	if got == "" {
		t.Fatal("manifest_json was not persisted — compat solver would see no peerVersions")
	}
	peers := peerVersionsFromManifest(got)
	if peers["@tinycld/core"] != ">=2.1 <3" {
		t.Errorf("peerVersions not round-tripped through seed: got %v", peers)
	}

	// And re-syncing (update branch) must keep manifest_json set.
	SyncBundledPackages(app)
	rec2, _ := app.FindFirstRecordByFilter("pkg_registry", "slug = 'mail'", nil)
	if rec2.GetString("manifest_json") == "" {
		t.Error("manifest_json lost on re-sync (update branch)")
	}
}

// ---- helpers ----

func newRegistryOnlyApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })
	addPkgRegistryWithManifest(t, app)
	return app
}

func writeBundledJSON(t *testing.T, dir string, rows []bundledPackage) {
	t.Helper()
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bundled-packages.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func withCwd(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(prev) })
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
