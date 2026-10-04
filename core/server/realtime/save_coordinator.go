package realtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"

	"tinycld.org/core/readonly"
)

// ErrReadOnly is what FlushNow returns while the server is read-only: the
// flush writes, and a caller that needs the stored file current (a copy, an
// export) must not read a file the pause will not let us update.
var ErrReadOnly = errors.New("realtime: server is read-only")

// Default trigger-policy intervals. Tests construct a coordinator
// with shorter values to keep their wall-clock cost down.
//
// DefaultDebounceInterval: time of inactivity (no new MsgDocUpdate) after
// which a save fires. Roughly the "user paused typing → save"
// experience.
//
// DefaultCeilingInterval: maximum time the saver will defer a save during
// continuous editing. This is the worst-case durability window for
// active typing — server crash mid-edit loses up to this much work.
//
// DefaultTeardownTimeout: how long OnRoomEmpty waits for the final
// synchronous save to complete before giving up. Above this the
// broker continues teardown anyway; the save will retry from the
// in-memory state on the *next* room open if it ever happens, but
// otherwise the edits are lost. Pick a value generous enough that
// even a slow flush (e.g. serializing a large document and writing
// it to PocketBase) completes.
const (
	DefaultDebounceInterval = 3 * time.Second
	DefaultCeilingInterval  = 15 * time.Second
	DefaultTeardownTimeout  = 30 * time.Second
)

// DefaultMaxSaveAttempts caps how many consecutive times the coordinator
// retries a failing flush for one room before giving up the automatic
// retry loop. A deterministic flush failure (e.g. a document the .docx
// exporter structurally can't represent) would otherwise retry every
// 30s forever, burning a save attempt per room indefinitely and never
// surfacing the problem. After this many attempts the coordinator
// reports the failure to Sentry and stops scheduling retries; the room
// stays dirty so a subsequent edit re-arms the timers and tries again,
// which recovers transient failures without the runaway loop.
const DefaultMaxSaveAttempts = 8

// RetryBackoff returns the delay before retrying after a save failure
// number `attempt` (0-indexed). Caps at 30s.
func RetryBackoff(attempt int) time.Duration {
	switch attempt {
	case 0:
		return 1 * time.Second
	case 1:
		return 2 * time.Second
	case 2:
		return 4 * time.Second
	case 3:
		return 8 * time.Second
	case 4:
		return 16 * time.Second
	default:
		return 30 * time.Second
	}
}

// FlushFn is the unit of work the SaveCoordinator schedules: persist
// the current state of the room identified by driveItemID. Returning
// an error triggers an exponential-backoff retry; returning nil ends
// the dirty cycle (until the next OnDocUpdate flips it again).
//
// The ctx bounds the flush itself: document parsers honour it, so a
// pathological workbook is abandoned rather than wedging the saver.
// The teardown path passes a ctx carrying teardownTimeout; the other
// paths pass a background ctx, since a timer-driven save has no
// deadline of its own.
//
// Implementations must be safe to call concurrently with other
// methods on the same coordinator (the coordinator never invokes
// FlushFn for the same room twice in parallel — see saveInFlight).
type FlushFn func(ctx context.Context, driveItemID string, handle DocHandle) error

// SaveCoordinator owns the per-room debounce/ceiling/teardown state
// machine that drives persistence in package consumers. One instance
// per process, shared across all rooms of a given kind.
type SaveCoordinator struct {
	flush           FlushFn
	debounceEvery   time.Duration
	ceilingEvery    time.Duration
	teardownTimeout time.Duration
	maxAttempts     int
	logger          *slog.Logger

	// backoff maps a 0-indexed attempt number to the delay before the
	// next retry. Injectable so tests can collapse the wait. Defaults to
	// RetryBackoff.
	backoff func(attempt int) time.Duration

	// captureGiveUp reports a give-up to Sentry. Injectable so tests can
	// observe the give-up without hitting the real SDK. Defaults to
	// captureGiveUpToSentry.
	captureGiveUp func(detail giveUpDetail)

	mu    sync.Mutex
	rooms map[string]*roomSaver

	// kind names the room kind this coordinator drives, for logs and the
	// Sentry report of a give-up. Set via SetKind.
	kind string
}

