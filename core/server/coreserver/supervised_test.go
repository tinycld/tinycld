//go:build unix

package coreserver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/drainhooks"
	"tinycld.org/core/installjob"
	"tinycld.org/core/listeners"
	"tinycld.org/core/readonly"
	"tinycld.org/core/supervise"
)

// supervisedFixture stands in for a supervisor: an inherited TCP listener
// named "http" and a socketpair whose child end is the inherited control fd.
// It returns the listener and the supervisor's end of the control socket.
func supervisedFixture(t *testing.T) (net.Listener, net.Conn) {
	t.Helper()
	l := loopbackListener(t)
	return l, supervisedFixtureWith(t, map[string]net.Listener{supervise.ListenerHTTP: l})
}

func loopbackListener(t *testing.T) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}

// supervisedFixtureWith is supervisedFixture with the inherited listeners
// given by name. It returns the supervisor's end of the control socket.
func supervisedFixtureWith(t *testing.T, ls map[string]net.Listener) net.Conn {
	t.Helper()
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	childEnd := os.NewFile(uintptr(fds[0]), "control-child")
	parentFile := os.NewFile(uintptr(fds[1]), "control-parent")
	parent, err := net.FileConn(parentFile)
	parentFile.Close()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { parent.Close(); childEnd.Close() })

	t.Cleanup(listeners.SetForTest(ls))
	t.Cleanup(listeners.SetFilesForTest(map[string]*os.File{supervise.ControlFD: childEnd}))
	t.Cleanup(closeControl)
	t.Cleanup(readonly.Leave)
	t.Cleanup(resetReplacementForTest)
	t.Cleanup(drainhooks.ResetForTest)
	return parent
}

// closeControl closes the connection registerSupervised opened on the
// control fd (a dup the fixture's own close does not reach) and forgets it.
func closeControl() {
	if c := currentControl(); c != nil {
		if conn, ok := c.conn.(net.Conn); ok {
			conn.Close()
		}
	}
	setControl(nil)
}

// recvLine reads one raw control line from the supervisor's end, bounded.
func recvLine(t *testing.T, c net.Conn, r *bufio.Reader) string {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("no control message: %v", err)
	}
	return strings.TrimSuffix(line, "\n")
}

