//go:build linux || darwin

package procgroup

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// startGroup starts script under sh as the leader of a new process group.
// The script gets a file path as $1 to write a child's pid to.
func startGroup(t *testing.T, script string) (*exec.Cmd, *Leader, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("starts real processes")
	}
	pidFile := filepath.Join(t.TempDir(), "pid")
	cmd := exec.Command("sh", "-c", script, "sh", pidFile)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	l := New(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return cmd, l, pidFile
}

// readPid waits for the script to write its child's pid.
func readPid(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		b, err := os.ReadFile(path)
		if err == nil && strings.HasSuffix(string(b), "\n") {
			pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
			if err != nil {
				t.Fatal(err)
			}
			return pid
		}
		if time.Now().After(deadline) {
			t.Fatal("the script never wrote its child's pid")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

// awaitGone waits until pid no longer names a process. An orphan is reaped
// by init (launchd on darwin), not by the test.
func awaitGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for alive(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("pid %d is still running", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// After the reap the leader's pid, and so its group id, can be reused.
// A Signal then must send nothing. The group here is kept alive by a member
// that outlives the leader: it stands in for a new group that took the
// number, and it must not get the signal.
func TestLeader_SignalAfterWaitSendsNothing(t *testing.T) {
	_, l, pidFile := startGroup(t, `sleep 30 & echo $! > "$1"`)
	member := readPid(t, pidFile)
	t.Cleanup(func() { _ = syscall.Kill(member, syscall.SIGKILL) })
	if err := l.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}

	if err := l.Signal(syscall.SIGKILL); !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("Signal after Wait = %v, want os.ErrProcessDone", err)
	}
	time.Sleep(100 * time.Millisecond)
	if !alive(member) {
		t.Fatal("a Signal after the reap reached the group")
	}
}

// Before the reap a Signal reaches the whole group, not just the leader.
func TestLeader_SignalReachesTheWholeGroup(t *testing.T) {
	_, l, pidFile := startGroup(t, `sleep 30 & echo $! > "$1"; wait`)
	member := readPid(t, pidFile)
	t.Cleanup(func() { _ = syscall.Kill(member, syscall.SIGKILL) })

	if err := l.Signal(syscall.SIGKILL); err != nil {
		t.Fatalf("Signal: %v", err)
	}
	if err := l.Wait(); err == nil {
		t.Fatal("Wait reported success for a killed leader")
	}
	awaitGone(t, member)
}

// A child that exited before Wait is waited for at once: Wait must not
// depend on seeing the exit happen.
func TestLeader_WaitForAChildThatAlreadyExited(t *testing.T) {
	cmd, l, _ := startGroup(t, `exit 3`)
	time.Sleep(200 * time.Millisecond)

	start := time.Now()
	err := l.Wait()
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("Wait took %s for a child that had already exited", d)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || cmd.ProcessState.ExitCode() != 3 {
		t.Fatalf("Wait = %v, want exit status 3", err)
	}
}

// Only the first Wait waits for the exit. After it the pid can name another
// process, possibly another child of this one, and a second wait for that
// pid would block on it. A later Wait returns exec's error at once.
func TestLeader_SecondWaitDoesNotWaitAgain(t *testing.T) {
	cmd, l, _ := startGroup(t, `exit 0`)
	if err := l.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	// Stand in for the pid's reuse: the process the command names is now
	// another running child.
	other := exec.Command("sleep", "30")
	if err := other.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = other.Process.Kill()
		_ = other.Wait()
	})
	cmd.Process = other.Process

	done := make(chan error, 1)
	go func() { done <- l.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a second Wait reported success")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a second Wait waited for the process that now has the pid")
	}
}