// roomSaver is the per-room state. All access is guarded by mu.
type roomSaver struct {
	mu sync.Mutex

	handle DocHandle

	dirty         bool
	firstDirtyAt  time.Time
	debounceTimer *time.Timer
	ceilingTimer  *time.Timer

	saveInFlight bool
	resaveQueued bool
	failures     int

	// readOnlyDeferrals counts the flushes deferred in a row because the
	// server is read-only. It picks the next deferral's backoff and is kept
	// apart from failures: a deferral is expected, not a failed save, and
	// must not move the room toward giving up.
	readOnlyDeferrals int

	// closed flips true the moment OnRoomEmpty's synchronous flush
	// returns. After that, no more save attempts may be scheduled
	// for this room (the broker is releasing the DocHandle).
	closed bool
}

// NewSaveCoordinator returns a SaveCoordinator with production
// defaults. Override intervals (e.g. for tests) by mutating the
// returned value's fields before any room is created.
//
// flush is called whenever a save fires. It receives the room
// identifier (whatever ID space the room kind uses — for calc and
// text it's a drive_items.id; other kinds may key differently) and
// the DocHandle the broker handed us at room creation. A nil flush
// is a programmer error; the coordinator panics on first save attempt.
func NewSaveCoordinator(flush FlushFn) *SaveCoordinator {
	return &SaveCoordinator{
		flush:           flush,
		debounceEvery:   DefaultDebounceInterval,
		ceilingEvery:    DefaultCeilingInterval,
		teardownTimeout: DefaultTeardownTimeout,
		maxAttempts:     DefaultMaxSaveAttempts,
		logger:          slog.Default(),
		backoff:         RetryBackoff,
		captureGiveUp:   captureGiveUpToSentry,
		rooms:           map[string]*roomSaver{},
	}
}

// SetLogger swaps the slog.Logger the coordinator uses for failure
// reporting. Tests use a discarding logger to keep output clean.
func (c *SaveCoordinator) SetLogger(l *slog.Logger) {
	c.logger = l
}

// SetKind names the room kind this coordinator drives, so a give-up
// report and the logs say which kind's flush failed.
func (c *SaveCoordinator) SetKind(kind string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.kind = kind
}

// OnRoomCreate is the realtime.RoomKindOptions.OnRoomCreate hook.
// Records the room's DocHandle so we can pass it to flush later.
// The room argument is unused here — SaveCoordinator only needs the
// handle — but it's part of the hook signature.
func (c *SaveCoordinator) OnRoomCreate(driveItemID string, handle DocHandle, _ *Room) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.rooms[driveItemID]; ok {
		// Should never happen — the broker only fires create
		// once per (kind, id). Log and overwrite anyway so the
		// process keeps running.
		c.logger.Warn("realtime: OnRoomCreate fired twice for the same room; overwriting", "driveItemID", driveItemID)
		existing.mu.Lock()
		existing.closed = true
		existing.mu.Unlock()
	}
	c.rooms[driveItemID] = &roomSaver{handle: handle}
}

// OnDocUpdate is the realtime.RoomKindOptions.OnDocUpdate hook.
// Marks the room dirty and arms the debounce/ceiling timers. Cheap;
// safe to call from the broker route path.
func (c *SaveCoordinator) OnDocUpdate(driveItemID string) {
	c.mu.Lock()
	rs := c.rooms[driveItemID]
	c.mu.Unlock()
	if rs == nil {
		// MsgDocUpdate arrived for a room we didn't see created.
		// Either NewDoc failed (and the broker fell back to pure
		// relay — no server doc to save) or OnRoomCreate hasn't
		// run yet. Either way, we have nothing to do.
		return
	}

	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.closed {
		return
	}

	now := time.Now()
	// Arm the ceiling timer on the first edit of a clean cycle. The
	// debounce timer is reset on every edit; the ceiling fires
	// unconditionally after ceilingEvery so a constant typist still gets
	// a save. Also armed when a room is dirty with no ceiling: a room left
	// dirty by a failed or deferred save has none, and without one a
	// constant typist would keep resetting the debounce and never save.
	if !rs.dirty || rs.ceilingTimer == nil {
		if rs.ceilingTimer != nil {
			rs.ceilingTimer.Stop()
		}
		rs.ceilingTimer = time.AfterFunc(c.ceilingEvery, func() {
			c.triggerSave(driveItemID, "ceiling")
		})
	}
	if !rs.dirty {
		rs.dirty = true
		rs.firstDirtyAt = now
	}
	// Reset debounce on every edit.
	if rs.debounceTimer != nil {
		rs.debounceTimer.Stop()
	}
	rs.debounceTimer = time.AfterFunc(c.debounceEvery, func() {
		c.triggerSave(driveItemID, "debounce")
	})
}

