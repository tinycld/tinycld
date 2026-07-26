package webdav

import (
	"errors"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"

	"github.com/pocketbase/pocketbase/core"
	"golang.org/x/net/webdav"
)

// davFile is the webdav.File implementation. A single struct serves three modes
// — read, write, directory — selected by which fields are populated. The handler
// always Stats first and never writes after a read or vice versa, so a
// discriminated union over field presence is sufficient and keeps allocation
// trivial.
type davFile struct {
	fs   *FileSystem
	info *fileInfo

	// Read mode (file)
	rdSeeker io.ReadSeekCloser
	tmpPath  string // non-empty when rdSeeker is a temp file we own

	// Write mode (file)
	wrBuf    *os.File
	wrName   string
	parentID string
	user     *core.Record
	existing *core.Record

	// Directory mode
	children []os.FileInfo
	cursor   int

	closed bool
}

var _ webdav.File = (*davFile)(nil)

func (f *davFile) Stat() (os.FileInfo, error) {
	if f.info == nil {
		return nil, os.ErrInvalid
	}
	return f.info, nil
}

func (f *davFile) Read(p []byte) (int, error) {
	if f.rdSeeker == nil {
		// Directory or write-only handle — readers shouldn't reach here.
		return 0, io.EOF
	}
	return f.rdSeeker.Read(p)
}

func (f *davFile) Seek(offset int64, whence int) (int64, error) {
	if f.rdSeeker != nil {
		return f.rdSeeker.Seek(offset, whence)
	}
	if f.wrBuf != nil {
		return f.wrBuf.Seek(offset, whence)
	}
	return 0, errors.New("webdav: seek on non-seekable file handle")
}

func (f *davFile) Write(p []byte) (int, error) {
	if f.wrBuf == nil {
		return 0, os.ErrPermission
	}
	return f.wrBuf.Write(p)
}

func (f *davFile) Readdir(count int) ([]os.FileInfo, error) {
	if f.children == nil {
		return nil, errors.New("webdav: readdir on non-directory")
	}
	if f.cursor >= len(f.children) {
		if count <= 0 {
			return nil, nil
		}
		return nil, io.EOF
	}
	if count <= 0 {
		out := f.children[f.cursor:]
		f.cursor = len(f.children)
		return out, nil
	}
	end := min(f.cursor+count, len(f.children))
	out := f.children[f.cursor:end]
	f.cursor = end
	return out, nil
}

// Close finalizes whichever mode the file is in: write mode runs the
// persistence path, read mode releases the reader and any temp file, directory
// mode is a no-op.
func (f *davFile) Close() error {
	if f.closed {
		return nil
	}
	f.closed = true

	if f.rdSeeker != nil {
		err := f.rdSeeker.Close()
		if f.tmpPath != "" {
			_ = os.Remove(f.tmpPath)
		}
		return err
	}

	if f.wrBuf != nil {
		return f.persistWrite()
	}

	return nil
}

// persistWrite commits the accumulated bytes on Close. It owns one filesystem
// path at a time — either the original temp file or, after a successful rename,
// the renamed file — and the deferred cleanup always points at whichever one
// exists, so every failure path is leak-free.
//
// Why the rename: PocketBase's filesystem.NewFileFromPath derives the blob's
// stored Name from the source file's basename, and that Name is what a
// download endpoint serves as the Content-Disposition filename. The temp file
// is named "tinycld-dav-XXXX", so it is renamed to the user's intended
// filename in the same directory before being handed to NewFileFromPath.
func (f *davFile) persistWrite() error {
	tmpPath := f.wrBuf.Name()
	owned := tmpPath
	defer func() {
		if owned != "" {
			_ = os.Remove(owned)
		}
	}()

	if err := f.wrBuf.Sync(); err != nil {
		_ = f.wrBuf.Close()
		return err
	}
	if err := f.wrBuf.Close(); err != nil {
		return err
	}

	stat, err := os.Stat(tmpPath)
	if err != nil {
		return err
	}
	newSize := stat.Size()

	uploadPath := tmpPath
	if renamed := filepath.Join(filepath.Dir(tmpPath), filepath.Base(f.wrName)); renamed != tmpPath {
		if err := os.Rename(tmpPath, renamed); err != nil {
			return err
		}
		owned = renamed
		uploadPath = renamed
	}

	fs := f.fs
	fields := fs.src.Fields
	hooks := fs.src.Hooks

	if f.existing != nil {
		if hooks.CanWrite != nil {
			if err := hooks.CanWrite(fs.app, f.user.Id, f.existing.Id); err != nil {
				return err
			}
		}

		if hooks.CheckQuota != nil {
			oldSize := int64(f.existing.GetInt(fields.Size))
			if delta := newSize - oldSize; delta > 0 {
				if err := fs.wrapQuotaErr(hooks.CheckQuota(fs.app, f.user.Id, delta)); err != nil {
					return err
				}
			}
		}

		if hooks.BeforeOverwrite != nil && f.existing.GetString(fields.File) != "" {
			// Archiving the outgoing blob is best-effort: losing a version
			// snapshot must not fail the write the user actually asked for.
			if err := hooks.BeforeOverwrite(fs.app, f.user.Id, f.existing); err != nil {
				fs.app.Logger().Warn("webdav: BeforeOverwrite failed",
					"slug", fs.src.Slug, "id", f.existing.Id, "error", err)
			}
		}

		if err := fs.writeBlobFromPath(f.existing, uploadPath); err != nil {
			return err
		}
		f.info = fs.recordToFileInfo(f.existing)
		return nil
	}

	if hooks.CheckQuota != nil {
		if err := fs.wrapQuotaErr(hooks.CheckQuota(fs.app, f.user.Id, newSize)); err != nil {
			return err
		}
	}

	collection, err := fs.app.FindCollectionByNameOrId(fs.src.Collection)
	if err != nil {
		return err
	}

	record := core.NewRecord(collection)
	record.Set(fields.Name, f.wrName)
	record.Set(fields.IsFolder, false)
	record.Set(fields.Parent, f.parentID)
	record.Set(fields.Owner, f.user.Id)
	if fields.MimeType != "" {
		record.Set(fields.MimeType, guessMimeType(f.wrName))
	}

	if err := fs.writeBlobFromPath(record, uploadPath); err != nil {
		return err
	}

	f.info = fs.recordToFileInfo(record)
	return nil
}

// wrapQuotaErr shapes a quota rejection so callers can discriminate it.
//
// x/net/webdav maps PUT close errors to 405 unconditionally (webdav.go:285),
// so there is no clean way to surface 507 Insufficient Storage — the client
// sees 405 either way. The wrap makes errors.Is(err, os.ErrPermission) hold for
// any other call site that wants to tell quota from transport/IO failure, and
// preserves the human-readable message so it reaches the Logger callback rather
// than being discarded.
func (fs *FileSystem) wrapQuotaErr(err error) error {
	if err == nil {
		return nil
	}
	return &os.PathError{Op: "write", Err: errors.Join(os.ErrPermission, err)}
}

// guessMimeType derives a Content-Type from a basename's extension, falling
// back to application/octet-stream. Needed because x/net/webdav reads bytes
// only — no client Content-Type ever reaches the FileSystem.
func guessMimeType(filename string) string {
	if t := mime.TypeByExtension(path.Ext(filename)); t != "" {
		return t
	}
	return "application/octet-stream"
}