// serveSupervised runs the OnServe chain the way apis.Serve does and starts
// serving on the listener the chain chose.
func serveSupervised(t *testing.T, app core.App, h http.Handler) (*core.ServeEvent, chan error) {
	t.Helper()
	e := &core.ServeEvent{App: app, Server: &http.Server{Handler: h}}
	if err := app.OnServe().Trigger(e, func(e *core.ServeEvent) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if e.Listener == nil {
		t.Fatal("OnServe left no listener")
	}
	served := make(chan error, 1)
	go func() { served <- e.Server.Serve(e.Listener) }()
	return e, served
}

// ackRestarts plays the supervisor's side of a restart: it acks every
// restart, and passes on every line it reads.
func ackRestarts(t *testing.T, parent net.Conn) <-chan string {
	t.Helper()
	lines := make(chan string, 8)
	go func() {
		r := bufio.NewReader(parent)
		for {
			m, err := supervise.Recv(r)
			if err != nil {
				return
			}
			line, _ := json.Marshal(m)
			lines <- string(line)
			if m.Type == supervise.MsgRestart {
				supervise.Send(parent, supervise.Msg{Type: supervise.MsgRestartAck})
			}
		}
	}()
	return lines
}

func nextLine(t *testing.T, lines <-chan string) string {
	t.Helper()
	select {
	case l := <-lines:
		return l
	case <-time.After(5 * time.Second):
		t.Fatal("no control message")
		return ""
	}
}

func shortAckTimeout(t *testing.T) {
	t.Helper()
	prev := restartAckTimeout
	restartAckTimeout = 200 * time.Millisecond
	t.Cleanup(func() { restartAckTimeout = prev })
}

func stubSelfTerminate(t *testing.T, fn func() error) {
	t.Helper()
	prev := selfTerminate
	selfTerminate = fn
	t.Cleanup(func() { selfTerminate = prev })
}

func notDevelopment(t *testing.T) {
	t.Helper()
	prev := isDevelopment
	isDevelopment = func() bool { return false }
	t.Cleanup(func() { isDevelopment = prev })
	t.Setenv("TINYCLD_STATE_DIR", t.TempDir())
}

func recordExit(t *testing.T) *[]int {
	t.Helper()
	var codes []int
	prev := exitProcess
	exitProcess = func(code int) { codes = append(codes, code) }
	t.Cleanup(func() { exitProcess = prev })
	return &codes
}

func TestSupervisedServesOnTheInheritedListener(t *testing.T) {
	l, _ := supervisedFixture(t)
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	registerSupervised(app)

	e := &core.ServeEvent{App: app, Server: &http.Server{}}
	var seen net.Listener
	if err := app.OnServe().Trigger(e, func(e *core.ServeEvent) error {
		seen = e.Listener
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// e.Listener wraps the inherited one so a drain can stop accepting on it.
	if seen == nil || seen.Addr().String() != l.Addr().String() {
		t.Fatalf("e.Listener = %v, want one on the inherited %v", seen, l.Addr())
	}
}

func TestSupervisedSendsReadyAfterServe(t *testing.T) {
	_, parent := supervisedFixture(t)
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	registerSupervised(app)

	reader := bufio.NewReader(parent)
	earlyReady := false
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		// Ready is written synchronously, so had it gone out before this
		// handler finished, it would already be waiting on the socket.
		parent.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
		if _, err := reader.Peek(1); err == nil {
			earlyReady = true
		}
		return nil
	})
	e, served := serveSupervised(t, app, http.NotFoundHandler())
	t.Cleanup(func() { e.Server.Close(); <-served })

	if earlyReady {
		t.Fatal("ready was sent before the other OnServe handlers finished")
	}
	if got := recvLine(t, parent, reader); got != `{"type":"ready","version":1}` {
		t.Fatalf("control message = %q", got)
	}
}

func TestSupervisedDrainStopsAcceptingAndFinishes(t *testing.T) {
	l, parent := supervisedFixture(t)
	addr := l.Addr().String()
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})

	// The real self-terminate raises SIGTERM, and PocketBase answers it by
	// triggering OnTerminate. Here the trigger is direct.
	terminated := make(chan struct{})
	stubSelfTerminate(t, func() error {
		go func() {
			app.OnTerminate().Trigger(&core.TerminateEvent{App: app}, func(*core.TerminateEvent) error { return nil })
			close(terminated)
		}()
		return nil
	})
	registerSupervised(app)

	inFlight := make(chan struct{})
	hold := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(hold) }) }
	e, served := serveSupervised(t, app, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(inFlight)
		<-hold
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(func() { e.Server.Close() })
	// Runs first: Close waits for the handler, so a test that fails while
	// the request is held must let it go.
	t.Cleanup(release)

	reader := bufio.NewReader(parent)
	if got := recvLine(t, parent, reader); got != `{"type":"ready","version":1}` {
		t.Fatalf("control message = %q", got)
	}

	slow := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://" + addr + "/slow")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				err = fmt.Errorf("status %d", resp.StatusCode)
			}
		}
		slow <- err
	}()
	<-inFlight

	if err := supervise.Send(parent, supervise.Msg{Type: supervise.MsgDrain}); err != nil {
		t.Fatal(err)
	}
	waitRefused(t, addr, "the server")
	select {
	case <-terminated:
		t.Fatal("the drain finished with a request still in flight")
	default:
	}

	release()
	if err := <-slow; err != nil {
		t.Fatalf("the in-flight request was cut: %v", err)
	}
	// The wait above for accepting to stop is generous so a loaded runner
	// passes; this bound keeps a slow drain from hiding behind it. With its
	// last request answered, the drain has nothing left to wait for, so it
	// must end far inside its budget.
	waitClosed(t, terminated, supervise.ChildDrainTimeout/3, "the drain to finish after its last request")
	waitServed(t, served)
}

// A supervisor that dies closes its end; the child must keep serving rather
// than take the deployment down with it.
func TestSupervisedControlCloseKeepsServing(t *testing.T) {
	l, parent := supervisedFixture(t)
	addr := l.Addr().String()
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	// A drain started by the close would stop the server, as the real one
	// does, so the requests below would fail.
	terminated := terminateOnDrain(t, app)
	closed := logSeen(t, "the supervisor closed the control socket; serving on without it")
	registerSupervised(app)
	e, served := serveSupervised(t, app, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "serving")
	}))
	t.Cleanup(func() { e.Server.Close(); <-served })
	if got := recvLine(t, parent, bufio.NewReader(parent)); got != `{"type":"ready","version":1}` {
		t.Fatalf("control message = %q", got)
	}

	parent.Close()
	waitClosed(t, closed, 5*time.Second, "the server to read the closed control socket")
	// A fresh connection per request: one kept alive from before the close
	// would not show that the server still accepts.
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	for range 3 {
		resp, err := client.Get("http://" + addr + "/")
		if err != nil {
			t.Fatalf("the server stopped serving after the control socket closed: %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if string(body) != "serving" {
			t.Fatalf("body = %q", body)
		}
	}
	select {
	case <-terminated:
		t.Fatal("the closed control socket started a drain")
	default:
	}
	if readonly.Active() {
		t.Fatal("the closed control socket made the server read-only")
	}
}

