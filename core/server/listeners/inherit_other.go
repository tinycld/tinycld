//go:build !unix

package listeners

import "net"
import "os"

// Inherited always returns false: there is no supervisor on this platform,
// so a caller falls back to binding the port itself.
func Inherited(name string) (net.Listener, bool) { return nil, false }

// Supervised is always false on this platform.
func Supervised() bool { return false }

// ExtraFD always returns false: non-unix never inherits a control socket.
func ExtraFD(name string) (*os.File, bool) { return nil, false }

// SetForTest is a no-op restore on this platform; nothing to override.
func SetForTest(ls map[string]net.Listener) (restore func()) {
	return func() {}
}

// SetFilesForTest is a no-op restore on this platform; nothing to override.
func SetFilesForTest(fs map[string]*os.File) (restore func()) {
	return func() {}
}
