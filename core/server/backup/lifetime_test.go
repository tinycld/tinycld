package backup

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
// process (a second test app) must still run.
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

// panickingFetchRepo stages part of a snapshot and then panics, as a repository
// client with a nil-pointer bug would midway through a fetch. The restore is
// armed by then: the marker names a pending dir that holds half a database.
type panickingFetchRepo struct{ failingFetchRepo }

func (panickingFetchRepo) Fetch(_ context.Context, _ repo.Ref, dir string) error {
	if err := os.WriteFile(filepath.Join(dir, "data.db"), []byte("half"), 0o600); err != nil {
		return err
	}
	panic("fetch blew up")
}

// A panic in the restore body used to escape the goroutine and end the process,
// leaving an armed marker over a half-staged pending dir. It must instead take
// the failure path a returned error takes: disarm, discard the staging, close
// the row as failed, tell the administrators, and release the interlock.
func TestARestoreThatPanicsFailsLikeAnyOtherRestore(t *testing.T) {
	app := newTestApp(t)
	resetRestoreState(t)
	makeUser(t, app, "owner@example.com", "owner")

	// On a goroutine of its own, as StartRestore runs it: a panic that escaped
	// would end the test binary rather than fail one test.
	type result struct {
		id  string
		err error
	}
	done := make(chan result, 1)
	go func() {
		id, err := Restore(app, RestoreRequest{Repo: panickingFetchRepo{}, Ref: repo.Ref("whatever"), Force: true})
		done <- result{id, err}
	}()
	var jobID string
	select {
	case res := <-done:
		if res.err == nil {
			t.Fatal("a restore that panicked reported success")
		}
		jobID = res.id
	case <-time.After(20 * time.Second):
		t.Fatal("the restore never finished")
	}

	row, err := app.FindRecordById("backups", jobID)
	if err != nil {
		t.Fatal(err)
	}
	if got := row.GetString("status"); got != "failed" {
		t.Fatalf("status %q, want failed", got)
	}
	if !strings.Contains(row.GetString("error"), "fetch blew up") {
		t.Errorf("the row does not say what panicked: %q", row.GetString("error"))
	}
	if _, err := os.Stat(armedPath(app)); !os.IsNotExist(err) {
		t.Error("the restore is still armed over a half-staged copy")
	}
	if _, err := os.Stat(pendingDir(app, jobID)); !os.IsNotExist(err) {
		t.Error("the half-staged pending dir was left behind")
	}
	if Restoring() {
		t.Error("the restoring flag is still set")
	}
	if installjob.Running() {
		t.Error("the interlock is still claimed")
	}
	notes, err := app.FindRecordsByFilter("notifications", "type = 'core.restore.failed'", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) == 0 {
		t.Error("nobody was told the restore failed")
	}
}

// A callback receiver that sends its headers and then never finishes the body
// held the run — and the job interlock behind it — until the process ended. The
// whole request is bounded, so the run finishes.
func TestACallbackThatStallsAfterItsHeadersIsAbandoned(t *testing.T) {
	prev := callbackTimeout
	callbackTimeout = 50 * time.Millisecond
	t.Cleanup(func() { callbackTimeout = prev })

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{"))
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	app := newTestApp(t)
	done := make(chan error, 1)
	var id string
	go func() {
		var err error
		id, err = Run(app, Request{Kind: KindScheduled, Repo: &recordingRepo{}, Callback: srv.URL})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the run never finished: its callback receiver stalled after the headers")
	}
	row, err := app.FindRecordById("backups", id)
	if err != nil {
		t.Fatal(err)
	}
	if got := row.GetString("status"); got != "succeeded" {
		t.Fatalf("status %q, want succeeded", got)
	}
	if installjob.Running() {
		t.Fatal("the interlock is still claimed")
	}
}
