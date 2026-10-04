//go:build unix

package supervise

import (
	"os"
	"syscall"
)

// matchOwner gives path the owner of ref. The supervisor runs as root while
// its children do not, so a file it writes in their place must keep the
// owner of the file it replaces, or the next child cannot write it. A
// supervisor that does not run as root changes nothing: what it writes is
// already its own, the same as its children's, and it may not give a file
// to another user (a directory it makes in a world-writable dir that another
// user owns would otherwise fail with EPERM).
func matchOwner(path string, ref os.FileInfo) error {
	if os.Geteuid() != 0 {
		return nil
	}
	want, ok := ref.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if have, ok := info.Sys().(*syscall.Stat_t); ok && have.Uid == want.Uid && have.Gid == want.Gid {
		return nil
	}
	return os.Chown(path, int(want.Uid), int(want.Gid))
}
