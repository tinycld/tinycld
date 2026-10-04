//go:build unix

package supervise

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"tinycld.org/core/listeners"
)

// controlSendTimeout bounds a write to a child's control socket, so a child
// that stopped reading cannot stall the supervisor.
const controlSendTimeout = 5 * time.Second

// heldPort is a listener the supervisor holds and passes to a child.
type heldPort struct {
	name, addr string
	l          *net.TCPListener
}

type childSpec struct {
	dir   string // the build's tinycld dir; the binary is dir/tinycld
	args  []string
	env   []string
	ports []heldPort
	cred  *syscall.Credential // nil: run as the supervisor's own user
}

// child is one running `tinycld serve` and the supervisor's end of its
// control socket.
type child struct {
	dir   string
	pid   int
	ports []heldPort
	ctl   net.Conn
	// msgs carries the ready and restart messages the child sends. Messages
	// of an unknown type never reach it.
	msgs chan Msg
	// pendingRestart is a restart the child asked for before it was ready,
	// kept until the supervisor can act on it. Only the supervisor's own
	// goroutine touches it.
	pendingRestart *Msg
	// done is closed once the child has exited and code is set.
	done  chan struct{}
	code  int
	ready atomic.Bool

	sendMu sync.Mutex
}

// startChild runs spec.dir/tinycld with every held port and a fresh control
// socket inherited, in its own process group so the whole group (and any
// build tools it runs) can be stopped together.
func startChild(spec childSpec) (*child, error) {
	ctl, childEnd, err := controlPair()
	if err != nil {
		return nil, err
	}

	set := &listeners.Set{}
	var passed []heldPort
	for _, p := range spec.ports {
		if err := set.AddListener(p.name, p.l); err != nil {
			log.Error("could not pass a port to the server; it will bind the port itself", "name", p.name, "addr", p.addr, "err", err)
			continue
		}
		passed = append(passed, p)
	}
	set.AddFile(ControlFD, childEnd)

	cmd := exec.Command(filepath.Join(spec.dir, "tinycld"), spec.args...)
	cmd.Dir = spec.dir
	cmd.Env = append([]string(nil), spec.env...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Credential: spec.cred}
	setDeathSignal(cmd.SysProcAttr)
	set.Apply(cmd)

	c := &child{
		dir:   spec.dir,
		ports: passed,
		ctl:   ctl,
		msgs:  make(chan Msg, 8),
		done:  make(chan struct{}),
	}
	started := make(chan error, 1)
	go c.run(cmd, started)
	err = <-started
	// The child has its own copies now, or never started. The supervisor
	// keeps only its listeners and its end of the control socket; a copy
	// left open here would keep the control socket from reporting EOF.
	for _, f := range cmd.ExtraFiles {
		f.Close()
	}
	if err != nil {
		ctl.Close()
		return nil, err
	}
	c.pid = cmd.Process.Pid
	go c.readControl()
	return c, nil
}

// run starts cmd, reports the start on started, and reaps the child. It
// holds one OS thread for the child's whole life: Linux sends the child's
// death signal when the thread that started it exits, not when the
// supervisor does, and the Go runtime ends a thread when a goroutine
// locked to it exits. A locked thread runs nothing else, so nothing can
// end it before the child is reaped.
func (c *child) run(cmd *exec.Cmd, started chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := cmd.Start(); err != nil {
		started <- err
		return
	}
	started <- nil
	err := cmd.Wait()
	c.code = exitCode(cmd.ProcessState, err)
	close(c.done)
	c.ctl.Close()
}

// controlPair returns the supervisor's end of a new control socket and the
// end to pass to the child. Both are close-on-exec, so neither leaks into
// another child; ExtraFiles clears the flag on the child's copy.
func controlPair() (net.Conn, *os.File, error) {
	syscall.ForkLock.RLock()
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err == nil {
		syscall.CloseOnExec(fds[0])
		syscall.CloseOnExec(fds[1])
	}
	syscall.ForkLock.RUnlock()
	if err != nil {
		return nil, nil, fmt.Errorf("control socket: %w", err)
	}
	childEnd := os.NewFile(uintptr(fds[0]), "control-child")
	own := os.NewFile(uintptr(fds[1]), "control")
	conn, err := net.FileConn(own)
	own.Close()
	if err != nil {
		childEnd.Close()
		return nil, nil, fmt.Errorf("control socket: %w", err)
	}
	return conn, childEnd, nil
}

// exitCode reports a signal death as 128+signal, the code a shell gives it,
// so the supervisor exits with the same code the entrypoint's loop did.
func exitCode(ps *os.ProcessState, err error) int {
	if ps == nil {
		log.Error("could not wait for a server", "err", err)
		return 1
	}
	if ws, ok := ps.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return ps.ExitCode()
}

