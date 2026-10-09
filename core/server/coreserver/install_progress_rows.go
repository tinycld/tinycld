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

	mu        sync.Mutex
	steps     []ProgressStep
	lastSave  time.Time
	pending   bool // a step arrived since the last save and was dropped by the throttle
	timer     *time.Timer
	finalized bool // finalize/unregister happened; no further saves are scheduled

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
		return
	}
	s.steps = append(s.steps, step)
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
	if s.finalized {
		s.mu.Unlock()
		return
	}
	steps := append([]ProgressStep{}, s.steps...)
	s.pending = false
	s.mu.Unlock()

	if len(steps) == 0 {
		return
	}
	s.save(steps)
}

// flushBlocking is flush's finalize-time counterpart: finalizeInstallLog
// calls it right before unregistering the saver, so a last step recorded
// while read-only mode happened to be on still lands on the row — otherwise
// unregister's timer-stop would cut off the retry flush schedules for itself
// and the pending step would never get saved. It waits out read-only mode
// (bounded by readonly.TailWait, the same bound the audit log's tail writes
// use for a write that follows a request already accepted) rather than
// re-arming a timer, since this call site can afford to block briefly and
// the caller is about to unregister the saver anyway.
func (s *installLogProgressSaver) flushBlocking() {
	if s == nil || s.record == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), readonly.TailWait)
	defer cancel()
	_ = readonly.WaitInactive(ctx)

	s.mu.Lock()
	if s.finalized {
		s.mu.Unlock()
		return
	}
	steps := append([]ProgressStep{}, s.steps...)
	s.pending = false
	s.mu.Unlock()

	if len(steps) == 0 {
		return
	}
	s.save(steps)
}

// save serializes the actual app.Save call per saver: at most one save is in
// flight at a time. A save request that arrives while one is already running
// is coalesced into a single follow-up save (using whatever the latest state
// is by the time that follow-up runs) rather than queued — a burst of
// concurrent callers (the job goroutine, a caller relaying milestones from
// another goroutine, the trailing-save timer) never results in more than one
// save running at once and never piles up a backlog of saves.
func (s *installLogProgressSaver) save(steps []ProgressStep) {
	s.saveMu.Lock()
	if s.saveRunning {
		s.saveAgain = true
		s.saveMu.Unlock()
		return
	}
	s.saveRunning = true
	s.saveMu.Unlock()

	s.doSave(steps)

	for {
		s.saveMu.Lock()
		if !s.saveAgain {
			s.saveRunning = false
			s.saveMu.Unlock()
			return
		}
		s.saveAgain = false
		s.saveMu.Unlock()

		// Re-read the latest state rather than the stale steps this loop
		// iteration started with: the whole point of coalescing is that the
		// follow-up save reflects whatever is current by the time it runs.
		s.mu.Lock()
		if s.finalized {
			s.mu.Unlock()
			continue
		}
		latestSteps := append([]ProgressStep{}, s.steps...)
		s.pending = false
		s.mu.Unlock()

		if readonly.Active() {
			s.mu.Lock()
			if !s.finalized {
				s.pending = true
				s.scheduleTrailingSave()
			}
			s.mu.Unlock()
			continue
		}
		if len(latestSteps) > 0 {
			s.doSave(latestSteps)
		}
	}
}

// doSave performs the actual record mutation + app.Save. Only ever called
// with s.saveRunning held true by save(), so no two calls to doSave for the
// same saver run concurrently — the record itself is never mutated/saved
// from two goroutines at once.
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
func (s *installLogProgressSaver) doSave(steps []ProgressStep) {
	s.mu.Lock()
	if s.finalized {
		s.mu.Unlock()
		return
	}
	s.inflight.Add(1)
	s.lastSave = time.Now()
	s.mu.Unlock()
	defer s.inflight.Done()

	latest := steps[len(steps)-1]
	s.record.Set("steps", steps)
	s.record.Set("current_step", latest.Step)
	s.record.Set("current_message", latest.Message)

	if err := s.app.Save(s.record); err != nil {
		// Best-effort: progress is a convenience, not the job's outcome. Log
		// and move on rather than failing the install over a UI nicety.
		srvLog.Warn("failed to save install progress", "recordID", s.record.Id, "err", err)
	}
}
