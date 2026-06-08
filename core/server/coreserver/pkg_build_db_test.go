package coreserver

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// newPkgBuildTestApp builds a test app with a minimal pkg_build collection
// matching the shape the build helpers read/write.
func newPkgBuildTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	c := core.NewBaseCollection("pkg_build")
	c.Fields.Add(&core.TextField{Name: "build_id", Required: true})
	c.Fields.Add(&core.TextField{Name: "pkg_slug"})
	c.Fields.Add(&core.TextField{Name: "npm_package"})
	c.Fields.Add(&core.TextField{Name: "version"})
	c.Fields.Add(&core.SelectField{
		Name: "action", Required: true, MaxSelect: 1,
		Values: []string{"install", "revert"},
	})
	c.Fields.Add(&core.BoolField{Name: "binary_archived"})
	c.Fields.Add(&core.TextField{Name: "release_id"})
	c.Fields.Add(&core.NumberField{Name: "migrations_applied"})
	c.Fields.Add(&core.JSONField{Name: "migration_files"})
	c.Fields.Add(&core.JSONField{Name: "pkg_migration_files"})
	c.Fields.Add(&core.JSONField{Name: "bundles"})
	c.Fields.Add(&core.TextField{Name: "reverted_from"})
	c.Fields.Add(&core.SelectField{
		Name: "status", Required: true, MaxSelect: 1,
		Values: []string{"available", "current", "superseded"},
	})
	c.Fields.Add(&core.TextField{Name: "notes"})
	c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
	if err := app.Save(c); err != nil {
		t.Fatalf("save pkg_build collection: %v", err)
	}
	return app
}

func TestRecordBuildDemotesPriorCurrent(t *testing.T) {
	app := newPkgBuildTestApp(t)

	first, err := recordBuild(app, map[string]any{
		"build_id": "build-1", "pkg_slug": "todo", "action": "install",
	})
	if err != nil {
		t.Fatalf("recordBuild #1: %v", err)
	}
	if first.GetString("status") != "current" {
		t.Fatalf("first build status = %q, want current", first.GetString("status"))
	}

	if _, err := recordBuild(app, map[string]any{
		"build_id": "build-2", "pkg_slug": "todo", "action": "install",
	}); err != nil {
		t.Fatalf("recordBuild #2: %v", err)
	}

	// The first build must have been demoted to available.
	reloaded, err := app.FindFirstRecordByFilter("pkg_build", "build_id = 'build-1'", nil)
	if err != nil {
		t.Fatalf("reload build-1: %v", err)
	}
	if reloaded.GetString("status") != "available" {
		t.Fatalf("build-1 status after second install = %q, want available", reloaded.GetString("status"))
	}

	current, err := app.FindRecordsByFilter("pkg_build", "status = 'current'", "", 0, 0)
	if err != nil {
		t.Fatalf("query current: %v", err)
	}
	if len(current) != 1 || current[0].GetString("build_id") != "build-2" {
		t.Fatalf("expected exactly build-2 current, got %d records", len(current))
	}
}

func TestSeedBaseBuildIsIdempotentAndDevSafe(t *testing.T) {
	app := newPkgBuildTestApp(t)

	// In the test/dev layout there is no binary or promoted release on disk, so
	// SeedBaseBuild must no-op (and definitely must not panic).
	SeedBaseBuild(app)
	if got := countBuilds(t, app); got != 0 {
		t.Fatalf("SeedBaseBuild created %d builds in dev layout, want 0", got)
	}

	// Even with a pre-existing build, a second call must not add another.
	if _, err := recordBuild(app, map[string]any{
		"build_id": "build-1", "pkg_slug": "todo", "action": "install",
	}); err != nil {
		t.Fatalf("seed existing build: %v", err)
	}
	SeedBaseBuild(app)
	if got := countBuilds(t, app); got != 1 {
		t.Fatalf("SeedBaseBuild added a build when one already existed (now %d)", got)
	}
}

func countBuilds(t *testing.T, app core.App) int {
	t.Helper()
	records, err := app.FindRecordsByFilter("pkg_build", "id != ''", "", 0, 0)
	if err != nil {
		t.Fatalf("count builds: %v", err)
	}
	return len(records)
}

