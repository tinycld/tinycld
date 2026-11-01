package pkgbuild

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"tinycld.org/core/logging"
)

// log is pkgbuild's package-wide structured logger. No file in this package
// imports stdlib "log" any longer, so the short identifier is free to use.
var log = logging.ForPackage("pkgbuild")

// CmdRunner runs a command in dir and returns its combined output. It is the
// sandbox seam for buffered steps: coreserver binds it to RunCmd (in-process
// exec); a managed deployment's builder wraps it in per-job confinement.
type CmdRunner func(dir, name string, args ...string) (string, error)

// StreamingRunner is CmdRunner for long steps whose output should reach the
// ProgressSink line-by-line as it arrives (pnpm install, expo export).
type StreamingRunner func(onLine func(line string), dir, name string, args ...string) (string, error)

// CmdEnvRunner is CmdRunner with extra environment entries — the seam for
// steps that must scope env to one command (GOOS/GOARCH/CGO_ENABLED
// cross-compiles). Signature matches RunCmdEnv.
type CmdEnvRunner func(dir string, extraEnv []string, name string, args ...string) (string, error)

// RunCmd runs a command, capturing its combined output to return to the
// caller (which surfaces it via its ProgressSink / install log) AND echoing
// the command line and its output to the process log so `docker logs` shows
// the full install trace — including the real npm/pnpm/go/expo errors that
// would otherwise be buried in the SSE stream / DB record only.
func RunCmd(dir string, name string, args ...string) (string, error) {
	return RunCmdEnvContext(context.Background(), dir, nil, name, args...)
}

// RunCmdContext is RunCmd bound to ctx: when ctx ends, the command and every
// process it started are killed (see commandContext).
func RunCmdContext(ctx context.Context, dir string, name string, args ...string) (string, error) {
	return RunCmdEnvContext(ctx, dir, nil, name, args...)
}

// RunnerFor is the CmdRunner that runs each command bound to ctx.
func RunnerFor(ctx context.Context) CmdRunner {
	return func(dir, name string, args ...string) (string, error) {
		return RunCmdContext(ctx, dir, name, args...)
	}
}

// EnvRunnerFor is the CmdEnvRunner that runs each command bound to ctx.
func EnvRunnerFor(ctx context.Context) CmdEnvRunner {
	return func(dir string, extraEnv []string, name string, args ...string) (string, error) {
		return RunCmdEnvContext(ctx, dir, extraEnv, name, args...)
	}
}

// StreamingRunnerFor is the StreamingRunner that runs each command bound to
// ctx.
func StreamingRunnerFor(ctx context.Context) StreamingRunner {
	return func(onLine func(line string), dir, name string, args ...string) (string, error) {
		return RunCmdStreamingEnvContext(ctx, onLine, dir, nil, name, args...)
	}
}

// cmdWaitDelay is how long a killed command's output pipes may stay open
// before Wait stops reading them. The kill reaches the whole process group,
// so the pipes normally close at once; the delay only bounds a process that
// left the group and kept a pipe.
const cmdWaitDelay = 2 * time.Second

