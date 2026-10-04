//go:build linux

package supervise

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// processGone reports whether pid has exited. A process whose parent died
// is reparented, and its new parent may never reap it, so a zombie counts
// as gone.
func processGone(pid int) bool {
	if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
		return true
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return true
	}
	// The state is the field after the ")" that ends the command name.
	i := bytes.LastIndexByte(data, ')')
	return i >= 0 && i+2 < len(data) && data[i+2] == 'Z'
}

// onThreadThatExits runs fn on an OS thread that the runtime ends when fn
// returns, and returns that thread's id. The main thread is never ended, so
// a try that lands on it holds it until a try lands elsewhere.
func onThreadThatExits(fn func()) int {
	hold := make(chan struct{})
	defer close(hold)
	for {
		res := make(chan int)
		go func() {
			runtime.LockOSThread()
			if syscall.Gettid() == syscall.Getpid() {
				res <- 0
				<-hold
				runtime.UnlockOSThread()
				return
			}
			fn()
			res <- syscall.Gettid()
			// Returning still locked: the runtime ends this thread.
		}()
		if tid := <-res; tid != 0 {
			return tid
		}
	}
}

// Linux sends the parent-death signal when the thread that started the
// child exits, not when the process does. A child must keep running while
// the supervisor lives, whatever thread called startChild.
func TestChildOutlivesTheThreadThatStartedIt(t *testing.T) {
	r := newTestRoot(t)
	dir := r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_BODY": "A"})
	l := bindLoopback(t)
	t.Setenv("TINYCLD_STATE_DIR", r.dir)
	var c *child
	var err error
	tid := onThreadThatExits(func() {
		c, err = startChild(childSpec{dir: dir, args: []string{"serve"}, env: os.Environ(), ports: []heldPort{{name: ListenerHTTP, addr: l.Addr().String(), l: l}}})
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c.signalGroup(syscall.SIGKILL)
		c.waitExit(5 * time.Second)
	})
	waitFor(t, 20*time.Second, "the starting thread to exit", func() bool {
		_, err := os.Stat(filepath.Join("/proc/self/task", strconv.Itoa(tid)))
		return os.IsNotExist(err)
	})

	if m := nextMsg(t, c); m.Type != MsgReady {
		t.Fatalf("first message = %+v, want ready", m)
	}
	if err := c.send(Msg{Type: MsgDrain}); err != nil {
		t.Fatal(err)
	}
	if !c.waitExit(10 * time.Second) {
		t.Fatal("the child did not exit after drain")
	}
	if c.code != 0 {
		t.Fatalf("exit code = %d, want 0: the child was killed when the thread that started it exited", c.code)
	}
}

// A supervisor that dies without draining (SIGKILL, the OOM killer) must
// not leave a child holding the public ports: the supervisor started next
// could not bind them.
func TestRunSupervisorKilledStopsItsChild(t *testing.T) {
	r := newTestRoot(t)
	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_BODY": "A"})
	r.point("a")
	addr := freeAddr(t)
	p := r.startFakeSupervisor(addr)

	pid := r.waitEvent("A", "ready", 1).pid
	waitBody(t, addr, "A")
	if processGone(pid) {
		t.Fatal("the child is gone while the supervisor runs")
	}
	if err := p.cmd.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	p.wait(t, 10*time.Second)
	waitFor(t, 2*time.Second, "the child to exit with its supervisor", func() bool { return processGone(pid) })
}
