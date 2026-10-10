//go:build unix

package supervise

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"tinycld.org/core/backup"
	"tinycld.org/core/backup/arm"
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
//	FAKE_SERVE_DB      answer with pb_data/data.db's bytes instead: the data it serves
//	FAKE_APPLY_RESTORE at start, apply a staged restore as the server's boot does, and log "data <bytes of data.db>"
//	FAKE_READY_DELAY   ms to wait before ready (a slow boot)
//	FAKE_NEVER_READY   never send ready and never serve (a hung boot)
//	FAKE_IGNORE_TERM   ignore SIGTERM (only SIGKILL stops it)
//	FAKE_BOOT_EXIT     exit with this code before ready (a failed boot)
//	FAKE_BOOT_EXIT_START  exit with code 1 before ready on this start (1-based) only
//	FAKE_SEND_JUNK     send messages the supervisor must ignore before ready
//	FAKE_READY_VERSION send ready with this protocol version
//	FAKE_RESTART_BEFORE_READY  on this build's first start only (a boot-time rebuild runs once), activate FAKE_ACTIVATE and ask for a restart before ready, then wait for the ack
//	FAKE_ACTIVATE      on SIGUSR1, arm the backup and point current at this build, as a rebuild does
//	FAKE_KEEP_DB       with FAKE_ACTIVATE, leave data.db as it is: a restore's rebuild runs no migration
//	FAKE_EXIT_CODE     on SIGUSR1, exit with this code instead of asking for a restart
//	FAKE_COLD          on SIGUSR1, ask for a cold restart
//	FAKE_DROP_CURRENT  on SIGUSR1, record this build as the previous one and remove current, so it no longer resolves
func fakeChild() int {
	root := os.Getenv("TINYCLD_STATE_DIR")
	ev := func(what string) {
		appendEvent(root, fmt.Sprintf("%s %d %s", os.Getenv("FAKE_NAME"), os.Getpid(), what))
	}
	ev("start")
	if os.Getenv("FAKE_APPLY_RESTORE") == "1" {
		pbData := State{Root: root}.pbDataDir()
		if err := backup.ApplyPendingRestore(pbData); err != nil {
			ev("restore-failed")
			return 2
		}
		data, _ := os.ReadFile(State{Root: root}.dbPath())
		ev("data " + string(data))
	}

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
	if n := os.Getenv("FAKE_BOOT_EXIT_START"); n != "" && startCount(root, os.Getenv("FAKE_NAME")) == atoi(n) {
		ev("exit")
		return 1
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
			fakeActivate(root, os.Getenv("FAKE_BUILD"), to, os.Getenv("FAKE_KEEP_DB") == "1")
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
		if os.Getenv("FAKE_SERVE_DB") == "1" {
			data, _ := os.ReadFile(State{Root: root}.dbPath())
			w.Write(data)
			return
		}
		io.WriteString(w, body)
	})}
	drainer := NewDrainer(srv)
	for _, l := range ls {
		go srv.Serve(drainer.Listener(l))
	}

	for {
		select {
		case <-trigger:
			if os.Getenv("FAKE_DROP_CURRENT") == "1" {
				s := State{Root: root}
				os.WriteFile(s.previousBuildPath(), []byte(os.Getenv("FAKE_BUILD")), 0o644)
				os.Remove(s.currentLinkPath())
			}
			if to := os.Getenv("FAKE_ACTIVATE"); to != "" {
				fakeActivate(root, os.Getenv("FAKE_BUILD"), to, os.Getenv("FAKE_KEEP_DB") == "1")
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
// a migrated database (unless keepDB) and current pointing at the new build.
func fakeActivate(root, from, to string, keepDB bool) {
	s := State{Root: root}
	data, _ := os.ReadFile(s.dbPath())
	os.WriteFile(s.dbBackupPath(), data, 0o644)
	os.WriteFile(s.dbArmedMarkerPath(), []byte(to), 0o644)
	os.WriteFile(s.previousBuildPath(), []byte(from), 0o644)
	if !keepDB {
		os.WriteFile(s.dbPath(), []byte("migrated-by-"+to), 0o644)
	}
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
	const limit = 20 * time.Second
	deadline := time.Now().Add(limit)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for r.index(name, what, nth) < 0 {
		if time.Now().After(deadline) {
			// On CI, where a failure cannot be rerun locally, what every child
			// did is the evidence a missing event needs.
			var log strings.Builder
			if os.Getenv("CI") != "" {
				log.WriteString("; events:")
				for _, e := range r.events() {
					fmt.Fprintf(&log, "\n  %s %d %s", e.name, e.pid, e.what)
				}
			}
			r.t.Fatalf("timed out after %s waiting for event %d of %q %q%s", limit, nth, name, what, log.String())
		}
		<-tick.C
	}
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
	return options{readyTimeout: 20 * time.Second, drainBound: 15 * time.Second, stopBound: 3 * time.Second, childUser: testChildUser()}
}

// testChildUser is the user running the tests. As root, children then run
// as root too, so a root test run needs no "tinycld" user on the host.
func testChildUser() string {
	u, err := user.Current()
	if err != nil {
		panic(fmt.Sprintf("look up the user running the tests: %v", err))
	}
	return u.Username
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

// keepAliveLoad sends a request every 5 ms on one kept-alive connection at
// a time. It speaks HTTP by hand so no client library retries a failed
// request out of sight: the test must see each one.
type keepAliveLoad struct {
	addr string

	mu     sync.Mutex
	bodies []string
	// closedUnder counts requests that failed because the server closed an
	// idle kept-alive connection as the request went out. Each was retried
	// at once on a fresh connection.
	closedUnder int
	// retryFailed holds the retries of those requests that failed too.
	retryFailed []error
	// failed holds every other failure.
	failed []error

	stop, stopped chan struct{}
}

func startKeepAliveLoad(addr string) *keepAliveLoad {
	l := &keepAliveLoad{addr: addr, stop: make(chan struct{}), stopped: make(chan struct{})}
	go l.run()
	return l
}

type keepAliveConn struct {
	net.Conn
	r *bufio.Reader
	// answered is how many requests this connection has carried.
	answered int
}

func (l *keepAliveLoad) run() {
	defer close(l.stopped)
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	var c *keepAliveConn
	defer func() {
		if c != nil {
			c.Close()
		}
	}()
	for {
		select {
		case <-l.stop:
			return
		case <-tick.C:
		}
		if c == nil {
			var err error
			if c, err = l.dial(); err != nil {
				l.record("", fmt.Errorf("dial: %w", err))
				continue
			}
		}
		body, again, err := c.roundTrip()
		if err != nil && c.answered > 0 && closedUnderRequest(err) {
			c.Close()
			l.mu.Lock()
			l.closedUnder++
			l.mu.Unlock()
			body, c, err = l.retry()
			if err != nil {
				l.mu.Lock()
				l.retryFailed = append(l.retryFailed, err)
				l.mu.Unlock()
				continue
			}
			l.record(body, nil)
			continue
		}
		l.record(body, err)
		if !again || err != nil {
			c.Close()
			c = nil
		}
	}
}

// retry sends the request again on a fresh connection, and returns that
// connection when the server keeps it open.
func (l *keepAliveLoad) retry() (string, *keepAliveConn, error) {
	c, err := l.dial()
	if err != nil {
		return "", nil, err
	}
	body, again, err := c.roundTrip()
	if err != nil || !again {
		c.Close()
		return body, nil, err
	}
	return body, c, nil
}

func (l *keepAliveLoad) dial() (*keepAliveConn, error) {
	c, err := net.DialTimeout("tcp", l.addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	return &keepAliveConn{Conn: c, r: bufio.NewReader(c)}, nil
}

// roundTrip sends one request and reads its answer. again reports whether
// the server keeps the connection open for the next one.
func (c *keepAliveConn) roundTrip() (body string, again bool, err error) {
	c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(c, "GET / HTTP/1.1\r\nHost: test\r\n\r\n"); err != nil {
		return "", false, err
	}
	// ReadResponse reports a connection that closed before any byte as
	// ErrUnexpectedEOF, the same as one cut mid-answer. Only the first is
	// the documented limit, so it is told apart here.
	if _, err := c.r.Peek(1); err != nil {
		return "", false, err
	}
	resp, err := http.ReadResponse(c.r, nil)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, err
	}
	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("status %d", resp.StatusCode)
	}
	c.answered++
	return string(b), !resp.Close, nil
}

// closedUnderRequest reports whether err is the server closing the
// connection before it read the request: nothing came back at all.
func closedUnderRequest(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE)
}

func (l *keepAliveLoad) record(body string, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err != nil {
		l.failed = append(l.failed, err)
		return
	}
	l.bodies = append(l.bodies, body)
}

func (l *keepAliveLoad) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.bodies)
}

