//go:build unix

package supervise

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"tinycld.org/core/listeners"
)

// The test binary plays two more roles, chosen by env: a fake server child
// (what a build's tinycld binary would be) and, for the signal test, the
// supervisor itself in its own process.
func TestMain(m *testing.M) {
	switch os.Getenv("SUPERVISE_TEST_ROLE") {
	case "child":
		os.Exit(fakeChild())
	case "supervisor":
		os.Exit(supervisorRole())
	}
	code := m.Run()
	removeServerBuild()
	os.Exit(code)
}

// fakeChild stands in for `tinycld serve`: it serves on every listener it
// inherits, speaks the control protocol, drains as the real server does, and
// records what happens to it in
// <state>/events.log, one "<name> <pid> <what>" line per event. Its build's
// script sets the knobs:
//
//	FAKE_NAME          name used in the events log
//	FAKE_BUILD         this build's id
//	FAKE_SERVE_BODY    what its HTTP handler answers
//	FAKE_READY_DELAY   ms to wait before ready (a slow boot)
//	FAKE_NEVER_READY   never send ready and never serve (a hung boot)
//	FAKE_IGNORE_TERM   ignore SIGTERM (only SIGKILL stops it)
//	FAKE_BOOT_EXIT     exit with this code before ready (a failed boot)
//	FAKE_SEND_JUNK     send messages the supervisor must ignore before ready
//	FAKE_READY_VERSION send ready with this protocol version
//	FAKE_RESTART_BEFORE_READY  on this build's first start only (a boot-time rebuild runs once), activate FAKE_ACTIVATE and ask for a restart before ready, then wait for the ack
//	FAKE_ACTIVATE      on SIGUSR1, arm the backup and point current at this build, as a rebuild does
//	FAKE_EXIT_CODE     on SIGUSR1, exit with this code instead of asking for a restart
//	FAKE_COLD          on SIGUSR1, ask for a cold restart
func fakeChild() int {
	root := os.Getenv("TINYCLD_STATE_DIR")
	ev := func(what string) {
		appendEvent(root, fmt.Sprintf("%s %d %s", os.Getenv("FAKE_NAME"), os.Getpid(), what))
	}
	ev("start")

	trigger := make(chan os.Signal, 1)
	signal.Notify(trigger, syscall.SIGUSR1)
	term := make(chan os.Signal, 1)
	signal.Notify(term, syscall.SIGTERM)

	var names []string
	var ls []net.Listener
	for _, n := range strings.Split(os.Getenv(listeners.EnvFDNames), ":") {
		if l, ok := listeners.Inherited(n); ok {
			names = append(names, n)
			ls = append(ls, l)
		}
	}
	ev("listeners " + strings.Join(names, ","))

	if code := os.Getenv("FAKE_BOOT_EXIT"); code != "" {
		ev("exit")
		return atoi(code)
	}

	f, ok := listeners.ExtraFD(ControlFD)
	if !ok {
		ev("no-control")
		return 2
	}
	ctl, err := net.FileConn(f)
	if err != nil {
		ev("bad-control")
		return 2
	}

	if os.Getenv("FAKE_NEVER_READY") == "1" {
		for range term {
			if os.Getenv("FAKE_IGNORE_TERM") != "1" {
				ev("term")
				return 0
			}
			ev("term-ignored")
		}
	}

	drain := make(chan struct{})
	acked := make(chan struct{}, 1)
	go func() {
		r := bufio.NewReader(ctl)
		for {
			m, err := Recv(r)
			if errors.Is(err, ErrBadMessage) {
				continue
			}
			if err != nil {
				return
			}
			switch m.Type {
			case MsgRestartAck:
				ev("restart-ack")
				select {
				case acked <- struct{}{}:
				default:
				}
			case MsgDrain:
				close(drain)
				return
			}
		}
	}()

	if os.Getenv("FAKE_SEND_JUNK") == "1" {
		io.WriteString(ctl, "not json\n")
		Send(ctl, Msg{Type: "bogus"})
		Send(ctl, Msg{Type: "bogus", Version: ProtocolVersion + 1})
	}
	if os.Getenv("FAKE_RESTART_BEFORE_READY") == "1" && startCount(root, os.Getenv("FAKE_NAME")) == 1 {
		if to := os.Getenv("FAKE_ACTIVATE"); to != "" {
			fakeActivate(root, os.Getenv("FAKE_BUILD"), to)
		}
		Send(ctl, Msg{Type: MsgRestart})
		ev("restart")
		select {
		case <-acked:
		case <-time.After(5 * time.Second):
			ev("no-ack")
		}
	}
	if d := os.Getenv("FAKE_READY_DELAY"); d != "" {
		time.Sleep(time.Duration(atoi(d)) * time.Millisecond)
	}
	if err := Send(ctl, Msg{Type: MsgReady, Version: atoi(os.Getenv("FAKE_READY_VERSION"))}); err != nil {
		ev("ready-failed")
		return 2
	}
	ev("ready")

	body := os.Getenv("FAKE_SERVE_BODY")
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	})}
	drainer := NewDrainer(srv)
	for _, l := range ls {
		go srv.Serve(drainer.Listener(l))
	}

	for {
		select {
		case <-trigger:
			if to := os.Getenv("FAKE_ACTIVATE"); to != "" {
				fakeActivate(root, os.Getenv("FAKE_BUILD"), to)
			}
			if code := os.Getenv("FAKE_EXIT_CODE"); code != "" {
				ev("exit")
				return atoi(code)
			}
			Send(ctl, Msg{Type: MsgRestart, Cold: os.Getenv("FAKE_COLD") == "1"})
			ev("restart")
		case <-drain:
			ev("drain")
			ctx, cancel := context.WithTimeout(context.Background(), ChildDrainTimeout)
			drainer.Drain(ctx)
			cancel()
			ev("exit")
			return 0
		case <-term:
			if os.Getenv("FAKE_IGNORE_TERM") == "1" {
				ev("term-ignored")
				continue
			}
			ev("term")
			srv.Close()
			return 0
		}
	}
}

