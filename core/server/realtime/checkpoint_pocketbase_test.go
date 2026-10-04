package realtime

import (
	"bytes"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// setupCheckpointTestApp builds a TestApp with the realtime_doc_checkpoints
// collection. tests.NewTestApp runs Go-side PB migrations only, so the JS
// migration is mirrored here, including the unique (room_kind, room_id)
// index that makes Save an upsert.
func setupCheckpointTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	if _, err := app.FindCollectionByNameOrId(CheckpointCollection); err == nil {
		return app
	}
	col := core.NewBaseCollection(CheckpointCollection)
	col.Fields.Add(&core.TextField{Name: "room_kind", Required: true, Max: 64})
	col.Fields.Add(&core.TextField{Name: "room_id", Required: true, Max: 64})
	col.Fields.Add(&core.NumberField{Name: "epoch", Required: true, Min: ptrFloat(1), OnlyInt: true})
	col.Fields.Add(&core.TextField{Name: "fingerprint", Max: 512})
	col.Fields.Add(&core.TextField{Name: "state", Required: true, Max: checkpointStateFieldMax})
	col.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	col.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
	col.AddIndex("idx_realtime_doc_checkpoints_room", true, "room_kind, room_id", "")
	if err := app.Save(col); err != nil {
		t.Fatalf("create %s: %v", CheckpointCollection, err)
	}
	return app
}

func countCheckpointRows(t *testing.T, app core.App, kind, id string) int {
	t.Helper()
	rows, err := app.FindRecordsByFilter(CheckpointCollection,
		"room_kind = {:k} && room_id = {:id}", "", 0, 0,
		map[string]any{"k": kind, "id": id})
	if err != nil {
		t.Fatalf("FindRecordsByFilter: %v", err)
	}
	return len(rows)
}

func TestPocketBaseCheckpointSaveCreatesThenUpdates(t *testing.T) {
	app := setupCheckpointTestApp(t)
	s := NewPocketBaseCheckpointStore(app)
	if err := s.Save("text-doc", "room-1", Checkpoint{Epoch: 5, Fingerprint: "a", State: []byte{1}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Save("text-doc", "room-1", Checkpoint{Epoch: 5, Fingerprint: "b", State: []byte{2, 3}}); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	if n := countCheckpointRows(t, app, "text-doc", "room-1"); n != 1 {
		t.Fatalf("rows after two saves = %d; want 1", n)
	}
	got, found, err := s.Load("text-doc", "room-1")
	if err != nil || !found {
		t.Fatalf("Load: found=%v err=%v", found, err)
	}
	if got.Epoch != 5 || got.Fingerprint != "b" || !bytes.Equal(got.State, []byte{2, 3}) {
		t.Fatalf("Load = %+v; want the second save", got)
	}
}

func TestPocketBaseCheckpointLoadMissing(t *testing.T) {
	app := setupCheckpointTestApp(t)
	s := NewPocketBaseCheckpointStore(app)
	_, found, err := s.Load("text-doc", "nope")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if found {
		t.Fatal("found a row that was never saved")
	}
}

func TestPocketBaseCheckpointScopedToRoom(t *testing.T) {
	app := setupCheckpointTestApp(t)
	s := NewPocketBaseCheckpointStore(app)
	for _, r := range []struct{ kind, id string }{{"text-doc", "a"}, {"text-doc", "b"}, {"calc", "a"}} {
		if err := s.Save(r.kind, r.id, Checkpoint{Epoch: 1, State: []byte(r.kind + r.id)}); err != nil {
			t.Fatalf("Save %s/%s: %v", r.kind, r.id, err)
		}
	}
	if err := s.Delete("text-doc", "a"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, found, _ := s.Load("text-doc", "a"); found {
		t.Fatal("text-doc/a survived Delete")
	}
	got, found, _ := s.Load("calc", "a")
	if !found || string(got.State) != "calca" {
		t.Fatalf("calc/a = %+v found=%v; want its own state", got, found)
	}
	if _, found, _ := s.Load("text-doc", "b"); !found {
		t.Fatal("text-doc/b was deleted with text-doc/a")
	}
}

func TestPocketBaseCheckpointDeleteIdempotent(t *testing.T) {
	app := setupCheckpointTestApp(t)
	s := NewPocketBaseCheckpointStore(app)
	if err := s.Delete("text-doc", "never"); err != nil {
		t.Fatalf("Delete of a missing row: %v", err)
	}
	_ = s.Save("text-doc", "x", Checkpoint{Epoch: 1, State: []byte{1}})
	if err := s.Delete("text-doc", "x"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := s.Delete("text-doc", "x"); err != nil {
		t.Fatalf("second Delete: %v", err)
	}
}

func TestPocketBaseCheckpointStateRoundTripsBinary(t *testing.T) {
	app := setupCheckpointTestApp(t)
	s := NewPocketBaseCheckpointStore(app)
	state := make([]byte, 4096)
	for i := range state {
		state[i] = byte(i * 7)
	}
	if err := s.Save("text-doc", "bin", Checkpoint{Epoch: 3, State: state}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, found, err := s.Load("text-doc", "bin")
	if err != nil || !found {
		t.Fatalf("Load: found=%v err=%v", found, err)
	}
	if !bytes.Equal(got.State, state) {
		t.Fatal("state bytes changed in the round trip")
	}
}

func ptrFloat(v float64) *float64 { return &v }