func (l *keepAliveLoad) end() {
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
	ka := startKeepAliveLoad(h.addr)
	waitFor(t, 10*time.Second, "load on A", func() bool { return ld.count() >= 10 && ka.count() >= 10 })

	r.trigger(a)
	r.waitEvent("A", "exit", 1)
	atExit, kaAtExit := ld.count(), ka.count()
	waitFor(t, 10*time.Second, "load on B", func() bool { return ld.count() >= atExit+20 && ka.count() >= kaAtExit+20 })
	ld.end()
	ka.end()

	if ld.refused != 0 || len(ld.failed) != 0 {
		t.Fatalf("refused connections = %d, failed requests = %d (%v)", ld.refused, len(ld.failed), ld.failed)
	}
	if first, last := ld.bodies[0], ld.bodies[len(ld.bodies)-1]; first != "A" || last != "B" {
		t.Fatalf("answers went %q ... %q, want A ... B", first, last)
	}
	// The limit docs/live-install.md states: the drain closes A's idle
	// kept-alive connection, so the one request sent on it as it closes
	// fails, and its retry on a fresh connection succeeds. Nothing else
	// fails.
	t.Logf("keep-alive client: %d answers, %d requests closed under", len(ka.bodies), ka.closedUnder)
	if len(ka.failed) != 0 || len(ka.retryFailed) != 0 {
		t.Fatalf("keep-alive client: failed requests %v, failed retries %v", ka.failed, ka.retryFailed)
	}
	if ka.closedUnder > 1 {
		t.Fatalf("keep-alive client: %d requests failed on a closed connection, want at most the one A's drain closed", ka.closedUnder)
	}
	if first, last := ka.bodies[0], ka.bodies[len(ka.bodies)-1]; first != "A" || last != "B" {
		t.Fatalf("keep-alive answers went %q ... %q, want A ... B", first, last)
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
	assertRollbackRecord(t, s, "b", "a")
	if got := readLink(t, s.currentLinkPath()); got != filepath.Join(s.buildsDir(), "a", "tinycld") {
		t.Fatalf("current -> %q, want the previous build", got)
	}
}

// assertRollbackRecord checks the record a rollback leaves for the next
// boot: in the state root, naming the build that failed and the build
// rolled back to, and not in pb_data, where a restore swap would carry it
// away.
func assertRollbackRecord(t *testing.T, s State, build, rolledTo string) {
	t.Helper()
	got := readRollbackRecord(t, s)
	if got.Build != build || got.RolledTo != rolledTo {
		t.Fatalf("rollback record = %+v, want build %q rolled to %q", got, build, rolledTo)
	}
	if got.At.IsZero() {
		t.Fatal("the rollback record has no time")
	}
	assertExists(t, filepath.Join(s.pbDataDir(), ".rollback-pending"), false)
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

// stageRestore leaves what a restore stages for the next boot: the archive's
// data.db in a pending dir marked complete, and the armed restore marker.
func (r *testRoot) stageRestore(id, data string) {
	r.t.Helper()
	dir := arm.Dir(r.state().pbDataDir())
	pending := arm.PendingDir(dir, id)
	if err := os.MkdirAll(pending, 0o700); err != nil {
		r.t.Fatal(err)
	}
	mustWrite(r.t, filepath.Join(pending, "data.db"), data)
	mustWrite(r.t, filepath.Join(pending, arm.StagedSentinel), "")
	if err := arm.WriteMarker(dir, arm.Marker{ID: id, Pending: pending}); err != nil {
		r.t.Fatal(err)
	}
}

// A restore whose archive needs another build is a cold restart onto that
// build, and the new build's boot swaps the staged data into pb_data before
// it can fail. The rollback must bring the previous build back on the data
// it had. The swap carried the armed database backup aside with pb_data, so
// the supervisor's restore step finds none, and the previous build's boot
// brings it back when it undoes the swap: the rollback must not leave it
// armed, or a later rollback would restore it over everything written since.
func TestRunColdRestoreFailingAfterTheSwapRollsBack(t *testing.T) {
	r := newTestRoot(t)
	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_DB": "1", "FAKE_APPLY_RESTORE": "1", "FAKE_ACTIVATE": "b", "FAKE_KEEP_DB": "1", "FAKE_COLD": "1"})
	r.build("b", knobs{"FAKE_NAME": "B", "FAKE_APPLY_RESTORE": "1", "FAKE_BOOT_EXIT": "1"})
	r.point("a")
	h := r.supervise(testOptions())

	a := r.waitEvent("A", "ready", 1)
	waitBody(t, h.addr, "live")
	r.stageRestore("restore-1", "restored")
	r.trigger(a)

	r.waitEvent("B", "data restored", 1)
	r.waitEvent("A", "ready", 2)
	waitBody(t, h.addr, "live")
	if r.index("A", "data live", 2) < 0 {
		t.Fatalf("the previous build did not boot on the data it had: %v", r.events())
	}
	s := r.state()
	if got := readLink(t, s.currentLinkPath()); got != filepath.Join(s.buildsDir(), "a", "tinycld") {
		t.Fatalf("current -> %q, want the previous build", got)
	}
	failed := filepath.Join(arm.Dir(s.pbDataDir()), "failed", "restore-1", "data.db")
	if got := mustRead(t, failed); got != "restored" {
		t.Fatalf("the restored data kept for inspection = %q", got)
	}
	waitFor(t, 5*time.Second, "the stale backup to be dropped", func() bool {
		_, armed := s.BackupArmed()
		return !armed
	})
	assertExists(t, s.dbBackupPath(), false)
	// The swap carried the armed marker aside with pb_data, so the record
	// must name the failed build without it.
	assertRollbackRecord(t, s, "b", "a")
}

// A restore that needs no other build is a cold restart of the build that
// serves. When that build then fails on the swapped-in data, no newer build
// exists to leave: the previous build is older than the data, so it must not
// start, and nothing is recorded as rolled back. The same build starts
// again, and its boot undoes the swap.
func TestRunColdRestoreWithoutARebuildFailingStartsTheSameBuild(t *testing.T) {
	r := newTestRoot(t)
	r.build("old", knobs{"FAKE_NAME": "OLD", "FAKE_SERVE_BODY": "OLD"})
	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_DB": "1", "FAKE_APPLY_RESTORE": "1", "FAKE_COLD": "1", "FAKE_BOOT_EXIT_START": "2"})
	r.point("a")
	s := r.state()
	// What an earlier rebuild onto a left behind.
	mustWrite(t, s.previousBuildPath(), "old")
	h := r.supervise(testOptions())

	a := r.waitEvent("A", "ready", 1)
	waitBody(t, h.addr, "live")
	r.stageRestore("restore-1", "restored")
	r.trigger(a)

	r.waitEvent("A", "data restored", 1)
	r.waitEvent("A", "ready", 2)
	waitBody(t, h.addr, "live")
	if r.index("A", "start", 3) < 0 {
		t.Fatalf("the serving build was not started again: %v", r.events())
	}
	if r.index("OLD", "start", 1) >= 0 {
		t.Fatalf("the older previous build started on the newer build's data: %v", r.events())
	}
	if got := readLink(t, s.currentLinkPath()); got != filepath.Join(s.buildsDir(), "a", "tinycld") {
		t.Fatalf("current -> %q, want the build that served", got)
	}
	assertExists(t, s.rollbackRecordPath(), false)
}

// promoteBThenColdRestartFailing swaps a to b, waits for the swap's backup
// to be committed, runs stale (when not nil) with b serving, and then asks b
// for a cold restart whose start fails once. It returns once b serves again.
func (r *testRoot) promoteBThenColdRestartFailing(stale func(s State)) *harness {
	t := r.t
	t.Helper()
	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_BODY": "A", "FAKE_ACTIVATE": "b"})
	r.build("b", knobs{"FAKE_NAME": "B", "FAKE_SERVE_BODY": "B", "FAKE_COLD": "1", "FAKE_BOOT_EXIT_START": "2"})
	r.point("a")
	h := r.supervise(testOptions())
	s := r.state()

	r.trigger(r.waitEvent("A", "ready", 1))
	b := r.waitEvent("B", "ready", 1)
	waitBody(t, h.addr, "B")
	waitFor(t, 5*time.Second, "the swap's backup to be committed", func() bool {
		_, armed := s.BackupArmed()
		return !armed
	})
	if stale != nil {
		stale(s)
	}
	r.trigger(b)
	r.waitEvent("B", "ready", 2)
	waitBody(t, h.addr, "B")
	return h
}

// A build that was promoted is the one that serves: when a later cold
// restart of it fails, no newer build exists to leave, so the same build
// starts again and current stays on it.
func TestRunPromotedBuildFailingAColdRestartStartsItAgain(t *testing.T) {
	r := newTestRoot(t)
	r.promoteBThenColdRestartFailing(nil)

	if r.index("B", "start", 3) < 0 {
		t.Fatalf("the serving build was not started again: %v", r.events())
	}
	if r.index("A", "start", 2) >= 0 {
		t.Fatalf("the previous build started on the promoted build's data: %v", r.events())
	}
	s := r.state()
	if got := readLink(t, s.currentLinkPath()); got != filepath.Join(s.buildsDir(), "b", "tinycld") {
		t.Fatalf("current -> %q, want the promoted build", got)
	}
	if got := mustRead(t, s.dbPath()); got != "migrated-by-b" {
		t.Fatalf("data.db = %q, want the promoted build's data", got)
	}
	assertExists(t, s.rollbackRecordPath(), false)
}

// A backup the promote could not commit stays armed after the build served
// live writes. A later failed restart of that same build must not restore
// it over them: the backup is set aside, and the data stays.
func TestRunServingBuildFailingKeepsLiveDataOverAStaleBackup(t *testing.T) {
	r := newTestRoot(t)
	r.promoteBThenColdRestartFailing(func(s State) {
		// What a failed CommitBackup in promote leaves, then a write.
		mustWrite(t, s.dbBackupPath(), "live")
		mustWrite(t, s.dbArmedMarkerPath(), "b")
		mustWrite(t, s.dbPath(), "written-since")
	})

	s := r.state()
	if got := mustRead(t, s.dbPath()); got != "written-since" {
		t.Fatalf("data.db = %q, want the data written since the backup", got)
	}
	if got, armed := s.BackupArmed(); armed {
		t.Fatalf("the stale backup is still armed for %q", got)
	}
	assertExists(t, s.dbBackupPath(), false)
	if got := mustRead(t, filepath.Join(s.unrestoredDir(), "b", "data.db")); got != "live" {
		t.Fatalf("unrestored/b/data.db = %q, want the stale backup", got)
	}
	notes, err := s.Unrestored()
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 || notes[0].Build != "b" || notes[0].RolledTo != "b" || !strings.Contains(notes[0].RestoreError, "served") {
		t.Fatalf("unrestored notes = %+v, want one for b, still on b, saying b served", notes)
	}
	if got := readLink(t, s.currentLinkPath()); got != filepath.Join(s.buildsDir(), "b", "tinycld") {
		t.Fatalf("current -> %q, want the build that served", got)
	}
	assertExists(t, s.rollbackRecordPath(), false)
}

// A current that no longer resolves names no failed build, so nothing is
// recorded, but the previous build is the only one that can start: the
// rollback still flips current back to it.
func TestRunRollbackWithAnUnresolvableCurrentStartsThePreviousBuild(t *testing.T) {
	r := newTestRoot(t)
	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_BODY": "A", "FAKE_DROP_CURRENT": "1", "FAKE_COLD": "1"})
	r.point("a")
	h := r.supervise(testOptions())

	r.trigger(r.waitEvent("A", "ready", 1))
	r.waitEvent("A", "ready", 2)
	waitBody(t, h.addr, "A")
	s := r.state()
	if got := readLink(t, s.currentLinkPath()); got != filepath.Join(s.buildsDir(), "a", "tinycld") {
		t.Fatalf("current -> %q, want the previous build", got)
	}
	assertExists(t, s.rollbackRecordPath(), false)
}

// failRestoreRename makes the restore step's rename over data.db fail with a
// full disk, so RestoreBackup cannot put the backup back. The seam is put
// back by the caller once no supervisor reads it, and at cleanup.
func failRestoreRename(t *testing.T, s State) (undo func()) {
	t.Helper()
	prev := renameFile
	renameFile = func(from, to string) error {
		if to == s.dbPath() {
			return &os.LinkError{Op: "rename", Old: from, New: to, Err: syscall.ENOSPC}
		}
		return prev(from, to)
	}
	undo = func() { renameFile = prev }
	// Registered before the supervisor's own cleanup, so it runs after the
	// supervisor has stopped reading it.
	t.Cleanup(undo)
	return undo
}

// rollBackOverAnUnrestorableBackup runs a rebuild from a to b whose build b
// fails and whose backup the rollback cannot restore, then stops the
// supervisor.
func (r *testRoot) rollBackOverAnUnrestorableBackup() {
	t := r.t
	t.Helper()
	undo := failRestoreRename(t, r.state())
	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_BODY": "A", "FAKE_ACTIVATE": "b"})
	r.build("b", knobs{"FAKE_NAME": "B", "FAKE_BOOT_EXIT": "1"})
	r.point("a")
	h := r.supervise(testOptions())

	readies := readyLogs(t)
	r.trigger(r.waitEvent("A", "ready", 1))
	again := r.waitEvent("A", "ready", 2)
	waitBody(t, h.addr, "A")
	// The rollback ends with its backup step once it has taken this ready,
	// and reads no stop signal until then; a stop sent after the ready is
	// taken is answered only once the rollback is over.
	waitClosed(t, readies.of(again.pid), 5*time.Second, "the supervisor to take the rolled-back child's ready")
	h.sigs <- syscall.SIGTERM
	if code := h.wait(20 * time.Second); code != 0 {
		t.Fatalf("supervisor exit code = %d, want 0", code)
	}
	undo()
}

// A backup the rollback could not restore (a full disk, an I/O error) is the
// only copy of the database from before the migration. Left armed for the
// failed build, the next rebuild would delete it, a healthy restart would
// commit it and an unhealthy one would restore it over every newer write. So
// the rollback moves it out of every armed path, to unrestored/<build>/,
// with a note naming the builds.
func TestRunRollbackKeepsABackupItCouldNotRestore(t *testing.T) {
	r := newTestRoot(t)
	r.rollBackOverAnUnrestorableBackup()

	s := r.state()
	if got, armed := s.BackupArmed(); armed {
		t.Fatalf("the unrestored backup is still armed for %q", got)
	}
	assertExists(t, s.dbBackupPath(), false)
	assertUnrestored(t, s, "b", "a", "live")
	if got := mustRead(t, s.dbPath()); got != "migrated-by-b" {
		t.Fatalf("data.db = %q, want the failed build's data, which nothing could restore", got)
	}
	assertRollbackRecord(t, s, "b", "a")
}

// assertUnrestored checks the copy kept in unrestored/<build>/ and its note.
func assertUnrestored(t *testing.T, s State, build, rolledTo, data string) {
	t.Helper()
	if got := mustRead(t, filepath.Join(s.unrestoredDir(), build, "data.db")); got != data {
		t.Fatalf("unrestored/%s/data.db = %q, want %q", build, got, data)
	}
	notes, err := s.Unrestored()
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 {
		t.Fatalf("unrestored notes = %+v, want one", notes)
	}
	n := notes[0]
	if n.Build != build || n.RolledTo != rolledTo || n.At.IsZero() || n.Size != int64(len(data)) {
		t.Fatalf("unrestored note = %+v, want build %q rolled to %q, %d bytes", n, build, rolledTo, len(data))
	}
	if !strings.Contains(n.RestoreError, syscall.ENOSPC.Error()) {
		t.Fatalf("unrestored note restore error = %q, want the restore's error", n.RestoreError)
	}
}

// A kept copy is in no armed path, so a later supervisor start neither
// commits nor restores it, and a later successful swap commits only its own
// backup. Each start reminds the operator that it is there.
func TestRunUnrestoredBackupSurvivesARestartAndASwap(t *testing.T) {
	r := newTestRoot(t)
	r.rollBackOverAnUnrestorableBackup()
	s := r.state()

	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_BODY": "A", "FAKE_ACTIVATE": "c"})
	r.build("c", knobs{"FAKE_NAME": "C", "FAKE_SERVE_BODY": "C"})
	logs := readyLogs(t)
	h := r.supervise(testOptions())

	a := r.waitEvent("A", "ready", 3)
	waitBody(t, h.addr, "A")
	if got := mustRead(t, s.dbPath()); got != "migrated-by-b" {
		t.Fatalf("data.db = %q after a restart, want it untouched", got)
	}
	assertUnrestored(t, s, "b", "a", "live")
	reminders := logs.find(slog.LevelError, unrestoredReminder)
	if len(reminders) != 1 || !strings.Contains(reminders[0].attrs, filepath.Join(s.unrestoredDir(), "b")) {
		t.Fatalf("start reminders = %+v, want one error naming unrestored/b", reminders)
	}

	r.trigger(a)
	r.waitEvent("C", "ready", 1)
	waitBody(t, h.addr, "C")
	waitFor(t, 5*time.Second, "the swap's backup to be committed", func() bool {
		_, armed := s.BackupArmed()
		return !armed
	})
	assertUnrestored(t, s, "b", "a", "live")
}

// readyLog follows the supervisor's "the server is ready" records by pid.
type readyLog struct {
	slog.Handler
	mu   *sync.Mutex
	seen map[int]chan struct{}
	recs *[]loggedRecord
}

// loggedRecord is one record the supervisor logged, its attrs as text.
type loggedRecord struct {
	level slog.Level
	msg   string
	attrs string
}

// readyLogs makes the supervisor's log report each ready it takes. The
// default logger is global, so this relies on the package's tests not
// running in parallel; the previous one is restored at cleanup.
func readyLogs(t *testing.T) *readyLog {
	t.Helper()
	prev := slog.Default()
	// Not prev's handler: wrapping slog's built-in default handler in a new
	// default deadlocks, because that handler writes through the log package,
	// which SetDefault points back at the new default.
	h := &readyLog{Handler: slog.NewTextHandler(os.Stderr, nil), mu: &sync.Mutex{}, seen: map[int]chan struct{}{}, recs: &[]loggedRecord{}}
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return h
}

// of returns a channel that closes once the supervisor took pid's ready.
func (h *readyLog) of(pid int) <-chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.chanLocked(pid)
}

func (h *readyLog) chanLocked(pid int) chan struct{} {
	ch, ok := h.seen[pid]
	if !ok {
		ch = make(chan struct{})
		h.seen[pid] = ch
	}
	return ch
}

// find returns the records logged at level with message msg.
func (h *readyLog) find(level slog.Level, msg string) []loggedRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []loggedRecord
	for _, r := range *h.recs {
		if r.level == level && r.msg == msg {
			out = append(out, r)
		}
	}
	return out
}