// fakeActivate does what a rebuild leaves behind before it asks for a
// restart: a backup of the database, the armed marker, the previous build,
// a migrated database and current pointing at the new build.
func fakeActivate(root, from, to string) {
	s := State{Root: root}
	data, _ := os.ReadFile(s.dbPath())
	os.WriteFile(s.dbBackupPath(), data, 0o644)
	os.WriteFile(s.dbArmedMarkerPath(), []byte(to), 0o644)
	os.WriteFile(s.previousBuildPath(), []byte(from), 0o644)
	os.WriteFile(s.dbPath(), []byte("migrated-by-"+to), 0o644)
	tmp := s.currentLinkPath() + ".tmp"
	os.Remove(tmp)
	os.Symlink(filepath.Join(s.buildsDir(), to, "tinycld"), tmp)
	os.Rename(tmp, s.currentLinkPath())
}

// startCount is how many times the fake child named name has started,
// counting this start.
func startCount(root, name string) int {
	eventsMu.Lock()
	defer eventsMu.Unlock()
	data, _ := os.ReadFile(filepath.Join(root, "events.log"))
	n := 0
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) == 3 && f[0] == name && f[2] == "start" {
			n++
		}
	}
	return n
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

var eventsMu sync.Mutex

func appendEvent(root, line string) {
	eventsMu.Lock()
	defer eventsMu.Unlock()
	f, err := os.OpenFile(filepath.Join(root, "events.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	f.WriteString(line + "\n")
	f.Close()
}

// --- harness ---

type knobs map[string]string

type testRoot struct {
	t   *testing.T
	dir string
}

type event struct {
	name, what string
	pid        int
}

func newTestRoot(t *testing.T) *testRoot {
	t.Helper()
	r := &testRoot{t: t, dir: t.TempDir()}
	if err := os.MkdirAll(filepath.Join(r.dir, "pb_data"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(r.dir, "pb_data", "data.db"), "live")
	return r
}

func (r *testRoot) state() State { return State{Root: r.dir} }

// build writes builds/<id>/tinycld/tinycld: a script that runs this test
// binary as a fake child with the given knobs.
func (r *testRoot) build(id string, k knobs) string {
	r.t.Helper()
	bin, err := os.Executable()
	if err != nil {
		r.t.Fatal(err)
	}
	dir := filepath.Join(r.dir, "builds", id, "tinycld")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		r.t.Fatal(err)
	}
	k["SUPERVISE_TEST_ROLE"] = "child"
	k["FAKE_BUILD"] = id
	keys := make([]string, 0, len(k))
	for key := range k {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	for _, key := range keys {
		fmt.Fprintf(&b, "export %s='%s'\n", key, k[key])
	}
	fmt.Fprintf(&b, "exec '%s' \"$@\"\n", bin)
	if err := os.WriteFile(filepath.Join(dir, "tinycld"), []byte(b.String()), 0o755); err != nil {
		r.t.Fatal(err)
	}
	return dir
}

func (r *testRoot) point(id string) {
	r.t.Helper()
	s := r.state()
	if err := os.Symlink(filepath.Join(s.buildsDir(), id, "tinycld"), s.currentLinkPath()); err != nil {
		r.t.Fatal(err)
	}
}

func (r *testRoot) stageRelease(id, releaseID string) {
	r.t.Helper()
	writeStagingRelease(r.t, filepath.Join(r.dir, "builds", id, "tinycld", "release-staging"), releaseID, time.Now(), false)
}

func (r *testRoot) stageReleaseAt(id, releaseID string, at time.Time) {
	r.t.Helper()
	writeStagingRelease(r.t, filepath.Join(r.dir, "builds", id, "tinycld", "release-staging"), releaseID, at, false)
}

func (r *testRoot) writePorts(id string, ports []listeners.Port) {
	r.t.Helper()
	dir := filepath.Join(r.dir, "builds", id, "tinycld", "server")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		r.t.Fatal(err)
	}
	var parts []string
	for _, p := range ports {
		s := fmt.Sprintf(`{"slug":%q,"name":%q,"port":%d`, p.Slug, p.Name, p.Port)
		if p.AddrEnv != "" {
			s += fmt.Sprintf(`,"addrEnv":%q`, p.AddrEnv)
		}
		if p.Enabled != nil {
			s += fmt.Sprintf(`,"enabled":{"env":%q,"default":%t}`, p.Enabled.Env, p.Enabled.Default)
		}
		parts = append(parts, s+"}")
	}
	mustWrite(r.t, filepath.Join(dir, "ports.json"), "["+strings.Join(parts, ",")+"]")
}

func (r *testRoot) events() []event {
	data, _ := os.ReadFile(filepath.Join(r.dir, "events.log"))
	var out []event
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		f := strings.SplitN(line, " ", 3)
		if len(f) < 3 {
			continue
		}
		out = append(out, event{name: f[0], pid: atoi(f[1]), what: f[2]})
	}
	return out
}

// index returns the position of the nth (1-based) event matching name and
// what, or -1.
func (r *testRoot) index(name, what string, nth int) int {
	seen := 0
	for i, e := range r.events() {
		if e.name == name && e.what == what {
			seen++
			if seen == nth {
				return i
			}
		}
	}
	return -1
}

func (r *testRoot) waitEvent(name, what string, nth int) event {
	r.t.Helper()
	waitFor(r.t, 20*time.Second, fmt.Sprintf("event %d of %q %q", nth, name, what), func() bool {
		return r.index(name, what, nth) >= 0
	})
	return r.events()[r.index(name, what, nth)]
}

func (r *testRoot) eventWith(name, prefix string, nth int) string {
	seen := 0
	for _, e := range r.events() {
		if e.name == name && strings.HasPrefix(e.what, prefix) {
			seen++
			if seen == nth {
				return e.what
			}
		}
	}
	return ""
}

func (r *testRoot) trigger(e event) {
	r.t.Helper()
	if err := syscall.Kill(e.pid, syscall.SIGUSR1); err != nil {
		r.t.Fatalf("trigger %s: %v", e.name, err)
	}
}

func waitFor(t *testing.T, limit time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for %s", limit, what)
		}
		<-tick.C
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

// A fresh connection per request: a refused connect is what the swap must
// never cause, and a reused connection would hide it.
var client = &http.Client{
	Timeout:   5 * time.Second,
	Transport: &http.Transport{DisableKeepAlives: true},
}

func get(addr string) (string, error) {
	return fetch("http://" + addr + "/")
}

func fetch(url string) (string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	return string(b), nil
}

func waitBody(t *testing.T, addr, want string) {
	t.Helper()
	waitFor(t, 20*time.Second, fmt.Sprintf("%s to answer %q", addr, want), func() bool {
		got, err := get(addr)
		return err == nil && got == want
	})
}

func testOptions() options {
	return options{readyTimeout: 20 * time.Second, drainBound: 15 * time.Second, stopBound: 3 * time.Second}
}

type harness struct {
	t    *testing.T
	s    *supervisor
	addr string
	sigs chan os.Signal
	done chan struct{}
	code int
}

// supervise runs a supervisor in this process on the root, in plain mode on
// a free loopback port. Cleanup stops it and fails the test if it left a
// child running.
func (r *testRoot) supervise(opts options) *harness {
	r.t.Helper()
	h := &harness{t: r.t, addr: freeAddr(r.t), sigs: make(chan os.Signal, 2), done: make(chan struct{})}
	r.t.Setenv("TINYCLD_STATE_DIR", r.dir)
	r.t.Setenv("HTTP_ADDR", h.addr)
	r.t.Setenv("AUTOCERT_ENABLED", "")
	s, err := newSupervisor(nil, os.Getenv, opts, h.sigs)
	if err != nil {
		r.t.Fatal(err)
	}
	h.s = s
	go func() {
		h.code = s.run()
		close(h.done)
	}()
	r.t.Cleanup(func() {
		select {
		case h.sigs <- syscall.SIGTERM:
		default:
		}
		select {
		case <-h.done:
		case <-time.After(opts.drainBound + 2*opts.stopBound + 10*time.Second):
			r.t.Error("the supervisor did not stop")
			return
		}
		for _, c := range s.live {
			if !c.exited() {
				r.t.Errorf("the supervisor left child %d running", c.pid)
				c.signalGroup(syscall.SIGKILL)
				c.waitExit(5 * time.Second)
			}
		}
	})
	return h
}

func (h *harness) wait(limit time.Duration) int {
	h.t.Helper()
	select {
	case <-h.done:
		return h.code
	case <-time.After(limit):
		h.t.Fatalf("the supervisor did not exit within %s", limit)
		return -1
	}
}

// load sends a request every 5 ms until stopped.
type load struct {
	mu      sync.Mutex
	bodies  []string
	refused int
	failed  []error
	// slowest is the longest any one request took: a request that waits in
	// the listen backlog while nothing accepts is slow, not refused.
	slowest time.Duration
	// lastFailedAt is when the last refused or failed request ended.
	lastFailedAt time.Time
	stop         chan struct{}
	stopped      chan struct{}
}

func startLoad(addr string) *load {
	return startLoadAt("http://" + addr + "/")
}

func startLoadAt(url string) *load {
	l := &load{stop: make(chan struct{}), stopped: make(chan struct{})}
	go func() {
		defer close(l.stopped)
		tick := time.NewTicker(5 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-l.stop:
				return
			case <-tick.C:
			}
			start := time.Now()
			body, err := fetch(url)
			took := time.Since(start)
			l.mu.Lock()
			l.slowest = max(l.slowest, took)
			if err != nil {
				l.lastFailedAt = time.Now()
			}
			switch {
			case errors.Is(err, syscall.ECONNREFUSED):
				l.refused++
			case err != nil:
				l.failed = append(l.failed, err)
			default:
				l.bodies = append(l.bodies, body)
			}
			l.mu.Unlock()
		}
	}()
	return l
}

