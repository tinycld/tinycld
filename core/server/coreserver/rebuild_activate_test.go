package coreserver

import (
	"os"
	"path/filepath"
	"testing"
)

func mkBuild(t *testing.T, state, id string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(state, "builds", id, "tinycld"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestActivateBuild_FlipsSymlink(t *testing.T) {
	state := t.TempDir()
	t.Setenv("TINYCLD_STATE_DIR", state)
	mkBuild(t, state, "build-1")
	mkBuild(t, state, "build-2")

	if err := activateBuild("build-1"); err != nil {
		t.Fatal(err)
	}
	if got := currentBuildID(); got != "build-1" {
		t.Fatalf("current = %s, want build-1", got)
	}

	// Re-activate to a different build; symlink moves + previous recorded.
	if err := activateBuild("build-2"); err != nil {
		t.Fatal(err)
	}
	if got := currentBuildID(); got != "build-2" {
		t.Fatalf("current = %s, want build-2", got)
	}
	prev, err := os.ReadFile(filepath.Join(state, ".previous-build"))
	if err != nil {
		t.Fatalf(".previous-build not written: %v", err)
	}
	if string(prev) != "build-1" {
		t.Fatalf(".previous-build = %q, want build-1", prev)
	}
}

func TestPruneBuilds_KeepsCurrentAndNewest(t *testing.T) {
	state := t.TempDir()
	t.Setenv("TINYCLD_STATE_DIR", state)
	for _, id := range []string{"build-1", "build-2", "build-3", "build-4"} {
		mkBuild(t, state, id)
	}
	if err := activateBuild("build-2"); err != nil {
		t.Fatal(err)
	}
	if err := pruneBuilds(2); err != nil {
		t.Fatal(err)
	}
	// build-2 (current) must survive even though it's not in the newest 2.
	if _, err := os.Stat(filepath.Join(state, "builds", "build-2")); err != nil {
		t.Fatal("pruned the current build")
	}
	// newest 2 survive.
	for _, id := range []string{"build-3", "build-4"} {
		if _, err := os.Stat(filepath.Join(state, "builds", id)); err != nil {
			t.Fatalf("pruned newest build %s", id)
		}
	}
	// build-1 (oldest, not current) is gone.
	if _, err := os.Stat(filepath.Join(state, "builds", "build-1")); !os.IsNotExist(err) {
		t.Fatal("build-1 should have been pruned")
	}
}
