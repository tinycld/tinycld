package coreserver

import (
	"encoding/json"
	"testing"
	"time"

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
	return &installLogProgressSaver{app: app, record: record}, id
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
	saver, id := newProgressSaverTestRecord(t, app)

	saver.recordStep(ProgressStep{Step: "A", Progress: 1, Message: "first"})
	saver.recordStep(ProgressStep{Step: "B", Progress: 2, Message: "second"})
	saver.recordStep(ProgressStep{Step: "C", Progress: 3, Message: "third"})

	col, _ := app.FindCollectionByNameOrId("pkg_install_log")
	rec, err := app.FindRecordById(col, id)
	if err != nil {
		t.Fatalf("find record: %v", err)
	}
	// The row still reflects the FIRST saved tick — B and C arrived inside the
	// throttle window and were recorded in memory (pending) but not yet saved.
	if rec.GetString("current_step") != "A" {
		t.Errorf("current_step = %q, want %q (burst must not have saved past the first tick)",
			rec.GetString("current_step"), "A")
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
// second process. recordStep/flush degrade to a silent no-op instead.
func TestRecordStep_SkipsSilentlyDuringReadOnlyMode(t *testing.T) {
	app := newProgressRowsTestApp(t)
	saver, id := newProgressSaverTestRecord(t, app)

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

	col, _ := app.FindCollectionByNameOrId("pkg_install_log")
	rec, err := app.FindRecordById(col, id)
	if err != nil {
		t.Fatalf("find record: %v", err)
	}
	if rec.GetString("current_step") != "" {
		t.Errorf("current_step = %q, want empty — a save during read-only mode must be skipped, not applied",
			rec.GetString("current_step"))
	}
}

// Once read-only mode lifts, the NEXT tick saves normally — a missed tick
// during the pause is not made up, but progress is not stuck either.
func TestRecordStep_ResumesAfterReadOnlyModeLifts(t *testing.T) {
	app := newProgressRowsTestApp(t)
	saver, id := newProgressSaverTestRecord(t, app)

	readonly.Enter()
	saver.recordStep(ProgressStep{Step: "skipped", Progress: 1, Message: "during pause"})
	readonly.Leave()

	saver.recordStep(ProgressStep{Step: "resumed", Progress: 2, Message: "after pause"})

	col, _ := app.FindCollectionByNameOrId("pkg_install_log")
	rec, err := app.FindRecordById(col, id)
	if err != nil {
		t.Fatalf("find record: %v", err)
	}
	if rec.GetString("current_step") != "resumed" {
		t.Errorf("current_step = %q, want %q after the mode lifts", rec.GetString("current_step"), "resumed")
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
