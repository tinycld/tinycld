//go:build unix

package coreserver

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

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
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })

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

	t.Cleanup(listeners.SetForTest(map[string]net.Listener{supervise.ListenerHTTP: l}))
	t.Cleanup(listeners.SetFilesForTest(map[string]*os.File{supervise.ControlFD: childEnd}))
	t.Cleanup(func() { setControl(nil) })
	t.Cleanup(readonly.Leave)
	t.Cleanup(resetReplacementForTest)
	return l, parent
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
	release := make(chan struct{})
	e, served := serveSupervised(t, app, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(inFlight)
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(func() { e.Server.Close() })

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
	// The test holds the only copy of the listener, so once the drain
	// stops accepting a connection is refused. Under a supervisor its own
	// copy keeps the port open and the next child accepts.
	stopped := time.Now().Add(time.Second)
	for {
		c, err := net.DialTimeout("tcp", addr, time.Second)
		if err != nil {
			break
		}
		c.Close()
		if time.Now().After(stopped) {
			t.Fatal("the server still accepts 1 s after drain")
		}
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case <-terminated:
		t.Fatal("the drain finished with a request still in flight")
	default:
	}

	close(release)
	if err := <-slow; err != nil {
		t.Fatalf("the in-flight request was cut: %v", err)
	}
	select {
	case <-terminated:
	case <-time.After(5 * time.Second):
		t.Fatal("the drain did not finish after the last request")
	}
	select {
	case err := <-served:
		if !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("Serve returned %v, want ErrServerClosed", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after the drain")
	}
}

// A supervisor that dies closes its end; the child must keep serving rather
// than take the deployment down with it.
func TestSupervisedControlCloseKeepsServing(t *testing.T) {
	called := false
	stubSelfTerminate(t, func() error { called = true; return nil })
	d := &drain{}
	watchControl(bufio.NewReader(strings.NewReader(`{"type":"bogus","version":1}`+"\n"+"not json\n")), d)
	if called || d.requested {
		t.Fatal("a closed or garbled control socket started a drain")
	}
}

func TestSupervisedRequestRestartSendsAndKeepsServing(t *testing.T) {
	_, parent := supervisedFixture(t)
	notDevelopment(t)
	exits := recordExit(t)
	registerSupervised(core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()}))

	if !requestRestart(false) {
		t.Fatal("requestRestart under a supervisor did not report the restart under way")
	}
	if got := recvLine(t, parent, bufio.NewReader(parent)); got != `{"type":"restart","version":1}` {
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

	if !restartForRestore(app) {
		t.Fatal("restartForRestore did not report the restart under way")
	}
	if got := recvLine(t, parent, bufio.NewReader(parent)); got != `{"type":"restart","cold":true,"version":1}` {
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

	restarting := installjob.New("install", "acme", "")
	if _, ok := installjob.Claim(restarting); !ok {
		t.Fatal("claim on an idle interlock lost")
	}
	if !requestRestart(false) {
		t.Fatal("restart not under way")
	}
	recvLine(t, parent, bufio.NewReader(parent))
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
