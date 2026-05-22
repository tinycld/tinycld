package coreserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeWsManifest writes a minimal workspace package.json for the helpers under
// test and returns its path.
func writeWsManifest(t *testing.T, dir string, workspaces []string) string {
	t.Helper()
	pkg := map[string]any{
		"name":       "@tinycld/workspace",
		"private":    true,
		"workspaces": workspaces,
		"scripts":    map[string]any{"postinstall": "cd app && npm run packages:generate"},
	}
	data, err := json.MarshalIndent(pkg, "", "    ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	path := filepath.Join(dir, "package.json")
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return path
}

func readWorkspacesArray(t *testing.T, path string) []string {
	t.Helper()
	pkg, err := readWorkspacePkg(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	return toStringSlice(pkg["workspaces"])
}

func TestAddWorkspaceMember(t *testing.T) {
	dir := t.TempDir()
	path := writeWsManifest(t, dir, []string{"app", "core", "package-scripts"})

	if err := addWorkspaceMember(path, "contacts"); err != nil {
		t.Fatalf("addWorkspaceMember: %v", err)
	}
	got := readWorkspacesArray(t, path)
	if !contains(got, "contacts") {
		t.Fatalf("expected contacts in workspaces, got %v", got)
	}
	// Existing members preserved.
	for _, m := range []string{"app", "core", "package-scripts"} {
		if !contains(got, m) {
			t.Fatalf("expected %s preserved, got %v", m, got)
		}
	}

	// Other top-level keys preserved (not just workspaces).
	pkg, _ := readWorkspacePkg(path)
	if pkg["name"] != "@tinycld/workspace" {
		t.Fatalf("expected name preserved, got %v", pkg["name"])
	}
	if _, ok := pkg["scripts"]; !ok {
		t.Fatalf("expected scripts preserved")
	}
}

func TestAddWorkspaceMemberIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := writeWsManifest(t, dir, []string{"app", "core", "contacts"})

	if err := addWorkspaceMember(path, "contacts"); err != nil {
		t.Fatalf("addWorkspaceMember: %v", err)
	}
	got := readWorkspacesArray(t, path)
	count := 0
	for _, m := range got {
		if m == "contacts" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one contacts entry, got %d (%v)", count, got)
	}
}

func TestRemoveWorkspaceMember(t *testing.T) {
	dir := t.TempDir()
	path := writeWsManifest(t, dir, []string{"app", "core", "contacts", "mail"})

	if err := removeWorkspaceMember(path, "contacts"); err != nil {
		t.Fatalf("removeWorkspaceMember: %v", err)
	}
	got := readWorkspacesArray(t, path)
	if contains(got, "contacts") {
		t.Fatalf("expected contacts removed, got %v", got)
	}
	for _, m := range []string{"app", "core", "mail"} {
		if !contains(got, m) {
			t.Fatalf("expected %s preserved, got %v", m, got)
		}
	}
}

func TestRemoveWorkspaceMemberAbsentIsNoop(t *testing.T) {
	dir := t.TempDir()
	path := writeWsManifest(t, dir, []string{"app", "core"})

	if err := removeWorkspaceMember(path, "contacts"); err != nil {
		t.Fatalf("removeWorkspaceMember (absent): %v", err)
	}
	got := readWorkspacesArray(t, path)
	if len(got) != 2 {
		t.Fatalf("expected 2 members unchanged, got %v", got)
	}
}

func TestStageReleaseLayout(t *testing.T) {
	appDir := t.TempDir()
	// Simulate an expo export output at <appDir>/dist.
	distDir := filepath.Join(appDir, "dist")
	if err := os.MkdirAll(filepath.Join(distDir, "_expo", "static"), 0o755); err != nil {
		t.Fatalf("mkdir dist: %v", err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "_expo", "static", "app.js"), []byte("//"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	stageDest, err := stageRelease(appDir)
	if err != nil {
		t.Fatalf("stageRelease: %v", err)
	}

	// Staged under <appDir>/release-staging/<id>/.
	stagingParent := filepath.Dir(stageDest)
	if stagingParent != filepath.Join(appDir, "release-staging") {
		t.Fatalf("expected stage under release-staging, got %s", stageDest)
	}

	// release-id.txt present and matches the dir name.
	idBytes, err := os.ReadFile(filepath.Join(stageDest, "release-id.txt"))
	if err != nil {
		t.Fatalf("read release-id.txt: %v", err)
	}
	if string(idBytes) != filepath.Base(stageDest) {
		t.Fatalf("release-id.txt %q != dir name %q", idBytes, filepath.Base(stageDest))
	}

	// index.html renamed to app.html; index.html gone.
	if _, err := os.Stat(filepath.Join(stageDest, "app.html")); err != nil {
		t.Fatalf("expected app.html, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(stageDest, "index.html")); !os.IsNotExist(err) {
		t.Fatalf("expected index.html removed, stat err = %v", err)
	}

	// Asset tree carried over.
	if _, err := os.Stat(filepath.Join(stageDest, "_expo", "static", "app.js")); err != nil {
		t.Fatalf("expected asset carried over, got %v", err)
	}

	// Original dist consumed (moved, not left behind).
	if _, err := os.Stat(distDir); !os.IsNotExist(err) {
		t.Fatalf("expected dist consumed, stat err = %v", err)
	}
}
