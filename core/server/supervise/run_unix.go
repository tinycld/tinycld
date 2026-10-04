//go:build unix

package supervise

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"

	"tinycld.org/core/listeners"
	"tinycld.org/core/logging"
)

// errStopping means a stop signal arrived and every child has been drained.
var errStopping = errors.New("supervise: stopping on a signal")

// errNoHealthyBuild means the build rolled back to did not become ready
// either. Exiting lets Docker or systemd start everything again.
var errNoHealthyBuild = errors.New("supervise: no build became ready")

// Run is the whole supervisor. args are the serve arguments to give each
// child (the entrypoint's PB_SERVE_DIRS); the mode flags come from the
// environment, read through getenv. Children inherit the process
// environment. Returns the exit code.
//
// The supervisor binds the public ports once and passes them to each
// `tinycld serve` child, so a new build can start beside the old one and
// take over without a refused connection. It never opens the database.
func Run(args []string, getenv func(string) string) int {
	return runWith(args, getenv, defaultOptions())
}

// runWith is Run with the given options, so a test can run the whole
// supervisor in its own process with shorter bounds or another child user.
func runWith(args []string, getenv func(string) string, opts options) int {
	logging.Install(nil)
	initSentry(getenv)
	// Every exit path returns through here, so a report logged just before
	// the supervisor exits (no build became ready) still reaches Sentry.
	defer sentry.Flush(sentryFlushTimeout)
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigs)

	s, err := newSupervisor(args, getenv, opts, sigs)
	if err != nil {
		log.Error("the supervisor could not start", "err", err)
		return 1
	}
	return s.run()
}

// sentryFlushTimeout bounds how long an exiting supervisor waits for its
// reports to reach Sentry.
const sentryFlushTimeout = 5 * time.Second

// initSentry sends the supervisor's warnings and errors (a rollback, no
// build becoming ready) to Sentry. The server reads its DSN from its
// database, which the supervisor never opens, so the supervisor reads
// SENTRY_DSN from its environment instead: a DSN set only in the admin
// settings does not reach it.
func initSentry(getenv func(string) string) {
	dsn := strings.TrimSpace(getenv("SENTRY_DSN"))
	if dsn == "" {
		log.Info("SENTRY_DSN is not set; the supervisor reports to stderr only")
		return
	}
	logging.InitSentry(dsn)
}

type supervisor struct {
	cfg    config
	opts   options
	state  State
	getenv func(string) string
	sigs   <-chan os.Signal
	cred   *syscall.Credential
	env    []string
	ports  portPool
	// live is every child started, so a stop signal can drain all of
	// them, including both sides of a swap.
	live []*child
	// lastGoodBuild is the id of the last build whose child became ready.
	// A rollback compares the failed build with it: only a newer build
	// failing has a previous build to go back to.
	lastGoodBuild string
}

// newSupervisor reads the config and binds the ports the current build
// needs. A port that cannot be bound is fatal here: nothing is serving yet,
// and a child would only fail on the same port.
func newSupervisor(args []string, getenv func(string) string, opts options, sigs <-chan os.Signal) (*supervisor, error) {
	cfg, err := readConfig(args, getenv)
	if err != nil {
		return nil, err
	}
	cred, home, err := childCredential(opts.childUser)
	if err != nil {
		return nil, err
	}
	s := &supervisor{
		cfg:    cfg,
		opts:   opts,
		state:  State{Root: cfg.stateDir},
		getenv: getenv,
		sigs:   sigs,
		cred:   cred,
		env:    childEnv(os.Environ(), home),
		ports:  newPortPool(),
	}
	current, err := s.state.Current()
	if err != nil {
		return nil, fmt.Errorf("resolve the current build: %w", err)
	}
	if _, err := s.ports.prepare(s.wantPorts(current), s.inUse); err != nil {
		s.ports.closeAll()
		return nil, err
	}
	return s, nil
}