// A garbled message must not stop the reader or start a drain.
func TestSupervisedGarbledControlStartsNoDrain(t *testing.T) {
	called := false
	stubSelfTerminate(t, func() error { called = true; return nil })
	d := &drain{}
	watchControl(bufio.NewReader(strings.NewReader(`{"type":"bogus","version":1}`+"\n"+"not json\n")), make(chan struct{}, 1), d)
	if called || d.requested {
		t.Fatal("a closed or garbled control socket started a drain")
	}
}

// registerSupervised opens its own connection on the control fd, which the
// fixture's close of the fd does not reach. The fixture must close it, or
// every supervised test leaks one.
func TestSupervisedFixtureClosesTheControlConn(t *testing.T) {
	var conn net.Conn
	t.Run("supervised", func(t *testing.T) {
		supervisedFixture(t)
		registerSupervised(core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()}))
		if c := currentControl(); c != nil {
			conn, _ = c.conn.(net.Conn)
		}
	})
	if conn == nil {
		t.Fatal("registerSupervised opened no control connection")
	}
	if _, err := conn.Write([]byte("\n")); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("the control connection outlived its test: write err = %v", err)
	}
}

// logSeen returns a channel that closes once a record with message msg is
// logged.
func logSeen(t *testing.T, msg string) <-chan struct{} {
	t.Helper()
	seen := make(chan struct{})
	var once sync.Once
	prev := slog.Default()
	// Not prev's handler: wrapping slog's built-in default handler in a new
	// default deadlocks, because that handler writes through the log package,
	// which SetDefault points back at the new default.
	inner := slog.NewTextHandler(os.Stderr, nil)
	slog.SetDefault(slog.New(&matchHandler{Handler: inner, msg: msg, hit: func() { once.Do(func() { close(seen) }) }}))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return seen
}

type matchHandler struct {
	slog.Handler
	msg string
	hit func()
}

func (h *matchHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Message == h.msg {
		h.hit()
	}
	return h.Handler.Handle(ctx, r)
}

func (h *matchHandler) WithAttrs(as []slog.Attr) slog.Handler {
	return &matchHandler{Handler: h.Handler.WithAttrs(as), msg: h.msg, hit: h.hit}
}

func (h *matchHandler) WithGroup(name string) slog.Handler {
	return &matchHandler{Handler: h.Handler.WithGroup(name), msg: h.msg, hit: h.hit}
}

// drain is a permanent v1 message: a supervisor that states a newer version
// must still be obeyed, and an unknown type must not stop the reader.
func TestSupervisedActsOnNewerVersionDrain(t *testing.T) {
	called := false
	stubSelfTerminate(t, func() error { called = true; return nil })
	d := &drain{}
	in := `{"type":"bogus","version":3}` + "\n" + `{"type":"drain","version":2}` + "\n"
	watchControl(bufio.NewReader(strings.NewReader(in)), make(chan struct{}, 1), d)
	if !called || !d.requested {
		t.Fatal("a drain from a newer protocol version was not acted on")
	}
}

func TestSupervisedRestartAckFromNewerVersionCounts(t *testing.T) {
	acks := make(chan struct{}, 1)
	watchControl(bufio.NewReader(strings.NewReader(`{"type":"restart-ack","version":2}`+"\n")), acks, &drain{})
	select {
	case <-acks:
	default:
		t.Fatal("a restart-ack from a newer protocol version was not passed on")
	}
}

// A supervisor that does not answer would leave this process read-only for
// good. Exit 75 is the restart every supervisor handles.
func TestSupervisedRestartWithoutAckExits75(t *testing.T) {
	_, parent := supervisedFixture(t)
	notDevelopment(t)
	shortAckTimeout(t)
	exits := recordExit(t)
	registerSupervised(core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()}))

	requestRestart(false)
	if got := recvLine(t, parent, bufio.NewReader(parent)); got != `{"type":"restart","version":1}` {
		t.Fatalf("control message = %q", got)
	}
	if len(*exits) != 1 || (*exits)[0] != restartExitCode {
		t.Fatalf("exits = %v, want [%d]", *exits, restartExitCode)
	}
}

