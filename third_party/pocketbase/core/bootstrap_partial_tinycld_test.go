package core_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// failingConnect opens databases with core.DefaultDBConnect and fails the
// failAt-th open (1-based). It keeps every database it opened.
type failingConnect struct {
	mu     sync.Mutex
	failAt int
	calls  int
	opened []*dbx.DB
}

func (f *failingConnect) connect(path string) (*dbx.DB, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.calls == f.failAt {
		return nil, fmt.Errorf("open %d refused by the test", f.calls)
	}
	db, err := core.DefaultDBConnect(path)
	if err == nil {
		f.opened = append(f.opened, db)
	}
	return db, err
}

func (f *failingConnect) stillOpen() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, db := range f.opened {
		if err := db.DB().Ping(); err == nil {
			n++
		}
	}
	return n
}

// Bootstrap opens four pools in order: data, data (single writer), auxiliary,
// auxiliary (single writer). A process that opens and closes many apps calls
// ClearBootstrap after a failed Bootstrap; whichever open failed, no pool that
// opened before it may stay open.
func TestClearBootstrap_ClosesThePoolsOfAFailedBootstrap(t *testing.T) {
	for failAt := 2; failAt <= 4; failAt++ {
		t.Run(fmt.Sprintf("open %d fails", failAt), func(t *testing.T) {
			f := &failingConnect{failAt: failAt}
			app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir(), DBConnect: f.connect})

			err := app.Bootstrap()
			if err == nil {
				t.Fatal("Bootstrap succeeded with a refused open")
			}
			if app.IsBootstrapped() {
				t.Fatal("IsBootstrapped after a failed open")
			}
			if len(f.opened) != failAt-1 {
				t.Fatalf("opened %d pools before the refused open, want %d", len(f.opened), failAt-1)
			}

			if err := app.ClearBootstrap(); err != nil {
				t.Fatalf("ClearBootstrap: %v", err)
			}
			if n := f.stillOpen(); n != 0 {
				t.Fatalf("%d of %d pools are still open after ClearBootstrap", n, len(f.opened))
			}

			// The app can bootstrap again once the opens succeed.
			f.failAt = 0
			if err := app.Bootstrap(); err != nil {
				t.Fatalf("second Bootstrap: %v", err)
			}
			if err := app.ClearBootstrap(); err != nil {
				t.Fatalf("second ClearBootstrap: %v", err)
			}
			if n := f.stillOpen(); n != 0 {
				t.Fatalf("%d pools are still open after the second ClearBootstrap", n)
			}
		})
	}
}

// ClearBootstrap on an app that never bootstrapped stays a no-op: it runs no
// OnBootstrapClear hook and returns nil.
func TestClearBootstrap_NeverBootstrappedRunsNoHook(t *testing.T) {
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	ran := false
	app.OnBootstrapClear().BindFunc(func(e *core.BootstrapEvent) error {
		ran = true
		return errors.New("must not run")
	})
	if err := app.ClearBootstrap(); err != nil {
		t.Fatalf("ClearBootstrap: %v", err)
	}
	if ran {
		t.Fatal("OnBootstrapClear ran for an app that never bootstrapped")
	}
}