// wantPorts is the main listeners plus every enabled port the build at
// current declares. A build's ports.json is read each time a child of it
// starts, so a package added by a rebuild gets its port on the swap.
func (s *supervisor) wantPorts(current string) []portWant {
	want := append([]portWant(nil), s.cfg.base...)
	taken := map[string]bool{}
	for _, w := range want {
		taken[w.name] = true
	}
	ports, err := listeners.ReadPorts(filepath.Join(current, "server", "ports.json"))
	if err != nil {
		log.Error("could not read the build's package ports; starting it without them", "build", current, "err", err)
		return want
	}
	for _, p := range ports {
		addr, on := p.Resolve(s.getenv)
		if !on {
			continue
		}
		if taken[p.Name] {
			log.Warn("ignoring a package port whose name is already used", "slug", p.Slug, "name", p.Name)
			continue
		}
		taken[p.Name] = true
		want = append(want, portWant{p.Name, addr})
	}
	return want
}

func (s *supervisor) run() int {
	defer s.ports.closeAll()
	cur, err := s.boot()
	if err != nil {
		return s.exitFor(err)
	}
	return s.loop(cur)
}

func (s *supervisor) exitFor(err error) int {
	if errors.Is(err, errStopping) {
		return 0
	}
	log.Error("the supervisor is exiting", "err", err)
	return 1
}

// boot starts the first child. An armed backup on a fresh start means a
// rebuild was interrupted (the machine stopped) before anything decided
// whether its build works, so that build must prove itself first, as the
// entrypoint's recover_interrupted_rebuild did.
func (s *supervisor) boot() (*child, error) {
	s.remindUnrestored()
	if buildID, armed := s.state.BackupArmed(); armed {
		log.Warn("a rebuild was interrupted before its build was checked; checking it now", "build", buildID)
		// The build being checked has not served yet; the one before it
		// did, so a failure of the checked build is a new build failing.
		s.lastGoodBuild, _ = s.state.PreviousBuild()
		c, err := s.launch()
		if err != nil {
			log.Error("could not start the interrupted build; rolling back", "err", err)
			return s.rollback(nil, nil)
		}
		if err := s.awaitReady(c); err != nil {
			if errors.Is(err, errStopping) {
				return nil, err
			}
			log.Error("the interrupted build did not become ready; rolling back", "err", err)
			return s.rollback(nil, c)
		}
		s.served(c)
		if c.pendingRestart == nil {
			if err := s.state.CommitBackup(); err != nil {
				log.Error("could not commit the database backup", "err", err)
			}
		}
		s.ports.retain(c.ports)
		return c, nil
	}
	c, err := s.launch()
	if err != nil {
		return nil, err
	}
	// No rebuild is in flight, so this is the build that served before the
	// supervisor started; its ready is taken later, in loop.
	s.served(c)
	s.ports.retain(c.ports)
	return c, nil
}

// loop is the steady state: one child serving, waiting for it to ask for a
// restart or to exit.
func (s *supervisor) loop(cur *child) int {
	for {
		if m, ok := cur.takePendingRestart(); ok {
			next, err := s.restart(cur, m)
			if err != nil {
				return s.exitFor(err)
			}
			cur = next
			continue
		}
		select {
		case sig := <-s.sigs:
			log.Info("stopping", "signal", sig)
			s.shutdown()
			return 0
		case m := <-cur.msgs:
			if m.Type == MsgReady {
				log.Info("the server is ready", "pid", cur.pid)
				continue
			}
			next, err := s.restart(cur, m)
			if err != nil {
				return s.exitFor(err)
			}
			cur = next
		case <-cur.done:
			if cur.code != childRestartExitCode {
				log.Info("the server exited; exiting with its code", "pid", cur.pid, "code", cur.code)
				s.shutdown()
				return cur.code
			}
			log.Info("the server exited to ask for a restart", "pid", cur.pid)
			next, err := s.replace()
			if err != nil {
				return s.exitFor(err)
			}
			cur = next
		}
	}
}

// restart answers cur's restart message m with a swap, or with a drain and
// a fresh start when m asks for a cold restart.
func (s *supervisor) restart(cur *child, m Msg) (*child, error) {
	if m.Cold {
		log.Info("the server asked for a cold restart", "pid", cur.pid)
		drainChild(cur, s.opts.drainBound)
		return s.replace()
	}
	log.Info("the server asked to be replaced", "pid", cur.pid)
	return s.swap(cur)
}

