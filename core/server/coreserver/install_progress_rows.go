package coreserver

import (
	"context"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/readonly"
)

// This file replaces the SSE event stream as the way a client learns an
// install/upgrade job's progress. The stream dies at the server restart that
// ends every successful apply (the new process has no in-memory job), which
// forced the client onto a token-in-the-URL EventSource plus a durable-poll
// fallback for the restart seam — leaking an admin credential into server
// logs. Saving progress onto the pkg_install_log row instead means a client
// watching the row with a live query gets the same update through one path,
// before AND after the restart (pbtsdb reconnects and reloads live queries
// once the new process is back).
//
// ProgressStep is one headline milestone — NOT pkgbuild's detail log lines
// (those stay in `log`, written once at finalize). Keeping only milestones on
// the live row bounds its size regardless of how chatty a build gets.
type ProgressStep struct {
	Step         string `json:"step"`
	Progress     int    `json:"progress"`
	Message      string `json:"message"`
	StepProgress *int   `json:"stepProgress,omitempty"`
}

// progressThrottle is ~1 save/sec — frequent enough that a client watching
// the row sees smooth progress, far below PocketBase's write capacity for a
// single row, and low enough that a build emitting dozens of milestones a
// second (the fast early assemble steps) doesn't turn into dozens of writes.
const progressThrottle = time.Second

// afterFunc is the subset of time.AfterFunc the saver needs, indirected so
// tests can inject a deterministic, non-sleeping fake instead of waiting out
// real throttle windows.
type afterFunc func(d time.Duration, f func()) *time.Timer

// installLogProgressSaver throttles a running job's progress writes onto its
// pkg_install_log row. One instance per job, registered at createInstallLog
// and looked up by emitProgress/emitStepProgress, which only have the job —
// threading app + the record through every one of their ~15 call sites
// would be far more invasive than a small per-job registry.
type installLogProgressSaver struct {
	app    core.App
	record *core.Record

	// newTimer defaults to time.AfterFunc; overridden in tests.
	newTimer afterFunc

	// beforeSave, when set, runs synchronously right before each app.Save —
	// test-only, so a test can force two callers of save() to genuinely
	// overlap around a single doSave rather than merely racing to acquire
	// saveMu first (which real timing rarely reproduces reliably).
	beforeSave func()

	mu        sync.Mutex
	steps     []ProgressStep
	lastSave  time.Time
	pending   bool // a step arrived since the last save and was dropped by the throttle
	timer     *time.Timer
	finalized bool // finalize/unregister happened; no further saves are scheduled

	// dataVersion counts every mergeStep call (a new or updated-in-place
	// step). savedVersion is the dataVersion that was current when doSave
	// last started writing. A caller that needs to know ITS state landed —
	// not merely that some save ran, which may have started before its data
	// existed — captures dataVersion when it reads s.steps, then waits
	// (waitSaved) for savedVersion to reach at least that number rather than
	// waiting on the save it happened to trigger or coalesce into: the
	// actual save that lands a given version can keep being deferred by
	// read-only mode and handed off to a retry several times, with no fixed
	// single call whose return would mean "done". changed is replaced (a
	// fresh channel, old one closed) every time savedVersion or finalized
	// changes, so a waiter blocked on it wakes to recheck rather than
	// polling — see waitSaved.
	dataVersion  int64
	savedVersion int64
	changed      chan struct{}

	saveMu      sync.Mutex // serializes the actual app.Save call
	saveRunning bool
	saveAgain   bool // a request arrived while a save was in flight; re-save after

	// inflight counts doSave calls that passed their finalized check and are
	// between here and their app.Save returning. stop() waits on it to zero
	// before returning, so a caller that unregisters the saver and moves on —
	// finalizeInstallLog does, right before closing out the record on its own
	// — never races a save this saver kicked off a moment earlier (see doSave's
	// doc for why the finalized check alone, without this wait, still leaves a
	// window).
	inflight sync.WaitGroup
}

var (
	saversMu sync.Mutex
	savers   = map[string]*installLogProgressSaver{}
)

