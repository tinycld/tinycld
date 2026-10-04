package supervise

import "syscall"

// setDeathSignal makes the kernel SIGKILL a child when the supervisor dies
// without draining it (SIGKILL, the OOM killer). This is the opposite of a
// process manager whose children are meant to outlive it: here the
// supervisor holds the public ports, and the supervisor started next must
// bind them, which it cannot while an orphaned child still serves on them.
//
// The child's own children (build tools in its process group) are not
// covered; the init process or service manager that restarts the
// supervisor stops them.
func setDeathSignal(attr *syscall.SysProcAttr) {
	attr.Pdeathsig = syscall.SIGKILL
}