func (c *child) readControl() {
	r := bufio.NewReader(c.ctl)
	for {
		m, err := Recv(r)
		if errors.Is(err, ErrBadMessage) {
			log.Warn("ignoring a control message that does not parse", "pid", c.pid, "err", err)
			continue
		}
		if err != nil {
			return // the child exited or closed its end
		}
		if !knownMessage(c.pid, m) {
			continue
		}
		if m.Type == MsgReady {
			c.ready.Store(true)
		}
		// The ack goes out on receipt, not when the supervisor gets to the
		// restart: a swap can keep the supervisor busy for longer than the
		// child waits for an ack.
		if m.Type == MsgRestart {
			if err := c.send(Msg{Type: MsgRestartAck}); err != nil {
				log.Warn("could not ack a server's restart; it will exit for a cold restart", "pid", c.pid, "err", err)
			}
		}
		select {
		case c.msgs <- m:
		case <-c.done:
			return
		}
	}
}

// knownMessage reports whether the supervisor acts on m. The supervisor is
// the image's baked binary, so its children can be newer. ready and restart
// are permanent messages and are acted on whatever version the child
// states; a type the supervisor does not know is ignored, because acting on
// a guess would be worse.
func knownMessage(pid int, m Msg) bool {
	switch m.Type {
	case MsgReady, MsgRestart:
		return true
	}
	log.Warn("ignoring an unknown control message", "pid", pid, "type", m.Type)
	return false
}

func (c *child) isReady() bool { return c.ready.Load() }

func (c *child) takePendingRestart() (Msg, bool) {
	if c.pendingRestart == nil {
		return Msg{}, false
	}
	m := *c.pendingRestart
	c.pendingRestart = nil
	return m, true
}

func (c *child) exited() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

func (c *child) send(m Msg) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if err := c.ctl.SetWriteDeadline(time.Now().Add(controlSendTimeout)); err != nil {
		return err
	}
	return Send(c.ctl, m)
}

// signalGroup signals the child's whole process group. It does nothing once
// the child has been reaped, so a reused process group id is never hit.
func (c *child) signalGroup(sig syscall.Signal) {
	if c.exited() {
		return
	}
	if err := syscall.Kill(-c.pid, sig); err != nil && !errors.Is(err, syscall.ESRCH) {
		log.Warn("could not signal a server", "pid", c.pid, "signal", sig, "err", err)
	}
}

// waitExit reports whether the child exited within d.
func (c *child) waitExit(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-c.done:
		return true
	case <-t.C:
		return false
	}
}

// stopChild ends a child that is not serving: SIGTERM, then SIGKILL after
// bound.
func stopChild(c *child, bound time.Duration) {
	if c.exited() {
		return
	}
	c.signalGroup(syscall.SIGTERM)
	if c.waitExit(bound) {
		return
	}
	log.Error("a server did not stop after SIGTERM; killing it", "pid", c.pid)
	c.signalGroup(syscall.SIGKILL)
	if !c.waitExit(bound) {
		log.Error("a server is still running after SIGKILL", "pid", c.pid)
	}
}

// drainChild asks a serving child to finish its requests and exit, and
// kills it if it has not exited after bound. A child that never said ready
// may not read its control socket (or has not started to), so it gets
// SIGTERM instead.
func drainChild(c *child, bound time.Duration) {
	if c.exited() {
		return
	}
	if !c.isReady() {
		c.signalGroup(syscall.SIGTERM)
	} else if err := c.send(Msg{Type: MsgDrain}); err != nil {
		log.Warn("could not ask a server to drain; stopping it", "pid", c.pid, "err", err)
		c.signalGroup(syscall.SIGTERM)
	}
	if c.waitExit(bound) {
		return
	}
	log.Error("a draining server did not exit in time; killing it", "pid", c.pid, "bound", bound)
	c.signalGroup(syscall.SIGKILL)
	if !c.waitExit(bound) {
		log.Error("a server is still running after SIGKILL", "pid", c.pid)
	}
}

// portPool is every listener the supervisor holds, keyed by name. Two
// children run side by side during a swap, so a listener stays held until
// no running child needs it.
type portPool struct {
	held map[string]heldPort
	// retiring are listeners whose name a newer build moved to another
	// address. A running child still serves on them, so they close only
	// when retain finds no child that uses them.
	retiring []heldPort
}

func newPortPool() portPool { return portPool{held: map[string]heldPort{}} }

// count is how many listeners the pool holds.
func (p *portPool) count() int { return len(p.held) + len(p.retiring) }

// prepare returns the listeners for want, reusing the ones already held and
// binding the rest. inUse reports whether a running child serves on a
// listener. A port that fails to bind is left out and reported; the rest
// are still returned.
func (p *portPool) prepare(want []portWant, inUse func(*net.TCPListener) bool) ([]heldPort, error) {
	var out []heldPort
	var errs []error
	for _, w := range want {
		hp, err := p.take(w, inUse)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		out = append(out, hp)
	}
	return out, errors.Join(errs...)
}