// OnRoomEmpty is the realtime.RoomKindOptions.OnEmpty hook. Waits out
// any in-flight timer-driven save, then fires the final synchronous
// save if the room is (still) dirty. Returns when (a) the save
// finished or (b) teardownTimeout elapsed — measured across both waits
// — and in both cases the broker is free to close the DocHandle.
func (c *SaveCoordinator) OnRoomEmpty(driveItemID string) {
	c.mu.Lock()
	rs := c.rooms[driveItemID]
	if rs != nil {
		delete(c.rooms, driveItemID)
	}
	c.mu.Unlock()
	if rs == nil {
		return
	}

	deadline := time.Now().Add(c.teardownTimeout)

	// Wait out any in-flight timer-driven save BEFORE reading dirty.
	// triggerSave clears dirty before running its flush unlocked, so a
	// bare read here mistakes "being saved right now" for "already
	// saved" — and the caller tears the room down on our return, closing
	// the DocHandle under the running flush. Worse, that flush may FAIL
	// and re-mark dirty, which only a post-wait read can observe; the
	// retry it schedules will find the room already deregistered and
	// silently do nothing, so this final flush is the edit's last chance
	// to reach durable storage.
	rs.mu.Lock()
	for rs.saveInFlight {
		rs.mu.Unlock()
		if time.Now().After(deadline) {
			c.logger.Error("realtime: teardown timed out waiting for an in-flight save",
				"driveItemID", driveItemID, "timeout", c.teardownTimeout)
			rs.mu.Lock()
			rs.closed = true
			rs.mu.Unlock()
			return
		}
		time.Sleep(5 * time.Millisecond)
		rs.mu.Lock()
	}
	// Still holding rs.mu from the loop's final check: the in-flight
	// save (if any) has fully finished — including arming any retry
	// timer — so stopping timers and reading dirty here races nothing.
	if rs.debounceTimer != nil {
		rs.debounceTimer.Stop()
		rs.debounceTimer = nil
	}
	if rs.ceilingTimer != nil {
		rs.ceilingTimer.Stop()
		rs.ceilingTimer = nil
	}
	wasDirty := rs.dirty
	rs.dirty = false
	handle := rs.handle
	rs.mu.Unlock()

	if wasDirty && readonly.Active() {
		// The flush writes. The broker parks the document with these
		// edits in it, and Suspend has already stored its state for the
		// case where this process is replaced.
		c.logger.Info("realtime: teardown save skipped: read-only; the parked document keeps the edits",
			"driveItemID", driveItemID)
		wasDirty = false
	}

	if !wasDirty {
		// Nothing to save; mark closed and return immediately.
		rs.mu.Lock()
		rs.closed = true
		rs.mu.Unlock()
		return
	}

	done := make(chan error, 1)
	flushCtx, cancelFlush := context.WithTimeout(context.Background(), c.teardownTimeout)
	defer cancelFlush()
	go func() {
		done <- c.flush(flushCtx, driveItemID, handle)
	}()
	select {
	case err := <-done:
		if err != nil {
			c.logger.Error("realtime: teardown save failed", "driveItemID", driveItemID, "err", err)
		}
	case <-time.After(time.Until(deadline)):
		// The deadline is shared with the in-flight wait above, so a
		// teardown never stalls the broker longer than teardownTimeout
		// in total.
		c.logger.Error("realtime: teardown save timed out", "driveItemID", driveItemID, "timeout", c.teardownTimeout)
	}

	rs.mu.Lock()
	rs.closed = true
	rs.mu.Unlock()
}

