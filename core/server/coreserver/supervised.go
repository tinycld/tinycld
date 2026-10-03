package coreserver

import (
	"io"
	"sync"
	"sync/atomic"

	"tinycld.org/core/listeners"
	"tinycld.org/core/readonly"
	"tinycld.org/core/supervise"
)

// controlChannel is this process's end of the supervisor's control socket.
// The serve hook (ready) and a rebuild job's goroutine (restart) both write
// to it, so writes are serialized: two messages must never interleave on
// the one stream.
type controlChannel struct {
	mu   sync.Mutex
	conn io.ReadWriter
}

func (c *controlChannel) send(m supervise.Msg) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return supervise.Send(c.conn, m)
}

// beingReplaced is set once a supervised process has asked for its
// replacement. It stays set: the process only waits to be drained.
var beingReplaced atomic.Bool

var (
	controlMu sync.Mutex
	control   *controlChannel
)

func setControl(c *controlChannel) {
	controlMu.Lock()
	defer controlMu.Unlock()
	control = c
}

func currentControl() *controlChannel {
	controlMu.Lock()
	defer controlMu.Unlock()
	return control
}

// askSupervisorToRestart reports whether the supervisor got the request.
// When it did not, the caller exits 75 instead: that is what a child too old
// for the protocol does, and the supervisor answers it with a cold restart.
func askSupervisorToRestart(cold bool) bool {
	c := currentControl()
	if c == nil {
		srvLog.Error("no control socket to ask the supervisor for a restart; exiting for a cold restart instead")
		return false
	}
	if err := c.send(supervise.Msg{Type: supervise.MsgRestart, Cold: cold}); err != nil {
		srvLog.Error("could not ask the supervisor for a restart; exiting for a cold restart instead", "err", err)
		return false
	}
	srvLog.Info("asked the supervisor for a restart; serving read-only until it drains this process", "cold", cold)
	return true
}

// pauseWritesForBackup puts a supervised server into read-only mode just
// before a rebuild backs up its database, and returns what a failure path
// calls to accept writes again. A supervisor runs the next build beside this
// one, so a write after the backup would be lost if that build is rolled
// back, and a write after the migration sync would hit a schema this process
// does not know. Without a supervisor this process exits before the next one
// starts, so there is nothing to pause and resume is a no-op.
//
// resume also does nothing once this process has asked to be replaced: read-
// only mode is one switch, and a late failure path must not re-open writes
// in a process that is only waiting to be drained.
func pauseWritesForBackup() (resume func()) {
	if !listeners.Supervised() {
		return func() {}
	}
	readonly.Enter()
	return func() {
		if !beingReplaced.Load() {
			readonly.Leave()
		}
	}
}
