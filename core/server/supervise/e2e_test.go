//go:build unix

package supervise

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"tinycld.org/core/listeners"
)

// The E2E tests run the supervisor in its own process, as `tinycld
// supervise` runs, and the real server binary as its children. A swap is
// started the way an operator starts one: a revert to a retained build
// through the admin API, which backs up the database, flips current, arms
// the backup and asks the supervisor for a restart from inside the server.

const (
	e2eBuildA = "build-1"
	e2eBuildB = "build-2"

	e2eMarkPath          = "/e2e-build"
	e2eSuperuserEmail    = "e2e@example.test"
	e2eSuperuserPassword = "e2e-password-1234"

	// e2eSlowBoot is how long the swap test's new build takes to boot at
	// least, as a build that runs migrations does.
	e2eSlowBoot = 3 * time.Second
)

// supervisorRole is the test binary's supervisor role. It is Run, except
// that children run as the user running the tests (so a root run needs no
// "tinycld" user) and a test may shorten the ready timeout.
func supervisorRole() int {
	var args []string
	if a := os.Getenv("SUPERVISE_TEST_ARGS"); a != "" {
		args = strings.Split(a, "\n")
	}
	opts := defaultOptions()
	opts.childUser = testChildUser()
	if d := os.Getenv("SUPERVISE_TEST_READY_TIMEOUT"); d != "" {
		readyTimeout, err := time.ParseDuration(d)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bad SUPERVISE_TEST_READY_TIMEOUT:", err)
			return 2
		}
		opts.readyTimeout = readyTimeout
	}
	return runWith(args, os.Getenv, opts)
}

var serverBuild struct {
	once sync.Once
	dir  string
	bin  string
	err  error
}

// serverBinary builds server/ once per run of the test binary.
//
// The build links core and no feature package: the generated
// package_extensions.go is replaced, through -overlay, by the form the
// generator writes for a workspace with no features, and a go.work in the
// temp dir lists only the app and core. A feature package binds its own
// fixed ports (and needs its own config) unless its build declares them, so
// linking whatever this workspace has installed would make the test depend
// on that set. Nothing under server/ is written.
func serverBinary(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds and runs the real server binary")
	}
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Fatal("the server backs up its database with the sqlite3 command, which is not on PATH")
	}
	serverBuild.once.Do(func() { serverBuild.bin, serverBuild.err = buildServer() })
	if serverBuild.err != nil {
		t.Fatal(serverBuild.err)
	}
	return serverBuild.bin
}

func buildServer() (string, error) {
	serverDir, err := filepath.Abs(filepath.Join("..", "..", "..", "server"))
	if err != nil {
		return "", err
	}
	coreDir, err := filepath.Abs("..")
	if err != nil {
		return "", err
	}
	goMod, err := os.ReadFile(filepath.Join(serverDir, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("find the server module: %w", err)
	}
	goLine := regexp.MustCompile(`(?m)^go \S+$`).Find(goMod)
	if goLine == nil {
		return "", fmt.Errorf("no go line in %s", filepath.Join(serverDir, "go.mod"))
	}

	dir, err := os.MkdirTemp("", "supervise-e2e-")
	if err != nil {
		return "", err
	}
	serverBuild.dir = dir
	ext := filepath.Join(dir, "package_extensions.go")
	if err := os.WriteFile(ext, []byte(`package main

import "github.com/pocketbase/pocketbase"

func registerPackageExtensions(_ *pocketbase.PocketBase) {}
`), 0o644); err != nil {
		return "", err
	}
	overlay, err := json.Marshal(map[string]map[string]string{
		"Replace": {filepath.Join(serverDir, "package_extensions.go"): ext},
	})
	if err != nil {
		return "", err
	}
	overlayPath := filepath.Join(dir, "overlay.json")
	if err := os.WriteFile(overlayPath, overlay, 0o644); err != nil {
		return "", err
	}
	work := fmt.Sprintf("%s\n\nuse (\n\t%s\n\t%s\n\t%s\n)\n",
		goLine, serverDir, coreDir, filepath.Join(coreDir, "backup", "format"))
	workPath := filepath.Join(dir, "go.work")
	if err := os.WriteFile(workPath, []byte(work), 0o644); err != nil {
		return "", err
	}

	bin := filepath.Join(dir, "tinycld")
	cmd := exec.Command("go", "build", "-overlay="+overlayPath, "-o", bin, ".")
	cmd.Dir = serverDir
	cmd.Env = append(os.Environ(), "GOWORK="+workPath, "GOFLAGS=-mod=readonly")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build the server binary: %v\n%s", err, out)
	}
	return bin, nil
}

