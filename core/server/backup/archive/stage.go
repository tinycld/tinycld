package archive

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"tinycld.org/core/backup/arm"
	"tinycld.org/core/backup/format"
)

// Stage writes every member into dir. It accepts only the members a backup
// contains: an archive naming anything else is either a different format or an
// attempt to write outside the staging directory.
func Stage(r *format.Reader, dir string) error {
	for {
		hdr, body, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// A backup writes regular files and nothing else. Refusing every other
		// type here is the check that matters: a symlink member named
		// storage/x pointing at /etc or at the live pb_data would otherwise be
		// judged solely on where its own name lands, and a later write through
		// that name would leave the staging directory entirely.
		if hdr.Typeflag != tar.TypeReg {
			return fmt.Errorf("%w: member %q is not a regular file", format.ErrFormat, hdr.Name)
		}
		if hdr.Name != format.MemberDB && !strings.HasPrefix(hdr.Name, format.StoragePrefix) {
			return fmt.Errorf("%w: unexpected member %q", format.ErrFormat, hdr.Name)
		}
		target := filepath.Join(dir, filepath.FromSlash(hdr.Name))
		if !strings.HasPrefix(target, dir+string(os.PathSeparator)) {
			return fmt.Errorf("%w: member %q escapes the staging directory", format.ErrFormat, hdr.Name)
		}
		if err := arm.WriteMember(target, body); err != nil {
			return err
		}
	}
	return r.Verify()
}