// swap starts the activated build beside old, which keeps serving
// read-only, and drains old only once the new child is ready.
func (s *supervisor) swap(old *child) (*child, error) {
	next, err := s.launch()
	if err != nil {
		log.Error("could not start the new build; rolling back", "err", err)
		return s.rollback(old, nil)
	}
	if err := s.awaitReady(next); err != nil {
		if errors.Is(err, errStopping) {
			return nil, err
		}
		log.Error("the new build did not become ready; rolling back", "err", err)
		return s.rollback(old, next)
	}
	s.promote(next)
	drainChild(old, s.opts.drainBound)
	s.ports.retain(next.ports)
	return next, nil
}

// replace starts the activated build once the old child is gone: after a
// cold-restart drain, or after a child exited to ask for a restart.
func (s *supervisor) replace() (*child, error) {
	next, err := s.launch()
	if err != nil {
		log.Error("could not start the new build; rolling back", "err", err)
		return s.rollback(nil, nil)
	}
	if err := s.awaitReady(next); err != nil {
		if errors.Is(err, errStopping) {
			return nil, err
		}
		log.Error("the new build did not become ready; rolling back", "err", err)
		return s.rollback(nil, next)
	}
	s.promote(next)
	s.ports.retain(next.ports)
	return next, nil
}

// promote runs once a new build is ready. The web bundle it built is
// promoted before the old child drains, and the database backup is no
// longer needed because the migrated schema works.
//
// A child that asked for its own replacement before it was ready has
// already moved current on and armed a backup for the next build, so the
// bundle and the backup are that build's: they wait until it is ready.
func (s *supervisor) promote(next *child) {
	s.served(next)
	if next.pendingRestart != nil {
		log.Info("the new server already asked to be replaced; promoting waits for the next build", "pid", next.pid)
		return
	}
	if err := s.state.PromoteRelease(); err != nil {
		log.Error("could not promote the new build's web bundle; serving the previous one", "err", err)
	}
	if err := s.state.CommitBackup(); err != nil {
		log.Error("could not commit the database backup", "err", err)
	}
}

// rollback is the cold rollback: both children stop, the database and
// current go back to the previous build, and that build starts again. The
// order matters: the old binary must never start against the migrated
// database.
func (s *supervisor) rollback(old, failed *child) (*child, error) {
	if failed != nil {
		stopChild(failed, s.opts.stopBound)
	}
	if old != nil {
		drainChild(old, s.opts.drainBound)
	}
	failedBuild := s.currentBuildID()
	// The build that served failed (a cold restore with no rebuild): there
	// is no newer build to leave, and the previous build is older than the
	// data this one migrated, so it must not start on it.
	sameBuildFailed := failedBuild != "" && failedBuild == s.lastGoodBuild
	// Read before RollbackCurrent changes what the previous build is.
	rolledTo, prevErr := s.state.PreviousBuild()
	// A current that does not resolve names no build to record, but the
	// previous build is then the only one that can start.
	if !sameBuildFailed && failedBuild != "" {
		s.recordRollback(failedBuild, rolledTo, prevErr)
	}
	restoreErr := s.state.RestoreBackup()
	if sameBuildFailed {
		// No rebuild armed a backup, so a missing one is the expected case.
		if restoreErr != nil && !errors.Is(restoreErr, os.ErrNotExist) {
			log.Warn("could not restore the database backup; starting the serving build again on the data it has", "err", restoreErr)
		}
		log.Error("the serving build failed to restart; starting it again", "build", failedBuild)
	} else {
		if restoreErr != nil {
			s.setAsideUnrestored(rolledTo, restoreErr)
		}
		if err := s.state.RollbackCurrent(); err != nil {
			log.Error("could not roll the build back; starting the current build again", "err", err)
		}
	}
	// A backup still armed here is one RestoreBackup could not restore and
	// the set-aside could not move. It is the only copy of the database from
	// before the migration, so nothing below may drop it.
	_, keptArmed := s.state.BackupArmed()

	c, err := s.launch()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errNoHealthyBuild, err)
	}
	if err := s.awaitReady(c); err != nil {
		if errors.Is(err, errStopping) {
			return nil, err
		}
		stopChild(c, s.opts.stopBound)
		return nil, fmt.Errorf("%w: the build rolled back to: %v", errNoHealthyBuild, err)
	}
	s.served(c)
	// The build rolled back to may have asked for its replacement before it
	// was ready, so its own ready never promoted its bundle and it would
	// serve an older build's.
	if err := s.state.PromoteReleaseIfNewer(c.dir); err != nil {
		log.Error("could not promote the rolled-back build's web bundle; serving the previous one", "err", err)
	}
	if !keptArmed {
		s.dropReturnedBackup(failedBuild, c)
	}
	s.ports.retain(c.ports)
	return c, nil
}