// addPkgRegistryCollection adds a minimal pkg_registry collection so the revert's
// registry-reconciliation logic can be exercised.
func addPkgRegistryCollection(t *testing.T, app *tests.TestApp) {
	t.Helper()
	c := core.NewBaseCollection("pkg_registry")
	c.Fields.Add(&core.TextField{Name: "slug", Required: true})
	c.Fields.Add(&core.TextField{Name: "version"})
	c.Fields.Add(&core.TextField{Name: "npm_package"})
	c.Fields.Add(&core.SelectField{
		Name: "status", Required: true, MaxSelect: 1,
		Values: []string{"bundled", "available", "installed", "disabled"},
	})
	if err := app.Save(c); err != nil {
		t.Fatalf("save pkg_registry collection: %v", err)
	}
}

func makeRegistryRow(t *testing.T, app core.App, slug, status string) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("pkg_registry")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.Set("slug", slug)
	r.Set("status", status)
	if err := app.Save(r); err != nil {
		t.Fatalf("save registry row %s: %v", slug, err)
	}
}

func makeBuild(t *testing.T, app core.App, buildID, slug, status string) *core.Record {
	t.Helper()
	r, err := app.FindCollectionByNameOrId("pkg_build")
	if err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(r)
	rec.Set("build_id", buildID)
	rec.Set("pkg_slug", slug)
	rec.Set("action", "install")
	rec.Set("status", status)
	if err := app.Save(rec); err != nil {
		t.Fatalf("save build %s: %v", buildID, err)
	}
	return rec
}

func registryStatus(t *testing.T, app core.App, slug string) string {
	t.Helper()
	r, err := app.FindFirstRecordByFilter("pkg_registry", "slug = {:s}", map[string]any{"s": slug})
	if err != nil {
		t.Fatalf("registry row %s not found: %v", slug, err)
	}
	return r.GetString("status")
}

func TestDisableRevertedPackages(t *testing.T) {
	app := newPkgBuildTestApp(t)
	addPkgRegistryCollection(t, app)

	// Reverting to `target` (a mail build) supersedes newer builds for calc and
	// drive. calc and drive should be disabled; mail (the target's own package)
	// stays installed; a bundled package is never touched.
	target := makeBuild(t, app, "build-1", "mail", "current")
	supCalc := makeBuild(t, app, "build-2", "calc", "superseded")
	supDrive := makeBuild(t, app, "build-3", "drive", "superseded")

	makeRegistryRow(t, app, "mail", "installed")
	makeRegistryRow(t, app, "calc", "installed")
	makeRegistryRow(t, app, "drive", "installed")
	makeRegistryRow(t, app, "contacts", "bundled")

	if err := disableRevertedPackages(app, target, []*core.Record{supCalc, supDrive}); err != nil {
		t.Fatalf("disableRevertedPackages: %v", err)
	}

	if s := registryStatus(t, app, "calc"); s != "disabled" {
		t.Errorf("calc status = %q, want disabled", s)
	}
	if s := registryStatus(t, app, "drive"); s != "disabled" {
		t.Errorf("drive status = %q, want disabled", s)
	}
	if s := registryStatus(t, app, "mail"); s != "installed" {
		t.Errorf("mail (target package) status = %q, want installed", s)
	}
	if s := registryStatus(t, app, "contacts"); s != "bundled" {
		t.Errorf("contacts (bundled) status = %q, want bundled (untouched)", s)
	}
}

func TestDisableRevertedPackagesKeepsSlugWithSurvivingBuild(t *testing.T) {
	app := newPkgBuildTestApp(t)
	addPkgRegistryCollection(t, app)

	// mail has an earlier surviving build (the target) AND a newer superseded
	// build (e.g. an upgrade that got reverted). The surviving build keeps mail
	// installed — reverting the upgrade must NOT disable mail.
	target := makeBuild(t, app, "build-1", "mail", "current")
	supMailUpgrade := makeBuild(t, app, "build-2", "mail", "superseded")
	makeRegistryRow(t, app, "mail", "installed")

	if err := disableRevertedPackages(app, target, []*core.Record{supMailUpgrade}); err != nil {
		t.Fatalf("disableRevertedPackages: %v", err)
	}
	if s := registryStatus(t, app, "mail"); s != "installed" {
		t.Errorf("mail status = %q, want installed (earlier build survives)", s)
	}
}
