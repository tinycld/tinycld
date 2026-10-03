package supervise

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeStagingRelease builds one release-staging/<id> dir the way the
// generator's install-time build produces, under the current build's
// release-staging dir. modTime lets a test control which staged release is
// "newest" (promote_release picks by mtime, not glob order).
func writeStagingRelease(t *testing.T, stagingDir, releaseID string, modTime time.Time, withManifest bool) string {
	t.Helper()
	dir := filepath.Join(stagingDir, releaseID)
	if err := os.MkdirAll(filepath.Join(dir, "_expo", "static"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "app.html"), "<html>"+releaseID+"</html>")
	mustWrite(t, filepath.Join(dir, "release-id.txt"), releaseID)
	mustWrite(t, filepath.Join(dir, "_expo", "static", "bundle.js"), "bundle-"+releaseID)
	mustWrite(t, filepath.Join(dir, "assets", "app-icon.png"), "icon-"+releaseID)
	if withManifest {
		mustWrite(t, filepath.Join(dir, "manifest.json"), `{"releaseID":"`+releaseID+`"}`)
	}
	if err := os.Chtimes(dir, modTime, modTime); err != nil {
		t.Fatal(err)
	}
	return dir
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestState_PromoteRelease(t *testing.T) {
	s := newTestState(t)
	buildDir := writeBuild(t, s, "build-1")
	pointCurrentAt(t, s, buildDir)
	stagingDir := filepath.Join(buildDir, "release-staging")

	now := time.Now()
	writeStagingRelease(t, stagingDir, "release-a", now, true)

	if err := s.PromoteRelease(); err != nil {
		t.Fatal(err)
	}

	releaseDir := filepath.Join(s.releasesDir(), "release-a")
	if got := mustRead(t, filepath.Join(releaseDir, "app.html")); got != "<html>release-a</html>" {
		t.Fatalf("app.html = %q", got)
	}
	if got := mustRead(t, filepath.Join(releaseDir, "release-id.txt")); got != "release-a" {
		t.Fatalf("release-id.txt = %q", got)
	}
	if got := mustRead(t, filepath.Join(releaseDir, "manifest.json")); got != `{"releaseID":"release-a"}` {
		t.Fatalf("manifest.json = %q", got)
	}

	pool := s.releaseStaticPoolDir()
	if got := mustRead(t, filepath.Join(pool, "_expo", "static", "bundle.js")); got != "bundle-release-a" {
		t.Fatalf("pool bundle.js = %q", got)
	}
	if got := mustRead(t, filepath.Join(pool, "assets", "app-icon.png")); got != "icon-release-a" {
		t.Fatalf("pool app-icon.png = %q", got)
	}

	link, err := os.Readlink(s.currentReleaseLinkPath())
	if err != nil {
		t.Fatal(err)
	}
	if link != "release-a" {
		t.Fatalf("releases/current -> %q, want release-a", link)
	}
}

func TestState_PromoteRelease_PicksNewestStagingDirByModTime(t *testing.T) {
	s := newTestState(t)
	buildDir := writeBuild(t, s, "build-1")
	pointCurrentAt(t, s, buildDir)
	stagingDir := filepath.Join(buildDir, "release-staging")

	older := time.Now().Add(-time.Hour)
	newer := time.Now()
	// Alphabetically "2026-base" would sort before "install-newer" under a
	// glob-order pick; mtime must win instead, matching entrypoint.sh's
	// `ls -1dt` note about in-app installs.
	writeStagingRelease(t, stagingDir, "2026-base", older, false)
	writeStagingRelease(t, stagingDir, "install-newer", newer, false)

	if err := s.PromoteRelease(); err != nil {
		t.Fatal(err)
	}

	link, err := os.Readlink(s.currentReleaseLinkPath())
	if err != nil {
		t.Fatal(err)
	}
	if link != "install-newer" {
		t.Fatalf("releases/current -> %q, want install-newer (newest by mtime)", link)
	}
}

func TestState_PromoteRelease_NoStagingDir(t *testing.T) {
	s := newTestState(t)
	buildDir := writeBuild(t, s, "build-1")
	pointCurrentAt(t, s, buildDir)
	// No release-staging dir created at all.

	if err := s.PromoteRelease(); err != nil {
		t.Fatalf("PromoteRelease() with no staging dir should not error: %v", err)
	}
	if _, err := os.Stat(s.currentReleaseLinkPath()); !os.IsNotExist(err) {
		t.Fatalf("releases/current should not be created, stat err = %v", err)
	}
}

func TestState_PromoteRelease_NoReleaseIDFile(t *testing.T) {
	s := newTestState(t)
	buildDir := writeBuild(t, s, "build-1")
	pointCurrentAt(t, s, buildDir)
	stagingDir := filepath.Join(buildDir, "release-staging", "broken")
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// No release-id.txt in the staged dir.

	if err := s.PromoteRelease(); err == nil {
		t.Fatal("PromoteRelease() should error when no staged dir carries release-id.txt")
	}
}

func TestState_PromoteRelease_AlreadyPromotedIsSkipped(t *testing.T) {
	s := newTestState(t)
	buildDir := writeBuild(t, s, "build-1")
	pointCurrentAt(t, s, buildDir)
	stagingDir := filepath.Join(buildDir, "release-staging")
	writeStagingRelease(t, stagingDir, "release-a", time.Now(), false)

	if err := s.PromoteRelease(); err != nil {
		t.Fatal(err)
	}
	// Mutate the already-promoted release dir to prove a second promote
	// run leaves a complete dir alone instead of re-copying over it.
	marker := filepath.Join(s.releasesDir(), "release-a", "app.html")
	mustWrite(t, marker, "<html>untouched</html>")

	if err := s.PromoteRelease(); err != nil {
		t.Fatal(err)
	}
	if got := mustRead(t, marker); got != "<html>untouched</html>" {
		t.Fatalf("already-promoted release dir should be left alone, got %q", got)
	}
}
