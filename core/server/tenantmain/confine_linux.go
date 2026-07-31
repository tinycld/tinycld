//go:build linux

package tenantmain

import (
	"fmt"
	"syscall"
)

// confinePackages remounts the package store read-only inside this process's
// own mount namespace.
//
// It must be a bind mount at the SAME absolute path, not a chroot into the org
// directory: materialize fills pb_hooks/pb_migrations/pb_public with absolute
// symlinks pointing into <MT_ROOT>/packages, and a chroot would break every one
// of them. Read-only because the store is immutable and shared across orgs.
func confinePackages(dir string) error {
	// Detach our mount table from the host's so nothing we do here propagates
	// back out (or to another tenant).
	if err := syscall.Mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""); err != nil {
		return fmt.Errorf("make mounts private: %w", err)
	}
	if err := syscall.Mount(dir, dir, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("bind %s: %w", dir, err)
	}
	// The read-only flag only takes effect on a remount of an existing bind.
	const roFlags = syscall.MS_BIND | syscall.MS_REMOUNT | syscall.MS_RDONLY | syscall.MS_REC
	if err := syscall.Mount("", dir, "", roFlags, ""); err != nil {
		return fmt.Errorf("remount %s read-only: %w", dir, err)
	}
	return nil
}