func (l *load) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.bodies)
}

func (l *load) end() {
	close(l.stop)
	<-l.stopped
}

// --- tests ---

func TestRunSwapUnderLoad(t *testing.T) {
	r := newTestRoot(t)
	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_BODY": "A", "FAKE_ACTIVATE": "b"})
	r.build("b", knobs{"FAKE_NAME": "B", "FAKE_SERVE_BODY": "B", "FAKE_READY_DELAY": "300"})
	r.stageRelease("b", "rel-b")
	r.point("a")
	h := r.supervise(testOptions())

	a := r.waitEvent("A", "ready", 1)
	waitBody(t, h.addr, "A")
	ld := startLoad(h.addr)
	waitFor(t, 10*time.Second, "load on A", func() bool { return ld.count() >= 10 })

	r.trigger(a)
	r.waitEvent("A", "exit", 1)
	atExit := ld.count()
	waitFor(t, 10*time.Second, "load on B", func() bool { return ld.count() >= atExit+20 })
	ld.end()

	if ld.refused != 0 || len(ld.failed) != 0 {
		t.Fatalf("refused connections = %d, failed requests = %d (%v)", ld.refused, len(ld.failed), ld.failed)
	}
	if first, last := ld.bodies[0], ld.bodies[len(ld.bodies)-1]; first != "A" || last != "B" {
		t.Fatalf("answers went %q ... %q, want A ... B", first, last)
	}
	bReady, aDrain, aExit := r.index("B", "ready", 1), r.index("A", "drain", 1), r.index("A", "exit", 1)
	if bReady >= aDrain || aDrain >= aExit {
		t.Fatalf("event order: B ready %d, A drain %d, A exit %d", bReady, aDrain, aExit)
	}
	s := r.state()
	waitFor(t, 5*time.Second, "the backup to be committed", func() bool {
		_, armed := s.BackupArmed()
		return !armed
	})
	assertExists(t, s.dbBackupPath(), false)
	if got := mustRead(t, s.dbPath()); got != "migrated-by-b" {
		t.Fatalf("data.db = %q", got)
	}
	if got := readLink(t, s.currentReleaseLinkPath()); got != "rel-b" {
		t.Fatalf("releases/current -> %q, want rel-b (PromoteRelease did not run)", got)
	}
}

