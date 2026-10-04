package backup

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"tinycld.org/core/readonly"
)

func runningBackupRow(t *testing.T, app core.App) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("backups")
	if err != nil {
		t.Fatal(err)
	}
	row := core.NewRecord(col)
	row.Set("kind", "restore")
	row.Set("status", "running")
	row.Set("started", types.NowDateTime())
	if err := app.Save(row); err != nil {
		t.Fatal(err)
	}
	return row
}

// saveTicks records, for every save of the row with id, the tick count at
// that moment.
func saveTicks(app core.App, id string, ticks *atomic.Int64) func() []int64 {
	var mu sync.Mutex
	var at []int64
	app.OnRecordAfterUpdateSuccess("backups").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.Id == id {
			mu.Lock()
			at = append(at, ticks.Load())
			mu.Unlock()
		}
		return e.Next()
	})
	return func() []int64 {
		mu.Lock()
		defer mu.Unlock()
		return append([]int64{}, at...)
	}
}

func waitForSave(t *testing.T, saves func() []int64) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if len(saves()) > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the row was never saved after read-only mode ended")
}

// The progress tick skips its save while read-only; the next tick after the
// mode ends writes the current count.
func TestProgressSkipsItsSaveWhileReadOnly(t *testing.T) {
	app := newTestApp(t)
	row := runningBackupRow(t, app)
	var ticks atomic.Int64
	saves := saveTicks(app, row.Id, &ticks)

	readonly.Enter()
	t.Cleanup(readonly.Leave)
	// Tick 1 runs while read-only; tick 2 ends the mode before its check.
	progressTickForTesting = func() {
		if ticks.Add(1) == 2 {
			readonly.Leave()
		}
	}
	t.Cleanup(func() { progressTickForTesting = nil })

	var total atomic.Int64
	total.Store(42)
	p := newProgress(app, row, total.Load)
	waitForSave(t, saves)
	p.stop()

	got := saves()
	if got[0] < 2 {
		t.Fatalf("the progress row was saved at tick %d, while read-only", got[0])
	}
	fresh, err := app.FindRecordById("backups", row.Id)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.GetInt("bytes") != 42 {
		t.Fatalf("bytes %d, want the current 42", fresh.GetInt("bytes"))
	}
}

type stalledSource struct{ offset int64 }

func (s stalledSource) Offset() int64 { return s.offset }
func (s stalledSource) Blocked() bool { return true }

// The watcher keeps the stalled state while read-only and writes it on the
// first tick after the mode ends.
func TestExpiryWatcherSkipsItsSaveWhileReadOnly(t *testing.T) {
	app := newTestApp(t)
	row := runningBackupRow(t, app)
	var ticks atomic.Int64
	saves := saveTicks(app, row.Id, &ticks)

	readonly.Enter()
	t.Cleanup(readonly.Leave)
	// The stall is reached at tick expiryStallPolls, while read-only; the
	// mode ends two ticks later.
	leaveAt := int64(expiryStallPolls + 2)
	expiryTickForTesting = func() {
		if ticks.Add(1) == leaveAt {
			readonly.Leave()
		}
	}
	t.Cleanup(func() { expiryTickForTesting = nil })

	w := newExpiryWatcher(app, row, stalledSource{offset: 7})
	waitForSave(t, saves)
	w.stop()

	got := saves()
	if got[0] < leaveAt {
		t.Fatalf("the restore row was saved at tick %d, while read-only", got[0])
	}
	if len(got) != 1 {
		t.Fatalf("want one status save, got %d", len(got))
	}
	fresh, err := app.FindRecordById("backups", row.Id)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.GetString("status") != "waiting_for_source" {
		t.Fatalf("status %q, want waiting_for_source", fresh.GetString("status"))
	}
}
