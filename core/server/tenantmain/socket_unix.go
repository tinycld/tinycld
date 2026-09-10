//go:build !windows

package tenantmain

import (
	"net"
	"syscall"
)

// listenUnixUmasked binds a unix socket with a 0177 umask so it is never
// world-reachable, even briefly. See BindTenantSocket for why the umask wraps
// the bind rather than relying on the follow-up Chmod alone.
//
// The umask is process-wide, but socket binds in this process are sequential.
func listenUnixUmasked(path string) (net.Listener, error) {
	oldUmask := syscall.Umask(0o177)
	defer syscall.Umask(oldUmask)
	return net.Listen("unix", path)
}