// assertColdRollback checks the state a failed new build leaves: the old
// child gone, the database restored, the rollback recorded, current back on
// the previous build and a fresh child of it serving.
func assertColdRollback(t *testing.T, r *testRoot, h *harness) {
	t.Helper()
	r.waitEvent("A", "ready", 2)
	waitBody(t, h.addr, "A")
	if r.index("A", "exit", 1) < 0 || r.index("A", "exit", 1) > r.index("A", "start", 2) {
		t.Fatal("the old child was not stopped before the previous build started again")
	}
	s := r.state()
	if got := mustRead(t, s.dbPath()); got != "live" {
		t.Fatalf("data.db = %q, want the backup's bytes", got)
	}
	if got := mustRead(t, s.rollbackPendingMarkerPath()); got != "b" {
		t.Fatalf(".rollback-pending = %q, want b", got)
	}
	if got := readLink(t, s.currentLinkPath()); got != filepath.Join(s.buildsDir(), "a", "tinycld") {
		t.Fatalf("current -> %q, want the previous build", got)
	}
}

func TestRunNewChildNeverReady(t *testing.T) {
	r := newTestRoot(t)
	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_BODY": "A", "FAKE_ACTIVATE": "b"})
	r.build("b", knobs{"FAKE_NAME": "B", "FAKE_NEVER_READY": "1"})
	r.point("a")
	opts := testOptions()
	opts.readyTimeout = time.Second
	h := r.supervise(opts)

	r.trigger(r.waitEvent("A", "ready", 1))
	assertColdRollback(t, r, h)
	if term := r.index("B", "term", 1); term < 0 || term > r.index("A", "start", 2) {
		t.Fatal("the new child was not stopped before the previous build started again")
	}
}

