package automation

import (
	"io"
	"sync"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
)

// TriggerFile is one file attached to a trigger record through its host's
// files contract: Record is the file row, FileField its file column.
type TriggerFile struct {
	Field     string
	Name      string
	Size      int64
	Record    *core.Record
	FileField string
}

// FilesProvider lists a record's files. The forms package registers one for
// each host collection with a files contract, so core names no collection.
type FilesProvider func(app core.App, record *core.Record) ([]TriggerFile, error)

var (
	filesMu        sync.RWMutex
	filesProviders = map[string]FilesProvider{}
)

func RegisterFilesProvider(collection string, p FilesProvider) {
	filesMu.Lock()
	defer filesMu.Unlock()
	filesProviders[collection] = p
}

// TriggerFiles returns the files of a trigger record, or nil when its
// collection has no files.
func TriggerFiles(app core.App, record *core.Record) ([]TriggerFile, error) {
	if record == nil {
		return nil, nil
	}
	filesMu.RLock()
	p := filesProviders[record.Collection().Name]
	filesMu.RUnlock()
	if p == nil {
		return nil, nil
	}
	return p(app, record)
}

// ReadTriggerFile reads a trigger file into memory, ready to set on a new
// record's file field (an action copying a submission's upload to a card).
func ReadTriggerFile(app core.App, f TriggerFile) (*filesystem.File, error) {
	fsys, err := app.NewFilesystem()
	if err != nil {
		return nil, err
	}
	defer fsys.Close()
	key := f.Record.BaseFilesPath() + "/" + f.Record.GetString(f.FileField)
	r, err := fsys.GetReader(key)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return filesystem.NewFileFromBytes(b, f.Name)
}
