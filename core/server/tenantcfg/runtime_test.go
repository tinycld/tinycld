package tenantcfg

import (
	"os"
	"path/filepath"
	"testing"
)

// A hostile tenant owns its .runtime tree (chownTree hands it over every
// spawn). WriteRuntimeFile must refuse to follow a symlink planted either AT
// the destination file or AS the .runtime directory, so the router's
// root-authored write cannot be redirected onto an arbitrary host path.
func TestWriteRuntimeFile_RefusesSymlinkedDestination(t *testing.T) {
	orgDir := t.TempDir()
	runtimeDir := filepath.Join(orgDir, ".runtime")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// The tenant plants a symlink where the router will write, pointing at a
	// file outside the org dir.
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(runtimeDir, "app.json")); err != nil {
		t.Fatal(err)
	}

	if _, err := WriteRuntimeFile(orgDir, "app.json", []byte("router-authored"), 0o644); err == nil {
		t.Fatal("write through a symlinked destination succeeded; O_NOFOLLOW guard missing")
	}
	if got, _ := os.ReadFile(victim); string(got) != "original" {
		t.Fatalf("victim file was overwritten through the symlink: %q", got)
	}
}

func TestWriteRuntimeFile_RefusesSymlinkedRuntimeDir(t *testing.T) {
	orgDir := t.TempDir()
	// The tenant replaces .runtime itself with a symlink to another directory.
	elsewhere := t.TempDir()
	if err := os.Symlink(elsewhere, filepath.Join(orgDir, ".runtime")); err != nil {
		t.Fatal(err)
	}

	if _, err := WriteRuntimeFile(orgDir, "quota.json", []byte("x"), 0o644); err == nil {
		t.Fatal("write into a symlinked .runtime succeeded; the dir guard is missing")
	}
	if _, err := os.Stat(filepath.Join(elsewhere, "quota.json")); !os.IsNotExist(err) {
		t.Fatalf("file landed in the symlink target (err=%v)", err)
	}
}

func TestWriteRuntimeFile_WritesRegularFile(t *testing.T) {
	orgDir := t.TempDir()
	path, err := WriteRuntimeFile(orgDir, "app.json", []byte("ok"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "ok" {
		t.Fatalf("content = %q", got)
	}
	// A second write truncates the existing regular file (no symlink refusal
	// for the legitimate re-materialize case).
	if _, err := WriteRuntimeFile(orgDir, "app.json", []byte("updated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "updated" {
		t.Fatalf("content after rewrite = %q", got)
	}
}
