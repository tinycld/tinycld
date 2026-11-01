package procgroup

import (
	"errors"
	"time"

	"golang.org/x/sys/unix"
)

// waitExit blocks until the child pid has exited, and leaves it unreaped.
// It reports no error: it waits until it sees the exit.
//
// darwin has waitid and WNOWAIT, but its waitid also returns when the child
// stops, even with only WEXITED (Go's os package stopped using it there for
// that reason, golang.org/issue/19314). So the exit comes from a kqueue
// NOTE_EXIT event instead. The kernel refuses that registration (ESRCH) for
// a child that has begun to exit, and the child is then still unreaped, as
// only its parent can reap it.
//
// The process table is checked at each recheck interval as well, where an
// exited, unreaped child is a zombie (SZOMB): a backstop for an exit the
// kqueue did not report. Without a kqueue (out of descriptors, say) the
// table is polled, and a new kqueue is tried at each poll. A failed table
// check is not an exit: returning then would let Wait mark a running child
// reaped, and its group would get no more signals.
func waitExit(pid int) error {
	kq := -1
	defer func() {
		if kq >= 0 {
			_ = unix.Close(kq)
		}
	}()
	timeout := unix.NsecToTimespec(int64(exitRecheck))
	events := make([]unix.Kevent_t, 1)
	warned := false
	warn := func(msg string, err error) {
		if !warned {
			warned = true
			log.Warn(msg, "pid", pid, "err", err)
		}
	}
	for {
		if kq < 0 {
			q, exiting, err := exitQueue(pid)
			if exiting {
				return nil
			}
			if err != nil {
				warn("no kqueue to wait for a child's exit; polling for it", err)
			}
			kq = q
		}
		exited, err := hasExited(pid)
		if err != nil {
			warn("could not read a child's state from the process table", err)
		} else if exited {
			return nil
		}
		if kq < 0 {
			time.Sleep(exitPoll)
			continue
		}
		n, err := unix.Kevent(kq, nil, events, &timeout)
		switch {
		case errors.Is(err, unix.EINTR):
		case err != nil:
			warn("waiting for a child's exit failed; polling for it", err)
			_ = unix.Close(kq)
			kq = -1
		case n > 0:
			return nil
		}
	}
}

// exitRecheck is how often waitExit checks the process table while it waits
// for the kqueue event; exitPoll is how often without a kqueue.
const (
	exitRecheck = time.Second
	exitPoll    = 50 * time.Millisecond
)

// exitQueue returns a kqueue that reports pid's exit, or exiting when the
// child has already begun to exit (ESRCH from the registration).
func exitQueue(pid int) (kq int, exiting bool, err error) {
	kq, err = unix.Kqueue()
	if err != nil {
		return -1, false, err
	}
	ev := unix.Kevent_t{Fflags: unix.NOTE_EXIT}
	unix.SetKevent(&ev, pid, unix.EVFILT_PROC, unix.EV_ADD|unix.EV_ONESHOT)
	if _, err := unix.Kevent(kq, []unix.Kevent_t{ev}, nil, nil); err != nil {
		_ = unix.Close(kq)
		return -1, errors.Is(err, unix.ESRCH), err
	}
	return kq, false, nil
}

// sZomb is SZOMB from <sys/proc.h>: the process has exited and its parent
// has not reaped it yet.
const sZomb = 5

// hasExited reports whether pid has exited. A variable so a test can make
// the process table fail.
var hasExited = processTableExited

// processTableExited reports whether pid has exited. A pid the process table
// does not hold (EIO from SysctlKinfoProc's short read) has been reaped
// already.
func processTableExited(pid int) (bool, error) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if errors.Is(err, unix.EIO) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return kp.Proc.P_stat == sZomb, nil
}
