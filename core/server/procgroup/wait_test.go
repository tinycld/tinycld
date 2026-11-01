//go:build linux || darwin

package procgroup

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func startChild(t *testing.T, name string, args ...string) *exec.Cmd {
	t.Helper()
	if testing.Short() {
		t.Skip("starts real processes")
	}
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	return cmd
}

// waitExit must leave the child for exec's Wait to reap: until then the pid,
// and the group id it is, stays taken.
func TestWaitExit_LeavesTheChildUnreaped(t *testing.T) {
	cmd := startChild(t, "true")
	if err := waitExit(cmd.Process.Pid); err != nil {
		t.Fatal(err)
	}
	var ws syscall.WaitStatus
	pid, err := syscall.Wait4(cmd.Process.Pid, &ws, syscall.WNOHANG, nil)
	if err != nil || pid != cmd.Process.Pid {
		t.Fatalf("Wait4 after waitExit = %d, %v; want the exited child, still waitable", pid, err)
	}
}

// An exit while waitExit waits is seen when it happens, not at a later
// recheck of the process table.
func TestWaitExit_SeesAnExitPromptly(t *testing.T) {
	cmd := startChild(t, "sleep", "0.2")
	start := time.Now()
	if err := waitExit(cmd.Process.Pid); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 800*time.Millisecond {
		t.Fatalf("waitExit took %s for a child that ran 200ms", d)
	}
}

// A child that exits while waitExit starts to wait is seen at once too, not
// at the next recheck: on darwin the kqueue registration then fails, and
// the child can still be running its exit, not yet a zombie.
func TestWaitExit_SeesAnExitAsTheWaitStarts(t *testing.T) {
	for i := range 50 {
		cmd := startChild(t, "sh", "-c", "exit 1")
		// Spread the wait's start over the child's short life.
		time.Sleep(time.Duration(i%7) * time.Millisecond)
		start := time.Now()
		if err := waitExit(cmd.Process.Pid); err != nil {
			t.Fatal(err)
		}
		if d := time.Since(start); d > 500*time.Millisecond {
			t.Fatalf("waitExit took %s for a child that exited at once", d)
		}
	}
}

// A stopped child has not exited. (darwin's waitid reports a stop even with
// only WEXITED, which is why waitExit does not use it there.)
func TestWaitExit_IgnoresAStop(t *testing.T) {
	cmd := startChild(t, "sleep", "30")
	if err := cmd.Process.Signal(syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- waitExit(cmd.Process.Pid) }()
	select {
	case err := <-done:
		t.Fatalf("waitExit returned for a stopped child: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waitExit did not return after the child was killed")
	}
}