// linkFile is os.Link behind a seam, so a test can make a link fail.
var linkFile = os.Link

// linkOrCopy hard-links src at dst, or copies it, mode and all, when no link
// can be made: the server binary is built in one temp dir and each test's
// root is another, and a link needs both on one filesystem.
func linkOrCopy(src, dst string) error {
	if err := linkFile(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	// Chmod, because the mode OpenFile gives is cut by the umask.
	if err := out.Chmod(info.Mode().Perm()); err != nil {
		out.Close()
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func removeServerBuild() {
	if serverBuild.dir != "" {
		os.RemoveAll(serverBuild.dir)
	}
}

// --- fixtures ---

func newServerRoot(t *testing.T) *testRoot {
	t.Helper()
	r := &testRoot{t: t, dir: t.TempDir()}
	for _, d := range []string{"pb_data", "tmp"} {
		if err := os.MkdirAll(filepath.Join(r.dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

// serverBuild lays out builds/<id> as a rebuild leaves it: the binary,
// core's migrations under server/pb_migrations, a hooks dir with one hook
// file, and the build's manifest.json, which a revert reads.
func (r *testRoot) serverBuild(id, bin, hook string) {
	r.t.Helper()
	dir := filepath.Join(r.dir, "builds", id, "tinycld")
	migrations := filepath.Join(dir, "server", "pb_migrations")
	hooks := filepath.Join(dir, "server", "pb_hooks")
	for _, d := range []string{migrations, hooks} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			r.t.Fatal(err)
		}
	}
	if err := linkOrCopy(bin, filepath.Join(dir, "tinycld")); err != nil {
		r.t.Fatal(err)
	}
	src, err := filepath.Glob(filepath.Join("..", "pb_migrations", "*.js"))
	if err != nil || len(src) == 0 {
		r.t.Fatalf("core migrations: %v (found %d)", err, len(src))
	}
	for _, f := range src {
		data, err := os.ReadFile(f)
		if err != nil {
			r.t.Fatal(err)
		}
		mustWrite(r.t, filepath.Join(migrations, filepath.Base(f)), string(data))
	}
	mustWrite(r.t, filepath.Join(hooks, "main.pb.js"), hook)
	mustWrite(r.t, filepath.Join(r.dir, "builds", id, "manifest.json"), "{}")
}

// markHook answers the mark route with the build's name, so a response
// shows which build served it.
func markHook(mark string) string {
	return fmt.Sprintf("routerAdd('GET', '%s', (e) => e.string(200, '%s'))\n", e2eMarkPath, mark)
}

// serveArgs are the entrypoint's serve dirs plus the hooks dir. The
// migrations and hooks resolve through current when a child starts, so each
// child loads its own build's files.
func (r *testRoot) serveArgs() []string {
	s := r.state()
	cur := s.currentLinkPath()
	return []string{
		"--dir=" + s.pbDataDir(),
		"--releasesDir=" + s.releasesDir(),
		"--websiteDir=" + filepath.Join(r.dir, "website"),
		"--migrationsDir=" + filepath.Join(cur, "server", "pb_migrations"),
		"--hooksDir=" + filepath.Join(cur, "server", "pb_hooks"),
	}
}

// serverEnv is the environment a real server child needs here. The server
// takes a binary under the temp dir for a `go run` dev binary, which never
// restarts, so TMPDIR points elsewhere. Auto-upgrade is off so no child
// reaches the network.
func (r *testRoot) serverEnv() []string {
	return []string{
		"TINYCLD_STATE_DIR=" + r.dir,
		"TMPDIR=" + filepath.Join(r.dir, "tmp"),
		"TINYCLD_AUTOUPGRADE_DISABLED=1",
	}
}

// createSuperuser runs the real binary's CLI against the empty pb_data, as
// an operator would; it also applies the migrations.
func (r *testRoot) createSuperuser(id string) {
	r.t.Helper()
	dir := filepath.Join(r.dir, "builds", id, "tinycld")
	args := append([]string{"superuser", "upsert", e2eSuperuserEmail, e2eSuperuserPassword}, r.serveArgs()...)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(dir, "tinycld"), args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), r.serverEnv()...)
	cmd.WaitDelay = 5 * time.Second
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("superuser upsert: %v\n%s", err, out)
	}
}

// --- the supervisor process ---

type supervisorProc struct {
	t       *testing.T
	cmd     *exec.Cmd
	addr    string
	logPath string
	exited  chan struct{}
	waitErr error
}

// startSupervisor runs the test binary as the supervisor in its own
// process, in plain mode on a free loopback port. Its output and its
// children's go to <root>/supervisor.log. Cleanup stops it with SIGTERM,
// fails the test if it does not exit 0 or leaves a child running, and
// prints the log when the test failed.
func (r *testRoot) startSupervisor(env, args []string) *supervisorProc {
	r.t.Helper()
	bin, err := os.Executable()
	if err != nil {
		r.t.Fatal(err)
	}
	p := &supervisorProc{
		t:       r.t,
		addr:    freeAddr(r.t),
		logPath: filepath.Join(r.dir, "supervisor.log"),
		exited:  make(chan struct{}),
	}
	logFile, err := os.OpenFile(p.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		r.t.Fatal(err)
	}
	defer logFile.Close()

	p.cmd = exec.Command(bin, "-test.run=^$")
	p.cmd.Env = append(os.Environ(),
		"SUPERVISE_TEST_ROLE=supervisor",
		"SUPERVISE_TEST_ARGS="+strings.Join(args, "\n"),
		"TINYCLD_STATE_DIR="+r.dir,
		"HTTP_ADDR="+p.addr,
		"AUTOCERT_ENABLED=",
	)
	p.cmd.Env = append(p.cmd.Env, env...)
	p.cmd.Stdout, p.cmd.Stderr = logFile, logFile
	if err := p.cmd.Start(); err != nil {
		r.t.Fatal(err)
	}
	go func() {
		p.waitErr = p.cmd.Wait()
		close(p.exited)
	}()
	r.t.Cleanup(p.stop)
	return p
}

func (p *supervisorProc) stop() {
	t := p.t
	defer func() {
		if t.Failed() {
			t.Logf("supervisor log (last 150 lines):\n%s", p.logTail(150))
		}
	}()
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Errorf("signal the supervisor: %v", err)
	}
	opts := defaultOptions()
	select {
	case <-p.exited:
		if p.waitErr != nil {
			t.Errorf("the supervisor exited with %v after SIGTERM, want 0", p.waitErr)
		}
	case <-time.After(opts.drainBound + 2*opts.stopBound + 10*time.Second):
		t.Error("the supervisor did not exit after SIGTERM")
		p.cmd.Process.Kill()
		<-p.exited
	}
	for _, c := range p.children() {
		if alive(c.pid) {
			t.Errorf("the supervisor left child %d (%s) running", c.pid, c.build)
			syscall.Kill(-c.pid, syscall.SIGKILL)
		}
	}
}

func (p *supervisorProc) log() string {
	data, _ := os.ReadFile(p.logPath)
	return string(data)
}

func (p *supervisorProc) logTail(n int) string {
	lines := strings.Split(strings.TrimRight(p.log(), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

type startedChild struct {
	build string // the build id
	pid   int
}

var startedLine = regexp.MustCompile(`msg="started a server" pkg=supervise build=("[^"]*"|\S+) pid=(\d+)`)

// children is every child the supervisor has started, in order, read from
// its log.
func (p *supervisorProc) children() []startedChild {
	var out []startedChild
	for _, m := range startedLine.FindAllStringSubmatch(p.log(), -1) {
		dir := m[1]
		if u, err := strconv.Unquote(dir); err == nil {
			dir = u
		}
		pid, _ := strconv.Atoi(m[2])
		out = append(out, startedChild{build: filepath.Base(filepath.Dir(dir)), pid: pid})
	}
	return out
}

// child returns the nth (1-based) child started from build.
func (p *supervisorProc) child(build string, nth int) (startedChild, bool) {
	seen := 0
	for _, c := range p.children() {
		if c.build == build {
			seen++
			if seen == nth {
				return c, true
			}
		}
	}
	return startedChild{}, false
}

func (p *supervisorProc) mustChild(build string, nth int) startedChild {
	p.t.Helper()
	c, ok := p.child(build, nth)
	if !ok {
		p.t.Fatalf("no child %d of %s was started", nth, build)
	}
	return c
}

// alive reports whether pid is a running process. The supervisor reaps its
// children, so an exited child does not linger as a zombie.
func alive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

// --- the admin API ---

var apiClient = &http.Client{Timeout: 10 * time.Second}

func (p *supervisorProc) api(method, path, token string, body any, wantStatus int) []byte {
	p.t.Helper()
	var in io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			p.t.Fatal(err)
		}
		in = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, "http://"+p.addr+path, in)
	if err != nil {
		p.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	resp, err := apiClient.Do(req)
	if err != nil {
		p.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		p.t.Fatal(err)
	}
	if resp.StatusCode != wantStatus {
		p.t.Fatalf("%s %s: status %d, want %d: %s", method, path, resp.StatusCode, wantStatus, out)
	}
	return out
}

func (p *supervisorProc) superuserToken() string {
	p.t.Helper()
	out := p.api(http.MethodPost, "/api/collections/_superusers/auth-with-password", "",
		map[string]string{"identity": e2eSuperuserEmail, "password": e2eSuperuserPassword}, http.StatusOK)
	var auth struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(out, &auth); err != nil || auth.Token == "" {
		p.t.Fatalf("superuser auth: %v: %s", err, out)
	}
	return auth.Token
}

// revert asks the server to go back to a retained build: the same job a
// package operation ends with, without a build step.
func (p *supervisorProc) revert(token, build string) {
	p.t.Helper()
	p.api(http.MethodPost, "/api/admin/packages/revert", token, map[string]string{"buildId": build}, http.StatusAccepted)
}

type installLogRow struct {
	Status string `json:"status"`
	Error  string `json:"error"`
}

// revertLog returns the newest revert row of the install log.
func (p *supervisorProc) revertLog(token string) installLogRow {
	p.t.Helper()
	q := url.Values{"filter": {"action='revert'"}, "sort": {"-created"}, "perPage": {"1"}}
	out := p.api(http.MethodGet, "/api/collections/pkg_install_log/records?"+q.Encode(), token, nil, http.StatusOK)
	var list struct {
		Items []installLogRow `json:"items"`
	}
	if err := json.Unmarshal(out, &list); err != nil || len(list.Items) != 1 {
		p.t.Fatalf("install log: %v: %s", err, out)
	}
	return list.Items[0]
}

// sqlite runs one statement list on the database with the sqlite3 command,
// beside the running servers (it waits on their locks), and returns its
// trimmed output.
func sqlite(t *testing.T, db, sql string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "sqlite3", "-cmd", ".timeout 10000", db, sql).CombinedOutput()
	if err != nil {
		t.Fatalf("sqlite3 %q: %v\n%s", sql, err, out)
	}
	return strings.TrimSpace(string(out))
}

func waitURLBody(t *testing.T, limit time.Duration, url, want string) {
	t.Helper()
	waitFor(t, limit, fmt.Sprintf("%s to answer %q", url, want), func() bool {
		got, err := fetch(url)
		return err == nil && got == want
	})
}

// assertSwitched checks that the load saw only want after the old server
// exited. The one request in flight when the exit was seen may still carry
// the old answer.
func assertSwitched(t *testing.T, ld *load, atExit int, from, want string) {
	t.Helper()
	if ld.refused != 0 || len(ld.failed) != 0 {
		t.Fatalf("refused connections = %d, failed requests = %d (%v)", ld.refused, len(ld.failed), ld.failed)
	}
	if ld.bodies[0] != from {
		t.Fatalf("the first answer was %q, want %q", ld.bodies[0], from)
	}
	for i, b := range ld.bodies[atExit+1:] {
		if b != want {
			t.Fatalf("answer %d after the old server exited was %q, want %q", atExit+1+i, b, want)
		}
	}
}

// --- tests ---

func TestE2ESwapUnderLoad(t *testing.T) {
	bin := serverBinary(t)
	r := newServerRoot(t)
	r.serverBuild(e2eBuildA, bin, markHook("A"))
	// A supervisor that let A go before B is ready would leave the port
	// with no server for as long as B boots. The supervisor still holds the
	// socket, so the load would see that as one request stalled in the
	// listen backlog, not as a refused connection.
	r.serverBuild(e2eBuildB, bin, fmt.Sprintf("sleep(%d)\n", e2eSlowBoot.Milliseconds())+markHook("B"))
	r.point(e2eBuildA)
	r.createSuperuser(e2eBuildA)
	p := r.startSupervisor(r.serverEnv(), r.serveArgs())
	mark := "http://" + p.addr + e2eMarkPath

	waitURLBody(t, 60*time.Second, mark, "A")
	a := p.mustChild(e2eBuildA, 1)
	ld := startLoadAt(mark)
	waitFor(t, 10*time.Second, "load on A", func() bool { return ld.count() >= 10 })

	token := p.superuserToken()
	p.revert(token, e2eBuildB)
	waitFor(t, 60*time.Second, "A to exit", func() bool { return !alive(a.pid) })
	atExit := ld.count()
	waitFor(t, 10*time.Second, "load on B", func() bool { return ld.count() >= atExit+20 })
	ld.end()

	assertSwitched(t, ld, atExit, "A", "B")
	t.Logf("%d requests, slowest %s", len(ld.bodies), ld.slowest)
	if ld.slowest >= e2eSlowBoot/2 {
		t.Fatalf("the slowest request took %s: nothing served while B booted", ld.slowest)
	}
	b := p.mustChild(e2eBuildB, 1)
	if !alive(b.pid) {
		t.Fatalf("B (pid %d) is not running", b.pid)
	}
	if n := len(p.children()); n != 2 {
		t.Fatalf("the supervisor started %d children, want 2 (no rollback)", n)
	}
	s := r.state()
	assertExists(t, s.dbArmedMarkerPath(), false)
	assertExists(t, s.dbBackupPath(), false)
	assertExists(t, s.rollbackPendingMarkerPath(), false)
	if got, want := readLink(t, s.currentLinkPath()), filepath.Join(s.buildsDir(), e2eBuildB, "tinycld"); got != want {
		t.Fatalf("current -> %q, want %q", got, want)
	}
	if row := p.revertLog(token); row.Status != "success" {
		t.Fatalf("the revert's install log row is %q (%s), want success", row.Status, row.Error)
	}
}

func TestE2EBrokenBuildRollsBack(t *testing.T) {
	bin := serverBinary(t)
	r := newServerRoot(t)
	r.serverBuild(e2eBuildA, bin, markHook("A"))
	// B fails only after a while, which leaves the test time to change the
	// database once the backup is armed and before the rollback starts.
	r.serverBuild(e2eBuildB, bin, "sleep(3000)\nthrow new Error('this build does not start')\n")
	r.point(e2eBuildA)
	r.createSuperuser(e2eBuildA)
	db := r.state().dbPath()
	sqlite(t, db, "CREATE TABLE e2e_marks (name TEXT); INSERT INTO e2e_marks VALUES ('before-backup');")
	readyTimeout := 20 * time.Second
	env := append(r.serverEnv(), "SUPERVISE_TEST_READY_TIMEOUT="+readyTimeout.String())
	p := r.startSupervisor(env, r.serveArgs())
	mark := "http://" + p.addr + e2eMarkPath

	waitURLBody(t, 60*time.Second, mark, "A")
	a := p.mustChild(e2eBuildA, 1)
	ld := startLoadAt(mark)
	waitFor(t, 10*time.Second, "load on A", func() bool { return ld.count() >= 10 })

	token := p.superuserToken()
	revertAt := time.Now()
	p.revert(token, e2eBuildB)
	// The armed marker is written after the backup and before the restart;
	// B then boots for 3 s before it fails, so this write lands in data.db
	// after the backup and before the rollback.
	waitFor(t, 30*time.Second, "the backup to be armed", func() bool {
		_, armed := r.state().BackupArmed()
		return armed
	})
	sqlite(t, db, "INSERT INTO e2e_marks VALUES ('after-backup');")
	// The rollback starts only once B has exited, so B still running after
	// the write means the write came before it.
	waitFor(t, 10*time.Second, "B to start", func() bool {
		_, ok := p.child(e2eBuildB, 1)
		return ok
	})
	if b := p.mustChild(e2eBuildB, 1); !alive(b.pid) {
		t.Fatal("B was not running when the test wrote after the backup; the write may have missed the window")
	}
	// The rollback is cold: the old server stops before the previous build
	// starts again, so a short outage is allowed. It must end within the
	// ready timeout plus a boot.
	waitFor(t, readyTimeout+30*time.Second, "the previous build to serve again", func() bool {
		if _, ok := p.child(e2eBuildA, 2); !ok || alive(a.pid) {
			return false
		}
		got, err := fetch(mark)
		return err == nil && got == "A"
	})
	recoveredAt := time.Now()
	atRecovery := ld.count()
	waitFor(t, 10*time.Second, "load on the restarted build", func() bool { return ld.count() >= atRecovery+20 })
	ld.end()
	t.Logf("recovered %s after the revert; the outage refused %d and failed %d requests; slowest request %s",
		recoveredAt.Sub(revertAt).Round(time.Millisecond), ld.refused, len(ld.failed), ld.slowest)

	if ld.lastFailedAt.After(recoveredAt) {
		t.Fatalf("a request failed after the previous build served again: %v", ld.failed)
	}
	for i, body := range ld.bodies {
		if body != "A" {
			t.Fatalf("answer %d was %q: the broken build served", i, body)
		}
	}
	b := p.mustChild(e2eBuildB, 1)
	again := p.mustChild(e2eBuildA, 2)
	if alive(b.pid) || !alive(again.pid) {
		t.Fatalf("B (pid %d) alive = %v, restarted A (pid %d) alive = %v; want only the restarted A",
			b.pid, alive(b.pid), again.pid, alive(again.pid))
	}
	s := r.state()
	if got, want := readLink(t, s.currentLinkPath()), filepath.Join(s.buildsDir(), e2eBuildA, "tinycld"); got != want {
		t.Fatalf("current -> %q, want %q", got, want)
	}
	// The row written before the backup survives and the row written after
	// it is gone: data.db is the backup.
	if got := sqlite(t, db, "SELECT group_concat(name, ',') FROM e2e_marks;"); got != "before-backup" {
		t.Fatalf("e2e_marks after the rollback = %q, want only before-backup", got)
	}
	assertExists(t, s.dbArmedMarkerPath(), false)
	assertExists(t, s.dbBackupPath(), false)
	// The backup was taken while the revert's install log row was still
	// running; the live database had it as success when the server asked
	// for the restart. So the row is the backup's only if the database was
	// restored, and the restarted server marks it rolled back, naming the
	// failed build, only from the .rollback-pending file the rollback
	// writes (which it then removes).
	row := p.revertLog(p.superuserToken())
	if row.Status != "rolled_back" || !strings.Contains(row.Error, "(build "+e2eBuildB+")") {
		t.Fatalf("the revert's install log row is %q (%q), want rolled_back naming %s", row.Status, row.Error, e2eBuildB)
	}
	assertExists(t, s.rollbackPendingMarkerPath(), false)
}

// A package port declared in a build's ports.json is held by the
// supervisor and served by each child through listeners.Inherited, with no
// refused connection across a swap. The children are the fake server
// (fakeChild): the server binary these tests build links no package that
// serves a port.
func TestE2EPackagePortAcrossSwap(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the supervisor in its own process")
	}
	r := newTestRoot(t)
	extra := freeAddr(t)
	port := acmePort("ACME_EXTRA_ADDR")
	r.build("a", knobs{"FAKE_NAME": "A", "FAKE_SERVE_BODY": "A", "FAKE_ACTIVATE": "b"})
	r.writePorts("a", []listeners.Port{port})
	r.build("b", knobs{"FAKE_NAME": "B", "FAKE_SERVE_BODY": "B", "FAKE_READY_DELAY": "300"})
	r.writePorts("b", []listeners.Port{port})
	r.point("a")
	r.startSupervisor([]string{"ACME_ENABLED=true", "ACME_EXTRA_ADDR=" + extra}, nil)

	a := r.waitEvent("A", "ready", 1)
	if got := r.eventWith("A", "listeners", 1); got != "listeners http,acme-extra" {
		t.Fatalf("A got %q", got)
	}
	waitBody(t, extra, "A")
	ld := startLoad(extra)
	waitFor(t, 10*time.Second, "load on A's package port", func() bool { return ld.count() >= 10 })

	r.trigger(a)
	r.waitEvent("A", "exit", 1)
	atExit := ld.count()
	waitFor(t, 10*time.Second, "load on B's package port", func() bool { return ld.count() >= atExit+20 })
	ld.end()

	assertSwitched(t, ld, atExit, "A", "B")
	if got := r.eventWith("B", "listeners", 1); got != "listeners http,acme-extra" {
		t.Fatalf("B got %q", got)
	}
}

// The server binary and a test's root can sit on two filesystems, where a
// hard link fails; the binary must then be copied, still executable.
func TestLinkOrCopyCopiesWhenALinkFails(t *testing.T) {
	prev := linkFile
	linkFile = func(old, new string) error {
		return &os.LinkError{Op: "link", Old: old, New: new, Err: syscall.EXDEV}
	}
	t.Cleanup(func() { linkFile = prev })
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "built"), filepath.Join(dir, "tinycld")
	if err := os.WriteFile(src, []byte("#!/bin/sh\necho built\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := linkOrCopy(src, dst); err != nil {
		t.Fatalf("linkOrCopy: %v", err)
	}
	if got := mustRead(t, dst); got != "#!/bin/sh\necho built\n" {
		t.Fatalf("copy = %q", got)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("copy mode = %v, want 0755", info.Mode().Perm())
	}
}
