package readonly

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/cron"
)

// A cron job writes without a request, so the middleware cannot refuse it.
// While the mode is on, the scheduler must not start it; once the mode is
// left, the job runs again at its next due time. The fake clock makes "it did
// not run for three ticks" exact rather than a guess about timing.
func TestCronJobSkippedWhileActiveRunsAfterLeave(t *testing.T) {
	t.Cleanup(Leave)
	Enter() // outside the bubble: the mode's channel is process-wide
	synctest.Test(t, func(t *testing.T) {
		c := cron.New()
		guardCron(c)

		var ran atomic.Int32
		c.MustAdd("probe", "* * * * *", func() { ran.Add(1) })
		c.Start()
		defer c.Stop()

		time.Sleep(3 * time.Minute)
		synctest.Wait()
		if got := ran.Load(); got != 0 {
			t.Fatalf("the cron job ran %d times while read-only", got)
		}

		Leave()
		time.Sleep(time.Minute)
		synctest.Wait()
		if got := ran.Load(); got == 0 {
			t.Fatal("the cron job did not run after the mode was left")
		}
	})
}

// Register puts the guard on the app's own scheduler, which every job added
// through app.Cron() (core's, a package's, PocketBase's, a JS hook's) runs on.
func TestRegisterGuardsAppCron(t *testing.T) {
	t.Cleanup(Leave)
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()
	Register(app)

	ran := make(chan struct{}, 1)
	app.Cron().MustAdd("probe", "* * * * *", func() {
		select {
		case ran <- struct{}{}:
		default:
		}
	})
	Enter()
	app.Cron().SetInterval(10 * time.Millisecond)
	app.Cron().Start()
	defer app.Cron().Stop()

	select {
	case <-ran:
		t.Fatal("a job on the app's scheduler ran while read-only")
	case <-time.After(200 * time.Millisecond):
	}

	Leave()
	select {
	case <-ran:
	case <-time.After(5 * time.Second):
		t.Fatal("a job on the app's scheduler did not run after the mode was left")
	}
}

func TestWaitInactiveReturnsAtOnceWhenInactive(t *testing.T) {
	Leave()
	if err := WaitInactive(context.Background()); err != nil {
		t.Fatalf("WaitInactive = %v, want nil", err)
	}
}

func TestWaitInactiveStopsWithContext(t *testing.T) {
	t.Cleanup(Leave)
	Enter()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := WaitInactive(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitInactive = %v, want context.DeadlineExceeded", err)
	}
}

func TestWaitInactiveReturnsWhenLeft(t *testing.T) {
	t.Cleanup(Leave)
	Enter()
	done := make(chan error, 1)
	go func() { done <- WaitInactive(context.Background()) }()

	Leave()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WaitInactive = %v after Leave, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("WaitInactive still waits after Leave")
	}
}