// commandContext is exec.Command bound to ctx. A build command starts
// children of its own (pnpm runs node, the generator, tsx; go runs the
// compiler), and exec.CommandContext kills only the direct child, which
// leaves its children running and holding the output pipes. So the command
// runs in a new process group, and the cancel kills that group (on unix).
//
// A ctx that can never end (context.Background) keeps the plain command in
// this process's group, so a terminal's Ctrl-C still reaches it.
func commandContext(ctx context.Context, name string, args ...string) *command {
	if ctx.Done() == nil {
		cmd := exec.Command(name, args...)
		return &command{Cmd: cmd, wait: cmd.Wait}
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = cmdWaitDelay
	return &command{Cmd: cmd, wait: killGroupOnCancel(cmd)}
}

// command is a build command from commandContext. Its Wait is the one
// killGroupOnCancel pairs with the cancel, so Run, Output and
// CombinedOutput are redefined here to wait through it; exec.Cmd's own
// would bypass it.
type command struct {
	*exec.Cmd
	wait func() error
}

func (c *command) Wait() error { return c.wait() }

func (c *command) Run() error {
	if err := c.Start(); err != nil {
		return err
	}
	return c.Wait()
}

func (c *command) CombinedOutput() ([]byte, error) {
	var b bytes.Buffer
	c.Stdout, c.Stderr = &b, &b
	err := c.Run()
	return b.Bytes(), err
}

// Output is exec.Cmd.Output: when Stderr is unset, stderr is kept in the
// *exec.ExitError a failure returns.
func (c *command) Output() ([]byte, error) {
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	keepStderr := c.Stderr == nil
	if keepStderr {
		c.Stderr = &stderr
	}
	err := c.Run()
	var exitErr *exec.ExitError
	if keepStderr && errors.As(err, &exitErr) {
		exitErr.Stderr = stderr.Bytes()
	}
	return stdout.Bytes(), err
}

// RunCmdEnv is RunCmd with extra environment entries ("KEY=VALUE") appended to
// the inherited env. Use this to pass SECRETS to a subprocess: the extra env is
// NOT logged (only the command + args are), so a value like a Sentry auth token
// never lands in the build log — unlike threading it through args.
func RunCmdEnv(dir string, extraEnv []string, name string, args ...string) (string, error) {
	return RunCmdEnvContext(context.Background(), dir, extraEnv, name, args...)
}

// RunCmdEnvContext is RunCmdEnv bound to ctx (see commandContext).
func RunCmdEnvContext(ctx context.Context, dir string, extraEnv []string, name string, args ...string) (string, error) {
	log.Debug("$ command", "dir", dir, "cmd", name, "args", strings.Join(args, " "))
	cmd := commandContext(ctx, name, args...)
	cmd.Dir = dir
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	out, err := cmd.CombinedOutput()
	if s := strings.TrimRight(string(out), "\n"); s != "" {
		log.Debug("command output", "cmd", name, "output", s)
	}
	if err != nil {
		// The caller's own step-level handling (e.g. coreserver's timeStep)
		// is what decides whether this failure escalates; logging it warn+
		// here would double-page the same error.
		log.Info("command failed", "cmd", name, "err", err)
	}
	return string(out), err
}

// RunCmdStdout is RunCmd for commands whose STDOUT is machine-parsed (e.g.
// `npm view … --json`): stdout comes back alone, while stderr still reaches
// the process log. RunCmd's combined capture poisons parsers — npm prints
// "npm warn config …" to stderr, and merged in front of the JSON it turned
// every version listing into a decode error whenever npm had anything to
// warn about (the hosted e2e's router cwd sat under a workspace .npmrc).
func RunCmdStdout(dir string, name string, args ...string) (string, error) {
	return RunCmdStdoutContext(context.Background(), dir, name, args...)
}

// RunCmdStdoutContext is RunCmdStdout bound to ctx (see commandContext).
func RunCmdStdoutContext(ctx context.Context, dir string, name string, args ...string) (string, error) {
	log.Debug("$ command", "dir", dir, "cmd", name, "args", strings.Join(args, " "))
	cmd := commandContext(ctx, name, args...)
	cmd.Dir = dir
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if s := strings.TrimRight(string(out), "\n"); s != "" {
		log.Debug("command output", "cmd", name, "output", s)
	}
	if s := strings.TrimSpace(stderr.String()); s != "" {
		log.Debug("command stderr", "cmd", name, "stderr", s)
	}
	if err != nil {
		// The caller surfaces the error (stderr carries npm's actual reason);
		// its own step-level handling decides whether this escalates.
		log.Info("command failed", "cmd", name, "err", err)
		return string(out), ErrFromCmd(name, stderr.String(), err)
	}
	return string(out), nil
}

// RunCmdStreaming is RunCmd that also invokes onLine for each line of combined
// output AS IT ARRIVES, instead of only after the command exits. Long steps
// (pnpm install) can forward their progress lines to the UI so the bar doesn't
// sit frozen for minutes. It still buffers + returns the full output and error
// so the buffered-RunCmd contract is preserved.
func RunCmdStreaming(onLine func(line string), dir, name string, args ...string) (string, error) {
	return RunCmdStreamingEnvContext(context.Background(), onLine, dir, nil, name, args...)
}

// RunCmdStreamingEnv is RunCmdStreaming with extra environment entries
// ("KEY=VALUE") appended to the inherited env — the streaming mirror of
// RunCmdEnv.
func RunCmdStreamingEnv(onLine func(line string), dir string, extraEnv []string, name string, args ...string) (string, error) {
	return RunCmdStreamingEnvContext(context.Background(), onLine, dir, extraEnv, name, args...)
}

// RunCmdStreamingEnvContext is RunCmdStreamingEnv bound to ctx (see
// commandContext).
func RunCmdStreamingEnvContext(ctx context.Context, onLine func(line string), dir string, extraEnv []string, name string, args ...string) (string, error) {
	log.Debug("$ command", "dir", dir, "cmd", name, "args", strings.Join(args, " "))
	cmd := commandContext(ctx, name, args...)
	cmd.Dir = dir
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	var buf strings.Builder
	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(pr)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			buf.WriteString(line)
			buf.WriteByte('\n')
			if onLine != nil {
				callOnLine(onLine, line)
			}
		}
	}()

	err := cmd.Start()
	if err == nil {
		err = cmd.Wait()
	}
	// Close the writer so the scanner goroutine sees EOF, then wait for it to
	// finish draining before reading the buffer.
	_ = pw.Close()
	<-done

	out := buf.String()
	if s := strings.TrimRight(out, "\n"); s != "" {
		log.Debug("command output", "cmd", name, "output", s)
	}
	if err != nil {
		// See RunCmdEnv: the caller's step-level handling is the escalation
		// point, so this stays below warn to avoid double-paging.
		log.Info("command failed", "cmd", name, "err", err)
	}
	return out, err
}

