package realtime

import (
	"bytes"
	"testing"
	"time"
)

func TestNoopCheckpointStoreLoadsNothing(t *testing.T) {
	var s NoopCheckpointStore
	if err := s.Save("k", "id", Checkpoint{Epoch: 1, State: []byte{1}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	_, found, err := s.Load("k", "id")
	if err != nil || found {
		t.Fatalf("Load after Save: found=%v err=%v; want not found", found, err)
	}
	if err := s.Delete("k", "id"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestMemoryCheckpointStoreRoundTrip(t *testing.T) {
	s := NewMemoryCheckpointStore()
	want := Checkpoint{Epoch: 7, Fingerprint: "fp-1", State: []byte{0, 1, 2, 255}}
	if err := s.Save("k", "id", want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, found, err := s.Load("k", "id")
	if err != nil || !found {
		t.Fatalf("Load: found=%v err=%v", found, err)
	}
	if got.Epoch != want.Epoch || got.Fingerprint != want.Fingerprint || !bytes.Equal(got.State, want.State) {
		t.Fatalf("Load = %+v; want %+v", got, want)
	}
	// A later Save replaces the row, and Delete is idempotent.
	if err := s.Save("k", "id", Checkpoint{Epoch: 8, Fingerprint: "fp-2", State: []byte{9}}); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	got, _, _ = s.Load("k", "id")
	if got.Epoch != 8 || got.Fingerprint != "fp-2" {
		t.Fatalf("second Load = %+v; want epoch 8 fp-2", got)
	}
	if _, found, _ := s.Load("k", "other"); found {
		t.Fatal("a different room id was found")
	}
	if err := s.Delete("k", "id"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := s.Delete("k", "id"); err != nil {
		t.Fatalf("second Delete: %v", err)
	}
	if _, found, _ := s.Load("k", "id"); found {
		t.Fatal("found after Delete")
	}
}

func TestMintEpochIsMonotonic(t *testing.T) {
	now := time.Now().UnixMilli()
	first := MintEpoch(0)
	if first < now {
		t.Fatalf("MintEpoch(0) = %d; want >= now %d", first, now)
	}
	// A previous epoch from the future (clock skew between processes) still
	// yields a strictly greater one, so a client always sees a change.
	future := now + 24*int64(time.Hour/time.Millisecond)
	if got := MintEpoch(future); got != future+1 {
		t.Fatalf("MintEpoch(future) = %d; want %d", got, future+1)
	}
}
