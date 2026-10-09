package coreserver

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"
	"tinycld.org/core/readonly"
)

func newProgressRowsTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })
	addInstallLogCollection(t, app)
	return app
}

// newProgressSaverTestRecord's saver uses a REAL time.AfterFunc, so a test
// using it that leaves a trailing save pending must not return while that
// timer could still fire — t.Cleanup(saver.stop) guarantees the timer is
// cancelled before the test's app is torn down. Without it, a pending
// timer fires after cleanup and calls app.Save on a closed test database,
// a process crash unrelated to anything the saver itself got wrong.
func newProgressSaverTestRecord(t *testing.T, app *tests.TestApp) (*installLogProgressSaver, string) {
	t.Helper()
	id := addInstallLog(t, app, "gizmos", "running")
	col, err := app.FindCollectionByNameOrId("pkg_install_log")
	if err != nil {
		t.Fatalf("find pkg_install_log: %v", err)
	}
	record, err := app.FindRecordById(col, id)
	if err != nil {
		t.Fatalf("find record: %v", err)
	}
	saver := &installLogProgressSaver{app: app, record: record, newTimer: time.AfterFunc}
	t.Cleanup(saver.stop)
	return saver, id
}

// newProgressSaverWithFakeTimer is newProgressSaverTestRecord but with the
// saver's timer indirected through a fakeTimers, so a test can fire the
// trailing save deterministically instead of sleeping out the real window.
func newProgressSaverWithFakeTimer(t *testing.T, app *tests.TestApp) (*installLogProgressSaver, *fakeTimers, string) {
	t.Helper()
	saver, id := newProgressSaverTestRecord(t, app)
	timers := &fakeTimers{}
	saver.newTimer = timers.newTimer
	return saver, timers, id
}

func fetchProgressRow(t *testing.T, app *tests.TestApp, id string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("pkg_install_log")
	if err != nil {
		t.Fatalf("find pkg_install_log: %v", err)
	}
	rec, err := app.FindRecordById(col, id)
	if err != nil {
		t.Fatalf("find record: %v", err)
	}
	return rec
}

func decodeSteps(t *testing.T, rec *core.Record) []ProgressStep {
	t.Helper()
	raw, ok := rec.Get("steps").(types.JSONRaw)
	if !ok {
		t.Fatalf("steps field is %T, want types.JSONRaw", rec.Get("steps"))
	}
	var steps []ProgressStep
	if err := json.Unmarshal(raw, &steps); err != nil {
		t.Fatalf("unmarshal steps: %v", err)
	}
	return steps
}

// fakeTimers lets a test fire a saver's trailing-save timer deterministically
// instead of sleeping out the real 1s throttle window. newTimer returns a
// *time.Timer (the saver's stop() calls Stop() on it), but the returned timer
// is never actually armed — fire() below calls the captured callback
// directly, synchronously, on the test's own goroutine.
type fakeTimers struct {
	mu    sync.Mutex
	calls []func()
}

func (f *fakeTimers) newTimer(_ time.Duration, cb func()) *time.Timer {
	f.mu.Lock()
	f.calls = append(f.calls, cb)
	f.mu.Unlock()
	// A real, never-fired timer: stop() can call Stop() on it harmlessly.
	return time.AfterFunc(time.Hour, func() {})
}

// fire runs the most recently scheduled callback, as if its window had
// elapsed. It runs synchronously so the test can assert on the row
// immediately after, with no sleep and no flakiness.
func (f *fakeTimers) fire() {
	f.mu.Lock()
	n := len(f.calls)
	var cb func()
	if n > 0 {
		cb = f.calls[n-1]
	}
	f.mu.Unlock()
	if cb != nil {
		cb()
	}
}