func (h *readyLog) Handle(ctx context.Context, rec slog.Record) error {
	var attrs strings.Builder
	rec.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&attrs, "%s=%v ", a.Key, a.Value)
		return true
	})
	h.mu.Lock()
	*h.recs = append(*h.recs, loggedRecord{level: rec.Level, msg: rec.Message, attrs: attrs.String()})
	h.mu.Unlock()
	if rec.Message == "the server is ready" {
		rec.Attrs(func(a slog.Attr) bool {
			if a.Key != "pid" {
				return true
			}
			h.mu.Lock()
			ch := h.chanLocked(int(a.Value.Int64()))
			select {
			case <-ch:
			default:
				close(ch)
			}
			h.mu.Unlock()
			return false
		})
	}
	return h.Handler.Handle(ctx, rec)
}

func (h *readyLog) WithAttrs(as []slog.Attr) slog.Handler {
	return &readyLog{Handler: h.Handler.WithAttrs(as), mu: h.mu, seen: h.seen, recs: h.recs}
}

func (h *readyLog) WithGroup(name string) slog.Handler {
	return &readyLog{Handler: h.Handler.WithGroup(name), mu: h.mu, seen: h.seen, recs: h.recs}
}

func waitClosed(t *testing.T, ch <-chan struct{}, limit time.Duration, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(limit):
		t.Fatalf("timed out after %s waiting for %s", limit, what)
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
	waitFor(t, 5*time.Second, "the undeclared port to close", func() bool { return dialRefused(extra) })
}