func TestRunNewChildExitsBeforeReady(t *testing.T) {
	r := newTestRoot(t)
	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_BODY": "A", "FAKE_ACTIVATE": "b"})
	r.build("b", knobs{"FAKE_NAME": "B", "FAKE_BOOT_EXIT": "1"})
	r.point("a")
	opts := testOptions()
	opts.readyTimeout = time.Minute // the rollback must not wait for it
	h := r.supervise(opts)

	r.trigger(r.waitEvent("A", "ready", 1))
	assertColdRollback(t, r, h)
}

func TestRunColdRestart(t *testing.T) {
	r := newTestRoot(t)
	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_BODY": "A", "FAKE_ACTIVATE": "b", "FAKE_COLD": "1"})
	r.build("b", knobs{"FAKE_NAME": "B", "FAKE_SERVE_BODY": "B"})
	r.point("a")
	h := r.supervise(testOptions())

	r.trigger(r.waitEvent("A", "ready", 1))
	r.waitEvent("B", "ready", 1)
	waitBody(t, h.addr, "B")
	if aExit, bStart := r.index("A", "exit", 1), r.index("B", "start", 1); aExit < 0 || aExit > bStart {
		t.Fatalf("A exit %d, B start %d: the old child must exit before the new one starts", aExit, bStart)
	}
	if r.index("A", "drain", 1) < 0 {
		t.Fatal("the old child was not drained")
	}
}

