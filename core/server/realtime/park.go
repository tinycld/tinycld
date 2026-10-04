package realtime

import (
	"bytes"
	"context"
	"maps"
	"sync"
	"time"

	ycrdt "github.com/skyterra/y-crdt"

	"tinycld.org/core/readonly"
)

// parkedDoc is a server document whose room has emptied but which the
// broker keeps in memory, so a client that reconnects within ParkIdle (a
// solo editor's network blip, a laptop waking up, a tab reopened) lands on
// the SAME document incarnation. A rebuilt document would make that client
// discard its local state and every edit it typed while away.
type parkedDoc struct {
	handle      DocHandle
	epoch       int64
	fingerprint string
	parkedAt    time.Time
	opts        RoomKindOptions
}

// close runs the kind's OnEvict and closes the document. The parked entry
// itself is the caller's to remove.
func (p *parkedDoc) close(key roomKey) {
	if p.opts.OnEvict != nil {
		p.opts.OnEvict(key.id)
	}
	closeHandle(key, p.handle)
}

// park keeps a document after its room emptied and starts the janitor on
// the first one. A document parked under a key that already holds one
// (a reopen that raced a park) replaces it; the older one is closed.
func (b *Broker) park(key roomKey, p *parkedDoc) {
	b.mu.Lock()
	prev := b.parked[key]
	b.parked[key] = p
	b.mu.Unlock()
	if prev != nil {
		prev.close(key)
	}
	b.startJanitor()
}

// takeParked removes and returns the parked document for key. The caller
// holds b.mu: join calls it on the path that constructs a room.
func (b *Broker) takeParked(key roomKey) *parkedDoc {
	p := b.parked[key]
	if p != nil {
		delete(b.parked, key)
	}
	return p
}

func (b *Broker) startJanitor() {
	b.janitorOnce.Do(func() {
		interval := ParkIdle / 5
		if interval <= 0 {
			interval = time.Millisecond
		}
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-b.stop:
					return
				case <-ticker.C:
					b.evictIdle(time.Now())
				}
			}
		}()
	})
}

// evictIdle checkpoints and closes every parked document idle for ParkIdle
// or longer. While the server is read-only nothing is written, so the
// documents stay parked until a later tick; Suspend has already stored
// their state at Enter.
func (b *Broker) evictIdle(now time.Time) {
	if readonly.Active() {
		return
	}
	cutoff := now.Add(-ParkIdle)
	b.mu.Lock()
	var due []roomKey
	for key, p := range b.parked {
		if !p.parkedAt.After(cutoff) {
			due = append(due, key)
		}
	}
	victims := make(map[roomKey]*parkedDoc, len(due))
	for _, key := range due {
		victims[key] = b.parked[key]
		delete(b.parked, key)
	}
	parkedLeft := len(b.parked)
	b.mu.Unlock()

	for key, p := range victims {
		writeCheckpoint(key, p.opts, p.handle, p.epoch, p.fingerprint, true)
		p.close(key)
	}
	if len(victims) > 0 {
		log.Info("evicted idle parked documents", "evicted", len(victims), "parked", parkedLeft)
	}
}

// writeCheckpoint stores a document's state under the given fingerprint.
// With compact set (eviction), a state above CompactCheckpointBytes is
// dropped instead, so the next open re-seeds from the derived source and
// sheds the tombstones a gc-less document accumulates; nobody is connected
// at that point, so no client has to discard live state. A state above
// MaxCheckpointBytes is never stored.
func writeCheckpoint(key roomKey, opts RoomKindOptions, handle DocHandle, epoch int64, fingerprint string, compact bool) {
	state, err := handle.EncodeStateAsUpdate()
	if err != nil {
		log.Error("checkpoint skipped: encode failed", "kind", key.kind, "roomID", key.id, "err", err)
		return
	}
	if len(state) > MaxCheckpointBytes || (compact && len(state) > CompactCheckpointBytes) {
		if len(state) > MaxCheckpointBytes {
			log.Warn("document state is too large to checkpoint; it will re-seed at its next open",
				"kind", key.kind, "roomID", key.id, "bytes", len(state), "max", MaxCheckpointBytes)
		}
		if err := opts.Checkpoints.Delete(key.kind, key.id); err != nil {
			log.Warn("checkpoint delete failed", "kind", key.kind, "roomID", key.id, "err", err)
		}
		return
	}
	cp := Checkpoint{Epoch: epoch, Fingerprint: fingerprint, State: state}
	if err := opts.Checkpoints.Save(key.kind, key.id, cp); err != nil {
		log.Error("checkpoint save failed; the room will re-seed at its next open",
			"kind", key.kind, "roomID", key.id, "err", err)
	}
}

