//go:build windows

package tenantmain

import (
	"fmt"
	"net"
)

// listenUnixUmasked refuses on Windows, which has no umask.
//
// Only the hosting router connects to these sockets, and it runs on Linux.
// This file exists so the single-binary SELF-HOST build cross-compiles for
// Windows, where tenant mode is never entered — a self-hosted deployment is
// one org in one process, served over TCP.
//
// It refuses rather than binding without the umask, matching confine_other.go:
// a socket that should be owner-only must not be served world-reachable
// because the host cannot express the restriction.
func listenUnixUmasked(path string) (net.Listener, error) {
	return nil, fmt.Errorf("tenant sockets require a unix host (umask); refusing to bind %s", path)
}