// registerProgressSaver wires a job id to the row its progress is saved onto.
// Safe to call with a nil record (createInstallLog already logs that failure);
// the saver then simply no-ops.
func registerProgressSaver(app core.App, jobID string, record *core.Record) {
	saversMu.Lock()
	defer saversMu.Unlock()
	savers[jobID] = &installLogProgressSaver{app: app, record: record, newTimer: time.AfterFunc}
}

// unregisterProgressSaver drops a finished job's saver. Called from
// finalizeInstallLog's caller via finishJob so the map does not grow for the
// life of the process. Also stops any pending throttle/retry timer so it
// cannot fire a save after the job (and its record) are done being written.
func unregisterProgressSaver(jobID string) {
	saversMu.Lock()
	saver := savers[jobID]
	delete(savers, jobID)
	saversMu.Unlock()

	if saver != nil {
		saver.stop()
	}
}

func progressSaverFor(jobID string) *installLogProgressSaver {
	saversMu.Lock()
	defer saversMu.Unlock()
	return savers[jobID]
}

// stop cancels any pending timer, marks the saver finalized so no later
// callback (already fired and waiting on mu, or racing in) schedules another
// one or saves again, and waits for any doSave already past its own
// finalized check to finish its app.Save. That wait is what makes finalized
// airtight: Timer.Stop() only stops a timer that has not fired yet, and
// flush()/save() both drop s.mu well before reaching doSave, so setting
// finalized here can otherwise still race a save already past its check and
// into app.Save — exactly the window that let a trailing save land on a
// record whose owning app had moved on.
func (s *installLogProgressSaver) stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.finalized = true
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	// Wake any waitSaved waiter (e.g. flushBlocking, if it ever raced a
	// stop() concurrent with its own wait) rather than leaving it blocked
	// until its own ctx timeout: finalized=true is itself a terminal
	// condition waitSaved checks for.
	s.notifyChanged()
	s.mu.Unlock()
	s.inflight.Wait()
}

// recordStep merges step into the saver's in-memory history and persists it
// to the row, throttled to roughly one write per second.
//
// A step with the same Step name as the last recorded entry updates that
// entry in place instead of appending — a step that reports percent ticks
// (Metro's bundling output, pnpm's resolve counter) would otherwise append a
// new array entry per tick and bloat the row without bound. A step that
// returns later under the same name after a different step ran in between
// still gets its own new entry, because only the LAST entry is checked.
//
// A save due right now runs inline. Otherwise the tick is marked pending and
// exactly one trailing save is scheduled for the end of the throttle window,
// so a client never sees progress freeze at a stale percentage for longer
// than the window — not just "until the next step arrives", which could be
// minutes into a long-running step.
func (s *installLogProgressSaver) recordStep(step ProgressStep) {
	if s == nil || s.record == nil {
		return
	}
	s.mu.Lock()
	if s.finalized {
		s.mu.Unlock()
		return
	}
	s.mergeStep(step)
	due := time.Since(s.lastSave) >= progressThrottle
	if due {
		s.mu.Unlock()
		s.flush()
		return
	}
	s.pending = true
	s.scheduleTrailingSave()
	s.mu.Unlock()
}

// mergeStep appends step, or — when it shares the last entry's Step name —
// updates that entry's progress/message/stepProgress in place. Caller holds
// mu.
func (s *installLogProgressSaver) mergeStep(step ProgressStep) {
	if n := len(s.steps); n > 0 && s.steps[n-1].Step == step.Step {
		s.steps[n-1] = step
	} else {
		s.steps = append(s.steps, step)
	}
	s.dataVersion++
}

// notifyChanged closes the current changed channel (waking every blocked
// waitSaved) and replaces it with a fresh one for the next wait. Caller
// holds mu. Safe to call whether or not changedChan has ever been read —
// changedChan lazily creates the channel on first use, so a saver that
// never has a waiter never allocates one.
func (s *installLogProgressSaver) notifyChanged() {
	if s.changed != nil {
		close(s.changed)
	}
	s.changed = make(chan struct{})
}

// changedChan returns the channel a waiter should select on to be woken by
// the next notifyChanged. Caller holds mu.
func (s *installLogProgressSaver) changedChan() chan struct{} {
	if s.changed == nil {
		s.changed = make(chan struct{})
	}
	return s.changed
}