// restoreCheckpoint applies a stored state to an empty document and
// verifies the result. y-crdt logs and returns on malformed input instead
// of failing, so the apply error alone proves nothing; the document's
// state vector must equal the checkpoint's. Any panic in the decoders
// counts as a failed restore.
func restoreCheckpoint(handle DocHandle, state []byte) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("checkpoint restore panicked", "panic", r)
			ok = false
		}
	}()
	if err := handle.ApplyUpdate(state); err != nil {
		log.Error("checkpoint apply failed", "err", err)
		return false
	}
	applied, err := handle.EncodeStateAsUpdate()
	if err != nil {
		log.Error("checkpoint verify failed: encode", "err", err)
		return false
	}
	want := ycrdt.DecodeStateVector(ycrdt.EncodeStateVectorFromUpdate(state))
	got := ycrdt.DecodeStateVector(ycrdt.EncodeStateVectorFromUpdate(applied))
	return maps.Equal(want, got) && (len(want) > 0 || bytes.Equal(state, applied))
}

// Suspend flushes every dirty room and stores every open and parked
// document's state, for a process that is about to pause writes, drain,
// or exit. It runs on the shared broker; see Broker.Suspend.
func Suspend(ctx context.Context, reason string) {
	sharedBroker().Suspend(ctx, reason)
}

// Suspend is the process-event checkpoint. Order matters: each kind's
// FlushDirty runs first so the derived file is current, then the
// fingerprint is read and the state stored, so the row names the file it
// corresponds to. A failed flush leaves the previous file and its
// fingerprint in place, and the stored state (which holds the unflushed
// edits) still pairs with it. Rooms stay open and documents stay parked:
// if the process goes on, nothing changed; if it is replaced, the next one
// opens every room from these rows under the same epoch.
//
// It writes regardless of read-only mode because it IS the pause's own
// write, run from readonly.OnEnter before anything else proceeds.
func (b *Broker) Suspend(ctx context.Context, reason string) {
	for _, kind := range registeredKinds() {
		opts, err := optionsFor(kind)
		if err != nil || opts.Checkpoints == nil || opts.FlushDirty == nil {
			continue
		}
		if err := opts.FlushDirty(ctx); err != nil {
			log.Error("flush before suspend failed; checkpoints pair with the previous file",
				"kind", kind, "reason", reason, "err", err)
		}
	}

	type target struct {
		key    roomKey
		opts   RoomKindOptions
		handle DocHandle
		epoch  int64
	}
	b.mu.Lock()
	targets := make([]target, 0, len(b.rooms)+len(b.parked))
	for key, r := range b.rooms {
		if r.serverDoc != nil && r.opts.Checkpoints != nil && r.opts.Fingerprint != nil {
			targets = append(targets, target{key, r.opts, r.serverDoc, r.epoch})
		}
	}
	for key, p := range b.parked {
		targets = append(targets, target{key, p.opts, p.handle, p.epoch})
	}
	b.mu.Unlock()

	var wg sync.WaitGroup
	for _, t := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fp, err := t.opts.Fingerprint(t.key.id)
			if err != nil {
				log.Error("checkpoint skipped: fingerprint failed",
					"kind", t.key.kind, "roomID", t.key.id, "err", err)
				return
			}
			writeCheckpoint(t.key, t.opts, t.handle, t.epoch, fp, false)
		}()
	}
	wg.Wait()
	log.Info("suspended collaborative documents", "reason", reason, "checkpointed", len(targets))
}

// DropRoom forgets everything the broker holds for a room whose record is
// gone: it closes a parked document and deletes the stored checkpoint. A
// room with clients still in it is left alone; its own teardown follows
// when they leave, and the kind's Authorize refuses new joiners.
func DropRoom(kind, id string) error {
	return sharedBroker().DropRoom(kind, id)
}

func (b *Broker) DropRoom(kind, id string) error {
	key := roomKey{kind: kind, id: id}
	b.mu.Lock()
	p := b.takeParked(key)
	b.mu.Unlock()
	if p != nil {
		p.close(key)
	}
	opts, err := optionsFor(kind)
	if err != nil || opts.Checkpoints == nil {
		return nil
	}
	return opts.Checkpoints.Delete(kind, id)
}

// parkedCount reports how many documents are parked. Tests only.
func (b *Broker) parkedCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.parked)
}

// lookupParkedForTest returns the parked document for (kind, id), or nil.
func (b *Broker) lookupParkedForTest(kind, id string) *parkedDoc {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.parked[roomKey{kind, id}]
}
