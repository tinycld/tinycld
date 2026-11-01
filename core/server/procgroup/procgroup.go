//go:build linux || darwin

// Package procgroup signals the process group a child leads, and never
// after the child has been reaped.
//
// A child started with Setpgid leads a new group whose id is its pid. The
// kernel keeps that number while the child is unreaped, as a zombie too, so
// until the reap kill(-pid) can reach only the child's own group. After the
// reap the number is open again: a new process can take it and lead a group
// with the same id, and a late kill(-pid) reaches that group instead.
//
// Leader closes the window. Wait first blocks until the child has exited,
// without reaping it, then marks it reaped under the lock that Signal holds
// while it signals, and only then reaps it.
package procgroup

import (
	"os"
	"os/exec"
	"sync"
	"syscall"

	"tinycld.org/core/logging"
)

var log = logging.ForPackage("procgroup")

// Leader is a command that leads its own process group.
type Leader struct {
	cmd *exec.Cmd
	mu  sync.Mutex
	// waited is set by the first Wait; reaped once the child has exited and
	// is about to be reaped.
	waited, reaped bool
}

// New returns the Leader for cmd. The command must start with Setpgid in
// its SysProcAttr, and only Leader.Wait may wait for it.
func New(cmd *exec.Cmd) *Leader { return &Leader{cmd: cmd} }

// Signal sends sig to the command's whole process group. Once Wait has
// marked the command reaped it sends nothing and returns os.ErrProcessDone.
// It must not be called before the command has started.
func (l *Leader) Signal(sig syscall.Signal) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.reaped {
		return os.ErrProcessDone
	}
	return syscall.Kill(-l.cmd.Process.Pid, sig)
}

// Wait waits for the started command to exit and reaps it, as
// exec.Cmd.Wait does. Only the first call waits; a later one returns
// exec.Cmd.Wait's error at once, since by then the pid can name another
// process.
//
// When the wait for the exit fails (it cannot on darwin), the command is
// marked reaped at once: a group signal that does not happen is less
// harmful than one that can reach some other group.
func (l *Leader) Wait() error {
	l.mu.Lock()
	first := !l.waited
	l.waited = true
	l.mu.Unlock()
	if !first {
		return l.cmd.Wait()
	}
	if err := waitExit(l.cmd.Process.Pid); err != nil {
		log.Error("could not wait for a command to exit; its process group gets no more signals",
			"pid", l.cmd.Process.Pid, "err", err)
	}
	l.mu.Lock()
	l.reaped = true
	l.mu.Unlock()
	return l.cmd.Wait()
}
