package coreserver

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestBaseSourceEntries_ExcludesRuntimeStateDirs(t *testing.T) {
	cloneEntries := []string{
		"app", "core", "scripts", "server", "app.json", "package.json",
		"metro.config.cjs", "tsconfig.json", "biome.json",
		// these must NEVER come from the clone — they're live runtime state:
		"pb_data", "builds", "releases", "node_modules", "tinycld",
	}
	got := baseSourceEntries(cloneEntries)
	sort.Strings(got)
	want := []string{
		"app", "app.json", "biome.json", "core", "metro.config.cjs",
		"package.json", "scripts", "server", "tsconfig.json",
	}
	if len(got) != len(want) {
		t.Fatalf("baseSourceEntries = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("baseSourceEntries[%d] = %q, want %q (full=%v)", i, got[i], want[i], got)
		}
	}
}

func TestBasePreserveList_CoversStateAndBinary(t *testing.T) {
	for _, name := range []string{"pb_data", "builds", "releases", "node_modules", "tinycld", "tinycld.prev", "tinycld.new"} {
		if !basePreserve(name) {
			t.Fatalf("basePreserve(%q) = false, want true (must be preserved across a base swap)", name)
		}
	}
	for _, name := range []string{"app", "core", "app.json", "scripts"} {
		if basePreserve(name) {
			t.Fatalf("basePreserve(%q) = true, want false (source must be swapped)", name)
		}
	}
}

func TestSynthesizeBaseManifest_PopulatesNameAndNav(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "core"), 0o755); err != nil {
		t.Fatal(err)
	}
	pkgJSON := `{"version":"0.0.5","tinycld":{"peerVersions":{"@tinycld/mail":">=1"}}}`
	if err := os.WriteFile(filepath.Join(dir, "core", "package.json"), []byte(pkgJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := synthesizeBaseManifest(dir)
	if err != nil {
		t.Fatalf("synthesizeBaseManifest: %v", err)
	}
	if m.Name != "TinyCld Base" {
		t.Errorf("Name = %q, want TinyCld Base", m.Name)
	}
	if m.Slug != "core" {
		t.Errorf("Slug = %q, want core", m.Slug)
	}
	if m.Version != "0.0.5" {
		t.Errorf("Version = %q, want 0.0.5", m.Version)
	}
	if !m.HasServer {
		t.Error("HasServer = false, want true")
	}
	// Nav must be non-nil — upsertPkgRegistry dereferences m.Nav.Icon/.Order.
	if m.Nav == nil {
		t.Fatal("Nav is nil — upsertPkgRegistry would nil-panic")
	}
}
