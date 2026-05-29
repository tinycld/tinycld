package versionhooks

import (
	"errors"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestRegister_StoresAndRetrieves(t *testing.T) {
	t.Cleanup(ResetForTest)
	ResetForTest()

	snapshotCalled := false
	restoreCalled := false

	Register("text", Hook{
		OnSnapshot: func(_ core.App, _, _ *core.Record) error {
			snapshotCalled = true
			return nil
		},
		OnRestore: func(_ core.App, _, _ *core.Record) error {
			restoreCalled = true
			return nil
		},
	})

	hook := For("text")
	if hook.OnSnapshot == nil || hook.OnRestore == nil {
		t.Fatalf("retrieved hook missing callbacks: %+v", hook)
	}
	if err := hook.OnSnapshot(nil, nil, nil); err != nil {
		t.Errorf("OnSnapshot: %v", err)
	}
	if err := hook.OnRestore(nil, nil, nil); err != nil {
		t.Errorf("OnRestore: %v", err)
	}
	if !snapshotCalled || !restoreCalled {
		t.Errorf("callbacks not invoked: snap=%v restore=%v", snapshotCalled, restoreCalled)
	}
}

func TestRegister_ReplacesOnRereg(t *testing.T) {
	t.Cleanup(ResetForTest)
	ResetForTest()

	firstErr := errors.New("first")
	secondErr := errors.New("second")

	Register("text", Hook{
		OnSnapshot: func(_ core.App, _, _ *core.Record) error { return firstErr },
		OnRestore:  func(_ core.App, _, _ *core.Record) error { return firstErr },
	})
	// Re-register with a hook that only sets OnSnapshot — confirms
	// replacement is full-struct, not field-merge.
	Register("text", Hook{
		OnSnapshot: func(_ core.App, _, _ *core.Record) error { return secondErr },
	})

	hook := For("text")
	if err := hook.OnSnapshot(nil, nil, nil); err != secondErr {
		t.Errorf("OnSnapshot returned %v, want %v (replacement)", err, secondErr)
	}
	if hook.OnRestore != nil {
		t.Errorf("OnRestore should be nil after replacement; got non-nil")
	}
}

func TestFor_ReturnsZeroValueWhenMissing(t *testing.T) {
	t.Cleanup(ResetForTest)
	ResetForTest()

	hook := For("never-registered")
	if hook.OnSnapshot != nil || hook.OnRestore != nil {
		t.Errorf("expected zero-value hook, got %+v", hook)
	}
}
