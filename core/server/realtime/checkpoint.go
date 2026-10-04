package realtime

import (
	"sync"
	"time"
)

// Checkpoint is one room's full Yjs state, as EncodeStateAsUpdate returns it,
// together with the identity of the document incarnation it belongs to.
//
// A Yjs document rebuilt from the derived file (docx, xlsx, markdown) is a new
// incarnation: y-crdt mints a fresh clientID and the seed order is not stable,
// so items get new identities even when the content is identical. A client
// that still holds the previous incarnation then duplicates content when it
// merges, and its unsent edits reference items the server never had. The
// checkpoint keeps the identities, so a room reopened from it is the SAME
// document and a returning client merges as a no-op and resends only what
// the server lacks.
type Checkpoint struct {
	// Epoch names the incarnation. It is minted when a room is seeded from
	// the derived source and kept across every reopen from this state, so a
	// client compares it to detect a rebuild.
	Epoch int64
	// Fingerprint identifies the derived source the state corresponds to, as
	// the room kind's FingerprintFn reported it after the last flush. A room
	// opens from the checkpoint only when the source still has this identity;
	// a source changed outside the room (an upload, a version restore, a REST
	// edit) makes the checkpoint stale and the room re-seeds.
	Fingerprint string
	// State is the encoded document.
	State []byte
}

// CheckpointStore keeps one Checkpoint per (kind, id). Implementations must be
// safe for concurrent use.
type CheckpointStore interface {
	Load(kind, id string) (cp Checkpoint, found bool, err error)
	// Save replaces any checkpoint stored for the room.
	Save(kind, id string, cp Checkpoint) error
	// Delete removes the room's checkpoint. A missing row is not an error.
	Delete(kind, id string) error
}

// FingerprintFn reports the identity of the derived source a room of this kind
// is seeded from. Two equal values mean the source has not changed in between.
// It must be cheap: the broker reads it at every room open, park and
// checkpoint. For a drive-backed kind the stored file name is enough, because
// every save gets a fresh random suffix.
type FingerprintFn func(roomID string) (string, error)

// CheckpointCollection is the PocketBase collection the production store uses.
const CheckpointCollection = "realtime_doc_checkpoints"

// MaxCheckpointBytes bounds a state the broker will store. A state above it is
// not written: the room then re-seeds at its next open after an eviction or a
// restart, under a new epoch. The bound exists so one pathological document
// cannot write tens of megabytes on every pause.
const MaxCheckpointBytes = 24 << 20

// checkpointStateFieldMax is the collection's cap on the base64 text of the
// state: 4/3 of MaxCheckpointBytes, rounded up to a plain number that the JS
// migration repeats.
const checkpointStateFieldMax = 40_000_000

// CompactCheckpointBytes is the size above which an evicted room's state is
// dropped instead of stored. Documents are built without garbage collection,
// so every deletion leaves a tombstone and the encoded state only grows; a
// re-seed from the derived file is the compaction. It happens at eviction,
// when nobody is connected, so no client has to discard anything live.
const CompactCheckpointBytes = 4 << 20

// ParkIdle is how long a document stays in memory after its room empties
// before the janitor checkpoints and closes it. A var so tests can shrink it.
var ParkIdle = 30 * time.Minute

// MintEpoch returns the epoch for a freshly seeded room. It is the wall clock
// in milliseconds, but never at or below previous, so a client that knew the
// old incarnation always sees a change even across clock skew between
// processes.
func MintEpoch(previous int64) int64 {
	now := time.Now().UnixMilli()
	if now <= previous {
		return previous + 1
	}
	return now
}

// NoopCheckpointStore stores nothing. A kind that uses it never parks and
// re-seeds at every open, which is the pre-checkpoint behaviour.
type NoopCheckpointStore struct{}

func (NoopCheckpointStore) Load(kind, id string) (Checkpoint, bool, error) {
	return Checkpoint{}, false, nil
}
func (NoopCheckpointStore) Save(kind, id string, cp Checkpoint) error { return nil }
func (NoopCheckpointStore) Delete(kind, id string) error              { return nil }

// MemoryCheckpointStore is the in-process store tests use.
type MemoryCheckpointStore struct {
	mu   sync.Mutex
	rows map[roomKey]Checkpoint
}

func NewMemoryCheckpointStore() *MemoryCheckpointStore {
	return &MemoryCheckpointStore{rows: map[roomKey]Checkpoint{}}
}

func (s *MemoryCheckpointStore) Load(kind, id string) (Checkpoint, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp, ok := s.rows[roomKey{kind: kind, id: id}]
	if !ok {
		return Checkpoint{}, false, nil
	}
	cp.State = append([]byte(nil), cp.State...)
	return cp, true, nil
}

func (s *MemoryCheckpointStore) Save(kind, id string, cp Checkpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp.State = append([]byte(nil), cp.State...)
	s.rows[roomKey{kind: kind, id: id}] = cp
	return nil
}

func (s *MemoryCheckpointStore) Delete(kind, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.rows, roomKey{kind: kind, id: id})
	return nil
}

var (
	_ CheckpointStore = NoopCheckpointStore{}
	_ CheckpointStore = (*MemoryCheckpointStore)(nil)
)