// A new child can ask for its own replacement before it is ready (its boot
// ran a rebuild). The supervisor must ack the restart and act on it once the
// child is ready; promoting and committing the backup at that ready would
// drop the backup the next build's rollback needs.
func TestRunRestartBeforeReadyIsKept(t *testing.T) {
	r := newTestRoot(t)
	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_BODY": "A", "FAKE_ACTIVATE": "b"})
	r.build("b", knobs{"FAKE_NAME": "B", "FAKE_SERVE_BODY": "B", "FAKE_ACTIVATE": "c", "FAKE_RESTART_BEFORE_READY": "1"})
	r.build("c", knobs{"FAKE_NAME": "C", "FAKE_SERVE_BODY": "C"})
	r.stageRelease("b", "rel-b")
	r.stageRelease("c", "rel-c")
	r.point("a")
	h := r.supervise(testOptions())

	r.trigger(r.waitEvent("A", "ready", 1))
	r.waitEvent("C", "ready", 1)
	waitBody(t, h.addr, "C")
	r.waitEvent("B", "exit", 1)

	if r.index("B", "restart-ack", 1) < 0 || r.index("B", "no-ack", 1) >= 0 {
		t.Fatal("the restart sent before ready was not acked")
	}
	if r.index("A", "drain", 1) < 0 || r.index("B", "drain", 1) < 0 {
		t.Fatal("A and B were not both drained")
	}
	s := r.state()
	waitFor(t, 5*time.Second, "the backup to be committed", func() bool {
		_, armed := s.BackupArmed()
		return !armed
	})
	if got := mustRead(t, s.dbPath()); got != "migrated-by-c" {
		t.Fatalf("data.db = %q", got)
	}
	if got := readLink(t, s.currentReleaseLinkPath()); got != "rel-c" {
		t.Fatalf("releases/current -> %q, want rel-c", got)
	}
	assertExists(t, filepath.Join(s.releasesDir(), "rel-b"), false)
}

// A→B→C where B asks for its restart before it is ready: B's release is
// never promoted at its ready, because C is the build that ready belongs to.
// When C then fails, the rollback brings B back, and B must serve its own
// release rather than A's.
func TestRunRollbackPromotesTheRolledBackRelease(t *testing.T) {
	r := newTestRoot(t)
	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_BODY": "A", "FAKE_ACTIVATE": "b"})
	r.build("b", knobs{"FAKE_NAME": "B", "FAKE_SERVE_BODY": "B", "FAKE_ACTIVATE": "c", "FAKE_RESTART_BEFORE_READY": "1"})
	r.build("c", knobs{"FAKE_NAME": "C", "FAKE_BOOT_EXIT": "1"})
	r.stageReleaseAt("a", "rel-a", time.Now().Add(-time.Hour))
	r.point("a")
	s := r.state()
	if err := s.PromoteRelease(); err != nil {
		t.Fatal(err)
	}
	r.stageRelease("b", "rel-b")
	r.stageRelease("c", "rel-c")
	h := r.supervise(testOptions())

	r.trigger(r.waitEvent("A", "ready", 1))
	r.waitEvent("C", "exit", 1)
	r.waitEvent("B", "ready", 2)
	waitBody(t, h.addr, "B")

	if got := readLink(t, s.currentLinkPath()); got != filepath.Join(s.buildsDir(), "b", "tinycld") {
		t.Fatalf("current -> %q, want build b", got)
	}
	waitFor(t, 5*time.Second, "releases/current to be B's release", func() bool {
		dest, _ := os.Readlink(s.currentReleaseLinkPath())
		return dest == "rel-b"
	})
	assertExists(t, filepath.Join(s.releasesDir(), "rel-c"), false)
}

func TestRunOldProtocolExit75(t *testing.T) {
	r := newTestRoot(t)
	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_BODY": "A", "FAKE_ACTIVATE": "b", "FAKE_EXIT_CODE": "75"})
	r.build("b", knobs{"FAKE_NAME": "B", "FAKE_SERVE_BODY": "B"})
	r.point("a")
	h := r.supervise(testOptions())

	r.trigger(r.waitEvent("A", "ready", 1))
	r.waitEvent("B", "ready", 1)
	waitBody(t, h.addr, "B")
	s := r.state()
	waitFor(t, 5*time.Second, "the backup to be committed", func() bool {
		_, armed := s.BackupArmed()
		return !armed
	})
	assertExists(t, s.dbBackupPath(), false)
}

