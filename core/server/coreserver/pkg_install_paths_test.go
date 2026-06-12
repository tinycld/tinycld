package coreserver

import (
	"os"
	"path/filepath"
	"testing"
)

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
