package pkgbuild

import (
	"bufio"
	"io"
	"log"
	"os"
	"os/exec"
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

// CopyDir copies src's contents into dst, preserving attributes and symlinks
// (cp -a semantics — pnpm workspace trees rely on both).
func CopyDir(src, dst string) error {
	_, err := RunCmd(".", "cp", "-a", src+"/.", dst+"/")
	return err
}
