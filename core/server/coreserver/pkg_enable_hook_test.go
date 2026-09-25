package coreserver

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func saveRegistryRow(t *testing.T, app core.App, slug, status string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("pkg_registry")
	if err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(col)
	rec.Set("slug", slug)
	rec.Set("name", slug)
	rec.Set("status", status)
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

// Re-enabling a disabled row must restore where the package came from. The
// client cannot know: by the time it re-enables, the status says only
// "disabled".
func TestPkgEnableHookRestoresSource(t *testing.T) {
	app := newRegistryOnlyApp(t)
	dir := t.TempDir()
	writeBundledJSON(t, dir, []bundledPackage{{Name: "Drive", Slug: "drive", Version: "1.0.0"}})
	withCwd(t, dir)
	RegisterPkgEnableHook(app)

	cases := []struct{ slug, want string }{
		{"drive", "bundled"},
		{"todo", "installed"},
	}
	for _, c := range cases {
		rec := saveRegistryRow(t, app, c.slug, "disabled")
		// Re-fetch: Original() reflects the state as of the last DB scan
		// (PostScan), which a freshly created in-memory record never had.
		// This mirrors the real path (fetch, then mutate, then save).
		fetched, err := app.FindRecordById("pkg_registry", rec.Id)
		if err != nil {
			t.Fatal(err)
		}
		fetched.Set("status", "installed")
		if err := app.Save(fetched); err != nil {
			t.Fatal(err)
		}
		got, _ := app.FindRecordById("pkg_registry", rec.Id)
		if got.GetString("status") != c.want {
			t.Errorf("%s: status = %q, want %q", c.slug, got.GetString("status"), c.want)
		}
	}
}

// Disabling and unrelated edits pass through untouched.
func TestPkgEnableHookLeavesOtherWritesAlone(t *testing.T) {
	app := newRegistryOnlyApp(t)
	dir := t.TempDir()
	writeBundledJSON(t, dir, []bundledPackage{{Name: "Drive", Slug: "drive", Version: "1.0.0"}})
	withCwd(t, dir)
	RegisterPkgEnableHook(app)

	rec := saveRegistryRow(t, app, "drive", "bundled")
	fetched, err := app.FindRecordById("pkg_registry", rec.Id)
	if err != nil {
		t.Fatal(err)
	}
	fetched.Set("status", "disabled")
	if err := app.Save(fetched); err != nil {
		t.Fatal(err)
	}
	got, _ := app.FindRecordById("pkg_registry", rec.Id)
	if got.GetString("status") != "disabled" {
		t.Fatalf("disable was rewritten to %q", got.GetString("status"))
	}
}
