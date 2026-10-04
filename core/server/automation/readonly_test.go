package automation

import (
	"context"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/readonly"
)

// readOnlyWorker builds a second, unstarted engine over engineApp's app and
// runs its worker by hand, so the test owns the worker's context and learns
// (through waiting) when the worker has reached the read-only wait. The
// event targets a disabled rule by RuleID: engineApp's own engine dispatches
// only enabled rules, so it cannot run this rule behind the test's back.
// ran receives readonly.Active() as seen by the rule's action.
func readOnlyWorker(t *testing.T) (w *Engine, ev event, waiting <-chan struct{}, ran <-chan bool, cancel context.CancelFunc) {
	t.Helper()
	app, eng, u := engineApp(t)

	ranCh := make(chan bool, 1)
	RegisterAction("tickets:boom", func(core.App, ActionRequest) error {
		ranCh <- readonly.Active()
		return nil
	})
	rule := makeRule(t, app, u.Id, "org", nil, []any{map[string]any{"ref": "tickets:boom"}}, 0, false)
	rule.Set("enabled", false)
	if err := app.Save(rule); err != nil {
		t.Fatal(err)
	}
	col, _ := app.FindCollectionByNameOrId("tickets")
	rec := core.NewRecord(col)
	rec.Set("title", "t")
	rec.Set("user", u.Id)
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
	trigger, _, _ := eng.defs.Trigger("tickets:ticket-created")

	waitingCh := make(chan struct{})
	w = NewEngine(app, eng.defs)
	w.waitWritable = func(ctx context.Context) error {
		if readonly.Active() {
			select {
			case waitingCh <- struct{}{}:
			default:
			}
		}
		return readonly.WaitInactive(ctx)
	}
	ctx, cancelFn := context.WithCancel(context.Background())
	go w.worker(ctx)
	// Runs before engineApp's cleanup (LIFO), so the worker is gone before
	// the app's database closes.
	t.Cleanup(func() {
		cancelFn()
		<-w.done
	})
	return w, event{TriggerRef: "tickets:ticket-created", Trigger: trigger, Record: rec, RuleID: rule.Id}, waitingCh, ranCh, cancelFn
}

func TestWorkerHoldsAnEventUntilReadOnlyEnds(t *testing.T) {
	readonly.Enter()
	t.Cleanup(readonly.Leave)
	w, ev, waiting, ran, _ := readOnlyWorker(t)

	w.enqueue(ev)
	select {
	case <-waiting:
	case <-time.After(5 * time.Second):
		t.Fatal("the worker did not wait for read-only mode to end before dispatching")
	}
	// The worker is now parked in the wait, so the action cannot have run.
	select {
	case <-ran:
		t.Fatal("the event was dispatched while the server was read-only")
	default:
	}

	readonly.Leave()
	select {
	case active := <-ran:
		if active {
			t.Fatal("the action ran while the server was read-only")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the held event was never dispatched after read-only mode ended")
	}
}

func TestWorkerStopsOnShutdownWhileReadOnly(t *testing.T) {
	readonly.Enter()
	t.Cleanup(readonly.Leave)
	w, ev, waiting, ran, cancel := readOnlyWorker(t)

	w.enqueue(ev)
	select {
	case <-waiting:
	case <-time.After(5 * time.Second):
		t.Fatal("the worker did not wait for read-only mode to end before dispatching")
	}
	cancel()
	select {
	case <-w.done:
	case <-time.After(5 * time.Second):
		t.Fatal("the worker did not stop on shutdown while it waited")
	}
	select {
	case <-ran:
		t.Fatal("a worker stopped by shutdown must not dispatch the event it held")
	default:
	}
}
