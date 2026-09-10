//go:build windows

package tenantcfg

import (
	"fmt"
	"os"
)

// openRuntimeFileNoFollow is the Windows stand-in for the O_NOFOLLOW open.
//
// Windows has no O_NOFOLLOW. This exists so the single-binary SELF-HOST build
// cross-compiles for Windows — a self-hosted deployment is one org in one
// process and never writes these files at all (only the hosting router does,
// and it runs on Linux).
//
// The guard is therefore not weakened where it matters, but it cannot be
// reproduced faithfully here either: the Lstat below is a best effort with an
// unavoidable TOCTOU window, where the real O_NOFOLLOW is atomic. If the
// router is ever ported to Windows, this needs a real implementation
// (CreateFile with FILE_FLAG_OPEN_REPARSE_POINT) rather than this check.
func openRuntimeFileNoFollow(path string, mode os.FileMode) (*os.File, error) {
	if fi, err := os.Lstat(path); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("refusing to write %s: destination is a symlink", path)
	}
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
}
