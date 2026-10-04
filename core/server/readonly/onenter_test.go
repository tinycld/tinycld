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
