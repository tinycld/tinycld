package readonly

import (
	"context"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// tailStopBound bounds how long the app's terminate waits for tail writes in
// flight: long enough for one to finish its save, short enough that a stuck
// push does not hold up shutdown.
const tailStopBound = 5 * time.Second

// tailSet tracks the tail writes of one registered app, so its terminate can
// end their waits and let the ones already writing finish before the
// database closes.
type tailSet struct {
	ctx    context.Context
	cancel context.CancelFunc

	// mu orders terminated with wg.Add: once terminated is set, no Add can
	// race the Wait in stop.
	mu         sync.Mutex
	terminated bool
	wg         sync.WaitGroup
}

// tailSets is keyed by the app value passed to Register. Every core caller
// passes the same app value it registers with; a test app that never calls
// Register has no entry.
var (
	tailSetsMu sync.Mutex
	tailSets   = map[core.App]*tailSet{}
)

func bindTails(app core.App) {
	ctx, cancel := context.WithCancel(context.Background())
	ts := &tailSet{ctx: ctx, cancel: cancel}
	tailSetsMu.Lock()
	tailSets[app] = ts
	tailSetsMu.Unlock()
	// The handler runs before the chain's finalizer, which closes the
	// database, so every parked tail is ended before the close.
	app.OnTerminate().BindFunc(func(e *core.TerminateEvent) error {
		ts.stop()
		return e.Next()
	})
}

func (ts *tailSet) stop() {
	ts.mu.Lock()
	ts.terminated = true
	ts.cancel()
	ts.mu.Unlock()

	done := make(chan struct{})
	go func() {
		ts.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(tailStopBound):
		log.Warn("closing the app under tail writes that did not finish in time", "bound", tailStopBound)
	}
}

// TailContext returns the context a tail write (one that follows a request
// already accepted) waits under in WhenWritable, and a release func the tail
// must call when it has finished. The context ends after TailWait or when
// app terminates, whichever comes first. A supervised upgrade keeps the mode
// on until the process exits, so without the terminate the wait would
// outlive the process and the drop would never be logged.
//
// Call it before starting the tail's goroutine. For an app that was not
// passed to Register, only TailWait bounds the wait.
func TailContext(app core.App) (context.Context, func()) {
	tailSetsMu.Lock()
	ts := tailSets[app]
	tailSetsMu.Unlock()
	if ts == nil {
		return context.WithTimeout(context.Background(), TailWait)
	}

	ctx, cancel := context.WithTimeout(ts.ctx, TailWait)
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.terminated {
		// ctx is already done: the app is closing.
		return ctx, cancel
	}
	ts.wg.Add(1)
	var once sync.Once
	return ctx, func() {
		once.Do(func() {
			cancel()
			ts.wg.Done()
		})
	}
}
