package realtime

import (
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"

	"github.com/pocketbase/pocketbase/core"
)

// PocketBaseCheckpointStore keeps one row per room in the
// realtime_doc_checkpoints collection, in the same SQLite database as the
// rest of the app. The state is base64 in a text field, as the journal's
// rows were, so no file-typed field and no file storage round trip is
// involved. The collection has no API rules: the broker reads and writes it
// through the Go SDK only.
type PocketBaseCheckpointStore struct {
	app core.App

	colOnce sync.Once
	col     *core.Collection
	colErr  error
}

// NewPocketBaseCheckpointStore returns a store on app's
// realtime_doc_checkpoints collection, which the core migration creates.
func NewPocketBaseCheckpointStore(app core.App) *PocketBaseCheckpointStore {
	return &PocketBaseCheckpointStore{app: app}
}

func (s *PocketBaseCheckpointStore) collection() (*core.Collection, error) {
	s.colOnce.Do(func() {
		s.col, s.colErr = s.app.FindCollectionByNameOrId(CheckpointCollection)
	})
	return s.col, s.colErr
}

func (s *PocketBaseCheckpointStore) find(kind, id string) (*core.Record, error) {
	rec, err := s.app.FindFirstRecordByFilter(CheckpointCollection,
		"room_kind = {:k} && room_id = {:id}",
		map[string]any{"k": kind, "id": id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return rec, err
}

func (s *PocketBaseCheckpointStore) Load(kind, id string) (Checkpoint, bool, error) {
	rec, err := s.find(kind, id)
	if err != nil {
		return Checkpoint{}, false, fmt.Errorf("realtime checkpoint: load kind=%s id=%s: %w", kind, id, err)
	}
	if rec == nil {
		return Checkpoint{}, false, nil
	}
	state, err := base64.StdEncoding.DecodeString(rec.GetString("state"))
	if err != nil {
		return Checkpoint{}, false, fmt.Errorf("realtime checkpoint: decode kind=%s id=%s: %w", kind, id, err)
	}
	return Checkpoint{
		Epoch:       int64(rec.GetInt("epoch")),
		Fingerprint: rec.GetString("fingerprint"),
		State:       state,
	}, true, nil
}

func (s *PocketBaseCheckpointStore) Save(kind, id string, cp Checkpoint) error {
	col, err := s.collection()
	if err != nil {
		return fmt.Errorf("realtime checkpoint: load collection: %w", err)
	}
	rec, err := s.find(kind, id)
	if err != nil {
		return fmt.Errorf("realtime checkpoint: save lookup kind=%s id=%s: %w", kind, id, err)
	}
	if rec == nil {
		rec = core.NewRecord(col)
		rec.Set("room_kind", kind)
		rec.Set("room_id", id)
	}
	rec.Set("epoch", cp.Epoch)
	rec.Set("fingerprint", cp.Fingerprint)
	rec.Set("state", base64.StdEncoding.EncodeToString(cp.State))
	if err := s.app.Save(rec); err != nil {
		return fmt.Errorf("realtime checkpoint: save kind=%s id=%s: %w", kind, id, err)
	}
	return nil
}

func (s *PocketBaseCheckpointStore) Delete(kind, id string) error {
	rec, err := s.find(kind, id)
	if err != nil {
		return fmt.Errorf("realtime checkpoint: delete lookup kind=%s id=%s: %w", kind, id, err)
	}
	if rec == nil {
		return nil
	}
	if err := s.app.Delete(rec); err != nil {
		return fmt.Errorf("realtime checkpoint: delete kind=%s id=%s: %w", kind, id, err)
	}
	return nil
}

var _ CheckpointStore = (*PocketBaseCheckpointStore)(nil)
