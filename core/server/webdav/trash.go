package webdav

import (
	"fmt"
	"os"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// Soft-delete support. When the Source carries a TrashConfig, a DAV DELETE
// stamps the feature's per-user trash state instead of destroying the record,
// and a trashed entry disappears from that user's DAV view (listings, Stat,
// reads, deletes) while remaining restorable from the feature's trash UI.
//
// The (parent, name) slot stays occupied — the record still exists, and
// drive's unique index enforces exactly that — so creates and moves onto a
// trashed name report a conflict, the same answer the web UI gives.

// isTrashedFor reports whether the record is in this user's trash.
func (fs *FileSystem) isTrashedFor(user *core.Record, record *core.Record) bool {
	tc := fs.src.Trash
	if tc == nil {
		return false
	}
	state, err := fs.trashState(user, record)
	if err != nil || state == nil {
		return false
	}
	return state.GetString(tc.TrashedAtField) != ""
}

// trashState fetches the user's state row for the record, nil when none exists.
func (fs *FileSystem) trashState(user *core.Record, record *core.Record) (*core.Record, error) {
	tc := fs.src.Trash
	states, err := fs.app.FindRecordsByFilter(tc.Collection,
		fmt.Sprintf("%s = {:item} && %s = {:user}", tc.ItemField, tc.UserField),
		"", 1, 0,
		map[string]any{"item": record.Id, "user": user.Id})
	if err != nil {
		return nil, err
	}
	if len(states) == 0 {
		return nil, nil
	}
	return states[0], nil
}

// trashedIDsFor returns the set of item ids in this user's trash — one query,
// for the listing path. Nil when trash is not configured.
func (fs *FileSystem) trashedIDsFor(user *core.Record) (map[string]bool, error) {
	tc := fs.src.Trash
	if tc == nil {
		return nil, nil
	}
	states, err := fs.app.FindRecordsByFilter(tc.Collection,
		fmt.Sprintf("%s = {:user} && %s != ''", tc.UserField, tc.TrashedAtField),
		"", 0, 0,
		map[string]any{"user": user.Id})
	if err != nil {
		return nil, err
	}
	ids := make(map[string]bool, len(states))
	for _, s := range states {
		ids[s.GetString(tc.ItemField)] = true
	}
	return ids, nil
}

// moveToTrash stamps (upserting if needed) the user's trash state for the
// record — the same write the feature's own trash action performs, so the
// entry lands on the trash screen with a restore path.
func (fs *FileSystem) moveToTrash(user *core.Record, record *core.Record) error {
	tc := fs.src.Trash
	state, err := fs.trashState(user, record)
	if err != nil {
		return err
	}
	if state == nil {
		collection, err := fs.app.FindCollectionByNameOrId(tc.Collection)
		if err != nil {
			return err
		}
		state = core.NewRecord(collection)
		state.Set(tc.ItemField, record.Id)
		state.Set(tc.UserField, user.Id)
	}
	state.Set(tc.TrashedAtField, time.Now().UTC().Format(time.RFC3339))
	return fs.app.Save(state)
}

// maskTrashed converts "in this user's trash" into not-found, so a trashed
// entry is as gone from the DAV view as a deleted one.
func (fs *FileSystem) maskTrashed(user *core.Record, record *core.Record) error {
	if fs.isTrashedFor(user, record) {
		return os.ErrNotExist
	}
	return nil
}