// recordRollback leaves the next boot a note that failedBuild was rolled
// back to rolledTo (prevErr is why there is none). The record is only a
// note for the install log; failing to write it must not stop the rollback.
func (s *supervisor) recordRollback(failedBuild, rolledTo string, prevErr error) {
	if prevErr != nil {
		log.Warn("the rollback record names no build to roll back to", "err", prevErr)
	}
	r := RollbackRecord{Build: failedBuild, RolledTo: rolledTo, At: time.Now().UTC()}
	if err := s.state.WriteRollbackRecord(r); err != nil {
		log.Error("could not record the rollback for the next boot", "build", failedBuild, "err", err)
	}
}

// setAsideUnrestored moves a backup RestoreBackup could not restore out of
// every armed path. Left armed for the failed build, the next rebuild would
// replace it, a healthy restart would commit it and an unhealthy one would
// restore it over every write made since; set aside, only an operator
// restores or removes it. A set-aside that fails leaves it armed.
func (s *supervisor) setAsideUnrestored(rolledTo string, restoreErr error) {
	build, armed := s.state.BackupArmed()
	if !armed {
		log.Warn("could not restore the database backup; rolling the build back anyway (the schema may be ahead of the previous build)", "err", restoreErr)
		return
	}
	note := UnrestoredNote{Build: build, RolledTo: rolledTo, At: time.Now().UTC(), RestoreError: restoreErr.Error()}
	if err := s.state.SetAsideUnrestored(note); err != nil {
		log.Error("could not restore the database backup or set it aside; it stays armed for the failed build (the schema may be ahead of the previous build)", "build", build, "restoreErr", restoreErr, "err", err)
		return
	}
	dir := filepath.Join(s.state.unrestoredDir(), build)
	// The build and dir are in the message: it is what an operator reads,
	// and the event is rare enough that a Sentry issue per build is wanted.
	log.Error(fmt.Sprintf("the database backup from before build %s could not be restored; it is kept in %s — the data now served was migrated by the failed build", build, dir), "build", build, "rolledTo", rolledTo, "err", restoreErr)
}

// unrestoredReminder is the message each supervisor start logs while a
// backup a rollback could not restore is kept.
const unrestoredReminder = "a database backup a rollback could not restore is kept; only an operator restores or removes it"

// remindUnrestored logs, once per start, every backup a rollback set aside,
// so the reminder reaches Sentry until an operator deals with them.
func (s *supervisor) remindUnrestored() {
	notes, err := s.state.Unrestored()
	if err != nil {
		log.Error("could not read every kept unrestored backup", "err", err)
	}
	if len(notes) == 0 {
		return
	}
	builds := make([]string, 0, len(notes))
	dirs := make([]string, 0, len(notes))
	for _, n := range notes {
		builds = append(builds, n.Build)
		dirs = append(dirs, filepath.Join(s.state.unrestoredDir(), n.Build))
	}
	log.Error(unrestoredReminder, "builds", builds, "dirs", dirs)
}

// served notes that c's build became ready (or, at boot, is the build that
// was serving), so a later failure of the same build is not taken for a new
// build's.
func (s *supervisor) served(c *child) {
	s.lastGoodBuild = buildIDOf(c.dir)
}

