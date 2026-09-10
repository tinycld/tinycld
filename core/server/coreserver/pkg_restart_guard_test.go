package coreserver

import (
	"testing"

	"tinycld.org/core/installjob"
)

// TestShouldSuppressRestart is the H4 regression guard: a hooks-watcher restart
// (IsRestart) must be vetoed while a package pipeline holds the single-flight
// lock, so the generator's mid-pipeline pb_hooks rewrite can't tear the process
// down between steps. Non-restart terminations and idle-state restarts proceed.
func TestShouldSuppressRestart(t *testing.T) {
	// The interlock is process-global; leave it as we found it.
	prev := installjob.Current()
	installjob.Release(prev)
	t.Cleanup(func() {
		installjob.Release(installjob.Current())
		if prev != nil {
			installjob.Claim(prev)
		}
	})

	// Idle: a restart is allowed.
	if shouldSuppressRestart(true) {
		t.Error("restart suppressed while idle — should proceed")
	}
	// A non-restart termination is never suppressed.
	if shouldSuppressRestart(false) {
		t.Error("non-restart termination suppressed — should proceed")
	}

	// Job in flight: a restart must be vetoed.
	inflight := installjob.New("version_change", "", "")
	if _, ok := installjob.Claim(inflight); !ok {
		t.Fatal("claim must win against an idle interlock")
	}

	if !shouldSuppressRestart(true) {
		t.Error("restart NOT suppressed during an in-flight pipeline — risks mid-pipeline teardown")
	}
	// Even mid-job, a non-restart termination (real shutdown) proceeds.
	if shouldSuppressRestart(false) {
		t.Error("non-restart termination suppressed mid-job — real shutdowns must proceed")
	}
}