// scheduleTrailingSave arms exactly one timer to fire at the end of the
// current throttle window, replacing any timer already armed so a burst of
// ticks inside the window never results in more than one trailing save.
// Caller holds mu.
func (s *installLogProgressSaver) scheduleTrailingSave() {
	if s.finalized {
		return
	}
	wait := progressThrottle - time.Since(s.lastSave)
	if wait < 0 {
		wait = 0
	}
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = s.newTimer(wait, s.onTrailingTimer)
}

// onTrailingTimer is the timer callback: it flushes whatever is pending. If
// the save is skipped because read-only mode is still on, flush re-arms the
// timer itself so the state keeps retrying on the throttle schedule until
// writes resume — see flush.
func (s *installLogProgressSaver) onTrailingTimer() {
	s.mu.Lock()
	if s.finalized {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	s.flush()
}

// flush saves the current step history + latest headline to the row right
// now, bypassing the throttle. The install pipeline also calls it once after
// the last emitProgress of a run so the terminal row the client sees
// reflects the very last milestone, not a throttled-away one.
//
// While read-only mode is on, the row's schema itself may be mid-migration
// under a second process, so the save is skipped — but the pending state is
// kept (not cleared) and a retry is re-armed on the normal throttle schedule,
// so progress recorded during the pause lands as soon as writes resume
// rather than waiting for the next distinct step or finalize.
func (s *installLogProgressSaver) flush() {
	if s == nil || s.record == nil {
		return
	}
	if readonly.Active() {
		s.mu.Lock()
		if !s.finalized {
			s.pending = true
			s.scheduleTrailingSave()
		}
		s.mu.Unlock()
		return
	}

	s.mu.Lock()
	if s.finalized || len(s.steps) == 0 {
		s.mu.Unlock()
		return
	}
	s.pending = false
	s.mu.Unlock()

	s.save()
}

// flushBlocking is flush's finalize-time counterpart: finalizeInstallLog
// calls it right before unregistering the saver, so a last step recorded
// while read-only mode happened to be on still lands on the row — otherwise
// unregister's timer-stop would cut off the retry flush schedules for itself
// and the pending step would never get saved. Unlike flush, it does not
// return once a save has merely been requested: it waits (waitSaved) until
// a save has actually written AT LEAST its own snapshot's dataVersion,
// because a save it triggers or coalesces into can itself be deferred by
// read-only mode and handed off to a retry timer that runs after this
// function would otherwise have already returned — see waitSaved.
func (s *installLogProgressSaver) flushBlocking() {
	if s == nil || s.record == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), readonly.TailWait)
	defer cancel()
	_ = readonly.WaitInactive(ctx)

	s.mu.Lock()
	if s.finalized || len(s.steps) == 0 {
		s.mu.Unlock()
		return
	}
	version := s.dataVersion
	s.pending = false
	s.mu.Unlock()

	s.save()
	s.waitSaved(ctx, version)
}

// waitSaved blocks until savedVersion has reached version, the saver is
// finalized, or ctx ends — whichever comes first. It never triggers a save
// itself: save() (or whatever re-arms the retry timer on read-only) is
// always what's actually driving progress toward version; this only waits
// for that progress to arrive, waking on notifyChanged instead of polling.
func (s *installLogProgressSaver) waitSaved(ctx context.Context, version int64) {
	for {
		s.mu.Lock()
		if s.finalized || s.savedVersion >= version {
			s.mu.Unlock()
			return
		}
		woken := s.changedChan()
		s.mu.Unlock()

		select {
		case <-woken:
		case <-ctx.Done():
			return
		}
	}
}

// save serializes the actual app.Save call per saver: at most one save is in
// flight at a time. A save request that arrives while one is already running
// is coalesced into a single follow-up save rather than queued — a burst of
// concurrent callers (the job goroutine, a caller relaying milestones from
// another goroutine, the trailing-save timer) never results in more than one
// save running at once and never piles up a backlog of saves. Every doSave
// call (the first and any coalesced follow-up) reads s.steps itself at the
// moment it writes, so "coalesced" here only means "don't bother running
// doSave again right this instant, it's about to read the latest state
// anyway" — it is not save() carrying a stale snapshot forward.
func (s *installLogProgressSaver) save() {
	s.saveMu.Lock()
	if s.saveRunning {
		s.saveAgain = true
		s.saveMu.Unlock()
		return
	}
	s.saveRunning = true
	s.saveMu.Unlock()

	s.doSave()

	for {
		s.saveMu.Lock()
		if !s.saveAgain {
			s.saveRunning = false
			s.saveMu.Unlock()
			return
		}
		s.saveAgain = false
		s.saveMu.Unlock()

		if readonly.Active() {
			s.mu.Lock()
			if !s.finalized {
				s.pending = true
				s.scheduleTrailingSave()
			}
			s.mu.Unlock()
			continue
		}
		s.doSave()
	}
}

