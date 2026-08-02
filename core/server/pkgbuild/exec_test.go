package pkgbuild

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestRunCmd_CapturesCombinedOutput(t *testing.T) {
	out, err := RunCmd(t.TempDir(), "sh", "-c", "echo out; echo err 1>&2")
	if err != nil {
		t.Fatalf("RunCmd: %v", err)
	}
	if !strings.Contains(out, "out") || !strings.Contains(out, "err") {
		t.Fatalf("expected combined stdout+stderr, got %q", out)
	}
}

func TestRunCmd_ReturnsOutputWithError(t *testing.T) {
	out, err := RunCmd(t.TempDir(), "sh", "-c", "echo boom; exit 3")
	if err == nil {
		t.Fatal("expected error from exit 3")
	}
	if !strings.Contains(out, "boom") {
		t.Fatalf("output should survive a failing command, got %q", out)
	}
}

func TestRunCmdEnv_PassesExtraEnv(t *testing.T) {
	out, err := RunCmdEnv(t.TempDir(), []string{"PKGBUILD_TEST_SECRET=s3cret"}, "sh", "-c", "echo $PKGBUILD_TEST_SECRET")
	if err != nil {
		t.Fatalf("RunCmdEnv: %v", err)
	}
	if !strings.Contains(out, "s3cret") {
		t.Fatalf("extra env not visible to subprocess, got %q", out)
	}
}

func TestRunCmdStreaming_DeliversLinesAndBuffers(t *testing.T) {
	var lines []string
	out, err := RunCmdStreaming(func(l string) { lines = append(lines, l) },
		t.TempDir(), "sh", "-c", "echo one; echo two")
	if err != nil {
		t.Fatalf("RunCmdStreaming: %v", err)
	}
	if len(lines) != 2 || lines[0] != "one" || lines[1] != "two" {
		t.Fatalf("streamed lines wrong: %v", lines)
	}
	if out != "one\ntwo\n" {
		t.Fatalf("buffered output wrong: %q", out)
	}
}

func TestCopyDir_CopiesContentsAndSymlinks(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "f.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("sub/f.txt", filepath.Join(src, "link")); err != nil {
		t.Fatal(err)
	}
	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dst, "sub", "f.txt"))
	if err != nil || string(data) != "hi" {
		t.Fatalf("file not copied: %v %q", err, data)
	}
	target, err := os.Readlink(filepath.Join(dst, "link"))
	if err != nil || target != "sub/f.txt" {
		t.Fatalf("symlink not preserved: %v %q", err, target)
	}
}

// Permission bits carry: the pipeline copies executable scripts (bin/, hooks)
// between build trees, and a copy that flattened the mode would produce an
// artifact whose entrypoints cannot run.
func TestCopyDir_PreservesMode(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "run.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "data.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}
	for name, want := range map[string]os.FileMode{"run.sh": 0o755, "data.json": 0o600} {
		info, err := os.Stat(filepath.Join(dst, name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s mode = %o, want %o", name, got, want)
		}
	}
}

// A dangling symlink must copy as a dangling symlink rather than fail the
// whole tree: the generator emits pb_hooks/pb_migrations as symlink farms, and
// a link whose target is not yet materialized is normal mid-assembly.
func TestCopyDir_DanglingSymlink(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	if err := os.Symlink("does/not/exist", filepath.Join(src, "dangling")); err != nil {
		t.Fatal(err)
	}
	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir with a dangling symlink: %v", err)
	}
	target, err := os.Readlink(filepath.Join(dst, "dangling"))
	if err != nil || target != "does/not/exist" {
		t.Fatalf("dangling symlink not preserved: %v %q", err, target)
	}
}

// The regression this function exists for: CopyDir must not try to reproduce
// the SOURCE's ownership on the destination. `cp -a` did, which fails with
// EINVAL inside the builder's user namespace (the pre-fetched member tree is
// root-owned and root is unmapped there). Asserting the destination is owned
// by the copying process — not the source uid — pins the behavior without
// needing a namespace to reproduce it in.
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
