package webdav

import (
	"bytes"
	"io"
	"os"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
)

// seekableMemoryThreshold caps the size below which reads are buffered in
// memory; larger blobs spill to a temp file. Small enough to hold many
// concurrent in-flight downloads without OOM, large enough that typical office
// documents avoid the temp-file detour.
const seekableMemoryThreshold = 32 * 1024 * 1024 // 32 MiB

// writeBlobFromPath stores the file at path as a PocketBase file on the record
// and saves. Used by the write path so large uploads accumulated in a temp file
// are not re-buffered in RAM. The caller owns path: this does not rename, move
// or remove it on either success or failure.
func (fs *FileSystem) writeBlobFromPath(record *core.Record, path string) error {
	f, err := filesystem.NewFileFromPath(path)
	if err != nil {
		return err
	}

	record.Set(fs.src.Fields.File, f)
	record.Set(fs.src.Fields.Size, f.Size)

	return fs.app.Save(record)
}

// openSeekableBlob returns a seekable reader over a record's blob. Files at or
// below seekableMemoryThreshold are buffered fully in memory; larger ones spill
// to a temp file whose path is returned so the caller can remove it after
// Close. The returned reader is self-contained — the underlying blob reader and
// PocketBase filesystem session are closed before this returns.
func (fs *FileSystem) openSeekableBlob(record *core.Record) (rdr io.ReadSeekCloser, tmpPath string, err error) {
	filename := record.GetString(fs.src.Fields.File)
	if filename == "" {
		return nopReadSeekCloser{ReadSeeker: bytes.NewReader(nil)}, "", nil
	}

	fsys, err := fs.app.NewFilesystem()
	if err != nil {
		return nil, "", err
	}
	defer fsys.Close()

	key := record.BaseFilesPath() + "/" + filename
	src, err := fsys.GetReader(key)
	if err != nil {
		return nil, "", err
	}
	defer src.Close()

	if record.GetInt(fs.src.Fields.Size) <= seekableMemoryThreshold {
		buf, err := io.ReadAll(src)
		if err != nil {
			return nil, "", err
		}
		return nopReadSeekCloser{ReadSeeker: bytes.NewReader(buf)}, "", nil
	}

	tmp, err := os.CreateTemp("", "tinycld-dav-*")
	if err != nil {
		return nil, "", err
	}
	if _, err := io.Copy(tmp, src); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, "", err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, "", err
	}
	return tmp, tmp.Name(), nil
}

// nopReadSeekCloser wraps an io.ReadSeeker with a no-op Close.
type nopReadSeekCloser struct {
	io.ReadSeeker
}

func (nopReadSeekCloser) Close() error { return nil }