func (f *fakeTimers) scheduledCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// A single recordStep saves immediately: the FIRST tick of a run has nothing
// to throttle against, so a client watching the row sees the opening
// milestone right away rather than waiting out the throttle window.
func TestRecordStep_SavesImmediatelyOnFirstTick(t *testing.T) {
	app := newProgressRowsTestApp(t)
	saver, id := newProgressSaverTestRecord(t, app)

	saver.recordStep(ProgressStep{Step: "Checking the package", Progress: 1, Message: "Checking"})

	col, _ := app.FindCollectionByNameOrId("pkg_install_log")
	rec, err := app.FindRecordById(col, id)
	if err != nil {
		t.Fatalf("find record: %v", err)
	}
	if rec.GetString("current_step") != "Checking the package" {
		t.Errorf("current_step = %q, want %q", rec.GetString("current_step"), "Checking the package")
	}
	if rec.GetString("current_message") != "Checking" {
		t.Errorf("current_message = %q, want %q", rec.GetString("current_message"), "Checking")
	}
}

// A burst of ticks inside the throttle window must not multiply writes: only
// the first tick's save actually lands until the window elapses, which is
// the whole point of throttling a chatty build down to ~1 write/sec.
func TestRecordStep_ThrottlesBurstsWithinTheWindow(t *testing.T) {
	app := newProgressRowsTestApp(t)
	saver, _, id := newProgressSaverWithFakeTimer(t, app)

	saver.recordStep(ProgressStep{Step: "A", Progress: 1, Message: "first"})
	saver.recordStep(ProgressStep{Step: "B", Progress: 2, Message: "second"})
	saver.recordStep(ProgressStep{Step: "C", Progress: 3, Message: "third"})

	rec := fetchProgressRow(t, app, id)
	// The row still reflects the FIRST saved tick — B and C arrived inside the
	// throttle window and were recorded in memory (pending) but not yet saved.
	if rec.GetString("current_step") != "A" {
		t.Errorf("current_step = %q, want %q (burst must not have saved past the first tick)",
			rec.GetString("current_step"), "A")
	}
}

// Defect 1: during a long step, nothing else ever arrives to trigger the
// "next step" save, so without a trailing save the UI would show a stale
// step for as long as the step runs. recordStep must schedule exactly one
// save for the end of the throttle window, and firing that timer must save
// the LATEST pending state — not the first tick that started the window.
func TestRecordStep_SchedulesExactlyOneTrailingSaveForTheWindow(t *testing.T) {
	app := newProgressRowsTestApp(t)
	saver, timers, id := newProgressSaverWithFakeTimer(t, app)

	saver.recordStep(ProgressStep{Step: "A", Progress: 1, Message: "first"})   // saves immediately, no timer
	saver.recordStep(ProgressStep{Step: "B", Progress: 2, Message: "second"})  // pending, schedules timer #1
	saver.recordStep(ProgressStep{Step: "B", Progress: 3, Message: "still B"}) // pending, REPLACES timer #1

	if got := timers.scheduledCount(); got != 2 {
		t.Fatalf("scheduled %d timers, want 2 (one replaced by the next)", got)
	}

	timers.fire()

	rec := fetchProgressRow(t, app, id)
	if rec.GetString("current_step") != "B" {
		t.Errorf("current_step = %q, want %q after the trailing save fires", rec.GetString("current_step"), "B")
	}
	if rec.GetString("current_message") != "still B" {
		t.Errorf("current_message = %q, want the LATEST pending message %q",
			rec.GetString("current_message"), "still B")
	}
}

// The trailing save must fire only once: a timer that is replaced (because a
// later tick arrived before it fired) must not ALSO fire and double-save.
// fakeTimers.fire only ever invokes the latest scheduled callback, so this
// exercises the saver's own replacement (Stop + reassign), not the test
// double standing in for it.
func TestRecordStep_TrailingSaveDoesNotDoubleFire(t *testing.T) {
	app := newProgressRowsTestApp(t)
	saver, timers, id := newProgressSaverWithFakeTimer(t, app)

	saver.recordStep(ProgressStep{Step: "A", Progress: 1, Message: "first"})
	saver.recordStep(ProgressStep{Step: "B", Progress: 2, Message: "second"})

	timers.fire()
	rec := fetchProgressRow(t, app, id)
	if rec.GetString("current_step") != "B" {
		t.Fatalf("current_step = %q, want %q", rec.GetString("current_step"), "B")
	}

	// No further ticks and no further fires: the row must stay exactly as the
	// one trailing save left it.
	rec2 := fetchProgressRow(t, app, id)
	if rec2.GetString("current_message") != "second" {
		t.Errorf("current_message = %q, want unchanged %q", rec2.GetString("current_message"), "second")
	}
}

