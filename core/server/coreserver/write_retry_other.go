//go:build !darwin

package coreserver

import (
	"net"

	"github.com/pocketbase/pocketbase/core"
)

// retryWritesOnNoBufferSpace changes nothing: only macOS fails a TCP write
// with ENOBUFS (see writeRetryListener).
func retryWritesOnNoBufferSpace(l net.Listener) net.Listener { return l }

func serveWithWriteRetry(*core.ServeEvent) error { return nil }
