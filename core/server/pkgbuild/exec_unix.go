//go:build linux || darwin

package pkgbuild

import (
	"errors"
	"os"
	"os/exec"
	"syscall"

	"tinycld.org/core/procgroup"
)

// killGroupOnCancel starts cmd as the leader of a new process group and makes
// its cancel SIGKILL that whole group, so the children a build tool started
// die with it. It returns the Wait that cmd must be waited with: the cancel
// runs on its own goroutine and can come after the reap, when the group id
// may already belong to another process, and procgroup's Wait keeps it from
// signalling then.
//
// The new group also takes the command out of this process's group, so a
// signal to this process's group (the supervisor's SIGKILL) does not reach
// it. When this process dies without running the cancel, the command would
// outlive it; setParentDeathSignal covers that where the kernel can.
func killGroupOnCancel(cmd *exec.Cmd) (wait func() error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	setParentDeathSignal(cmd.SysProcAttr)
	group := procgroup.New(cmd)
	cmd.Cancel = func() error {
		err := group.Signal(syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	return group.Wait
}
