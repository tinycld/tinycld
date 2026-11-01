//go:build !linux && !darwin

package supervise

import "tinycld.org/core/logging"

// Run is for linux and darwin only: it passes listening sockets and a
// control socket to its children as inherited file descriptors, and it
// signals their process groups through procgroup.
func Run(args []string, getenv func(string) string) int {
	logging.Install()
	log.Error("supervise is only supported on linux and darwin")
	return 1
}
