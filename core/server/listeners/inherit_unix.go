//go:build unix

package listeners

import (
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"tinycld.org/core/logging"
)

// EnvFDs is the count of inherited file descriptors, starting at fd 3.
const EnvFDs = "TINYCLD_LISTEN_FDS"

// EnvFDNames is a colon-separated list of names, one per inherited fd, in
// fd order.
const EnvFDNames = "TINYCLD_LISTEN_FDNAMES"

// firstInheritedFD is the first fd a child inherits past stdin/stdout/stderr.
const firstInheritedFD = 3

var (
	parseOnce sync.Once

	// mu guards every field below, including during SetForTest's
	// override/restore, so a concurrent Inherited/Supervised call never
	// observes a half-updated state.
	mu          sync.Mutex
	listenersBy map[string]net.Listener
	filesBy     map[string]*os.File
	supervised  bool
)

// Inherited returns the listener the supervisor passed under name. It
// returns the same *net.TCPListener-backed net.Listener on every call for
// that name; callers never need to cache it themselves. It returns false
// for a name that was never passed, so a caller can fall back to binding
// the port itself (dev, no supervisor).
func Inherited(name string) (net.Listener, bool) {
	parseEnvOnce()
	mu.Lock()
	defer mu.Unlock()
	l, ok := listenersBy[name]
	return l, ok
}

// Supervised reports whether TINYCLD_LISTEN_FDS was set and parsed with at
// least one usable listener.
func Supervised() bool {
	parseEnvOnce()
	mu.Lock()
	defer mu.Unlock()
	return supervised
}

// ExtraFD returns a non-listener inherited fd by name, e.g. the control
// socket to the supervisor. The returned file is the process's one
// long-lived handle; callers must not close it.
func ExtraFD(name string) (*os.File, bool) {
	parseEnvOnce()
	mu.Lock()
	defer mu.Unlock()
	f, ok := filesBy[name]
	return f, ok
}

// SetForTest replaces the inherited set for the life of a test in a
// feature package's server tests, bypassing the env/fd machinery entirely.
// It is a test seam: production code never calls it. The returned restore
// func puts back whatever state existed before, including "not supervised"
// when nothing had parsed the env yet.
func SetForTest(ls map[string]net.Listener) (restore func()) {
	parseEnvOnce() // settle any real env first, so restore has a defined prior state

	mu.Lock()
	prevListeners, prevFiles, prevSupervised := listenersBy, filesBy, supervised
	listenersBy = ls
	filesBy = map[string]*os.File{}
	supervised = len(ls) > 0
	mu.Unlock()

	return func() {
		mu.Lock()
		listenersBy, filesBy, supervised = prevListeners, prevFiles, prevSupervised
		mu.Unlock()
	}
}

// SetFilesForTest replaces the inherited non-listener fds (what ExtraFD
// returns) for the life of a test, the way SetForTest does for listeners. It
// changes neither the listeners nor Supervised(), so a test pairs it with
// SetForTest to stand in for a supervisor that also passes a control socket.
// Test seam only; production code never calls it.
func SetFilesForTest(fs map[string]*os.File) (restore func()) {
	parseEnvOnce()

	mu.Lock()
	prevFiles := filesBy
	filesBy = fs
	mu.Unlock()

	return func() {
		mu.Lock()
		filesBy = prevFiles
		mu.Unlock()
	}
}

// init makes every inherited fd close-on-exec as the process starts. The fds
// arrive without it (that is how they survived the exec into this process),
// so until it is set every process this one starts (go build, pnpm, git)
// inherits them too and keeps the ports open after this process exits. The
// lookup functions parse the fds on their first call, which may come after
// such a start, so the flag cannot wait for them.
func init() {
	markInheritedCloseOnExec()
}

// markInheritedCloseOnExec sets the flag on the fds the environment names,
// checked as parseEnv checks them but without logging, because logging is
// not set up yet when init runs. Setting the flag on an fd the process does
// not use only keeps it from a child, so a wrong count cannot do harm.
func markInheritedCloseOnExec() {
	n, err := strconv.Atoi(os.Getenv(EnvFDs))
	if err != nil || n <= 0 || len(strings.Split(os.Getenv(EnvFDNames), ":")) != n {
		return
	}
	for fd := firstInheritedFD; fd < firstInheritedFD+n; fd++ {
		syscall.CloseOnExec(fd)
	}
}

// parseEnvOnce reads TINYCLD_LISTEN_FDS/TINYCLD_LISTEN_FDNAMES exactly once
// per process and files each fd as a listener or an extra file.
func parseEnvOnce() {
	parseOnce.Do(func() {
		mu.Lock()
		listenersBy = map[string]net.Listener{}
		filesBy = map[string]*os.File{}
		mu.Unlock()

		count, names := parseEnv()
		if count == 0 {
			return
		}

		log := logging.ForPackage("listeners")
		local := map[string]net.Listener{}
		localFiles := map[string]*os.File{}
		for i := 0; i < count; i++ {
			fd := firstInheritedFD + i
			name := names[i]

			f := os.NewFile(uintptr(fd), name)
			if f == nil {
				log.Warn("inherited fd is not usable", "fd", fd, "name", name)
				continue
			}

			if !isTCPSocket(fd) {
				// The control socket is a unix socketpair end, and
				// net.FileListener would wrap any stream socket as a
				// listener, so only TCP sockets (all a Set passes as
				// listeners) are filed as one.
				localFiles[name] = f
				continue
			}

			// FileListener works on a dup of the fd, so the inherited fd is
			// closed here. Left to the garbage collector, it would hold the
			// socket open after the listener is closed, and the port would
			// keep taking connections that nobody accepts.
			l, err := net.FileListener(f)
			_ = f.Close()
			if err != nil {
				log.Warn("inherited fd is a socket but not a listener", "fd", fd, "name", name, "error", err)
				continue
			}
			local[name] = l
		}

		mu.Lock()
		listenersBy = local
		filesBy = localFiles
		supervised = len(local) > 0
		mu.Unlock()
	})
}

func isTCPSocket(fd int) bool {
	sa, err := syscall.Getsockname(fd)
	if err != nil {
		return false
	}
	switch sa.(type) {
	case *syscall.SockaddrInet4, *syscall.SockaddrInet6:
		return true
	}
	return false
}

// parseEnv reads and validates the two env vars without touching any
// package state, so parseEnvOnce can hold the lock only around the state
// it actually mutates.
func parseEnv() (count int, names []string) {
	raw := os.Getenv(EnvFDs)
	if raw == "" {
		return 0, nil
	}

	log := logging.ForPackage("listeners")
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		log.Warn("invalid inherited fd count", "env", EnvFDs, "value", raw)
		return 0, nil
	}
	// A Set with no listeners writes EnvFDs=0; that's a valid "nothing to
	// inherit" rather than a parse error, so it must not warn.
	if n == 0 {
		return 0, nil
	}

	names = strings.Split(os.Getenv(EnvFDNames), ":")
	if len(names) != n {
		log.Warn("fd name count does not match fd count", "env", EnvFDNames, "names", len(names), "fds", n)
		return 0, nil
	}
	return n, names
}
