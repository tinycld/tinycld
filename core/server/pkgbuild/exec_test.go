package pkgbuild

import (
	"os"
	"path/filepath"
	"strings"
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
