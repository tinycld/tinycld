//go:build unix

package coreserver

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"syscall"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"

	"tinycld.org/core/listeners"
	"tinycld.org/core/supervise"
)

// selfTerminate sends this process SIGTERM, so PocketBase's own signal path
// runs every OnTerminate hook and exits, as it does for any stop. It is a
// variable so a test can drive a drain without ending the test binary.
var selfTerminate = func() error { return syscall.Kill(os.Getpid(), syscall.SIGTERM) }

// supervisedServePriority runs the supervised OnServe handler before every
// other one, so its code after e.Next() runs after all of theirs: ready must
// not go out before the last route is bound.
const supervisedServePriority = -99999

// drainShutdownPriority runs the drain's shutdown before PocketBase's own
// graceful shutdown (priority -9999). That one cancels every request's
// context and allows one second, which would cut the requests a drain exists
// to let finish.
const drainShutdownPriority = -10000

// drain is a supervisor's request that this process stop. Its shutdown runs
// inside OnTerminate rather than here because PocketBase's serve command
// returns as soon as the server stops serving, and PocketBase then runs its
// terminate path at once; shutting down from OnTerminate is what keeps that
// path waiting for the in-flight requests.
type drain struct {
	mu        sync.Mutex
	drainer   *supervise.Drainer
	requested bool
}

func (d *drain) setDrainer(dr *supervise.Drainer) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.drainer = dr
}

func (d *drain) begin() {
	d.mu.Lock()
	d.requested = true
	d.mu.Unlock()
	srvLog.Info("the supervisor asked this server to drain")
	if err := selfTerminate(); err != nil {
		srvLog.Error("could not signal this process to stop for a drain", "err", err)
	}
}

// shutdown stops accepting (it closes the listener this process serves on,
// which is a dup: the supervisor's own copy stays open for the next child)
// and answers every request already accepted, within ChildDrainTimeout.
func (d *drain) shutdown() {
	d.mu.Lock()
	dr, requested := d.drainer, d.requested
	d.mu.Unlock()
	if !requested || dr == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), supervise.ChildDrainTimeout)
	defer cancel()
	if err := dr.Drain(ctx); err != nil {
		srvLog.Warn("drain budget ran out; cutting the connections still open", "err", err)
	}
}

// registerSupervised makes a server started by a supervisor serve on the
// listeners the supervisor holds, report when it is ready, and drain when
// asked. Without a supervisor it binds nothing.
func registerSupervised(app core.App) {
	if !listeners.Supervised() {
		return
	}
	ctl := openControl()
	setControl(ctl)
	d := &drain{}

	app.OnServe().Bind(&hook.Handler[*core.ServeEvent]{
		Id:       "tinycldSupervisedServe",
		Priority: supervisedServePriority,
		Func: func(e *core.ServeEvent) error {
			useInheritedListeners(e)
			// The drainer must see every connection from the first one, so
			// it wraps the listener before PocketBase starts serving on it.
			dr := supervise.NewDrainer(e.Server)
			if e.Listener != nil {
				e.Listener = dr.Listener(e.Listener)
			}
			if err := e.Next(); err != nil {
				return err
			}
			d.setDrainer(dr)
			if ctl == nil {
				return nil
			}
			if err := ctl.send(supervise.Msg{Type: supervise.MsgReady}); err != nil {
				srvLog.Error("could not tell the supervisor this server is ready", "err", err)
				return nil
			}
			go watchControl(bufio.NewReader(ctl.conn), d)
			return nil
		},
	})

	app.OnTerminate().Bind(&hook.Handler[*core.TerminateEvent]{
		Id:       "tinycldSupervisedDrain",
		Priority: drainShutdownPriority,
		Func: func(e *core.TerminateEvent) error {
			d.shutdown()
			return e.Next()
		},
	})
}

// useInheritedListeners must run before e.Next(): PocketBase binds its own
// port when e.Listener is still nil, and the supervisor already holds it.
func useInheritedListeners(e *core.ServeEvent) {
	if l, ok := listeners.Inherited(supervise.ListenerHTTPS); ok {
		e.Listener = l
	} else if l, ok := listeners.Inherited(supervise.ListenerHTTP); ok {
		e.Listener = l
	} else {
		srvLog.Error("supervised, but no HTTP listener was passed; binding the address directly")
	}
	if l, ok := listeners.Inherited(supervise.ListenerHTTPRedirect); ok {
		apis.SetRedirectListener(e.App, l)
	}
}

// openControl returns nil when the supervisor passed no usable control
// socket. The server still serves; the supervisor, hearing no ready, times
// out and rolls back.
func openControl() *controlChannel {
	f, ok := listeners.ExtraFD(supervise.ControlFD)
	if !ok {
		srvLog.Error("supervised, but no control socket was passed; the supervisor will not hear that this server is ready")
		return nil
	}
	// FileConn dups the fd into the network poller, so the reader can block
	// while the writers still send, and listeners keeps its own handle.
	conn, err := net.FileConn(f)
	if err != nil {
		srvLog.Error("the control socket the supervisor passed is not usable", "err", err)
		return nil
	}
	return &controlChannel{conn: conn}
}

// watchControl reads the supervisor's messages until the socket closes. A
// closed socket means the supervisor is gone; this process keeps serving,
// because exiting would take the deployment down with nothing to restart it.
func watchControl(r *bufio.Reader, d *drain) {
	for {
		m, err := supervise.Recv(r)
		if errors.Is(err, supervise.ErrBadMessage) {
			srvLog.Warn("ignoring a control message that does not parse", "err", err)
			continue
		}
		if errors.Is(err, io.EOF) {
			srvLog.Error("the supervisor closed the control socket; serving on without it")
			return
		}
		if err != nil {
			srvLog.Error("could not read the supervisor's control socket; serving on without it", "err", err)
			return
		}
		if m.Version > supervise.ProtocolVersion {
			srvLog.Warn("ignoring a control message from a newer protocol", "type", m.Type, "version", m.Version)
			continue
		}
		if m.Type != supervise.MsgDrain {
			srvLog.Warn("ignoring an unknown control message", "type", m.Type)
			continue
		}
		d.begin()
		return
	}
}