func acmePort(addrEnv string) listeners.Port {
	return listeners.Port{
		Slug: "acme", Name: "acme-extra", Port: 1, AddrEnv: addrEnv,
		Enabled: &listeners.EnableRule{Env: "ACME_ENABLED", Default: false},
	}
}

func TestRunPackagePortAdded(t *testing.T) {
	r := newTestRoot(t)
	extra := freeAddr(t)
	t.Setenv("ACME_ENABLED", "true")
	t.Setenv("ACME_EXTRA_ADDR", extra)
	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_BODY": "A", "FAKE_ACTIVATE": "b"})
	r.build("b", knobs{"FAKE_NAME": "B", "FAKE_SERVE_BODY": "B"})
	r.writePorts("b", []listeners.Port{acmePort("ACME_EXTRA_ADDR")})
	r.point("a")
	r.supervise(testOptions())

	r.trigger(r.waitEvent("A", "ready", 1))
	r.waitEvent("A", "exit", 1)
	if got := r.eventWith("A", "listeners", 1); got != "listeners http" {
		t.Fatalf("A got %q", got)
	}
	if got := r.eventWith("B", "listeners", 1); got != "listeners http,acme-extra" {
		t.Fatalf("B got %q", got)
	}
	waitBody(t, extra, "B")
}

func TestRunPackagePortRemovedIsClosed(t *testing.T) {
	r := newTestRoot(t)
	extra := freeAddr(t)
	t.Setenv("ACME_ENABLED", "true")
	t.Setenv("ACME_EXTRA_ADDR", extra)
	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_BODY": "A", "FAKE_ACTIVATE": "b"})
	r.writePorts("a", []listeners.Port{acmePort("ACME_EXTRA_ADDR")})
	r.build("b", knobs{"FAKE_NAME": "B", "FAKE_SERVE_BODY": "B"})
	r.point("a")
	h := r.supervise(testOptions())

	waitBody(t, extra, "A")
	r.trigger(r.waitEvent("A", "ready", 1))
	r.waitEvent("A", "exit", 1)
	waitBody(t, h.addr, "B")
	waitFor(t, 5*time.Second, "the undeclared port to close", func() bool {
		c, err := net.DialTimeout("tcp", extra, time.Second)
		if err == nil {
			c.Close()
		}
		return errors.Is(err, syscall.ECONNREFUSED)
	})
}

func TestRunOtherExitCode(t *testing.T) {
	r := newTestRoot(t)
	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_BODY": "A", "FAKE_EXIT_CODE": "3"})
	r.point("a")
	h := r.supervise(testOptions())

	r.trigger(r.waitEvent("A", "ready", 1))
	if code := h.wait(10 * time.Second); code != 3 {
		t.Fatalf("supervisor exit code = %d, want 3", code)
	}
}

// An interrupted rebuild (the armed marker on a fresh start) whose build
// becomes ready is committed.
func TestRunStartupRecoveryCommits(t *testing.T) {
	r := newTestRoot(t)
	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_BODY": "A"})
	r.point("a")
	fakeActivate(r.dir, "a", "a")
	r.supervise(testOptions())

	r.waitEvent("A", "ready", 1)
	s := r.state()
	waitFor(t, 5*time.Second, "the backup to be committed", func() bool {
		_, armed := s.BackupArmed()
		return !armed
	})
	assertExists(t, s.rollbackPendingMarkerPath(), false)
}

// An interrupted rebuild whose build fails is rolled back to the previous
// build before anything serves.
func TestRunStartupRecoveryRollsBack(t *testing.T) {
	r := newTestRoot(t)
	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_BODY": "A"})
	r.build("b", knobs{"FAKE_NAME": "B", "FAKE_BOOT_EXIT": "1"})
	r.point("a")
	fakeActivate(r.dir, "a", "b")
	h := r.supervise(testOptions())

	r.waitEvent("A", "ready", 1)
	waitBody(t, h.addr, "A")
	s := r.state()
	if got := mustRead(t, s.dbPath()); got != "live" {
		t.Fatalf("data.db = %q, want the backup's bytes", got)
	}
	if got := mustRead(t, s.rollbackPendingMarkerPath()); got != "b" {
		t.Fatalf(".rollback-pending = %q, want b", got)
	}
}

