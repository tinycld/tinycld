//go:build linux || darwin

package pkgbuild

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The regression this function exists for: CopyDir must not try to reproduce
// the SOURCE's ownership on the destination. `cp -a` did, which fails with
// EINVAL inside the builder's user namespace (the pre-fetched member tree is
// root-owned and root is unmapped there). Asserting the destination is owned
// by the copying process — not the source uid — pins the behavior without
// needing a namespace to reproduce it in. Unix-only: syscall.Stat_t and uids
// are a POSIX concept.
func TestCopyDir_DoesNotPreserveOwnership(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: every uid is writable, so the assertion proves nothing")
	}
	src := t.TempDir()
	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}
	info, err := os.Stat(filepath.Join(dst, "f"))
	if err != nil {
		t.Fatal(err)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Skip("no stat_t on this platform")
	}
	if int(st.Uid) != os.Geteuid() {
		t.Errorf("copied file uid = %d, want the copying process's %d", st.Uid, os.Geteuid())
	}
}

// A cancelled command dies with every process it started: a build tool's
// children (pnpm's node, go's compiler) would otherwise keep running and
// keep the output pipes open.
func TestRunCmdContext_CancelKillsTheProcessGroup(t *testing.T) {
	heartbeat := filepath.Join(t.TempDir(), "heartbeat")
	script := `(while true; do echo beat >> "$1"; sleep 0.05; done) & wait`
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := RunCmdContext(ctx, t.TempDir(), "sh", "-c", script, "sh", heartbeat)
		done <- err
	}()
	deadline := time.Now().Add(10 * time.Second)
	for size(heartbeat) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the command never started its child")
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a cancelled command reported success")
		}
	case <-time.After(cmdWaitDelay):
		t.Fatal("the cancelled command did not return before its pipes' wait delay")
	}
	time.Sleep(200 * time.Millisecond)
	before := size(heartbeat)
	time.Sleep(300 * time.Millisecond)
	if after := size(heartbeat); after != before {
		t.Fatalf("the command's child still runs after the cancel (heartbeat %d -> %d bytes)", before, after)
	}
}

// Once the command has been reaped, another process can take its pid,
// which is its process group id, so a cancel then must signal nothing. The
// group here is kept alive by a member that outlives the command: it stands
// in for a new group that took the number.
func TestCommandContext_CancelAfterTheReapSendsNothing(t *testing.T) {
	if testing.Short() {
		t.Skip("starts real processes")
	}
	pidFile := filepath.Join(t.TempDir(), "pid")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := commandContext(ctx, "sh", "-c", `sleep 30 & echo $! > "$1"`, "sh", pidFile)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	member, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(member, syscall.SIGKILL) })

	if err := cmd.Cancel(); !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("cancel after the reap = %v, want os.ErrProcessDone", err)
	}
	time.Sleep(100 * time.Millisecond)
	if syscall.Kill(member, 0) != nil {
		t.Fatal("a cancel after the reap reached the process group")
	}
}

func size(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
