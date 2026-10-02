package backup

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"tinycld.org/core/backup/repo"
	"tinycld.org/core/backup/snapshot"
	"tinycld.org/core/installjob"
)

// When a stop's bound runs out, the app closes under a run that is still going.
// Its terminal work then hits a closed database, and a panic there used to escape
// the run's finalizer — the recover guarding the run sits earlier in that same
// deferred func — and end the process mid-shutdown with the wrong exit status.
func TestATerminalStepThatPanicsIsLoggedNotFatal(t *testing.T) {
	buf := captureLog(t)
	t.Cleanup(ResetForTesting)
	SetShutdown(context.Background())

	app := newTestApp(t)
	makeUser(t, app, "owner@example.com", "owner")
	r := newBlockingRepo("put")
	done := make(chan error, 1)
	go func() {
		_, err := Run(app, Request{Kind: KindManual, Repo: r})
		done <- err
	}()
	select {
	case <-r.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the run never reached its repository")
	}

	if err := app.ResetBootstrapState(); err != nil {
		t.Fatal(err)
	}
	CancelAll()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the run never finished after the app closed under it")
	}
	if !strings.Contains(buf.String(), "terminal step panicked") {
		t.Fatalf("the panic was not logged:\n%s", buf.String())
	}
}

// A run that begins after the stop started would outlive the wait, so the stop
// refuses it. The refusal is a kind of busy: callers already turn that away
// without writing a row.
func TestStopAllRefusesEveryLaterRunOnThatApp(t *testing.T) {
	app := newTestApp(t)
	if err := StopAll(context.Background(), app); err != nil {
		t.Fatal(err)
	}

	_, startErr := Start(app, Request{Kind: KindScheduled, Repo: &recordingRepo{}})
	_, runErr := Run(app, Request{Kind: KindScheduled, Repo: &recordingRepo{}})
	_, restoreErr := StartRestore(app, RestoreRequest{Repo: &recordingRepo{}})
	for name, err := range map[string]error{"Start": startErr, "Run": runErr, "StartRestore": restoreErr} {
		if !errors.Is(err, ErrStopping) || !errors.Is(err, ErrBusy) {
			t.Errorf("%s after StopAll = %v, want ErrStopping (a kind of ErrBusy)", name, err)
		}
	}
	if installjob.Running() {
		t.Error("a refused run claimed the interlock")
	}
	rows, err := app.FindRecordsByFilter("backups", "id != ''", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("a refused run wrote %d ledger row(s)", len(rows))
	}
}

// The latch belongs to the app that is stopping. Another app in the same
// process — a second test app, a boot probe — must still run.
func TestStopAllLeavesAnotherAppRunning(t *testing.T) {
	stopped := newTestApp(t)
	if err := StopAll(context.Background(), stopped); err != nil {
		t.Fatal(err)
	}
	other := newTestApp(t)
	if _, err := Run(other, Request{Kind: KindScheduled, Repo: &recordingRepo{}}); err != nil {
		t.Fatalf("a run on another app was refused: %v", err)
	}
}

// stubbornRepo's Put ignores its context, as work cancellation cannot reach
// does — a snapshot's VACUUM INTO, a restore staging an uploaded archive.
type stubbornRepo struct {
	recordingRepo
	entered chan struct{}
	release chan struct{}
}

func (r *stubbornRepo) Put(context.Context, *snapshot.Snapshot, func(int64)) (repo.PutResult, error) {
	close(r.entered)
	<-r.release
	return repo.PutResult{Ref: "stubborn/1"}, nil
}

// The stop is bounded: a run it cannot end must not hold the process open, and
// the caller is told how many runs it is closing the app under.
func TestStopAllReportsARunItCouldNotWaitFor(t *testing.T) {
	t.Cleanup(ResetForTesting)
	SetShutdown(context.Background())

	app := newTestApp(t)
	r := &stubbornRepo{entered: make(chan struct{}), release: make(chan struct{})}
	if _, err := Start(app, Request{Kind: KindScheduled, Repo: r}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-r.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the run never reached its repository")
	}

	expired, cancel := context.WithCancel(context.Background())
	cancel()
	err := StopAll(expired, app)
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "1 backup or restore run(s)") {
		t.Fatalf("StopAll past its bound = %v, want the count of runs still going", err)
	}

	close(r.release)
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	if err := StopAll(ctx, app); err != nil {
		t.Fatalf("StopAll after the run let go = %v", err)
	}
	if installjob.Running() {
		t.Fatal("the run returned but still holds the interlock")
	}
}