func TestRunSIGTERMDrainsAndExits0(t *testing.T) {
	r := newTestRoot(t)
	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_BODY": "A"})
	r.point("a")
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "-test.run=^$")
	cmd.Env = append(os.Environ(),
		"SUPERVISE_TEST_ROLE=supervisor",
		"TINYCLD_STATE_DIR="+r.dir,
		"HTTP_ADDR="+freeAddr(t),
		"AUTOCERT_ENABLED=",
	)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var waitErr error
	exited := make(chan struct{})
	go func() {
		waitErr = cmd.Wait()
		close(exited)
	}()
	t.Cleanup(func() {
		select {
		case <-exited:
		default:
			cmd.Process.Kill()
			<-exited
		}
		if a := r.index("A", "start", 1); a >= 0 && r.index("A", "exit", 1) < 0 {
			syscall.Kill(-r.events()[a].pid, syscall.SIGKILL)
		}
	})

	r.waitEvent("A", "ready", 1)
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case <-exited:
		if waitErr != nil {
			t.Fatalf("supervisor exited with %v, want 0", waitErr)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the supervisor did not exit after SIGTERM")
	}
	if r.index("A", "drain", 1) < 0 || r.index("A", "exit", 1) < 0 {
		t.Fatalf("the child was not drained: %v", r.events())
	}
}

func TestRunConfigPlain(t *testing.T) {
	env := map[string]string{"TINYCLD_STATE_DIR": "/state"}
	cfg, err := readConfig([]string{"--dir=/state/pb_data"}, func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"serve", "--dir=/state/pb_data", "--http=0.0.0.0:7090"}; !reflect.DeepEqual(cfg.childArgs, want) {
		t.Fatalf("child args = %q, want %q", cfg.childArgs, want)
	}
	if want := []portWant{{ListenerHTTP, "0.0.0.0:7090"}}; !reflect.DeepEqual(cfg.base, want) {
		t.Fatalf("ports = %v, want %v", cfg.base, want)
	}

	env["HTTP_ADDR"] = "127.0.0.1:9000"
	env["AUTOCERT_ENABLED"] = "true" // without a domain: plain HTTP, as the entrypoint does
	cfg, err = readConfig(nil, func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"serve", "--http=127.0.0.1:9000"}; !reflect.DeepEqual(cfg.childArgs, want) {
		t.Fatalf("child args = %q, want %q", cfg.childArgs, want)
	}
}

func TestRunConfigAutocert(t *testing.T) {
	env := map[string]string{
		"TINYCLD_STATE_DIR":  "/state",
		"AUTOCERT_ENABLED":   " Yes ",
		"PRIMARY_DOMAIN":     " example.test ",
		"ADDITIONAL_DOMAINS": "a.example.test, ,b.example.test ",
		"HTTP_ADDR":          "127.0.0.1:9000",
	}
	cfg, err := readConfig([]string{"--dir=/state/pb_data"}, func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"serve", "--dir=/state/pb_data", "example.test", "a.example.test", "b.example.test", "--http=0.0.0.0:80", "--https=0.0.0.0:443"}
	if !reflect.DeepEqual(cfg.childArgs, want) {
		t.Fatalf("child args = %q, want %q", cfg.childArgs, want)
	}
	if want := []portWant{{ListenerHTTPS, "0.0.0.0:443"}, {ListenerHTTPRedirect, "0.0.0.0:80"}}; !reflect.DeepEqual(cfg.base, want) {
		t.Fatalf("ports = %v, want %v", cfg.base, want)
	}
}

func TestRunConfigNeedsStateDir(t *testing.T) {
	if _, err := readConfig(nil, func(string) string { return "" }); err == nil {
		t.Fatal("readConfig without TINYCLD_STATE_DIR did not fail")
	}
}

func TestRunBindErrorNamesTheAddress(t *testing.T) {
	r := newTestRoot(t)
	r.build("a", knobs{"FAKE_NAME": "A"})
	r.point("a")
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { taken.Close() })
	t.Setenv("TINYCLD_STATE_DIR", r.dir)
	t.Setenv("HTTP_ADDR", taken.Addr().String())
	t.Setenv("AUTOCERT_ENABLED", "")
	_, err = newSupervisor(nil, os.Getenv, testOptions(), make(chan os.Signal))
	if err == nil || !strings.Contains(err.Error(), taken.Addr().String()) {
		t.Fatalf("newSupervisor on a taken port = %v, want an error naming %s", err, taken.Addr())
	}
}
