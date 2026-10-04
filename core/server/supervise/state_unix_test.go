//go:build unix

package supervise

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// otherOwner is a uid/gid other than root's that every unix test host has.
const otherOwner = 1

func requireRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("changing a file's owner needs root; this test runs only as root (the supervisor runs as root in the image)")
	}
}

func ownerOf(t *testing.T, path string) (uid, gid uint32) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	st := info.Sys().(*syscall.Stat_t)
	return st.Uid, st.Gid
}

func TestState_RestoreBackup_KeepsDataDBOwner(t *testing.T) {
	requireRoot(t)
	s := newTestState(t)
	armBackup(t, s, "build-3", []byte("snapshot-bytes"))
	mustWrite(t, s.dbPath(), "forward-migrated")
	if err := os.Chown(s.dbPath(), otherOwner, otherOwner); err != nil {
		t.Fatal(err)
	}

	if err := s.RestoreBackup(); err != nil {
		t.Fatal(err)
	}
	if uid, gid := ownerOf(t, s.dbPath()); uid != otherOwner || gid != otherOwner {
		t.Fatalf("restored data.db owner = %d:%d, want %d:%d", uid, gid, otherOwner, otherOwner)
	}
}

func TestState_PromoteRelease_ReplacedPoolFileKeepsOwner(t *testing.T) {
	requireRoot(t)
	s := newTestState(t)
	buildDir := writeBuild(t, s, "build-1")
	pointCurrentAt(t, s, buildDir)
	stagingDir := filepath.Join(buildDir, "release-staging")

	writeStagingRelease(t, stagingDir, "release-a", time.Now().Add(-time.Hour), false)
	if err := s.PromoteRelease(); err != nil {
		t.Fatal(err)
	}
	pooled := filepath.Join(s.releaseStaticPoolDir(), "_expo", "static", "bundle.js")
	if err := os.Chown(pooled, otherOwner, otherOwner); err != nil {
		t.Fatal(err)
	}

	writeStagingRelease(t, stagingDir, "release-b", time.Now(), false)
	if err := s.PromoteRelease(); err != nil {
		t.Fatal(err)
	}
	if got := mustRead(t, pooled); got != "bundle-release-b" {
		t.Fatalf("pool bundle.js = %q", got)
	}
	if uid, gid := ownerOf(t, pooled); uid != otherOwner || gid != otherOwner {
		t.Fatalf("replaced pool file owner = %d:%d, want %d:%d", uid, gid, otherOwner, otherOwner)
	}
}

func TestState_PromoteRelease_NewPoolFileTakesSourceOwner(t *testing.T) {
	requireRoot(t)
	s := newTestState(t)
	buildDir := writeBuild(t, s, "build-1")
	pointCurrentAt(t, s, buildDir)
	src := writeStagingRelease(t, filepath.Join(buildDir, "release-staging"), "release-a", time.Now(), false)
	if err := os.Chown(filepath.Join(src, "assets", "app-icon.png"), otherOwner, otherOwner); err != nil {
		t.Fatal(err)
	}

	if err := s.PromoteRelease(); err != nil {
		t.Fatal(err)
	}
	pooled := filepath.Join(s.releaseStaticPoolDir(), "assets", "app-icon.png")
	if uid, gid := ownerOf(t, pooled); uid != otherOwner || gid != otherOwner {
		t.Fatalf("new pool file owner = %d:%d, want the source's %d:%d", uid, gid, otherOwner, otherOwner)
	}
}

// The supervisor runs as root, so a directory it creates under releases/
// would be root-owned unless it takes its parent's owner.
func TestState_PromoteRelease_NewDirsTakeParentOwner(t *testing.T) {
	requireRoot(t)
	s := newTestState(t)
	buildDir := writeBuild(t, s, "build-1")
	pointCurrentAt(t, s, buildDir)
	src := writeStagingRelease(t, filepath.Join(buildDir, "release-staging"), "release-a", time.Now(), false)
	if err := os.MkdirAll(filepath.Join(src, "assets", "fonts"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(src, "assets", "fonts", "body.ttf"), "font")
	if err := os.Mkdir(s.releasesDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(s.releasesDir(), otherOwner, otherOwner); err != nil {
		t.Fatal(err)
	}

	if err := s.PromoteRelease(); err != nil {
		t.Fatal(err)
	}
	pool := s.releaseStaticPoolDir()
	for _, dir := range []string{
		pool,
		filepath.Join(pool, "_expo"),
		filepath.Join(pool, "_expo", "static"),
		filepath.Join(pool, "assets"),
		filepath.Join(pool, "assets", "fonts"),
		filepath.Join(s.releasesDir(), "release-a"),
	} {
		if uid, gid := ownerOf(t, dir); uid != otherOwner || gid != otherOwner {
			t.Errorf("%s owner = %d:%d, want releases/'s %d:%d", dir, uid, gid, otherOwner, otherOwner)
		}
	}
}
