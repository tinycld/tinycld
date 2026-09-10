//go:build !windows

package tenantcfg

import (
	"os"
	"syscall"
)

// openRuntimeFileNoFollow opens a router-authored .runtime file with
// O_NOFOLLOW, so a symlink planted AT the destination by a hostile tenant
// fails the open rather than being written through. See WriteRuntimeFile for
// the threat model.
func openRuntimeFileNoFollow(path string, mode os.FileMode) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, mode)
}
