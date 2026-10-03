// Package drainhooks lets a package act the moment a supervised server
// begins to drain. A supervisor drains the old server only once the next
// one is ready on the same ports, so from that moment new connections
// belong to the next server. Core drains its HTTP server itself; a package
// that serves its own protocol on a port the supervisor passed in registers
// here to stop accepting on that port at once, instead of when PocketBase's
// terminate hooks reach it after the HTTP drain.
//
// Core names no package: each package calls OnBegin from its own Register().
// The registry is process-global and is meant to be populated at startup.
package drainhooks

import (
	"sync"

	"tinycld.org/core/logging"
)

var log = logging.ForPackage("drainhooks")

type handler struct {
	name string
	fn   func()
}

var (
	mu       sync.Mutex
	handlers []handler
)

// OnBegin registers fn to run when a drain begins. fn must return quickly:
// the drain waits for it before it lets in-flight requests finish. A second
// registration under the same name replaces the first, so a Register() that
// runs again does not run its handler twice.
func OnBegin(name string, fn func()) {
	mu.Lock()
	defer mu.Unlock()
	for i, h := range handlers {
		if h.name == name {
			handlers[i].fn = fn
			return
		}
	}
	handlers = append(handlers, handler{name: name, fn: fn})
}

// RunBegin runs every registered handler in registration order. A handler
// that panics is logged and the rest still run: one package's failure must
// not leave another package's port accepting.
func RunBegin() {
	mu.Lock()
	hs := append([]handler(nil), handlers...)
	mu.Unlock()
	for _, h := range hs {
		run(h)
	}
}

func run(h handler) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("a drain-begin handler panicked", "handler", h.name, "panic", r)
		}
	}()
	h.fn()
}

// ResetForTest clears the registry.
func ResetForTest() {
	mu.Lock()
	defer mu.Unlock()
	handlers = nil
}