// A rebuild can ask for a restart before this process has said ready. The
// ack must still be read, or the process would exit 75 for nothing.
func TestSupervisedRestartBeforeReadyIsAcked(t *testing.T) {
	_, parent := supervisedFixture(t)
	notDevelopment(t)
	shortAckTimeout(t)
	exits := recordExit(t)
	registerSupervised(core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()}))
	lines := ackRestarts(t, parent)

	if !requestRestart(false) {
		t.Fatal("restart not under way")
	}
	if got := nextLine(t, lines); got != `{"type":"restart","version":1}` {
		t.Fatalf("control message = %q", got)
	}
	if len(*exits) != 0 {
		t.Fatalf("exited %v although the supervisor acked", *exits)
	}
}

func TestSupervisedRequestRestartSendsAndKeepsServing(t *testing.T) {
	_, parent := supervisedFixture(t)
	notDevelopment(t)
	exits := recordExit(t)
	registerSupervised(core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()}))
	lines := ackRestarts(t, parent)

	if !requestRestart(false) {
		t.Fatal("requestRestart under a supervisor did not report the restart under way")
	}
	if got := nextLine(t, lines); got != `{"type":"restart","version":1}` {
		t.Fatalf("control message = %q", got)
	}
	if len(*exits) != 0 {
		t.Fatalf("requestRestart exited %v under a supervisor", *exits)
	}
	if !readonly.Active() {
		t.Fatal("a process that asked to be replaced must stop writing")
	}
}

func TestSupervisedBackupRestoreRestartIsCold(t *testing.T) {
	_, parent := supervisedFixture(t)
	notDevelopment(t)
	exits := recordExit(t)
	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.ResetBootstrapState() })
	registerSupervised(app)
	lines := ackRestarts(t, parent)

	if !restartForRestore(app) {
		t.Fatal("restartForRestore did not report the restart under way")
	}
	if got := nextLine(t, lines); got != `{"type":"restart","cold":true,"version":1}` {
		t.Fatalf("control message = %q", got)
	}
	if len(*exits) != 0 {
		t.Fatalf("exited %v under a supervisor", *exits)
	}
}

func TestRequestRestartWithoutSupervisorExits75(t *testing.T) {
	notDevelopment(t)
	exits := recordExit(t)
	if listeners.Supervised() {
		t.Fatal("test process is supervised")
	}
	requestRestart(false)
	if len(*exits) != 1 || (*exits)[0] != restartExitCode {
		t.Fatalf("exits = %v, want [%d]", *exits, restartExitCode)
	}
}

// A supervised process that has no control socket cannot ask for a swap. Exit
// 75 is what a child too old for the protocol does, and the supervisor already
// answers it with a cold restart.
func TestSupervisedRestartWithoutControlSocketExits75(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	t.Cleanup(listeners.SetForTest(map[string]net.Listener{supervise.ListenerHTTP: l}))
	t.Cleanup(func() { setControl(nil) })
	t.Cleanup(readonly.Leave)
	t.Cleanup(resetReplacementForTest)
	notDevelopment(t)
	exits := recordExit(t)
	registerSupervised(core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()}))

	requestRestart(false)
	if len(*exits) != 1 || (*exits)[0] != restartExitCode {
		t.Fatalf("exits = %v, want [%d]", *exits, restartExitCode)
	}
}

func TestSupervisedRebuildIsReadOnlyFromTheBackup(t *testing.T) {
	supervisedFixture(t)
	for _, failAt := range []string{"", "backup", "migrate", "activate"} {
		t.Run("fail="+failAt, func(t *testing.T) {
			t.Setenv("TINYCLD_STATE_DIR", t.TempDir())
			t.Cleanup(readonly.Leave)
			var restored, restarted bool
			var atPipeline, atBackup bool
			deps := happyDeps(&restored, &restarted)
			deps.pipeline = func(*installjob.Job, string) (buildOutput, error) {
				atPipeline = readonly.Active()
				return buildOutput{}, nil
			}
			deps.backupDB = func() error {
				atBackup = readonly.Active()
				if failAt == "backup" {
					return errors.New("disk full")
				}
				return nil
			}
			if failAt == "migrate" {
				deps.syncMig = func(string) (SyncResult, error) { return SyncResult{}, errors.New("down broke") }
			}
			if failAt == "activate" {
				deps.activate = func(string) error { return errors.New("symlink") }
			}

			job := &installjob.Job{ID: "j", Done: make(chan struct{})}
			err := rebuildWith(job, RebuildManifest{BuildID: "build-ro"}, deps)
			if (err != nil) != (failAt != "") {
				t.Fatalf("rebuildWith err = %v", err)
			}
			if atPipeline {
				t.Fatal("read-only began before the backup step")
			}
			if !atBackup {
				t.Fatal("the backup ran while writes were still accepted")
			}
			if failAt == "" && !readonly.Active() {
				t.Fatal("a rebuild that asked for a restart left read-only mode")
			}
			if failAt != "" && readonly.Active() {
				t.Fatalf("a rebuild that failed at %s left the server read-only", failAt)
			}
		})
	}
}

