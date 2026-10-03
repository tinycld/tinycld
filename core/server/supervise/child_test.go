//go:build unix

package supervise

import (
	"net"
	"os"
	"syscall"
	"testing"
	"time"
)

func bindLoopback(t *testing.T) *net.TCPListener {
	t.Helper()
	l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}

func startTestChild(t *testing.T, r *testRoot, dir string, ports []heldPort) *child {
	t.Helper()
	t.Setenv("TINYCLD_STATE_DIR", r.dir)
	c, err := startChild(childSpec{dir: dir, args: []string{"serve"}, env: os.Environ(), ports: ports})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if !c.exited() {
			c.signalGroup(syscall.SIGKILL)
			c.waitExit(5 * time.Second)
		}
	})
	return c
}

func nextMsg(t *testing.T, c *child) Msg {
	t.Helper()
	select {
	case m := <-c.msgs:
		return m
	case <-c.done:
		t.Fatalf("the child exited with %d before sending a message", c.code)
	case <-time.After(20 * time.Second):
		t.Fatal("no message from the child")
	}
	return Msg{}
}

func TestChildGetsListenersAndControl(t *testing.T) {
	r := newTestRoot(t)
	dir := r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_BODY": "A"})
	l := bindLoopback(t)
	c := startTestChild(t, r, dir, []heldPort{{name: ListenerHTTP, addr: l.Addr().String(), l: l}})

	if m := nextMsg(t, c); m.Type != MsgReady || !c.isReady() {
		t.Fatalf("first message = %+v, ready = %v", m, c.isReady())
	}
	if got, err := get(l.Addr().String()); err != nil || got != "A" {
		t.Fatalf("GET = %q %v", got, err)
	}
	if err := c.send(Msg{Type: MsgDrain}); err != nil {
		t.Fatal(err)
	}
	if !c.waitExit(10 * time.Second) {
		t.Fatal("the child did not exit after drain")
	}
	if c.code != 0 {
		t.Fatalf("exit code = %d", c.code)
	}
	if r.index("A", "drain", 1) < 0 {
		t.Fatal("the child did not see drain")
	}
	// The child's exit must not close the supervisor's own listener.
	conn, err := net.DialTimeout("tcp", l.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("the supervisor's listener closed with the child: %v", err)
	}
	conn.Close()
}

func TestChildRunsInItsOwnProcessGroup(t *testing.T) {
	r := newTestRoot(t)
	dir := r.build("a", knobs{"FAKE_NAME": "A"})
	c := startTestChild(t, r, dir, nil)
	nextMsg(t, c)
	pgid, err := syscall.Getpgid(c.pid)
	if err != nil {
		t.Fatal(err)
	}
	if pgid != c.pid {
		t.Fatalf("child pgid = %d, want its own pid %d", pgid, c.pid)
	}
}

func TestChildIgnoresUnknownMessages(t *testing.T) {
	r := newTestRoot(t)
	dir := r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SEND_JUNK": "1"})
	c := startTestChild(t, r, dir, nil)
	if m := nextMsg(t, c); m.Type != MsgReady {
		t.Fatalf("first message passed on = %+v, want ready", m)
	}
}

// ready is a permanent v1 message: a child newer than the supervisor states
// a higher version, and the supervisor must still act on it.
func TestChildActsOnNewerVersionReady(t *testing.T) {
	r := newTestRoot(t)
	dir := r.build("a", knobs{"FAKE_NAME": "A", "FAKE_READY_VERSION": "2"})
	c := startTestChild(t, r, dir, nil)
	if m := nextMsg(t, c); m.Type != MsgReady || m.Version != 2 || !c.isReady() {
		t.Fatalf("first message = %+v, ready = %v", m, c.isReady())
	}
}

func TestChildAcksRestart(t *testing.T) {
	r := newTestRoot(t)
	dir := r.build("a", knobs{"FAKE_NAME": "A", "FAKE_RESTART_BEFORE_READY": "1"})
	c := startTestChild(t, r, dir, nil)
	if m := nextMsg(t, c); m.Type != MsgRestart {
		t.Fatalf("first message = %+v, want restart", m)
	}
	if m := nextMsg(t, c); m.Type != MsgReady {
		t.Fatalf("second message = %+v, want ready", m)
	}
	if r.index("A", "restart-ack", 1) < 0 || r.index("A", "no-ack", 1) >= 0 {
		t.Fatal("the child got no ack for its restart")
	}
}

func TestChildStopKillsAChildThatIgnoresTerm(t *testing.T) {
	r := newTestRoot(t)
	dir := r.build("a", knobs{"FAKE_NAME": "A", "FAKE_NEVER_READY": "1", "FAKE_IGNORE_TERM": "1"})
	c := startTestChild(t, r, dir, nil)
	r.waitEvent("A", "listeners", 1)

	start := time.Now()
	stopChild(c, 500*time.Millisecond)
	if !c.exited() {
		t.Fatal("stop returned with the child still running")
	}
	if c.code != 128+int(syscall.SIGKILL) {
		t.Fatalf("exit code = %d, want SIGKILL's", c.code)
	}
	if r.index("A", "term-ignored", 1) < 0 {
		t.Fatal("stop did not send SIGTERM first")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("stop took %s", time.Since(start))
	}
}

func TestChildPortsReuseAndRetire(t *testing.T) {
	p := &portPool{held: map[string]*net.TCPListener{}}
	t.Cleanup(p.closeAll)
	keep := portWant{"http", "127.0.0.1:0"}
	first, err := p.prepare([]portWant{keep})
	if err != nil || len(first) != 1 {
		t.Fatalf("prepare = %v %v", first, err)
	}
	// A port bound to :0 gets a new port each bind, so reuse is keyed by
	// the address asked for, not the one bound.
	second, err := p.prepare([]portWant{keep, {"acme-extra", "127.0.0.1:0"}})
	if err != nil || len(second) != 2 {
		t.Fatalf("prepare = %v %v", second, err)
	}
	if second[0].l != first[0].l {
		t.Fatal("prepare bound a port it already held")
	}
	p.retain(first)
	if _, err := second[1].l.Accept(); err == nil {
		t.Fatal("retain left an unused listener open")
	}
	if len(p.held) != 1 {
		t.Fatalf("held %d listeners after retain, want 1", len(p.held))
	}
}
