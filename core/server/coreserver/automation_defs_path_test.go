package coreserver

import (
	"os"
	"path/filepath"
	"testing"
)

// The generator writes automation_defs.json into the app's server/ dir. In
// the image (and bare metal, and an in-app rebuild) the binary sits one level
// up, at tinycld/tinycld, so resolveServerDir() is tinycld/ and the file is in
// tinycld/server/.
func TestAutomationDefsPathFindsTheGeneratorOutputInTheImageLayout(t *testing.T) {
	appDir := filepath.Join(t.TempDir(), "tinycld")
	want := filepath.Join(appDir, "server", "automation_defs.json")
	writeDefsFile(t, want)

	if got := automationDefsPath(appDir); got != want {
		t.Fatalf("automationDefsPath(%q) = %q, want %q", appDir, got, want)
	}
}

// In dev the binary is built into tinycld/server/ itself, so resolveServerDir()
// is already the generator's output dir.
func TestAutomationDefsPathFindsTheGeneratorOutputInTheDevLayout(t *testing.T) {
	serverDir := filepath.Join(t.TempDir(), "tinycld", "server")
	want := filepath.Join(serverDir, "automation_defs.json")
	writeDefsFile(t, want)

	if got := automationDefsPath(serverDir); got != want {
		t.Fatalf("automationDefsPath(%q) = %q, want %q", serverDir, got, want)
	}
}

func writeDefsFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"packages":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
}