// No save after finalize/unregister: a timer that was armed before the job
// finalized must not go on to save once the saver has been stopped — the
// record may already be mid-save for the terminal status by then.
func TestStop_PreventsTrailingSaveAfterFinalize(t *testing.T) {
	app := newProgressRowsTestApp(t)
	saver, timers, id := newProgressSaverWithFakeTimer(t, app)

	saver.recordStep(ProgressStep{Step: "A", Progress: 1, Message: "first"})
	saver.recordStep(ProgressStep{Step: "B", Progress: 2, Message: "second"}) // schedules the trailing save

	saver.stop() // simulates finalize/unregister before the window elapses
	timers.fire()

	rec := fetchProgressRow(t, app, id)
	if rec.GetString("current_step") != "A" {
		t.Errorf("current_step = %q, want unchanged %q — stop must prevent the trailing save", rec.GetString("current_step"), "A")
	}

	// A recordStep arriving after stop must also no-op rather than scheduling
	// a new timer or saving.
	saver.recordStep(ProgressStep{Step: "C", Progress: 3, Message: "third"})
	rec2 := fetchProgressRow(t, app, id)
	if rec2.GetString("current_step") != "A" {
		t.Errorf("current_step = %q, want unchanged %q — recordStep after stop must no-op", rec2.GetString("current_step"), "A")
	}
}

// stop() must not return while a save it did not see in time to cancel is
// still writing — otherwise a caller that stops a saver and immediately
// moves on (closing the app, reusing the record for something else) can
// still have that save land afterwards. Timer.Stop() only prevents a timer
// that has not fired yet from firing; it cannot recall a callback whose
// goroutine already started, and flush()/save() both drop s.mu well before
// reaching doSave() — so a naive "stop sets finalized" has a window where a
// save already past its own finalized check is not yet registered as
// something stop() needs to wait for.
//
// Reproducing that exact goroutine interleaving through the real timer/flush
// path is inherently racy to assert on (whether doSave wins s.mu before
// stop() is a scheduling accident either way). This test instead drives the
// invariant stop() relies on directly and deterministically: s.inflight is
// the handshake — anything that got past the finalized check registers with
// it BEFORE doing its write, and stop() must block on it draining. Simulate
// "a save already past its check" by registering with s.inflight directly,
// confirm stop() blocks for exactly as long as that registration is held,
// and confirm it unblocks the instant it's released — with no sleep and no
// dependence on which goroutine wins a lock.
func TestStop_WaitsForInflightSaveToFinish(t *testing.T) {
	app := newProgressRowsTestApp(t)
	saver, _ := newProgressSaverTestRecord(t, app)

	// Stand in for doSave having just passed its own finalized check and
	// registered, about to call app.Save.
	saver.inflight.Add(1)

	stopDone := make(chan struct{})
	go func() {
		saver.stop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
		t.Fatal("stop() returned while a registered save was still in flight")
	case <-time.After(50 * time.Millisecond):
		// Expected: stop() is blocked on s.inflight.Wait().
	}

	saver.inflight.Done() // the simulated save "finishes"

	select {
	case <-stopDone:
		// stop() unblocked once the in-flight save released it.
	case <-time.After(2 * time.Second):
		t.Fatal("stop() never returned after the in-flight save finished")
	}
}