// doSave performs the actual record mutation + app.Save. Only ever called
// with s.saveRunning held true by save(), so no two calls to doSave for the
// same saver run concurrently — the record itself is never mutated/saved
// from two goroutines at once.
//
// It takes NO steps/version argument and re-reads s.steps/s.dataVersion
// itself, fresh, under mu, right before writing — it never writes a snapshot
// a caller captured earlier. Two independent (non-coalesced) save() calls
// are still serialized by saveMu, but with no ordering guarantee on which
// one's EARLIER-captured snapshot is newer: recordStep's own inline flush
// racing a trailing timer's deferred retry (both triggered by the read-only
// episode ending) could have the retry's older snapshot win saveMu and land
// AFTER recordStep's newer one — reverting the row to a stale step. Reading
// fresh at write time removes the snapshot entirely, so whichever doSave
// call runs always writes the CURRENT state, and writes are therefore
// monotonic by construction, not by also getting the lock order right.
//
// Re-checks finalized immediately before the write, under the SAME lock
// acquisition that registers the call with s.inflight. Every caller (flush,
// save's coalesced retry) already checked finalized earlier in its own call
// chain, but each of them drops s.mu between that check and reaching here —
// flush in particular copies s.steps and unlocks before calling save(), which
// calls doSave() outside any lock on s.mu at all. Without the inflight
// handshake, stop() could run in that window: Timer.Stop() only prevents a
// timer that has not yet fired from firing, it does not interrupt
// onTrailingTimer once its goroutine has already started, so a trailing save
// could otherwise still reach app.Save after finalizeInstallLog's
// unregisterProgressSaver (and the record/app it was handed) are done being
// used — observed as the install pipeline's test harness tearing down its
// app while a 1s-throttled trailing timer from an earlier job was still in
// flight, panicking inside PocketBase's hook chain on the torn-down app.
// Registering with inflight BEFORE releasing s.mu is what lets stop() (which
// takes the same lock to set finalized) either see this call registered and
// wait for it, or see finalized already true and never incur the wait at
// all — there is no gap between the check and the registration for stop() to
// land in.
func (s *installLogProgressSaver) doSave() {
	s.mu.Lock()
	if s.finalized {
		s.mu.Unlock()
		return
	}
	steps := append([]ProgressStep{}, s.steps...)
	version := s.dataVersion
	s.inflight.Add(1)
	s.lastSave = time.Now()
	s.pending = false
	s.mu.Unlock()
	defer s.inflight.Done()

	if len(steps) > 0 {
		latest := steps[len(steps)-1]
		s.record.Set("steps", steps)
		s.record.Set("current_step", latest.Step)
		s.record.Set("current_message", latest.Message)

		if s.beforeSave != nil {
			s.beforeSave()
		}
		if err := s.app.Save(s.record); err != nil {
			// Best-effort: progress is a convenience, not the job's outcome.
			// Log and move on rather than failing the install over a UI
			// nicety.
			srvLog.Warn("failed to save install progress", "recordID", s.record.Id, "err", err)
		}
	}

	// Advance savedVersion (and wake any waitSaved waiter) even on a failed
	// app.Save, or when there was nothing to save: a retry isn't scheduled
	// for a plain save failure (only for a read-only deferral), so a waiter
	// blocked for THIS version would otherwise wait out its full ctx timeout
	// for a save that already ran (or had nothing to do) and is never going
	// to be retried.
	s.mu.Lock()
	if version > s.savedVersion {
		s.savedVersion = version
	}
	s.notifyChanged()
	s.mu.Unlock()
}
