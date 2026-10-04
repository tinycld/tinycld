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
	p := newPortPool()
	t.Cleanup(p.closeAll)
	keep := portWant{"http", "127.0.0.1:0"}
	first, err := p.prepare([]portWant{keep}, notInUse)
	if err != nil || len(first) != 1 {
		t.Fatalf("prepare = %v %v", first, err)
	}
	// A port bound to :0 gets a new port each bind, so reuse is keyed by
	// the address asked for, not the one bound.
	second, err := p.prepare([]portWant{keep, {"acme-extra", "127.0.0.1:0"}}, allInUse)
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
	if n := p.count(); n != 1 {
		t.Fatalf("held %d listeners after retain, want 1", n)
	}
}

func notInUse(*net.TCPListener) bool { return false }
func allInUse(*net.TCPListener) bool { return true }

func freePort(t *testing.T) string {
	t.Helper()
	_, port, err := net.SplitHostPort(freeAddr(t))
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func assertClosed(t *testing.T, l *net.TCPListener) {
	t.Helper()
	if err := l.SetDeadline(time.Now()); err == nil {
		t.Fatalf("the listener on %s is still open", l.Addr())
	}
}

// A build that only respells a port's address (":P" for "0.0.0.0:P") gets
// the listener already held. Binding the port again would fail while the
// running child serves on it, or hold the port twice.
func TestChildPortsRespelledAddressKeepsOneListener(t *testing.T) {
	port := freePort(t)
	p := newPortPool()
	t.Cleanup(p.closeAll)
	first, err := p.prepare([]portWant{{"acme-extra", ":" + port}}, notInUse)
	if err != nil || len(first) != 1 {
		t.Fatalf("prepare = %v %v", first, err)
	}
	second, err := p.prepare([]portWant{{"acme-extra", "0.0.0.0:" + port}}, allInUse)
	if err != nil || len(second) != 1 {
		t.Fatalf("prepare after the respelling = %v %v", second, err)
	}
	if second[0].l != first[0].l {
		t.Fatal("a respelled address got a second listener")
	}
	if n := p.count(); n != 1 {
		t.Fatalf("held %d listeners, want 1", n)
	}
}

// A port that moves while the old child serves on it gets the new address
// at once; the old address stays open for the old child until retain.
func TestChildPortsMovedWhileServedRetiresTheOld(t *testing.T) {
	p := newPortPool()
	t.Cleanup(p.closeAll)
	first, err := p.prepare([]portWant{{"acme-extra", freeAddr(t)}}, notInUse)
	if err != nil || len(first) != 1 {
		t.Fatalf("prepare = %v %v", first, err)
	}
	moved := freeAddr(t)
	second, err := p.prepare([]portWant{{"acme-extra", moved}}, allInUse)
	if err != nil || len(second) != 1 {
		t.Fatalf("prepare after the move = %v %v", second, err)
	}
	if second[0].l == first[0].l || second[0].l.Addr().String() != moved {
		t.Fatalf("the moved port is on %s, want %s", second[0].l.Addr(), moved)
	}
	if err := first[0].l.SetDeadline(time.Time{}); err != nil {
		t.Fatal("the old address closed while a child still serves on it")
	}
	p.retain(second)
	assertClosed(t, first[0].l)
	if n := p.count(); n != 1 {
		t.Fatalf("held %d listeners after retain, want 1", n)
	}
}

// Once no child serves on a port's old address, the old listener closes
// before the new address is bound: the two can overlap (a wildcard and a
// specific address on one port), so binding first could fail.
func TestChildPortsMovedWhenUnusedClosesTheOldFirst(t *testing.T) {
	port := freePort(t)
	p := newPortPool()
	t.Cleanup(p.closeAll)
	first, err := p.prepare([]portWant{{"acme-extra", ":" + port}}, notInUse)
	if err != nil || len(first) != 1 {
		t.Fatalf("prepare = %v %v", first, err)
	}
	second, err := p.prepare([]portWant{{"acme-extra", "127.0.0.1:" + port}}, notInUse)
	if err != nil || len(second) != 1 {
		t.Fatalf("prepare after the move = %v %v", second, err)
	}
	assertClosed(t, first[0].l)
	if n := p.count(); n != 1 {
		t.Fatalf("held %d listeners, want 1", n)
	}
}

// When the new address cannot be bound while the old child serves on the
// old one, the new child keeps the old address rather than lose the port.
func TestChildPortsMoveThatCannotBindKeepsTheOld(t *testing.T) {
	p := newPortPool()
	t.Cleanup(p.closeAll)
	first, err := p.prepare([]portWant{{"acme-extra", freeAddr(t)}}, notInUse)
	if err != nil || len(first) != 1 {
		t.Fatalf("prepare = %v %v", first, err)
	}
	taken := bindLoopback(t)
	second, err := p.prepare([]portWant{{"acme-extra", taken.Addr().String()}}, allInUse)
	if err != nil || len(second) != 1 || second[0].l != first[0].l {
		t.Fatalf("prepare onto a taken address = %v %v, want the old listener", second, err)
	}
}

func TestSameAddr(t *testing.T) {
	for _, c := range []struct {
		a, b string
		same bool
	}{
		{":993", "0.0.0.0:993", true},
		{"[::]:993", ":993", true},
		{"127.0.0.1:993", "127.0.0.1:993", true},
		{"LOCALHOST:993", "localhost:993", true},
		{":993", "127.0.0.1:993", false},
		{":993", ":994", false},
		{"127.0.0.1:993", "[::1]:993", false},
	} {
		if got := sameAddr(c.a, c.b); got != c.same {
			t.Errorf("sameAddr(%q, %q) = %v, want %v", c.a, c.b, got, c.same)
		}
	}
}