// Defect 2: a step reporting percent ticks (Metro's bundler output, pnpm's
// resolve counter) must update the last row entry in place rather than
// appending — otherwise the row grows by one entry per percent tick and the
// "bounded row size" requirement is violated.
func TestRecordStep_SameStepTicksUpdateInPlace(t *testing.T) {
	app := newProgressRowsTestApp(t)
	saver, id := newProgressSaverTestRecord(t, app)

	sp := func(v int) *int { return &v }
	saver.recordStep(ProgressStep{Step: "Bundling", Progress: 10, Message: "0%", StepProgress: sp(0)})
	saver.flush()
	saver.recordStep(ProgressStep{Step: "Bundling", Progress: 10, Message: "40%", StepProgress: sp(40)})
	saver.flush()
	saver.recordStep(ProgressStep{Step: "Bundling", Progress: 10, Message: "80%", StepProgress: sp(80)})
	saver.flush()
	saver.recordStep(ProgressStep{Step: "Packaging", Progress: 20, Message: "starting"})
	saver.flush()

	rec := fetchProgressRow(t, app, id)
	steps := decodeSteps(t, rec)
	if len(steps) != 2 {
		t.Fatalf("steps has %d entries, want 2 (Bundling collapsed to one entry, then Packaging)", len(steps))
	}
	if steps[0].Step != "Bundling" || steps[0].Message != "80%" || steps[0].StepProgress == nil || *steps[0].StepProgress != 80 {
		t.Errorf("steps[0] = %+v, want the LATEST Bundling tick merged in place", steps[0])
	}
	if steps[1].Step != "Packaging" {
		t.Errorf("steps[1].Step = %q, want %q (a different step always gets its own entry)", steps[1].Step, "Packaging")
	}
}

// A step name that recurs non-consecutively (it ran, a different step ran,
// then the first step's name comes back — e.g. a reported failure attributed
// back to an earlier step) gets its own new entry: only the immediately
// preceding entry is checked for an in-place merge.
func TestRecordStep_NonConsecutiveRepeatGetsNewEntry(t *testing.T) {
	app := newProgressRowsTestApp(t)
	saver, id := newProgressSaverTestRecord(t, app)

	saver.recordStep(ProgressStep{Step: "A", Progress: 1, Message: "first"})
	saver.flush()
	saver.recordStep(ProgressStep{Step: "B", Progress: 2, Message: "second"})
	saver.flush()
	saver.recordStep(ProgressStep{Step: "A", Progress: 3, Message: "FAILED: retried"})
	saver.flush()

	rec := fetchProgressRow(t, app, id)
	steps := decodeSteps(t, rec)
	if len(steps) != 3 {
		t.Fatalf("steps has %d entries, want 3 (A, B, A again)", len(steps))
	}
}

// flush bypasses the throttle so a pending burst's LAST step lands on the
// row — the install pipeline calls this once at finalize, so the terminal
// row reflects the true last milestone rather than a throttled-away one.
func TestFlush_SavesThePendingBurstsLastStep(t *testing.T) {
	app := newProgressRowsTestApp(t)
	saver, id := newProgressSaverTestRecord(t, app)

	saver.recordStep(ProgressStep{Step: "A", Progress: 1, Message: "first"})
	saver.recordStep(ProgressStep{Step: "B", Progress: 50, Message: "second"})
	saver.flush()

	col, _ := app.FindCollectionByNameOrId("pkg_install_log")
	rec, err := app.FindRecordById(col, id)
	if err != nil {
		t.Fatalf("find record: %v", err)
	}
	if rec.GetString("current_step") != "B" {
		t.Errorf("current_step = %q, want %q after flush", rec.GetString("current_step"), "B")
	}
	// A json field read back from a fresh fetch arrives as types.JSONRaw
	// (bytes), not the typed []ProgressStep that was set in-process — decode
	// it to check the history both steps actually persisted.
	raw, ok := rec.Get("steps").(types.JSONRaw)
	if !ok {
		t.Fatalf("steps field is %T, want types.JSONRaw", rec.Get("steps"))
	}
	var steps []ProgressStep
	if err := json.Unmarshal(raw, &steps); err != nil {
		t.Fatalf("unmarshal steps: %v", err)
	}
	if len(steps) != 2 {
		t.Errorf("steps has %d entries, want 2 (A and B)", len(steps))
	}
}