// triggerSave is the central save scheduler. Runs on a timer
// goroutine (or recursively via resave) and is therefore concurrent
// with OnDocUpdate calls.
func (c *SaveCoordinator) triggerSave(driveItemID, reason string) {
	c.mu.Lock()
	rs := c.rooms[driveItemID]
	c.mu.Unlock()
	if rs == nil {
		// Room torn down between timer fire and dispatch. The
		// teardown path runs the final save itself.
		return
	}

	rs.mu.Lock()
	if rs.closed {
		rs.mu.Unlock()
		return
	}
	if rs.saveInFlight {
		// Coalesce: just mark the queue and let the in-flight
		// save scheduler pick this up when it finishes.
		rs.resaveQueued = true
		rs.mu.Unlock()
		return
	}
	if !rs.dirty {
		// Nothing to flush — possibly an awareness storm, or a
		// debounce/ceiling fire that raced with a teardown.
		rs.mu.Unlock()
		return
	}
	if readonly.Active() {
		c.deferForReadOnly(driveItemID, rs)
		rs.mu.Unlock()
		return
	}
	rs.readOnlyDeferrals = 0
	rs.saveInFlight = true
	rs.dirty = false
	if rs.debounceTimer != nil {
		rs.debounceTimer.Stop()
		rs.debounceTimer = nil
	}
	if rs.ceilingTimer != nil {
		rs.ceilingTimer.Stop()
		rs.ceilingTimer = nil
	}
	handle := rs.handle
	rs.mu.Unlock()

	err := c.flush(context.Background(), driveItemID, handle)

	rs.mu.Lock()
	rs.saveInFlight = false

	if err != nil {
		// Save failed: re-mark dirty so the work isn't forgotten.
		rs.dirty = true
		rs.failures++

		if rs.failures >= c.maxAttempts {
			// Give up the automatic retry loop. A deterministic
			// failure (e.g. a doc the exporter can't represent)
			// would otherwise retry forever. We DON'T clear dirty:
			// a future OnDocUpdate re-arms the timers and retries,
			// recovering transient failures. We just stop the
			// self-perpetuating loop and report it.
			detail := giveUpDetail{
				DriveItemID: driveItemID,
				Kind:        c.kind,
				Reason:      reason,
				Attempts:    rs.failures,
				Err:         err,
			}
			if rs.debounceTimer != nil {
				rs.debounceTimer.Stop()
				rs.debounceTimer = nil
			}
			rs.mu.Unlock()
			c.logger.Error("realtime: save failed; giving up after max attempts",
				"driveItemID", driveItemID, "kind", c.kind, "reason", reason,
				"attempts", detail.Attempts, "err", err)
			if c.captureGiveUp != nil {
				c.captureGiveUp(detail)
			}
			return
		}

		// Schedule a retry with exponential backoff. The next
		// OnDocUpdate would also re-arm timers, but we want to retry
		// even if no further edits arrive.
		backoff := c.backoff(rs.failures - 1)
		c.logger.Warn("realtime: save failed; scheduling retry",
			"driveItemID", driveItemID, "reason", reason,
			"attempt", rs.failures, "backoff", backoff, "err", err)
		// Use the debounce slot for the retry timer; if the
		// user types in the meantime, OnDocUpdate will reset
		// it to its normal debounce window.
		if rs.debounceTimer != nil {
			rs.debounceTimer.Stop()
		}
		rs.debounceTimer = time.AfterFunc(backoff, func() {
			c.triggerSave(driveItemID, "retry")
		})
		rs.mu.Unlock()
		return
	}

	// Success.
	rs.failures = 0
	resave := rs.resaveQueued
	rs.resaveQueued = false
	rs.mu.Unlock()

	if resave {
		// Edits arrived during the in-flight save; immediately
		// re-fire so they don't sit until the next debounce.
		go c.triggerSave(driveItemID, "coalesced")
	}
}

// deferForReadOnly re-arms the room's save for later, without flushing,
// because the flush writes. The room stays dirty and the document keeps
// the edits, so nothing is lost while the save waits. Logged once per
// pause at Info: a deferral is expected, so it is neither a failure nor a
// warning. Caller holds rs.mu.
func (c *SaveCoordinator) deferForReadOnly(driveItemID string, rs *roomSaver) {
	backoff := c.backoff(rs.readOnlyDeferrals)
	if rs.readOnlyDeferrals == 0 {
		c.logger.Info("realtime: save deferred: read-only",
			"driveItemID", driveItemID, "retryIn", backoff)
	}
	rs.readOnlyDeferrals++
	if rs.debounceTimer != nil {
		rs.debounceTimer.Stop()
	}
	if rs.ceilingTimer != nil {
		rs.ceilingTimer.Stop()
		rs.ceilingTimer = nil
	}
	rs.debounceTimer = time.AfterFunc(backoff, func() {
		c.triggerSave(driveItemID, "read-only retry")
	})
}

