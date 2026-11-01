package procgroup

import (
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// A failing process table check is not an exit: waitExit must keep waiting
// for the child. Returning would let Wait mark a running child reaped, and
// a later Kill would then send nothing.
func TestWaitExit_ProcessTableErrorIsNotAnExit(t *testing.T) {
	cmd := startChild(t, "sleep", "0.3")
	hasExited = func(int) (bool, error) { return false, unix.EPERM }
	t.Cleanup(func() { hasExited = processTableExited })

	start := time.Now()
	if err := waitExit(cmd.Process.Pid); err != nil {
		t.Fatalf("waitExit = %v, want it to wait for the exit", err)
	}
	if d := time.Since(start); d < 200*time.Millisecond {
		t.Fatalf("waitExit returned after %s, before the child exited", d)
	}
}
