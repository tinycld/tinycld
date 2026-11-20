package coreserver

import (
	"net"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/listeners"
)

// retryWritesOnNoBufferSpace wraps l on macOS only; see writeRetryListener.
func retryWritesOnNoBufferSpace(l net.Listener) net.Listener {
	return newWriteRetryListener(l)
}

// registerWriteRetry binds on macOS only: no other platform has the error, so
// no other platform pays for a handler.
func registerWriteRetry(app core.App) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		if err := serveWithWriteRetry(e); err != nil {
			return err
		}
		return e.Next()
	})
}

// serveWithWriteRetry covers a server with no supervisor. A supervised server
// wraps its inherited listener in registerSupervised, under the drainer.
func serveWithWriteRetry(e *core.ServeEvent) error {
	if listeners.Supervised() {
		return nil
	}
	// PocketBase binds the address itself when e.Listener is nil, and that
	// listener cannot be wrapped, so bind it here. With no address,
	// PocketBase picks a default this code does not know; leave it alone.
	if e.Listener == nil && e.Server != nil && e.Server.Addr != "" {
		l, err := net.Listen("tcp", e.Server.Addr)
		if err != nil {
			return err
		}
		e.Listener = l
	}
	if e.Listener != nil {
		e.Listener = retryWritesOnNoBufferSpace(e.Listener)
	}
	return nil
}