// FlushNow runs a synchronous flush for the room identified by
// driveItemID and returns only once the flush has completed (or failed).
// Unlike the timer-driven triggerSave, it ignores the debounce/ceiling
// schedule and the dirty flag — callers use it to guarantee the durable
// blob reflects live edits before reading it (e.g. "Export as template"
// and "Make a copy", which copy the stored drive_items.file).
//
// If the room is not open (no live editor since the last flush), the
// stored blob is already current, so this is a no-op success. While the
// server is read-only it returns ErrReadOnly without flushing.
func (c *SaveCoordinator) FlushNow(driveItemID string) error {
	if readonly.Active() {
		return ErrReadOnly
	}
	return c.flushRoom(context.Background(), driveItemID, false)
}

// FlushDirty flushes every dirty room now and returns the errors joined.
// It ignores read-only mode: the broker's Suspend calls it from inside the
// pause's own entry, so the stored file is current before the state is
// checkpointed. Rooms flush concurrently, bounded by ctx.
func (c *SaveCoordinator) FlushDirty(ctx context.Context) error {
	c.mu.Lock()
	ids := make([]string, 0, len(c.rooms))
	for id := range c.rooms {
		ids = append(ids, id)
	}
	c.mu.Unlock()

	errs := make([]error, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = c.flushRoom(ctx, id, true)
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}

// flushRoom claims the room's in-flight slot and runs one flush. With
// onlyIfDirty set, a room that is clean once the slot is claimed (a save
// in flight just cleaned it) is left alone.
//
// If a timer-driven save is already in flight for the room, this waits
// for it to finish rather than running a second concurrent flush — the
// coordinator never invokes FlushFn for the same room twice in parallel.
// It spins with a short sleep rather than a condition variable to keep
// the existing lock discipline; flushes are infrequent and brief.
func (c *SaveCoordinator) flushRoom(ctx context.Context, driveItemID string, onlyIfDirty bool) error {
	c.mu.Lock()
	rs := c.rooms[driveItemID]
	c.mu.Unlock()
	if rs == nil {
		// No open room: the last flush (or teardown) already wrote the
		// current state to durable storage. Nothing to do.
		return nil
	}
	for {
		rs.mu.Lock()
		if rs.closed {
			rs.mu.Unlock()
			return nil
		}
		if rs.saveInFlight {
			rs.mu.Unlock()
			if err := ctx.Err(); err != nil {
				return err
			}
			time.Sleep(5 * time.Millisecond)
			continue
		}
		if onlyIfDirty && !rs.dirty {
			rs.mu.Unlock()
			return nil
		}
		rs.saveInFlight = true
		rs.dirty = false
		handle := rs.handle
		rs.mu.Unlock()

		err := c.flush(ctx, driveItemID, handle)

		rs.mu.Lock()
		rs.saveInFlight = false
		if err != nil {
			// Leave the room marked dirty so the normal retry path picks
			// it up; this path doesn't retry — it reports the error to its
			// caller, who decides whether to proceed.
			rs.dirty = true
		}
		rs.mu.Unlock()
		return err
	}
}

// giveUpDetail is the full set of facts captured when the coordinator
// abandons the automatic retry loop for a room. Everything here goes to
// Sentry so an operator can identify the stuck document and the failing
// flush without correlating log lines by hand.
type giveUpDetail struct {
	DriveItemID string
	Kind        string
	Reason      string
	Attempts    int
	Err         error
}

// captureGiveUpToSentry reports a save give-up to Sentry as an exception,
// tagged and contextualized with the room identity and retry state.
// Capturing the underlying flush error (rather than a synthetic message)
// preserves Sentry's grouping by error value, so all give-ups sharing a
// root cause (e.g. one unrepresentable document) collapse into a single
// issue. No-op when the Sentry SDK has no DSN configured (dev).
func captureGiveUpToSentry(d giveUpDetail) {
	hub := sentry.CurrentHub().Clone()
	hub.WithScope(func(scope *sentry.Scope) {
		scope.SetLevel(sentry.LevelError)
		scope.SetTag("realtime.give_up", "true")
		scope.SetTag("realtime.kind", d.Kind)
		scope.SetTag("realtime.drive_item_id", d.DriveItemID)
		scope.SetContext("realtime_save", map[string]any{
			"driveItemID": d.DriveItemID,
			"kind":        d.Kind,
			"reason":      d.Reason,
			"attempts":    d.Attempts,
			"error":       fmt.Sprintf("%v", d.Err),
		})
		if d.Err != nil {
			hub.CaptureException(d.Err)
		} else {
			hub.CaptureMessage(fmt.Sprintf(
				"realtime: gave up saving %s/%s after %d attempts",
				d.Kind, d.DriveItemID, d.Attempts))
		}
	})
}