// The throttled save must never block the job or fail it outright while
// read-only mode is on — the row's own schema may be mid-migration under a
// second process. recordStep/flush must not save, but defect 4 means the
// state stays pending and a retry is scheduled rather than dropped.
func TestRecordStep_SkipsSilentlyDuringReadOnlyMode(t *testing.T) {
	app := newProgressRowsTestApp(t)
	saver, timers, id := newProgressSaverWithFakeTimer(t, app)

	readonly.Enter()
	defer readonly.Leave()

	done := make(chan struct{})
	go func() {
		saver.recordStep(ProgressStep{Step: "A", Progress: 1, Message: "first"})
		saver.flush()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("recordStep/flush blocked during read-only mode instead of skipping")
	}

	rec := fetchProgressRow(t, app, id)
	if rec.GetString("current_step") != "" {
		t.Errorf("current_step = %q, want empty — a save during read-only mode must be skipped, not applied",
			rec.GetString("current_step"))
	}
	// Defect 4: the state must not be dropped — a retry must be armed so it
	// is not stuck until some unrelated later step arrives.
	if got := timers.scheduledCount(); got == 0 {
		t.Error("no retry was scheduled while read-only mode was active")
	}
}

// Once read-only mode lifts, the pending state saves as soon as the ALREADY
// SCHEDULED retry fires — not only when some unrelated next step arrives.
// This is defect 4: before the fix, a step recorded during read-only mode
// was only ever picked up by the next distinct recordStep/flush call.
func TestRecordStep_RetrySavesPendingStateOnceReadOnlyModeLifts(t *testing.T) {
	app := newProgressRowsTestApp(t)
	saver, timers, id := newProgressSaverWithFakeTimer(t, app)

	readonly.Enter()
	saver.recordStep(ProgressStep{Step: "duringPause", Progress: 1, Message: "during pause"})
	saver.flush() // the throttle's own flush call during read-only: schedules a retry
	readonly.Leave()

	timers.fire() // the retry the read-only period itself armed

	rec := fetchProgressRow(t, app, id)
	if rec.GetString("current_step") != "duringPause" {
		t.Errorf("current_step = %q, want %q saved by the retry alone, no new step required",
			rec.GetString("current_step"), "duringPause")
	}
}

// Once read-only mode lifts, the NEXT tick also still saves normally.
//
// recordStep/flush are deliberately fire-and-forget (see their own docs):
// the "skipped" tick's own read-only deferral armed a REAL timer that is
// still in flight when Leave() returns, so asserting on the row immediately
// after the "resumed" tick races that leftover timer with no synchronization
// at all — not a saver bug, a test-timing one. waitSaved (same package)
// blocks until the saver's own bookkeeping confirms the "resumed" tick's
// dataVersion has actually been written, which is what this test needs to
// assert against a settled row rather than guessing how long two real,
// concurrent timers take.
func TestRecordStep_ResumesAfterReadOnlyModeLifts(t *testing.T) {
	app := newProgressRowsTestApp(t)
	saver, id := newProgressSaverTestRecord(t, app)

	readonly.Enter()
	saver.recordStep(ProgressStep{Step: "skipped", Progress: 1, Message: "during pause"})
	readonly.Leave()

	saver.recordStep(ProgressStep{Step: "resumed", Progress: 2, Message: "after pause"})

	saver.mu.Lock()
	version := saver.dataVersion
	saver.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	saver.waitSaved(ctx, version)

	rec := fetchProgressRow(t, app, id)
	if rec.GetString("current_step") != "resumed" {
		t.Errorf("current_step = %q, want %q after the mode lifts", rec.GetString("current_step"), "resumed")
	}
}

