//go:build unix

package supervise

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

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
	logging.Install(nil)
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigs)

	s, err := newSupervisor(args, getenv, defaultOptions(), sigs)
	if err != nil {
		log.Error("the supervisor could not start", "err", err)
		return 1
	}
	return s.run()
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
}

// newSupervisor reads the config and binds the ports the current build
// needs. A port that cannot be bound is fatal here: nothing is serving yet,
// and a child would only fail on the same port.
func newSupervisor(args []string, getenv func(string) string, opts options, sigs <-chan os.Signal) (*supervisor, error) {
	cfg, err := readConfig(args, getenv)
	if err != nil {
		return nil, err
	}
	cred, home, err := childCredential()
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
		ports:  portPool{held: map[string]*net.TCPListener{}},
	}
	current, err := s.state.Current()
	if err != nil {
		return nil, fmt.Errorf("resolve the current build: %w", err)
	}
	if _, err := s.ports.prepare(s.wantPorts(current)); err != nil {
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
	if buildID, armed := s.state.BackupArmed(); armed {
		log.Warn("a rebuild was interrupted before its build was checked; checking it now", "build", buildID)
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
	// The rollback record is only a note for the next boot's install log;
	// failing to write it must not stop the rollback.
	if err := s.state.WriteRollbackPending(); err != nil {
		log.Error("could not record the rollback for the next boot", "err", err)
	}
	if err := s.state.RestoreBackup(); err != nil {
		log.Warn("could not restore the database backup; rolling the build back anyway (the schema may be ahead of the previous build)", "err", err)
	}
	if err := s.state.RollbackCurrent(); err != nil {
		log.Error("could not roll the build back; starting the current build again", "err", err)
	}

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
	s.ports.retain(c.ports)
	return c, nil
}

// launch starts a child of the build current points at now. Ports the
// build declares that are not held yet are bound; one that fails to bind
// is logged and left for the child to bind itself.
func (s *supervisor) launch() (*child, error) {
	current, err := s.state.Current()
	if err != nil {
		return nil, fmt.Errorf("resolve the current build: %w", err)
	}
	ports, err := s.ports.prepare(s.wantPorts(current))
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
