//go:build unix

package pkgbuild

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// The regression this function exists for: CopyDir must not try to reproduce
// the SOURCE's ownership on the destination. `cp -a` did, which fails with
// EINVAL inside the builder's user namespace (the pre-fetched member tree is
// root-owned and root is unmapped there). Asserting the destination is owned
// by the copying process — not the source uid — pins the behavior without
// needing a namespace to reproduce it in. Unix-only: syscall.Stat_t and uids
// are a POSIX concept.
func TestCopyDir_DoesNotPreserveOwnership(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: every uid is writable, so the assertion proves nothing")
	}
	src := t.TempDir()
	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}
	info, err := os.Stat(filepath.Join(dst, "f"))
	if err != nil {
		t.Fatal(err)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Skip("no stat_t on this platform")
	}
	if int(st.Uid) != os.Geteuid() {
		t.Errorf("copied file uid = %d, want the copying process's %d", st.Uid, os.Geteuid())
	}
}