// take returns the listener for w. A name whose address changed gets a
// listener on the new address, and the old one closes as soon as no
// running child serves on it.
func (p *portPool) take(w portWant, inUse func(*net.TCPListener) bool) (heldPort, error) {
	cur, held := p.held[w.name]
	if held && sameAddr(cur.addr, w.addr) {
		return cur, nil
	}
	next, found := p.unretire(w)
	if !found {
		if held && !inUse(cur.l) {
			// Close first: nothing serves on the old address now, and the
			// two addresses can overlap (a wildcard and a specific address
			// on one port), so binding first could fail.
			log.Info("moving a port to a new address", "name", w.name, "from", cur.addr, "to", w.addr)
			cur.l.Close()
			delete(p.held, w.name)
			held = false
		}
		l, err := bindTCP(w.addr)
		if err != nil {
			if held {
				log.Warn("could not bind a port's new address while a server serves on the old one; keeping the old address until a restart",
					"name", w.name, "from", cur.addr, "to", w.addr, "err", err)
				return cur, nil
			}
			return heldPort{}, fmt.Errorf("bind %s on %s: %w", w.name, w.addr, err)
		}
		next = heldPort{name: w.name, addr: w.addr, l: l}
	}
	if held {
		p.retire(cur, inUse)
	}
	p.held[w.name] = next
	return next, nil
}

// unretire takes back a retiring listener on w's address: a rollback asks
// again for the address a failed build moved its port off.
func (p *portPool) unretire(w portWant) (heldPort, bool) {
	for i, r := range p.retiring {
		if r.name == w.name && sameAddr(r.addr, w.addr) {
			p.retiring = append(p.retiring[:i], p.retiring[i+1:]...)
			return r, true
		}
	}
	return heldPort{}, false
}

func (p *portPool) retire(h heldPort, inUse func(*net.TCPListener) bool) {
	if inUse(h.l) {
		p.retiring = append(p.retiring, h)
		return
	}
	h.l.Close()
}

// retain closes every held listener that keep does not use: the ports a
// build no longer declares, and the old addresses of ports it moved.
func (p *portPool) retain(keep []heldPort) {
	used := map[*net.TCPListener]bool{}
	for _, k := range keep {
		used[k.l] = true
	}
	for name, h := range p.held {
		if used[h.l] {
			continue
		}
		log.Info("closing a port the running build does not declare", "name", name, "addr", h.addr)
		h.l.Close()
		delete(p.held, name)
	}
	var still []heldPort
	for _, h := range p.retiring {
		if used[h.l] {
			still = append(still, h)
			continue
		}
		log.Info("closing a port's old address", "name", h.name, "addr", h.addr)
		h.l.Close()
	}
	p.retiring = still
}

func (p *portPool) closeAll() {
	for name, h := range p.held {
		h.l.Close()
		delete(p.held, name)
	}
	for _, h := range p.retiring {
		h.l.Close()
	}
	p.retiring = nil
}

// sameAddr reports whether two spellings of a bind address name the same
// socket, so a build that only respells one (":993" for "0.0.0.0:993")
// keeps the listener it has. The wildcard spellings differ only in whether
// IPv6 is served too, which is not worth a rebind.
func sameAddr(a, b string) bool {
	if a == b {
		return true
	}
	hostA, portA, errA := net.SplitHostPort(a)
	hostB, portB, errB := net.SplitHostPort(b)
	if errA != nil || errB != nil || portA != portB {
		return false
	}
	if anyHost(hostA) && anyHost(hostB) {
		return true
	}
	ipA, ipB := net.ParseIP(hostA), net.ParseIP(hostB)
	if ipA != nil && ipB != nil {
		return ipA.Equal(ipB)
	}
	return strings.EqualFold(hostA, hostB)
}

// anyHost reports whether host binds every interface.
func anyHost(host string) bool {
	if host == "" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsUnspecified()
}

func bindTCP(addr string) (*net.TCPListener, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	return l.(*net.TCPListener), nil
}

// childCredential returns the user children run as. As root, that is
// childUser with its groups and home, as gosu set them; otherwise children
// run as the supervisor's own user.
func childCredential(childUser string) (cred *syscall.Credential, home string, err error) {
	if os.Geteuid() != 0 {
		return nil, "", nil
	}
	u, err := user.Lookup(childUser)
	if err != nil {
		return nil, "", fmt.Errorf("look up user %s to run servers as: %w", childUser, err)
	}
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return nil, "", fmt.Errorf("user %s: uid %q: %w", childUser, u.Uid, err)
	}
	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return nil, "", fmt.Errorf("user %s: gid %q: %w", childUser, u.Gid, err)
	}
	cred = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}
	if ids, err := u.GroupIds(); err == nil {
		for _, id := range ids {
			if g, err := strconv.ParseUint(id, 10, 32); err == nil {
				cred.Groups = append(cred.Groups, uint32(g))
			}
		}
	}
	return cred, u.HomeDir, nil
}

// childEnv is the supervisor's environment, with HOME pointing at the
// child user's home when the supervisor drops to that user.
func childEnv(env []string, home string) []string {
	if home == "" {
		return env
	}
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if !strings.HasPrefix(kv, "HOME=") {
			out = append(out, kv)
		}
	}
	return append(out, "HOME="+home)
}