func dialRefused(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err == nil {
		c.Close()
	}
	return errors.Is(err, syscall.ECONNREFUSED)
}

// A build that moves a package port to a new address gets it on the swap,
// while the old child keeps its old address until it has drained.
func TestRunPackagePortMoved(t *testing.T) {
	r := newTestRoot(t)
	oldAddr, newAddr := freeAddr(t), freeAddr(t)
	t.Setenv("ACME_ENABLED", "true")
	t.Setenv("ACME_OLD_ADDR", oldAddr)
	t.Setenv("ACME_NEW_ADDR", newAddr)
	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_BODY": "A", "FAKE_ACTIVATE": "b"})
	r.writePorts("a", []listeners.Port{acmePort("ACME_OLD_ADDR")})
	r.build("b", knobs{"FAKE_NAME": "B", "FAKE_SERVE_BODY": "B"})
	r.writePorts("b", []listeners.Port{acmePort("ACME_NEW_ADDR")})
	r.point("a")
	h := r.supervise(testOptions())

	waitBody(t, oldAddr, "A")
	r.trigger(r.waitEvent("A", "ready", 1))
	r.waitEvent("A", "exit", 1)
	if got := r.eventWith("B", "listeners", 1); got != "listeners http,acme-extra" {
		t.Fatalf("B got %q", got)
	}
	waitBody(t, newAddr, "B")
	waitBody(t, h.addr, "B")
	waitFor(t, 5*time.Second, "the old address to close", func() bool { return dialRefused(oldAddr) })
}

