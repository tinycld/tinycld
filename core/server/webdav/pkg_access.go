package webdav

import (
	"os"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/pkgaccess"
)

// requirePkgWrite refuses the write when the caller's org_pkg_access level
// for this tree's owning package is not full. DAV bypasses the REST layer,
// where core's request-hook guard lives — without this check a readonly
// user's Finder mount could still upload, rename, and delete. Reads are
// untouched: readonly means read.
//
// os.ErrPermission, not not-found: the caller can SEE the tree, they just
// may not change it, and every write verb already surfaces ErrPermission as
// 403.
func (fs *FileSystem) requirePkgWrite(user *core.Record) error {
	if pkgaccess.CanWrite(fs.app, user, fs.src.Slug) {
		return nil
	}
	return os.ErrPermission
}
