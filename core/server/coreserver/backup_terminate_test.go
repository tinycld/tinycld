package coreserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/backup"
	"tinycld.org/core/backup/format"
	"tinycld.org/core/backup/repo"
	"tinycld.org/core/backup/snapshot"
	"tinycld.org/core/installjob"
)

// heldRepo is a repository whose Put reports that it started and then, when
// hold is set, waits for its context as a client whose server went silent does.
type heldRepo struct {
	hold    bool
	entered chan struct{}
}

func newHeldRepo(hold bool) *heldRepo {
	return &heldRepo{hold: hold, entered: make(chan struct{}, 1)}
}

func (r *heldRepo) Kind() string { return "held" }
func (r *heldRepo) Put(ctx context.Context, _ *snapshot.Snapshot, _ func(int64)) (repo.PutResult, error) {
	r.entered <- struct{}{}
	if !r.hold {
		return repo.PutResult{Ref: "held/1"}, nil
	}
	<-ctx.Done()
	return repo.PutResult{}, ctx.Err()
}
func (r *heldRepo) Manifest(context.Context, repo.Ref) (format.Manifest, error) {
	return format.Manifest{}, nil
}
func (r *heldRepo) Fetch(context.Context, repo.Ref, string) error     { return nil }
func (r *heldRepo) List(context.Context) ([]repo.SnapshotInfo, error) { return nil, nil }

// stuckCallback is a callback receiver that takes the POST and never answers.
// It lets go only when the client gives up on the request, or when the test
// ends, so a run that never cancels its callback is still unblocked for cleanup.
func stuckCallback(t *testing.T) (url string, hit <-chan struct{}) {
	t.Helper()
	got := make(chan struct{}, 1)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case got <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(srv.Close)
	// Registered after srv.Close, so it runs first: Close waits for the handler.
	t.Cleanup(func() { close(release) })
	return srv.URL, got
}

// terminateSnapshot is what the app looked like at the instant every hook before
// the finalizer had run: the moment PocketBase is about to close the database.
type terminateSnapshot struct {
	status      string
	interlocked bool
}

// terminate runs the app's OnTerminate chain the way PocketBase does on SIGTERM,
// finalizer included, and reports the state just before the finalizer. The
// observer is bound last, so it runs after every hook the composition bound.
// It looks only once: the test app's own cleanup runs the chain again, on a
// database this run has already closed.
func terminate(t *testing.T, app core.App, rowID string) terminateSnapshot {
	t.Helper()
	var seen terminateSnapshot
	var observe sync.Once
	app.OnTerminate().BindFunc(func(e *core.TerminateEvent) error {
		observe.Do(func() {
			if row, err := e.App.FindRecordById("backups", rowID); err == nil {
				seen.status = row.GetString("status")
			}
			seen.interlocked = installjob.Running()
		})
		return e.Next()
	})
	done := make(chan error, 1)
	go func() {
		done <- app.OnTerminate().Trigger(&core.TerminateEvent{App: app}, func(e *core.TerminateEvent) error {
			return e.App.ResetBootstrapState()
		})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("terminate: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the terminate hooks never returned")
	}
	return seen
}

// terminateApp is the backup app with the boot wiring bound and the process
// lifetime set, as OnServe sets it in production.
func terminateApp(t *testing.T) core.App {
	t.Helper()
	app := backupTestApp(t)
	RegisterBackupBoot(app)
	backup.SetShutdown(context.Background())
	return app
}

// A stop during a backup must not close the database under the run: the run
// still writes its ledger row, notifies administrators and posts its callback.
// Before the fix the hook cancelled the run and returned at once, so all of that
// landed on a closed app and could crash the process on its way out.
func TestTerminateWaitsForABackupInItsRepository(t *testing.T) {
	app := terminateApp(t)
	r := newHeldRepo(true)
	cb, _ := stuckCallback(t)

	id, err := backup.Start(app, backup.Request{Kind: backup.KindManual, Repo: r, TargetHost: r.Kind(), Callback: cb})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-r.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the run never reached its repository")
	}

	seen := terminate(t, app, id)
	if seen.status != "failed" {
		t.Errorf("the row was %q when the database closed; the run's terminal write had not landed", seen.status)
	}
	if seen.interlocked {
		t.Error("the run still held the interlock when the database closed")
	}
}

// A run whose archive is written and whose callback receiver never answers is
// past every transfer the shutdown used to cancel. The stop must reach the
// callback too, or the hook either returns under a live run or waits on a peer.
func TestTerminateCancelsACallbackInFlight(t *testing.T) {
	app := terminateApp(t)
	r := newHeldRepo(false)
	cb, hit := stuckCallback(t)

	id, err := backup.Start(app, backup.Request{Kind: backup.KindManual, Repo: r, TargetHost: r.Kind(), Callback: cb})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-hit:
	case <-time.After(10 * time.Second):
		t.Fatal("the run never posted its callback")
	}

	seen := terminate(t, app, id)
	if seen.status != "succeeded" {
		t.Errorf("the row was %q, want succeeded", seen.status)
	}
	if seen.interlocked {
		t.Error("the run was still in its callback when the database closed")
	}
}