// dropReturnedBackup removes a backup armed for the build just rolled back
// from that the rolled-back child's boot brought back. The caller calls it
// only when no backup was armed once the rollback's restore step ran, so a
// backup armed now appeared during that boot: a restore swapped in by the
// failed build's boot moved pb_data aside with the armed backup in it, so
// the restore step found nothing, and the child's boot undid the swap and
// returned both. The child serves that data now. Left armed, the backup
// would be restored over every write since by the next rollback or
// interrupted-rebuild check.
//
// A child that asked for its own replacement before it was ready armed a
// backup for its next build, which must be kept.
func (s *supervisor) dropReturnedBackup(failedBuild string, c *child) {
	armedFor, armed := s.state.BackupArmed()
	if !armed || failedBuild == "" || armedFor != failedBuild || c.pendingRestart != nil {
		return
	}
	log.Warn("the rolled-back build's boot put back data that carries a backup armed for the failed build; dropping it", "build", failedBuild)
	if err := s.state.CommitBackup(); err != nil {
		log.Error("could not drop the backup the rolled-back build's boot put back", "err", err)
	}
}

// currentBuildID is the id of the build current points at, or "" when
// current does not resolve.
func (s *supervisor) currentBuildID() string {
	cur, err := s.state.Current()
	if err != nil {
		return ""
	}
	return buildIDOf(cur)
}

// buildIDOf is the id of the build whose tinycld dir is dir
// (<Root>/builds/<id>/tinycld).
func buildIDOf(dir string) string {
	return filepath.Base(filepath.Dir(dir))
}

// launch starts a child of the build current points at now. Ports the
// build declares that are not held yet are bound; one that fails to bind
// is logged and left for the child to bind itself.
func (s *supervisor) launch() (*child, error) {
	current, err := s.state.Current()
	if err != nil {
		return nil, fmt.Errorf("resolve the current build: %w", err)
	}
	ports, err := s.ports.prepare(s.wantPorts(current), s.inUse)
	if err != nil {
		log.Error("could not bind a port the build declares", "err", err)
	}
	c, err := startChild(childSpec{dir: current, args: s.cfg.childArgs, env: s.env, ports: ports, cred: s.cred})
	if err != nil {
		return nil, fmt.Errorf("start %s: %w", current, err)
	}
	s.track(c)
	log.Info("started a server", "build", current, "pid", c.pid)
	return c, nil
}

// inUse reports whether a running child serves on l.
func (s *supervisor) inUse(l *net.TCPListener) bool {
	for _, c := range s.live {
		if c.exited() {
			continue
		}
		for _, p := range c.ports {
			if p.l == l {
				return true
			}
		}
	}
	return false
}

func (s *supervisor) track(c *child) {
	live := s.live[:0]
	for _, l := range s.live {
		if !l.exited() {
			live = append(live, l)
		}
	}
	s.live = append(live, c)
}

// awaitReady waits for c's ready message. A stop signal while waiting
// drains every child and returns errStopping.
func (s *supervisor) awaitReady(c *child) error {
	timer := time.NewTimer(s.opts.readyTimeout)
	defer timer.Stop()
	for {
		select {
		case m := <-c.msgs:
			if m.Type == MsgReady {
				log.Info("the server is ready", "pid", c.pid)
				return nil
			}
			if m.Type == MsgRestart {
				log.Info("a server asked for a restart before it was ready; acting on it once it is", "pid", c.pid)
				c.pendingRestart = &m
				continue
			}
			log.Warn("ignoring a message from a server that is not ready", "pid", c.pid, "type", m.Type)
		case <-c.done:
			return fmt.Errorf("the server exited with code %d before it was ready", c.code)
		case <-timer.C:
			return fmt.Errorf("the server was not ready within %s", s.opts.readyTimeout)
		case sig := <-s.sigs:
			log.Info("stopping", "signal", sig)
			s.shutdown()
			return errStopping
		}
	}
}

// shutdown drains every running child at once, so a stop during a swap
// takes no longer than one drain.
func (s *supervisor) shutdown() {
	var wg sync.WaitGroup
	for _, c := range s.live {
		if c.exited() {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			drainChild(c, s.opts.drainBound)
		}()
	}
	wg.Wait()
}
