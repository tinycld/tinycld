//go:build unix

package supervise

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A supervisor that does not run as root can make a directory inside a
// world-writable one that another user owns (/tmp), but it cannot give the
// new directory that user's owner. The chown is only for a root supervisor,
// so the non-root one must make the directory and go on.
func TestMkdirAllOwned_NonRootUnderAnotherUsersDir(t *testing.T) {
	const shared = "/tmp"
	info, err := os.Stat(shared)
	if err != nil {
		t.Skipf("no %s: %v", shared, err)
	}
	if st := info.Sys().(*syscall.Stat_t); int(st.Uid) == os.Geteuid() {
		t.Skipf("%s is owned by this user; the test needs a parent owned by another user", shared)
	}
	top := filepath.Join(shared, fmt.Sprintf("supervise-owned-%d-%d", os.Getpid(), time.Now().UnixNano()))
	t.Cleanup(func() { _ = os.RemoveAll(top) })

	dir := filepath.Join(top, "sub")
	if err := mkdirAllOwned(dir); err != nil {
		t.Fatalf("mkdirAllOwned(%s): %v", dir, err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("stat %s: %v", dir, err)
	}
}
