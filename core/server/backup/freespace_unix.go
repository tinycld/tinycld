//go:build !windows

package backup

import "syscall"

// statfsAvailable reads the free space a NON-privileged process may actually use
// (Bavail, not Bfree: the reserve blocks only root can touch are not ours).
//
// syscall rather than golang.org/x/sys: x/sys is only an indirect dependency of
// this module, and one Statfs call is not worth promoting it to a direct one.
func statfsAvailable(dir string) (int64, error) {
	var fs syscall.Statfs_t
	if err := syscall.Statfs(dir, &fs); err != nil {
		return 0, err
	}
	return int64(fs.Bavail) * int64(fs.Bsize), nil
}
