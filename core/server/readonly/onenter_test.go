package readonly

import (
	"testing"
)

func TestOnEnterRunsBeforeEnterReturns(t *testing.T) {
	resetOnEnterForTest()
	t.Cleanup(resetOnEnterForTest)
	t.Cleanup(Leave)

	ran := false
	OnEnter(func() {
		if !Active() {
			t.Error("the hook ran before the mode was active")
		}
		ran = true
	})
	Enter()
	if !ran {
		t.Fatal("the hook did not run before Enter returned")
	}
}

func TestOnEnterRunsOnTheFirstTransitionOnly(t *testing.T) {
	resetOnEnterForTest()
	t.Cleanup(resetOnEnterForTest)
	t.Cleanup(Leave)

	runs := 0
	OnEnter(func() { runs++ })
	Enter()
	Enter()
	if runs != 1 {
		t.Fatalf("hook runs after a repeated Enter: got %d, want 1", runs)
	}
	Leave()
	Enter()
	if runs != 2 {
		t.Fatalf("hook runs after Leave and Enter: got %d, want 2", runs)
	}
}

func TestOnEnterRecoversAPanicAndRunsTheOthers(t *testing.T) {
	resetOnEnterForTest()
	t.Cleanup(resetOnEnterForTest)
	t.Cleanup(Leave)

	second := false
	OnEnter(func() { panic("boom") })
	OnEnter(func() { second = true })
	Enter()
	if !second {
		t.Fatal("the hook after the panicking one did not run")
	}
	if !Active() {
		t.Fatal("the mode is not active after a hook panicked")
	}
}
