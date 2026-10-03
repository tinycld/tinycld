//go:build unix

package supervise

import (
	"os"
	"syscall"
)

// matchOwner gives path the owner of ref. The supervisor runs as root while
// its children do not, so a file it writes in their place must keep the
// owner of the file it replaces, or the next child cannot write it. Nothing
// is changed when the owners already match, so a non-root supervisor never
// needs a chown it may not make.
func matchOwner(path string, ref os.FileInfo) error {
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