// A port moved by a build that then fails moves back with the rollback: the
// rolled-back build gets its old address again, and the failed build's
// address closes.
func TestRunPackagePortMovedThenRolledBack(t *testing.T) {
	r := newTestRoot(t)
	oldAddr, newAddr := freeAddr(t), freeAddr(t)
	t.Setenv("ACME_ENABLED", "true")
	t.Setenv("ACME_OLD_ADDR", oldAddr)
	t.Setenv("ACME_NEW_ADDR", newAddr)
	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_BODY": "A", "FAKE_ACTIVATE": "b"})
	r.writePorts("a", []listeners.Port{acmePort("ACME_OLD_ADDR")})
	r.build("b", knobs{"FAKE_NAME": "B", "FAKE_BOOT_EXIT": "1"})
	r.writePorts("b", []listeners.Port{acmePort("ACME_NEW_ADDR")})
	r.point("a")
	h := r.supervise(testOptions())

	r.trigger(r.waitEvent("A", "ready", 1))
	assertColdRollback(t, r, h)
	if got := r.eventWith("A", "listeners", 2); got != "listeners http,acme-extra" {
		t.Fatalf("the rolled-back A got %q", got)
	}
	waitBody(t, oldAddr, "A")
	waitFor(t, 5*time.Second, "the failed build's address to close", func() bool { return dialRefused(newAddr) })
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
	fakeActivate(r.dir, "a", "a", false)
	r.supervise(testOptions())

	r.waitEvent("A", "ready", 1)
	s := r.state()
	waitFor(t, 5*time.Second, "the backup to be committed", func() bool {
		_, armed := s.BackupArmed()
		return !armed
	})
	assertExists(t, s.rollbackRecordPath(), false)
}