// CopyDir copies src's contents into dst, preserving mode, timestamps and
// symlinks (pnpm workspace trees rely on all three) but NOT ownership.
//
// This replaced a `cp -a`. `-a` implies --preserve=all, which makes cp chown
// every entry to the source's uid/gid — and that FAILS with EINVAL when the
// copy runs inside a user namespace that does not map the source owner. The
// confined builder is exactly that case: the job maps only its own uid, so a
// root-owned pre-fetched member tree is unmappable, every entry errors, and cp
// exits nonzero after a copy that otherwise succeeded. Ownership is not
// something a copy into a job's own workspace should carry anyway — the
// destination belongs to whoever runs the build.
//
// Done in Go rather than by re-flagging cp because the flag spellings diverge:
// GNU wants --preserve=mode,timestamps,links while BSD/macOS cp rejects it
// outright (exit 64), and BSD -p attempts ownership regardless. This runs the
// same everywhere and makes the ownership decision explicit.
func CopyDir(src, dst string) error {
	return copyTree(src, dst)
}

// copyTree recursively copies src into dst. Symlinks are recreated as symlinks
// (never followed), so a dangling link — the generator emits plenty — copies
// as a dangling link instead of failing.
func copyTree(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		info, err := e.Info()
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(srcPath)
			if err != nil {
				return err
			}
			if err := os.RemoveAll(dstPath); err != nil {
				return err
			}
			if err := os.Symlink(target, dstPath); err != nil {
				return err
			}
			continue // symlink mode/times belong to the target, not the link
		case e.IsDir():
			if err := copyTree(srcPath, dstPath); err != nil {
				return err
			}
		default:
			if err := copyFileMode(srcPath, dstPath, info); err != nil {
				return err
			}
		}
		if err := os.Chmod(dstPath, info.Mode().Perm()); err != nil {
			return err
		}
		if err := os.Chtimes(dstPath, info.ModTime(), info.ModTime()); err != nil {
			return err
		}
	}
	return nil
}

// copyFileMode copies one regular file, creating the destination with the
// source's permission bits. Distinct from nativeexport.go's copyFile, which
// fsyncs for OTA durability and does not carry mode.
func copyFileMode(src, dst string, info os.FileInfo) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// callOnLine runs a caller's line callback. A panic in it is logged and the
// scan goes on: the scanner must keep draining the pipe, or the subprocess
// blocks on its next write and the build never ends.
func callOnLine(onLine func(string), line string) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("command output callback panicked", "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
		}
	}()
	onLine(line)
}
