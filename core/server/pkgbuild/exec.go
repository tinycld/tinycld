package pkgbuild

import (
	"bufio"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CmdRunner runs a command in dir and returns its combined output. It is the
// sandbox seam for buffered steps: coreserver binds it to RunCmd (in-process
// exec); the multi-org builder wraps it in per-job confinement.
type CmdRunner func(dir, name string, args ...string) (string, error)

// StreamingRunner is CmdRunner for long steps whose output should reach the
// ProgressSink line-by-line as it arrives (pnpm install, expo export).
type StreamingRunner func(onLine func(line string), dir, name string, args ...string) (string, error)

// LogPrefix tags every command echo in the process log. The default keeps the
// single-tenant server's historical "[pkg_install]" prefix so operator `docker
// logs` greps keep working; the multi-org builder sets its own at startup
// (write-once, before jobs run — it is not synchronized).
var LogPrefix = "[pkg_install]"

// RunCmd runs a command, capturing its combined output to return to the
// caller (which surfaces it via its ProgressSink / install log) AND echoing
// the command line and its output to the process log so `docker logs` shows
// the full install trace — including the real npm/pnpm/go/expo errors that
// would otherwise be buried in the SSE stream / DB record only.
func RunCmd(dir string, name string, args ...string) (string, error) {
	return RunCmdEnv(dir, nil, name, args...)
}

// RunCmdEnv is RunCmd with extra environment entries ("KEY=VALUE") appended to
// the inherited env. Use this to pass SECRETS to a subprocess: the extra env is
// NOT logged (only the command + args are), so a value like a Sentry auth token
// never lands in the build log — unlike threading it through args.
func RunCmdEnv(dir string, extraEnv []string, name string, args ...string) (string, error) {
	log.Printf("%s $ (cd %s && %s %s)", LogPrefix, dir, name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	out, err := cmd.CombinedOutput()
	if s := strings.TrimRight(string(out), "\n"); s != "" {
		log.Printf("%s output of %s:\n%s", LogPrefix, name, s)
	}
	if err != nil {
		log.Printf("%s %s FAILED: %v", LogPrefix, name, err)
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
	log.Printf("%s $ (cd %s && %s %s)", LogPrefix, dir, name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if s := strings.TrimRight(string(out), "\n"); s != "" {
		log.Printf("%s output of %s:\n%s", LogPrefix, name, s)
	}
	if s := strings.TrimSpace(stderr.String()); s != "" {
		log.Printf("%s stderr of %s:\n%s", LogPrefix, name, s)
	}
	if err != nil {
		log.Printf("%s %s FAILED: %v", LogPrefix, name, err)
		// The caller surfaces the error; stderr carries npm's actual reason.
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
	log.Printf("%s $ (cd %s && %s %s)", LogPrefix, dir, name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Dir = dir

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
				onLine(line)
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
		log.Printf("%s output of %s:\n%s", LogPrefix, name, s)
	}
	if err != nil {
		log.Printf("%s %s FAILED: %v", LogPrefix, name, err)
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
