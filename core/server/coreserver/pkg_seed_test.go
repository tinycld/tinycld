package coreserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pocketbase/pocketbase/core"
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

// TestSyncBundledPackagesSeedsSourceAsNpmPackage guards the bundled-feature
// upgrade unlock: a bundled row's `source` git spec must land in npm_package so
// version discovery + the version-change pipeline treat it like an installed
// package. A row with no source must stay spec-less.
func TestSyncBundledPackagesSeedsSourceAsNpmPackage(t *testing.T) {
	app := newRegistryOnlyApp(t)
	dir := t.TempDir()
	rows := []bundledPackage{
		{Name: "Mail", Slug: "mail", Version: "1.2.0", Source: "github:tinycld/mail"},
		{Name: "Contacts", Slug: "contacts", Version: "0.0.4"}, // no source -> spec-less
	}
	writeBundledJSON(t, dir, rows)
	withCwd(t, dir)

	SyncBundledPackages(app)

	mail, err := app.FindFirstRecordByFilter("pkg_registry", "slug = 'mail'", nil)
	if err != nil {
		t.Fatalf("mail row not seeded: %v", err)
	}
	if got := mail.GetString("npm_package"); got != "github:tinycld/mail" {
		t.Errorf("mail npm_package = %q, want github:tinycld/mail", got)
	}
	contactsRec, err := app.FindFirstRecordByFilter("pkg_registry", "slug = 'contacts'", nil)
	if err != nil {
		t.Fatalf("contacts row not seeded: %v", err)
	}
	if got := contactsRec.GetString("npm_package"); got != "" {
		t.Errorf("contacts npm_package = %q, want empty (no source -> spec-less)", got)
	}
}

// TestSyncBundledPackages_CoreHasSourceAndServer guards that core flows through
// the generic seed path: now that the generator emits core's row with a `source`
// git spec and hasServer:true, seeding must land npm_package + has_server on the
// `core` row with no slug-specific branching — so the base upgrades like any
// other bundled package.
func TestSyncBundledPackages_CoreHasSourceAndServer(t *testing.T) {
	app := newRegistryOnlyApp(t)
	addHasServerField(t, app)
	dir := t.TempDir()
	rows := []bundledPackage{
		{Name: "TinyCld Base", Slug: "core", Version: "0.0.4", HasServer: true, Source: "github:tinycld/tinycld"},
	}
	writeBundledJSON(t, dir, rows)
	withCwd(t, dir)

	SyncBundledPackages(app)

	rec, err := app.FindFirstRecordByFilter("pkg_registry", "slug = 'core'", nil)
	if err != nil {
		t.Fatalf("core row missing: %v", err)
	}
	if got := rec.GetString("npm_package"); got != "github:tinycld/tinycld" {
		t.Fatalf("core npm_package = %q, want github:tinycld/tinycld", got)
	}
	if !rec.GetBool("has_server") {
		t.Fatalf("core has_server = false, want true")
	}
}

// TestSyncBundledPackagesBackfillDoesNotClobberPinnedSpec verifies the update
// branch only backfills npm_package when empty: an in-app upgrade that pinned a
// `#<tag>` ref (or any prior spec) must survive a re-sync, so a redeploy can't
// silently drop the resolved source back to the bare default.
func TestSyncBundledPackagesBackfillDoesNotClobberPinnedSpec(t *testing.T) {
	app := newRegistryOnlyApp(t)
	dir := t.TempDir()
	rows := []bundledPackage{{Name: "Mail", Slug: "mail", Version: "1.2.0", Source: "github:tinycld/mail"}}
	writeBundledJSON(t, dir, rows)
	withCwd(t, dir)

	// First sync backfills the bare source.
	SyncBundledPackages(app)
	mail, err := app.FindFirstRecordByFilter("pkg_registry", "slug = 'mail'", nil)
	if err != nil {
		t.Fatalf("mail row not seeded: %v", err)
	}
	// Simulate an in-app upgrade pinning a tag.
	mail.Set("npm_package", "github:tinycld/mail#v1.3.0")
	if err := app.Save(mail); err != nil {
		t.Fatal(err)
	}

	// Re-sync (update branch) must leave the pinned spec alone.
	SyncBundledPackages(app)
	mail2, _ := app.FindFirstRecordByFilter("pkg_registry", "slug = 'mail'", nil)
	if got := mail2.GetString("npm_package"); got != "github:tinycld/mail#v1.3.0" {
		t.Errorf("re-sync clobbered pinned spec: got %q", got)
	}
}

// TestUpsertPkgRegistryPreservesBundledStatus guards that an in-app version
// change of a bundled feature keeps status=bundled (so the uninstall guard still
// blocks it), while a non-bundled row still promotes to installed.
func TestUpsertPkgRegistryPreservesBundledStatus(t *testing.T) {
	app := newRegistryOnlyApp(t)
	col, err := app.FindCollectionByNameOrId("pkg_registry")
	if err != nil {
		t.Fatalf("pkg_registry collection not found: %v", err)
	}

	// A bundled row being upgraded.
	bundled := core.NewRecord(col)
	bundled.Set("slug", "mail")
	bundled.Set("version", "1.0.0")
	bundled.Set("status", "bundled")
	if err := app.Save(bundled); err != nil {
		t.Fatal(err)
	}
	// An available row being installed.
	avail := core.NewRecord(col)
	avail.Set("slug", "contacts")
	avail.Set("version", "")
	avail.Set("status", "available")
	if err := app.Save(avail); err != nil {
		t.Fatal(err)
	}

	mailManifest := &parsedManifest{Slug: "mail", Version: "1.1.0", Nav: &manifestNav{}}
	if err := upsertPkgRegistry(app, mailManifest,
		"github:tinycld/mail#v1.1.0", []byte(`{"slug":"mail"}`)); err != nil {
		t.Fatalf("upsert mail: %v", err)
	}
	contactsManifest := &parsedManifest{Slug: "contacts", Version: "0.5.0", Nav: &manifestNav{}}
	if err := upsertPkgRegistry(app, contactsManifest,
		"@tinycld/contacts@0.5.0", []byte(`{"slug":"contacts"}`)); err != nil {
		t.Fatalf("upsert contacts: %v", err)
	}

	mail, err := app.FindFirstRecordByFilter("pkg_registry", "slug = 'mail'", nil)
	if err != nil {
		t.Fatalf("mail row not found: %v", err)
	}
	if got := mail.GetString("status"); got != "bundled" {
		t.Errorf("bundled mail status = %q after upgrade, want bundled", got)
	}
	contacts, err := app.FindFirstRecordByFilter("pkg_registry", "slug = 'contacts'", nil)
	if err != nil {
		t.Fatalf("contacts row not found: %v", err)
	}
	if got := contacts.GetString("status"); got != "installed" {
		t.Errorf("available contacts status = %q after install, want installed", got)
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

// addHasServerField backfills the has_server bool onto the test pkg_registry
// collection. The shared addPkgRegistryWithManifest helper omits it (the older
// seed tests never asserted on it), so PocketBase would silently drop the value
// SyncBundledPackages writes. The real migration (create_pkg_registry) defines
// has_server, so adding it here just matches production schema.
func addHasServerField(t *testing.T, app *tests.TestApp) {
	t.Helper()
	c, err := app.FindCollectionByNameOrId("pkg_registry")
	if err != nil {
		t.Fatalf("find pkg_registry: %v", err)
	}
	c.Fields.Add(&core.BoolField{Name: "has_server"})
	if err := app.Save(c); err != nil {
		t.Fatalf("add has_server field: %v", err)
	}
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