func TestUnsupervisedRebuildNeverReadOnly(t *testing.T) {
	t.Setenv("TINYCLD_STATE_DIR", t.TempDir())
	t.Cleanup(readonly.Leave)
	var restored, restarted, atBackup bool
	deps := happyDeps(&restored, &restarted)
	deps.backupDB = func() error { atBackup = readonly.Active(); return nil }
	job := &installjob.Job{ID: "j", Done: make(chan struct{})}
	if err := rebuildWith(job, RebuildManifest{BuildID: "build-plain"}, deps); err != nil {
		t.Fatal(err)
	}
	if atBackup || readonly.Active() {
		t.Fatal("an unsupervised rebuild entered read-only mode")
	}
}

// After a supervised restart request this process only waits to be drained.
// A job started in that time (an auto-upgrade tick, a scheduled backup) would
// work on data and a build the next process already owns, and a failing one
// would re-open writes on its way out.
func TestSupervisedRestartHoldsTheInterlockAndReadOnly(t *testing.T) {
	_, parent := supervisedFixture(t)
	notDevelopment(t)
	recordExit(t)
	registerSupervised(core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()}))
	lines := ackRestarts(t, parent)

	restarting := installjob.New("install", "acme", "")
	if _, ok := installjob.Claim(restarting); !ok {
		t.Fatal("claim on an idle interlock lost")
	}
	if !requestRestart(false) {
		t.Fatal("restart not under way")
	}
	nextLine(t, lines)
	finishJob(restarting)

	if _, ok := installjob.Claim(installjob.New("backup", "", "")); ok {
		t.Fatal("a new job claimed the interlock after this process asked to be replaced")
	}
	if !installjob.Running() {
		t.Fatal("the auto-upgrade tick would see an idle interlock")
	}

	pauseWritesForBackup()()
	if !readonly.Active() {
		t.Fatal("a failure path re-opened writes after this process asked to be replaced")
	}
}

// Without a supervisor the process exits on a restart, so nothing is held; a
// dev-mode restart that does nothing must not wedge the interlock either.
func TestUnsupervisedRestartHoldsNothing(t *testing.T) {
	t.Cleanup(resetReplacementForTest)
	recordExit(t)
	requestRestart(false) // dev mode: the test binary runs from the temp dir
	notDevelopment(t)
	requestRestart(false)

	job := installjob.New("backup", "", "")
	if _, ok := installjob.Claim(job); !ok {
		t.Fatal("an unsupervised restart left the interlock held")
	}
	installjob.Release(job)
}

func resetReplacementForTest() {
	beingReplaced.Store(false)
	installjob.ResetForTesting()
}

// A package that serves its own port must stop accepting when the drain
// begins, not after the HTTP drain: the handler has to run while an HTTP
// request is still in flight. Here the handler is what lets that request
// finish, so a handler that ran after the HTTP drain would leave the drain
// waiting out its whole budget.
func TestSupervisedDrainRunsBeginHandlersFirst(t *testing.T) {
	t.Cleanup(drainhooks.ResetForTest)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	inFlight := make(chan struct{})
	release := make(chan struct{})
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(inFlight)
		<-release
	})}
	dr := supervise.NewDrainer(srv)
	go srv.Serve(dr.Listener(l))
	t.Cleanup(func() { srv.Close() })
	go func() {
		if resp, err := http.Get("http://" + l.Addr().String()); err == nil {
			resp.Body.Close()
		}
	}()
	<-inFlight

	drainhooks.OnBegin("acme-listeners", func() { close(release) })
	d := &drain{drainer: dr, requested: true}
	done := make(chan struct{})
	go func() { d.shutdown(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("the drain waited on the request before running the drain-begin handlers")
	}
}
