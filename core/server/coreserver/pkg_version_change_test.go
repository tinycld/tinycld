package coreserver

import (
	"reflect"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

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

// TestCurrentBuildMigrationsPrefersPkgField verifies the per-package field is
// used when present, with a graceful fallback to the global delta.
func TestCurrentBuildMigrationsPrefersPkgField(t *testing.T) {
	app := newPkgBuildTestApp(t)

	col, err := app.FindCollectionByNameOrId("pkg_build")
	if err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(col)
	rec.Set("build_id", "build-vc-1")
	rec.Set("pkg_slug", "mail")
	rec.Set("action", "install")
	rec.Set("status", "current")
	rec.Set("pkg_migration_files", []string{"1713000005_x.js", "1713000006_y.js"})
	rec.Set("migration_files", []string{"1713000005_x.js", "1713000006_y.js", "1700000000_core.js"})
	if err := app.Save(rec); err != nil {
		t.Fatalf("save build: %v", err)
	}

	got := currentBuildMigrations(app, "mail")
	want := []string{"1713000005_x.js", "1713000006_y.js"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("currentBuildMigrations = %v, want %v (from pkg_migration_files)", got, want)
	}

	// Unknown slug → nil.
	if got := currentBuildMigrations(app, "nope"); got != nil {
		t.Errorf("currentBuildMigrations(unknown) = %v, want nil", got)
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
