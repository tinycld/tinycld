//go:build unix

package coreserver

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/listeners"
	"tinycld.org/core/supervise"
)

// The test binary also plays a supervised server in its own process, so a
// drain can go through the real signal path: the drain message makes the
// server SIGTERM itself, and PocketBase answers the signal.
func TestMain(m *testing.M) {
	if os.Getenv("CORESERVER_TEST_ROLE") == "supervised-serve" {
		os.Exit(supervisedServeRole())
	}
	os.Exit(m.Run())
}

// supervisedServeRole serves the way `tinycld serve` does under a
// supervisor, with one route: /slow answers "started", then holds the
// request until a line arrives on stdin, then answers "finished", or
// "canceled" when the request's context ended while it was held.
func supervisedServeRole() int {
	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: os.Getenv("CORESERVER_TEST_DATA")})
	registerSupervised(app)
	release := make(chan struct{})
	go func() {
		bufio.NewReader(os.Stdin).ReadString('\n')
		close(release)
	}()
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.InstallerFunc = nil
		e.Router.GET("/slow", func(re *core.RequestEvent) error {
			re.Response.WriteHeader(http.StatusOK)
			io.WriteString(re.Response, "started\n")
			re.Flush()
			<-release
			if re.Request.Context().Err() != nil {
				io.WriteString(re.Response, "canceled\n")
				return nil
			}
			io.WriteString(re.Response, "finished\n")
			return nil
		})
		return e.Next()
	})
	app.RootCmd.SetArgs([]string{"serve", "--http=" + os.Getenv("CORESERVER_TEST_ADDR")})
	if err := app.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "serve:", err)
		return 1
	}
	return 0
}

// supervisedServer is a supervised server process and the supervisor's side
// of it.
type supervisedServer struct {
	cmd     *exec.Cmd
	addr    string
	control net.Conn
	release io.WriteCloser
	exited  chan struct{}
	output  *bytes.Buffer
}

// startSupervisedServer starts the test binary as a supervised server, the
// way a supervisor starts a child: the listener and the control socket go
// over as inherited fds. This process keeps no copy of the listener, so
// once the server stops accepting a connect is refused.
func startSupervisedServer(t *testing.T) *supervisedServer {
	t.Helper()
	l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		l.Close()
		t.Fatal(err)
	}
	childEnd := os.NewFile(uintptr(fds[0]), "control-child")
	parentFile := os.NewFile(uintptr(fds[1]), "control-parent")
	control, err := net.FileConn(parentFile)
	parentFile.Close()
	if err != nil {
		l.Close()
		childEnd.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { control.Close() })

	s := &supervisedServer{addr: l.Addr().String(), control: control, exited: make(chan struct{}), output: &bytes.Buffer{}}
	set := &listeners.Set{}
	if err := set.AddListener(supervise.ListenerHTTP, l); err != nil {
		t.Fatal(err)
	}
	set.AddFile(supervise.ControlFD, childEnd)
	s.cmd = exec.Command(os.Args[0], "-test.run=^$")
	s.cmd.Env = append(os.Environ(),
		"CORESERVER_TEST_ROLE=supervised-serve",
		"CORESERVER_TEST_DATA="+t.TempDir(),
		"CORESERVER_TEST_ADDR="+s.addr,
	)
	s.cmd.Stdout, s.cmd.Stderr = s.output, s.output
	set.Apply(s.cmd)
	if s.release, err = s.cmd.StdinPipe(); err != nil {
		t.Fatal(err)
	}
	startErr := s.cmd.Start()
	// The child has its copies; this process keeps none.
	l.Close()
	for _, f := range s.cmd.ExtraFiles {
		f.Close()
	}
	if startErr != nil {
		t.Fatal(startErr)
	}
	go func() {
		s.cmd.Wait()
		close(s.exited)
	}()
	t.Cleanup(func() {
		s.release.Close()
		s.cmd.Process.Kill()
		<-s.exited
		if t.Failed() {
			t.Logf("server output:\n%s", s.output)
		}
	})
	return s
}

// slowRequest starts a GET of /slow and returns once the server is holding
// it. The returned channel gets the whole body once the request ends.
func (s *supervisedServer) slowRequest(t *testing.T) <-chan string {
	t.Helper()
	client := &http.Client{Timeout: supervise.ChildDrainTimeout}
	resp, err := client.Get("http://" + s.addr + "/slow")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	r := bufio.NewReader(resp.Body)
	started, err := r.ReadString('\n')
	if err != nil || started != "started\n" {
		t.Fatalf("the slow request began with %q, err %v", started, err)
	}
	body := make(chan string, 1)
	go func() {
		rest, err := io.ReadAll(r)
		if err != nil {
			rest = append(rest, []byte("read error: "+err.Error())...)
		}
		body <- started + string(rest)
	}()
	return body
}

// A drain under a supervisor goes through the process's own SIGTERM, which
// PocketBase answers by running its terminate path; that path must stop
// accepting and wait for a request in flight, rather than give it the one
// second PocketBase's own shutdown allows and cancel it.
func TestSupervisedDrainThroughSIGTERM(t *testing.T) {
	s := startSupervisedServer(t)
	if got := recvLine(t, s.control, bufio.NewReader(s.control)); got != `{"type":"ready","version":1}` {
		t.Fatalf("control message = %q", got)
	}
	body := s.slowRequest(t)

	if err := supervise.Send(s.control, supervise.Msg{Type: supervise.MsgDrain}); err != nil {
		t.Fatal(err)
	}
	waitRefused(t, s.addr, "the server")
	select {
	case <-s.exited:
		t.Fatal("the server exited with a request in flight")
	case <-time.After(pocketBaseShutdownWindow):
	}

	start := time.Now()
	if _, err := io.WriteString(s.release, "\n"); err != nil {
		t.Fatal(err)
	}
	if got := <-body; got != "started\nfinished\n" {
		t.Fatalf("the request in flight got %q, want it finished", got)
	}
	// With its last request answered, the drain has nothing to wait for.
	waitClosed(t, s.exited, supervise.ChildDrainTimeout/3, "the server to exit after the drain")
	if code := s.cmd.ProcessState.ExitCode(); code != 0 {
		t.Fatalf("the drained server exited %d, want 0", code)
	}
	t.Logf("the drain ended %s after its last request", time.Since(start))
}
