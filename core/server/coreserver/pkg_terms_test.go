package coreserver

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/pkgterms"
	"tinycld.org/core/rlstest"
)

func registryRow(t *testing.T, app core.App, slug string) *core.Record {
	t.Helper()
	row, err := app.FindFirstRecordByFilter("pkg_registry", "slug = {:slug}", map[string]any{"slug": slug})
	if err != nil {
		t.Fatalf("registry row of %s: %v", slug, err)
	}
	return row
}

func termsText(t *testing.T, app core.App, slug string) (version, body string, found bool) {
	t.Helper()
	rows, err := app.FindRecordsByFilter(pkgterms.TermsCollection, "pkg = {:pkg}", "", 0, 0, map[string]any{"pkg": slug})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		return "", "", false
	}
	return rows[0].GetString("version"), rows[0].GetString("body"), true
}

// The boot sync carries a package's terms from bundled-packages.json into
// the registry and pkg_terms, never touches the accepted version, and so
// makes a package pending again when its terms version changes.
func TestSyncBundledPackagesCarriesTheTerms(t *testing.T) {
	app := rlstest.NewMigratedApp(t, rlstest.MigrationsDir(t, "../pb_migrations"))
	dir := t.TempDir()
	withCwd(t, dir)
	gizmos := bundledPackage{Name: "Gizmos", Slug: "gizmos", Version: "1.0.0", TermsVersion: "2026-01", TermsBody: "# Gizmos terms"}
	trinkets := bundledPackage{Name: "Trinkets", Slug: "trinkets", Version: "1.0.0"}

	writeBundledJSON(t, dir, []bundledPackage{gizmos, trinkets})
	SyncBundledPackages(app)

	row := registryRow(t, app, "gizmos")
	if row.GetString("terms_version") != "2026-01" || row.GetString("terms_accepted_version") != "" || !pkgterms.Pending(row) {
		t.Fatalf("a new package with terms: version %q, accepted %q", row.GetString("terms_version"), row.GetString("terms_accepted_version"))
	}
	if version, body, ok := termsText(t, app, "gizmos"); !ok || version != "2026-01" || body != "# Gizmos terms" {
		t.Fatalf("the stored terms: %q %q %v", version, body, ok)
	}
	if plain := registryRow(t, app, "trinkets"); plain.GetString("terms_version") != "" || pkgterms.Pending(plain) {
		t.Fatal("a package with no terms is pending")
	}
	if _, _, ok := termsText(t, app, "trinkets"); ok {
		t.Fatal("a package with no terms has a terms row")
	}

	// The owner accepts; the next boot with the same terms keeps that.
	row.Set("terms_accepted_version", "2026-01")
	if err := app.Save(row); err != nil {
		t.Fatal(err)
	}
	SyncBundledPackages(app)
	if row = registryRow(t, app, "gizmos"); pkgterms.Pending(row) {
		t.Fatal("a boot with unchanged terms made the package pending")
	}

	// A new release with new terms: pending again, the text replaced.
	gizmos.Version, gizmos.TermsVersion, gizmos.TermsBody = "1.1.0", "2026-06", "# Gizmos terms, June"
	writeBundledJSON(t, dir, []bundledPackage{gizmos, trinkets})
	SyncBundledPackages(app)
	row = registryRow(t, app, "gizmos")
	if !pkgterms.Pending(row) || row.GetString("terms_version") != "2026-06" || row.GetString("terms_accepted_version") != "2026-01" {
		t.Fatalf("after new terms: version %q, accepted %q", row.GetString("terms_version"), row.GetString("terms_accepted_version"))
	}
	if version, body, _ := termsText(t, app, "gizmos"); version != "2026-06" || body != "# Gizmos terms, June" {
		t.Fatalf("the stored terms after a new version: %q %q", version, body)
	}

	// A release that drops its terms: nothing to accept, no text.
	gizmos.TermsVersion, gizmos.TermsBody = "", ""
	writeBundledJSON(t, dir, []bundledPackage{gizmos, trinkets})
	SyncBundledPackages(app)
	if row = registryRow(t, app, "gizmos"); row.GetString("terms_version") != "" || pkgterms.Pending(row) {
		t.Fatal("a package that dropped its terms is still pending")
	}
	if _, _, ok := termsText(t, app, "gizmos"); ok {
		t.Fatal("the text of dropped terms is still stored")
	}
}

// The seed tests' registry has no terms fields and no pkg_terms collection
// (newRegistryOnlyApp): a sync of packages with no terms still works there.
func TestSyncBundledPackagesWithoutTheTermsSchema(t *testing.T) {
	app := newRegistryOnlyApp(t)
	dir := t.TempDir()
	withCwd(t, dir)
	writeBundledJSON(t, dir, []bundledPackage{{Name: "Trinkets", Slug: "trinkets", Version: "1.0.0"}})
	SyncBundledPackages(app)
	if row := registryRow(t, app, "trinkets"); row.GetString("status") != "bundled" {
		t.Fatalf("status %q", row.GetString("status"))
	}
}
