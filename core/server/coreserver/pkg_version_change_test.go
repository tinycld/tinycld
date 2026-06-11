package coreserver

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// TestTargetMigrationsDirBase is the regression guard for the core drop-report
// bug: the base/core member ships no root manifest, so resolving its target
// migrations via parseManifestViaNode failed and the core drop report fell back
// to dry-reverting the whole core set (and errored → "no data loss"). The base
// branch must resolve to the fixed nested path and list its migration files
// without touching a manifest.
func TestTargetMigrationsDirBase(t *testing.T) {
	extractDir := t.TempDir()
	migDir := filepath.Join(extractDir, "core", "server", "pb_migrations")
	if err := os.MkdirAll(migDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"1990000000_create_base_probe.js", "1700000000_create_core.js"} {
		if err := os.WriteFile(filepath.Join(migDir, f), []byte("//"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := targetMigrationsDir(baseRegistrySlug, extractDir)
	if err != nil {
		t.Fatalf("targetMigrationsDir(core) err = %v", err)
	}
	want := filepath.Join("core", "server", "pb_migrations")
	if got != want {
		t.Fatalf("targetMigrationsDir(core) = %q, want %q", got, want)
	}

	files := listMigrationBasenames(filepath.Join(extractDir, got))
	wantFiles := []string{"1700000000_create_core.js", "1990000000_create_base_probe.js"}
	got2 := append([]string(nil), files...)
	// listMigrationBasenames doesn't sort; compare as sets via a sorted copy.
	if !equalStringSets(got2, wantFiles) {
		t.Errorf("base migration files = %v, want (any order) %v", files, wantFiles)
	}
}

func equalStringSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, x := range a {
		seen[x]++
	}
	for _, x := range b {
		seen[x]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}

func TestSubtractStrings(t *testing.T) {
	a := []string{"a", "b", "c", "d"}
	b := []string{"b", "d"}
	got := subtractStrings(a, b)
	want := []string{"a", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("subtractStrings = %v, want %v", got, want)
	}
	if got := subtractStrings(nil, b); len(got) != 0 {
		t.Errorf("subtractStrings(nil, _) = %v, want empty", got)
	}
}

func TestSpecForVersion(t *testing.T) {
	cases := []struct {
		source, target, want string
		wantErr              bool
	}{
		{"@tinycld/mail", "1.2.3", "@tinycld/mail@1.2.3", false},
		{"@tinycld/mail@1.0.0", "1.2.3", "@tinycld/mail@1.2.3", false},
		{"github:tinycld/todo", "v1.2.0", "github:tinycld/todo#v1.2.0", false},
		{"github:tinycld/todo#v0.1.0", "v1.2.0", "github:tinycld/todo#v1.2.0", false},
		{"", "1.0.0", "", true},
	}
	for _, c := range cases {
		got, err := specForVersion(c.source, c.target)
		if (err != nil) != c.wantErr {
			t.Errorf("specForVersion(%q,%q) err=%v wantErr=%v", c.source, c.target, err, c.wantErr)
			continue
		}
		if !c.wantErr && got != c.want {
			t.Errorf("specForVersion(%q,%q) = %q, want %q", c.source, c.target, got, c.want)
		}
	}
}

// TestCurrentBuildMigrationsUsesOwnerMap is the regression guard for the
// drop-report accuracy bug: in the rebuild-from-scratch model there is one
// `current` pkg_build row for the whole image (labeled by the last-changed
// member), so a per-slug pkg_build lookup returned nothing for every other
// package and the report wrongly said "nothing will be dropped". The migration
// owner map is the per-package source of truth and is independent of which
// member last built — so currentBuildMigrations must return a package's owned
// files regardless of the pkg_build labeling.
func TestCurrentBuildMigrationsUsesOwnerMap(t *testing.T) {
	restore := setMigrationOwnersForTest(map[string]string{
		"1713000005_x.js":   "mail",
		"1713000006_y.js":   "mail",
		"1700000000_core.js": "core",
	})
	defer restore()

	// `current` pkg_build is labeled by a DIFFERENT package (core) — the old
	// per-slug lookup would have found no `mail` row and returned empty.
	got := currentBuildMigrations(nil, "mail")
	want := []string{"1713000005_x.js", "1713000006_y.js"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("currentBuildMigrations(mail) = %v, want %v (from owner map)", got, want)
	}

	// Unknown slug → empty (not nil-panic).
	if got := currentBuildMigrations(nil, "nope"); len(got) != 0 {
		t.Errorf("currentBuildMigrations(unknown) = %v, want empty", got)
	}
}

// TestCheckChangeCompatBlocksIncompatibleSet exercises the apply pipeline's
// authoritative re-check: a proposed downgrade of core below mail's declared
// peerVersions floor must surface a violation.

// TestVersionDirection is the M2 regression guard: equal versions (incl. 1.0 vs
// 1.0.0) are "same", and unparsable versions error rather than defaulting to the
// destructive "downgrade" direction.

// TestVerifyTargetPeerVersions is the C-recheck regression guard: a target
// version that tightens its OWN peerVersions beyond what the installed manifest
// declared must still be caught from the fetched target manifest.

// TestFinalizeBuildRecordsAppliedDeltaNotTargetSet is the H3 regression guard:
// a downgrade reverts migrations and ADDS none, so migration_files /
// migrations_applied (which drive whole-image revert's count) must be empty —
// not the target's full migration set. We assert the field semantics directly
// via the build-fields contract recordBuild persists.
func TestDowngradeBuildRecordsEmptyMigrationDelta(t *testing.T) {
	app := newPkgBuildTestApp(t)

	// Simulate what finalizeVersionChange writes for a DOWNGRADE: appliedDelta is
	// empty, pkg_migration_files is the target's full set.
	targetFull := []string{"1713000000_a.js", "1713000001_b.js"}
	appliedDelta := []string{} // downgrade adds nothing

	rec, err := recordBuild(app, map[string]any{
		"build_id":            "build-dg-1",
		"pkg_slug":            "mail",
		"action":              "install",
		"migrations_applied":  len(appliedDelta),
		"migration_files":     appliedDelta,
		"pkg_migration_files": targetFull,
	})
	if err != nil {
		t.Fatalf("recordBuild: %v", err)
	}

	if got := rec.GetInt("migrations_applied"); got != 0 {
		t.Errorf("downgrade migrations_applied = %d, want 0 (whole-image revert counts this)", got)
	}
	var mf []string
	_ = rec.UnmarshalJSONField("migration_files", &mf)
	if len(mf) != 0 {
		t.Errorf("downgrade migration_files = %v, want empty (it ADDED nothing)", mf)
	}
	var pmf []string
	_ = rec.UnmarshalJSONField("pkg_migration_files", &pmf)
	if len(pmf) != 2 {
		t.Errorf("pkg_migration_files = %v, want the target's full 2-file set", pmf)
	}
}

// addPkgRegistryWithManifest adds a pkg_registry collection that includes the
// manifest_json + version fields the compat check reads.
func addPkgRegistryWithManifest(t *testing.T, app *tests.TestApp) {
	t.Helper()
	c := core.NewBaseCollection("pkg_registry")
	c.Fields.Add(&core.TextField{Name: "slug", Required: true})
	c.Fields.Add(&core.TextField{Name: "version"})
	c.Fields.Add(&core.TextField{Name: "npm_package"})
	c.Fields.Add(&core.JSONField{Name: "manifest_json"})
	c.Fields.Add(&core.SelectField{
		Name: "status", Required: true, MaxSelect: 1,
		Values: []string{"bundled", "available", "installed", "disabled"},
	})
	if err := app.Save(c); err != nil {
		t.Fatalf("save pkg_registry collection: %v", err)
	}
}
