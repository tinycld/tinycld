package webdav

import (
	"os"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// pbTimeFormat is PocketBase's autodate string layout.
const pbTimeFormat = "2006-01-02 15:04:05.000Z"

// fileInfo is the os.FileInfo implementation returned by Stat and from Readdir
// on directories. The backing record (when non-nil) is returned via Sys() so
// callers can downcast without re-querying PocketBase — that is the seam a
// feature reaches through when it needs the row behind an entry.
type fileInfo struct {
	name    string
	size    int64
	modTime time.Time
	isDir   bool
	record  *core.Record
}

func (i *fileInfo) Name() string       { return i.name }
func (i *fileInfo) Size() int64        { return i.size }
func (i *fileInfo) ModTime() time.Time { return i.modTime }
func (i *fileInfo) IsDir() bool        { return i.isDir }

func (i *fileInfo) Sys() any {
	if i.record == nil {
		return nil
	}
	return i.record
}

func (i *fileInfo) Mode() os.FileMode {
	if i.isDir {
		return os.ModeDir | 0o755
	}
	return 0o644
}

// recordToFileInfo builds an os.FileInfo from a backing record, reading the
// fields the Source declared.
func (fs *FileSystem) recordToFileInfo(record *core.Record) *fileInfo {
	f := fs.src.Fields

	modTime := time.Time{}
	if f.Updated != "" {
		if updated := record.GetString(f.Updated); updated != "" {
			if t, err := time.Parse(pbTimeFormat, updated); err == nil {
				modTime = t
			}
		}
	}

	return &fileInfo{
		name:    record.GetString(f.Name),
		size:    int64(record.GetInt(f.Size)),
		modTime: modTime,
		isDir:   record.GetBool(f.IsFolder),
		record:  record,
	}
}