// flushBlocking is finalize's call site: it must wait out a read-only pause
// (bounded) and land the pending step before returning, so the terminal
// save that follows in finalizeInstallLog isn't missing the last milestone.
func TestFlushBlocking_LandsPendingStepOnceReadOnlyModeLifts(t *testing.T) {
	app := newProgressRowsTestApp(t)
	saver, id := newProgressSaverTestRecord(t, app)

	readonly.Enter()
	saver.recordStep(ProgressStep{Step: "lastStep", Progress: 99, Message: "almost done"})

	done := make(chan struct{})
	go func() {
		saver.flushBlocking()
		close(done)
	}()

	// flushBlocking must be waiting on read-only mode, not returning early.
	select {
	case <-done:
		t.Fatal("flushBlocking returned while read-only mode was still active")
	case <-time.After(50 * time.Millisecond):
	}

	readonly.Leave()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("flushBlocking did not return once read-only mode lifted")
	}

	rec := fetchProgressRow(t, app, id)
	if rec.GetString("current_step") != "lastStep" {
		t.Errorf("current_step = %q, want %q", rec.GetString("current_step"), "lastStep")
	}
}

// Deterministic reproduction of a real bug: when read-only mode lifts, the
// retry timer armed by the earlier read-only deferral and flushBlocking's
// own resumed save can both reach save() around the same moment. Before the
// fix, whichever call coalesced (saveRunning already true -> set saveAgain,
// return) returned immediately, so flushBlocking could report success
// before the winning doSave's app.Save had actually run — observed as
// finalizeInstallLog's terminal save landing without the last milestone.
//
// beforeSave pauses the FIRST doSave right before its app.Save, which lets
// the test force the exact interleaving (flushBlocking's save() call
// arriving and coalescing) instead of relying on real scheduling luck — this
// is what TestFlushBlocking_LandsPendingStepOnceReadOnlyModeLifts could only
// hit by chance (~1 run in 200).
func TestFlushBlocking_WaitsForATrailingSaveThatCoalescedConcurrently(t *testing.T) {
	app := newProgressRowsTestApp(t)
	saver, timers, id := newProgressSaverWithFakeTimer(t, app)

	readonly.Enter()
	saver.recordStep(ProgressStep{Step: "lastStep", Progress: 99, Message: "almost done"})
	// The throttle's own flush() call (inside recordStep, since this is the
	// saver's first ever tick and so immediately "due") sees read-only and
	// arms the fake trailing-save timer rather than saving — exactly
	// recordStep's normal read-only deferral path.
	if got := timers.scheduledCount(); got != 1 {
		t.Fatalf("scheduled %d timers, want 1 (the read-only deferral)", got)
	}

	enteredSave := make(chan struct{})
	releaseSave := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseSave) }) }
	// A t.Fatal from the main goroutine below runs this test's deferred
	// cleanup via runtime.Goexit, but does NOT stop the background
	// goroutines still waiting on releaseSave (the paused doSave, and
	// saver.stop's inflight.Wait registered by newProgressSaverTestRecord's
	// own t.Cleanup) — without this, a failed assertion here would hang the
	// whole test binary instead of reporting FAIL.
	t.Cleanup(release)

	var pauseOnce sync.Once
	saver.beforeSave = func() {
		// Only the FIRST doSave call pauses — once release fires, let every
		// later call (there should be none here) through immediately rather
		// than deadlocking a re-entrant beforeSave.
		pauseOnce.Do(func() {
			close(enteredSave)
			<-releaseSave
		})
	}

	readonly.Leave()

	// Fire the retry timer on its own goroutine — this is the saver's
	// genuine retry path (armed above), just driven by the test instead of
	// a real clock, and it is what starts the doSave that beforeSave pauses.
	timerDone := make(chan struct{})
	go func() {
		timers.fire()
		close(timerDone)
	}()
	<-enteredSave // the timer's doSave is now paused right before app.Save

	// flushBlocking starts concurrently with the paused doSave. Its own
	// save() call MUST coalesce (saveRunning is already true) rather than
	// racing app.Save directly — proving save()'s single-flight still holds
	// — and then it must BLOCK, not return, until the paused save (which it
	// coalesced into) actually finishes writing.
	flushDone := make(chan struct{})
	go func() {
		saver.flushBlocking()
		close(flushDone)
	}()

	select {
	case <-flushDone:
		t.Fatal("flushBlocking returned before the trailing save it coalesced into had written the row")
	case <-time.After(50 * time.Millisecond):
	}

	release() // let the paused doSave finally call app.Save

	select {
	case <-timerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("the trailing timer's save never finished")
	}
	select {
	case <-flushDone:
	case <-time.After(2 * time.Second):
		t.Fatal("flushBlocking did not return once the save it coalesced into finished")
	}

	rec := fetchProgressRow(t, app, id)
	if rec.GetString("current_step") != "lastStep" {
		t.Errorf("current_step = %q, want %q — flushBlocking returned without its state having landed",
			rec.GetString("current_step"), "lastStep")
	}
}

