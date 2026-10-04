//go:build !unix

package supervise

import "tinycld.org/core/logging"

// Run is unix only: it passes listening sockets and a control socket to its
// children as inherited file descriptors.
func Run(args []string, getenv func(string) string) int {
	logging.Install(nil)
	log.Error("supervise is only supported on unix")
	return 1
}