// An interrupted rebuild whose build fails is rolled back to the previous
// build before anything serves.
func TestRunStartupRecoveryRollsBack(t *testing.T) {
	r := newTestRoot(t)
	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_BODY": "A"})
	r.build("b", knobs{"FAKE_NAME": "B", "FAKE_BOOT_EXIT": "1"})
	r.point("a")
	fakeActivate(r.dir, "a", "b", false)
	h := r.supervise(testOptions())

	r.waitEvent("A", "ready", 1)
	waitBody(t, h.addr, "A")
	s := r.state()
	if got := mustRead(t, s.dbPath()); got != "live" {
		t.Fatalf("data.db = %q, want the backup's bytes", got)
	}
	assertRollbackRecord(t, s, "b", "a")
}

func TestRunSIGTERMDrainsAndExits0(t *testing.T) {
	r := newTestRoot(t)
	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_BODY": "A"})
	r.point("a")
	p := r.startFakeSupervisor(freeAddr(t))

	r.waitEvent("A", "ready", 1)
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if code := p.wait(t, 20*time.Second); code != 0 {
		t.Fatalf("supervisor exit code = %d, want 0", code)
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

// fakeSupervisor is the supervisor running in its own process over fake
// children: the real Run path, with its signal handling, Sentry setup and
// exit. (supervisorProc in e2e_test.go runs it over the real server.)
type fakeSupervisor struct {
	cmd    *exec.Cmd
	exited chan struct{}
	err    error
}

// startFakeSupervisor runs the supervisor role on the root, in plain mode
// on addr, with env added to the test's own. Cleanup kills it and every
// child it started.
func (r *testRoot) startFakeSupervisor(addr string, env ...string) *fakeSupervisor {
	r.t.Helper()
	bin, err := os.Executable()
	if err != nil {
		r.t.Fatal(err)
	}
	cmd := exec.Command(bin, "-test.run=^$")
	cmd.Env = append(os.Environ(),
		"SUPERVISE_TEST_ROLE=supervisor",
		"TINYCLD_STATE_DIR="+r.dir,
		"HTTP_ADDR="+addr,
		"AUTOCERT_ENABLED=",
	)
	cmd.Env = append(cmd.Env, env...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		r.t.Fatal(err)
	}
	p := &fakeSupervisor{cmd: cmd, exited: make(chan struct{})}
	go func() {
		p.err = cmd.Wait()
		close(p.exited)
	}()
	r.t.Cleanup(func() {
		select {
		case <-p.exited:
		default:
			cmd.Process.Kill()
			<-p.exited
		}
		for _, e := range r.events() {
			if e.what == "start" {
				syscall.Kill(-e.pid, syscall.SIGKILL)
			}
		}
	})
	return p
}

// wait returns the supervisor's exit code once it has exited.
func (p *fakeSupervisor) wait(t *testing.T, limit time.Duration) int {
	t.Helper()
	select {
	case <-p.exited:
		return p.cmd.ProcessState.ExitCode()
	case <-time.After(limit):
		t.Fatalf("the supervisor did not exit within %s", limit)
		return -1
	}
}

// sentrySink is a Sentry endpoint that keeps every event it receives.
type sentrySink struct {
	srv    *httptest.Server
	mu     sync.Mutex
	events []sentryEvent
}

type sentryEvent struct {
	Message  string         `json:"message"`
	Level    string         `json:"level"`
	Contexts map[string]any `json:"contexts"`
}

func newSentrySink(t *testing.T) *sentrySink {
	t.Helper()
	s := &sentrySink{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		// An envelope is JSON lines: its header, then each item's header and
		// payload. Only an event payload carries a message.
		for _, line := range bytes.Split(body, []byte("\n")) {
			var ev sentryEvent
			if json.Unmarshal(line, &ev) == nil && ev.Message != "" {
				s.mu.Lock()
				s.events = append(s.events, ev)
				s.mu.Unlock()
			}
		}
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *sentrySink) dsn() string {
	return "http://public@" + strings.TrimPrefix(s.srv.URL, "http://") + "/1"
}

func (s *sentrySink) with(message string) []sentryEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []sentryEvent
	for _, ev := range s.events {
		if ev.Message == message {
			out = append(out, ev)
		}
	}
	return out
}

// A rollback is reported to Sentry, from the DSN in the supervisor's
// environment, and reaches it before the supervisor exits.
func TestRunRollbackReachesSentry(t *testing.T) {
	r := newTestRoot(t)
	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_BODY": "A", "FAKE_ACTIVATE": "b"})
	r.build("b", knobs{"FAKE_NAME": "B", "FAKE_BOOT_EXIT": "1"})
	r.point("a")
	sink := newSentrySink(t)
	p := r.startFakeSupervisor(freeAddr(t), "SENTRY_DSN="+sink.dsn())

	r.trigger(r.waitEvent("A", "ready", 1))
	r.waitEvent("A", "ready", 2)
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if code := p.wait(t, 20*time.Second); code != 0 {
		t.Fatalf("supervisor exit code = %d, want 0", code)
	}
	got := sink.with("the new build did not become ready; rolling back")
	if len(got) != 1 || got[0].Level != "error" {
		t.Fatalf("rollback events = %+v, want one error", got)
	}
}

// Setting aside a backup the rollback could not restore leaves the data
// served migrated by the failed build: an operator must hear of it.
func TestRunUnrestoredBackupReachesSentry(t *testing.T) {
	r := newTestRoot(t)
	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_BODY": "A", "FAKE_ACTIVATE": "b"})
	r.build("b", knobs{"FAKE_NAME": "B", "FAKE_BOOT_EXIT": "1"})
	r.point("a")
	sink := newSentrySink(t)
	p := r.startFakeSupervisor(freeAddr(t), "SENTRY_DSN="+sink.dsn(), "SUPERVISE_TEST_FAIL_RESTORE=1")

	r.trigger(r.waitEvent("A", "ready", 1))
	r.waitEvent("A", "ready", 2)
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if code := p.wait(t, 20*time.Second); code != 0 {
		t.Fatalf("supervisor exit code = %d, want 0", code)
	}
	dir := filepath.Join(r.state().unrestoredDir(), "b")
	want := "the database backup from before build b could not be restored; it is kept in " + dir + " — the data now served was migrated by the failed build"
	got := sink.with(want)
	if len(got) != 1 || got[0].Level != "error" {
		t.Fatalf("set-aside events = %+v, want one error", got)
	}
}

// No build becoming ready ends the supervisor; the report must reach Sentry
// before the process exits.
func TestRunNoHealthyBuildReachesSentry(t *testing.T) {
	r := newTestRoot(t)
	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_BOOT_EXIT": "1"})
	r.build("b", knobs{"FAKE_NAME": "B", "FAKE_BOOT_EXIT": "1"})
	r.point("a")
	fakeActivate(r.dir, "a", "b", false)
	sink := newSentrySink(t)
	p := r.startFakeSupervisor(freeAddr(t), "SENTRY_DSN="+sink.dsn())

	if code := p.wait(t, 20*time.Second); code != 1 {
		t.Fatalf("supervisor exit code = %d, want 1", code)
	}
	got := sink.with("the supervisor is exiting")
	if len(got) != 1 {
		t.Fatalf("exit events = %+v, want one", got)
	}
	if detail := fmt.Sprint(got[0].Contexts["log"]); !strings.Contains(detail, errNoHealthyBuild.Error()) {
		t.Fatalf("exit event context = %s, want it to name %q", detail, errNoHealthyBuild)
	}
}
