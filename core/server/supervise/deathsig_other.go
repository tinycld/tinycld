//go:build unix && !linux

package supervise

import "syscall"

// setDeathSignal does nothing: only Linux has a parent-death signal.
func setDeathSignal(*syscall.SysProcAttr) {}
