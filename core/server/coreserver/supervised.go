package coreserver

import (
	"io"
	"sync"
	"sync/atomic"
	"time"

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
	// acks carries the supervisor's restart-acks from the control reader to
	// the goroutine waiting on one.
	acks chan struct{}
}

func (c *controlChannel) send(m supervise.Msg) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return supervise.Send(c.conn, m)
}

// restartAckTimeout is how long a restart waits for the supervisor's ack.
// The supervisor acks on receipt, so the wait is short when it is there; a
// supervisor that does not answer would leave this process read-only for
// good, so on no ack the process exits 75, the restart every supervisor
// handles. It is a variable so a test need not wait the full time.
var restartAckTimeout = 10 * time.Second

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

// askSupervisorToRestart reports whether the supervisor acked the request.
// When it did not, the caller exits 75 instead, which the supervisor
// answers with a cold restart.
func askSupervisorToRestart(cold bool) bool {
	c := currentControl()
	if c == nil {
		srvLog.Error("no control socket to ask the supervisor for a restart; exiting for a cold restart instead")
		return false
	}
	// A stale ack from an earlier request must not answer this one.
	select {
	case <-c.acks:
	default:
	}
	if err := c.send(supervise.Msg{Type: supervise.MsgRestart, Cold: cold}); err != nil {
		srvLog.Error("could not ask the supervisor for a restart; exiting for a cold restart instead", "err", err)
		return false
	}
	t := time.NewTimer(restartAckTimeout)
	defer t.Stop()
	select {
	case <-c.acks:
	case <-t.C:
		srvLog.Error("the supervisor did not ack the restart; exiting for a cold restart instead", "waited", restartAckTimeout)
		return false
	}
	srvLog.Info("the supervisor accepted the restart; serving read-only until it drains this process", "cold", cold)
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