// Defect 3: concurrent recordStep calls (the job goroutine, a caller relaying
// milestones from another goroutine, and the saver's own trailing-save
// timer) must never race on the record, and the row must end up with the
// single latest state rather than a torn mix. Run with -race to catch any
// unsynchronized access.
func TestRecordStep_ConcurrentCallsAreSerializedAndRaceFree(t *testing.T) {
	app := newProgressRowsTestApp(t)
	saver, id := newProgressSaverTestRecord(t, app)

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			saver.recordStep(ProgressStep{Step: "concurrent", Progress: i, Message: "tick"})
			saver.flush()
		}(i)
	}
	wg.Wait()

	rec := fetchProgressRow(t, app, id)
	if rec.GetString("current_step") != "concurrent" {
		t.Errorf("current_step = %q, want %q", rec.GetString("current_step"), "concurrent")
	}
	steps := decodeSteps(t, rec)
	if len(steps) != 1 {
		t.Errorf("steps has %d entries, want 1 — concurrent ticks of the same step must merge in place, not append",
			len(steps))
	}
}

// A nil record (createInstallLog failed, e.g. the collection was missing)
// must not panic a running job over a progress-reporting nicety.
func TestRecordStep_NilRecordIsANoOp(t *testing.T) {
	saver := &installLogProgressSaver{}
	saver.recordStep(ProgressStep{Step: "A", Progress: 1, Message: "first"})
	saver.flush()
}

// A lookup for a job id nothing registered (e.g. createInstallLog failed, or
// the job already finished and unregistered) must degrade to a no-op saver,
// never a nil-pointer panic at the call site.
func TestProgressSaverFor_UnknownJobIDDegradesSafely(t *testing.T) {
	progressSaverFor("job_does_not_exist").recordStep(ProgressStep{Step: "A", Progress: 1})
}

func TestRegisterAndUnregisterProgressSaver(t *testing.T) {
	app := newProgressRowsTestApp(t)
	_, id := newProgressSaverTestRecord(t, app)
	col, _ := app.FindCollectionByNameOrId("pkg_install_log")
	record, err := app.FindRecordById(col, id)
	if err != nil {
		t.Fatalf("find record: %v", err)
	}

	registerProgressSaver(app, "job_reg_test", record)
	progressSaverFor("job_reg_test").recordStep(ProgressStep{Step: "A", Progress: 1, Message: "m"})

	rec, err := app.FindRecordById(col, id)
	if err != nil {
		t.Fatalf("find record: %v", err)
	}
	if rec.GetString("current_step") != "A" {
		t.Fatalf("expected the registered saver to write to the row, current_step = %q", rec.GetString("current_step"))
	}

	unregisterProgressSaver("job_reg_test")
	// After unregistering, the lookup must degrade safely rather than find a
	// stale saver still pointed at this record.
	progressSaverFor("job_reg_test").recordStep(ProgressStep{Step: "B", Progress: 2, Message: "n"})
	rec2, err := app.FindRecordById(col, id)
	if err != nil {
		t.Fatalf("find record: %v", err)
	}
	if rec2.GetString("current_step") != "A" {
		t.Errorf("current_step = %q, want unchanged %q after unregister", rec2.GetString("current_step"), "A")
	}
}
