package procgroup

import (
	"errors"

	"golang.org/x/sys/unix"
)

// waitExit blocks until the child pid has exited, and leaves it unreaped.
// A child that is already gone (ECHILD) has nothing left to wait for.
func waitExit(pid int) error {
	var info unix.Siginfo
	for {
		err := unix.Waitid(unix.P_PID, pid, &info, unix.WEXITED|unix.WNOWAIT, nil)
		switch {
		case errors.Is(err, unix.EINTR):
			continue
		case errors.Is(err, unix.ECHILD):
			return nil
		}
		return err
	}
}
