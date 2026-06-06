package coreserver

import "testing"

// TestShouldSuppressRestart is the H4 regression guard: a hooks-watcher restart
// (IsRestart) must be vetoed while a package pipeline holds the single-flight
// lock, so the generator's mid-pipeline pb_hooks rewrite can't tear the process
// down between steps. Non-restart terminations and idle-state restarts proceed.
func TestShouldSuppressRestart(t *testing.T) {
	// Ensure a clean baseline and restore it (this is process-global state).
	installMu.Lock()
	prev := currentJob
	currentJob = nil
	installMu.Unlock()
	t.Cleanup(func() {
		installMu.Lock()
		currentJob = prev
		installMu.Unlock()
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
	installMu.Lock()
	currentJob = &installJob{ID: "job_test", Action: "version_change", Status: "running"}
	installMu.Unlock()

	if !shouldSuppressRestart(true) {
		t.Error("restart NOT suppressed during an in-flight pipeline — risks mid-pipeline teardown")
	}
	// Even mid-job, a non-restart termination (real shutdown) proceeds.
	if shouldSuppressRestart(false) {
		t.Error("non-restart termination suppressed mid-job — real shutdowns must proceed")
	}
}
