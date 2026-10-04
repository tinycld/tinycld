//go:build unix

package listeners

import (
	"bufio"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("LISTENERS_TEST_CHILD") == "1" {
		childMain()
		return
	}
	if os.Getenv("LISTENERS_TEST_EMPTY_SET") == "1" {
		emptySetChildMain()
		return
	}
	if os.Getenv("LISTENERS_TEST_CONTROL") == "1" {
		controlChildMain()
		return
	}
	if os.Getenv("LISTENERS_TEST_EXTRA") == "1" {
		extraChildMain()
		return
	}
	if os.Getenv("LISTENERS_TEST_CLOEXEC") == "1" {
		cloexecChildMain()
		return
	}
	if os.Getenv("LISTENERS_TEST_CLOSE") == "1" {
		closeChildMain()
		return
	}
	os.Exit(m.Run())
}

// childMain accepts one connection on each named listener and echoes its name.
func childMain() {
	for _, name := range strings.Split(os.Getenv("LISTENERS_TEST_NAMES"), ",") {
		l, ok := Inherited(name)
		if !ok {
			os.Stdout.WriteString("missing " + name + "\n")
			os.Exit(2)
		}
		go func(name string, l net.Listener) {
			c, err := l.Accept()
			if err == nil {
				c.Write([]byte(name + "\n"))
				c.Close()
			}
		}(name, l)
	}
	os.Stdout.WriteString("ok\n")
	time.Sleep(5 * time.Second)
}

// emptySetChildMain runs with TINYCLD_LISTEN_FDS=0 (a Set with no
// listeners applied it): that's a valid "nothing inherited", not a parse
// error, so Supervised must be false and no name must resolve.
func emptySetChildMain() {
	if Supervised() {
		os.Stdout.WriteString("supervised\n")
		os.Exit(2)
	}
	if _, ok := Inherited("anything"); ok {
		os.Stdout.WriteString("found\n")
		os.Exit(2)
	}
	os.Stdout.WriteString("ok\n")
}

// controlChildMain gets one TCP listener and one end of a socketpair. The
// socketpair end must come back as an extra fd: net.FileListener accepts any
// stream socket, so a check that stops at "is it a socket" files it as a
// listener and the control channel is lost.
func controlChildMain() {
	if _, ok := Inherited("acme-secure"); !ok {
		os.Stdout.WriteString("no listener\n")
		os.Exit(2)
	}
	if _, ok := Inherited("control"); ok {
		os.Stdout.WriteString("control is a listener\n")
		os.Exit(2)
	}
	if _, ok := ExtraFD("control"); !ok {
		os.Stdout.WriteString("control is not an extra fd\n")
		os.Exit(2)
	}
	os.Stdout.WriteString("ok\n")
}

// extraChildMain writes through the extra fd named "control", so the parent
// can tell it is the very socket it passed under that name, not merely an fd
// filed under it.
func extraChildMain() {
	f, ok := ExtraFD("control")
	if !ok {
		os.Stdout.WriteString("no control\n")
		os.Exit(2)
	}
	if again, _ := ExtraFD("control"); again != f {
		os.Stdout.WriteString("a second lookup returned another file\n")
		os.Exit(2)
	}
	if _, ok := ExtraFD("acme-secure"); ok {
		os.Stdout.WriteString("the listener is an extra fd\n")
		os.Exit(2)
	}
	if _, err := f.WriteString("hello over control\n"); err != nil {
		os.Stdout.WriteString("write: " + err.Error() + "\n")
		os.Exit(2)
	}
	os.Stdout.WriteString("ok\n")
}

// cloexecChildMain starts a process of its own before it looks anything up,
// and prints which of fds 3 and 4 that process has open. An inherited fd
// must be close-on-exec from the start: a process that runs a command before
// its first Inherited or ExtraFD call must not pass the port on.
func cloexecChildMain() {
	out, err := exec.Command("sh", "-c",
		`for fd in 3 4; do if [ -e /dev/fd/$fd ]; then echo "$fd open"; else echo "$fd closed"; fi; done`).Output()
	if err != nil {
		os.Stdout.WriteString("sh: " + err.Error() + "\n")
		os.Exit(2)
	}
	os.Stdout.Write(out)
}

// closeChildMain closes its inherited listener, says so, and stays alive, so
// the parent can check that the port is no longer held by this process.
func closeChildMain() {
	l, ok := Inherited("acme-secure")
	if !ok {
		os.Stdout.WriteString("no listener\n")
		os.Exit(2)
	}
	if err := l.Close(); err != nil {
		os.Stdout.WriteString("close: " + err.Error() + "\n")
		os.Exit(2)
	}
	os.Stdout.WriteString("closed\n")
	time.Sleep(10 * time.Second)
}

// closeDups closes, at cleanup, every file set holds: AddListener keeps a
// dup of each listener, which the listener's own close does not reach.
// Files a test passed in are closed twice, which is harmless.
func closeDups(t *testing.T, set *Set) {
	t.Helper()
	t.Cleanup(func() {
		for _, f := range set.files {
			f.Close()
		}
	})
}

func startChild(t *testing.T, names ...string) (*exec.Cmd, *Set) {
	t.Helper()
	set := &Set{}
	for _, n := range names {
		l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { l.Close() })
		if err := set.AddListener(n, l); err != nil {
			t.Fatal(err)
		}
	}
	closeDups(t, set)
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "LISTENERS_TEST_CHILD=1", "LISTENERS_TEST_NAMES="+strings.Join(names, ","))
	set.Apply(cmd)
	return cmd, set
}

func TestChildServesOnInheritedListenersByName(t *testing.T) {
	cmd, set := startChild(t, "acme-secure", "acme-inbound")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	line, _ := bufio.NewReader(out).ReadString('\n')
	if line != "ok\n" {
		t.Fatalf("child said %q", line)
	}
	for _, name := range []string{"acme-secure", "acme-inbound"} {
		c, err := net.DialTimeout("tcp", set.Addr(name).String(), 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		c.SetReadDeadline(time.Now().Add(5 * time.Second))
		got, _ := bufio.NewReader(c).ReadString('\n')
		c.Close()
		if got != name+"\n" {
			t.Fatalf("listener %s answered %q", name, got)
		}
	}
}

func TestSocketpairEndIsAnExtraFDNotAListener(t *testing.T) {
	l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	childEnd := os.NewFile(uintptr(fds[0]), "control-child")
	parentEnd := os.NewFile(uintptr(fds[1]), "control-parent")
	t.Cleanup(func() { childEnd.Close(); parentEnd.Close() })

	set := &Set{}
	if err := set.AddListener("acme-secure", l); err != nil {
		t.Fatal(err)
	}
	set.AddFile("control", childEnd)
	closeDups(t, set)
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "LISTENERS_TEST_CONTROL=1")
	set.Apply(cmd)

	out, err := cmd.Output()
	if err != nil || string(out) != "ok\n" {
		t.Fatalf("child said %q, err %v", out, err)
	}
}

func TestInheritedMissingNameIsFalse(t *testing.T) {
	if _, ok := Inherited("nope"); ok {
		t.Fatal("found a listener that was never passed")
	}
	if Supervised() {
		t.Fatal("Supervised() true with no env")
	}
}

// TestEmptySetIsNotASupervisorError pins EnvFDs=0 (what an empty Set
// applies) as a valid "nothing to inherit", not a parse error: it must
// not warn, and Supervised() must read false in the child.
func TestEmptySetIsNotASupervisorError(t *testing.T) {
	set := &Set{}
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "LISTENERS_TEST_EMPTY_SET=1")
	set.Apply(cmd)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child failed: %v, output %q", err, out)
	}
	// childMain's own stdout is just "ok\n"; the default slog handler
	// writes any Warn to stderr, which CombinedOutput also captures, so
	// an unexpected "invalid inherited fd count" warning would show up
	// here even though it doesn't affect the child's exit code.
	if strings.Contains(string(out), "invalid") {
		t.Fatalf("EnvFDs=0 logged a warning: %q", out)
	}
	if string(out) != "ok\n" {
		t.Fatalf("child said %q", out)
	}
}

// SetForTest is the seam a feature package's server tests use in place of
// a real supervisor. It must make Inherited return the given listeners and
// Supervised() true while set, then restore prior state exactly.
func TestSetForTestOverridesAndRestores(t *testing.T) {
	l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	restore := SetForTest(map[string]net.Listener{"acme-secure": l})
	if !Supervised() {
		t.Fatal("Supervised() false with SetForTest listeners")
	}
	got, ok := Inherited("acme-secure")
	if !ok || got != l {
		t.Fatalf("Inherited(acme-secure) = %v %v, want %v true", got, ok, l)
	}
	if _, ok := Inherited("nope"); ok {
		t.Fatal("found a listener that was never set")
	}
	restore()

	if Supervised() {
		t.Fatal("Supervised() true after restore with no env")
	}
	if _, ok := Inherited("acme-secure"); ok {
		t.Fatal("Inherited still sees listener after restore")
	}
}

func TestSetFilesForTestOverridesAndRestores(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	restore := SetFilesForTest(map[string]*os.File{"control": f})
	if got, ok := ExtraFD("control"); !ok || got != f {
		t.Fatalf("ExtraFD(control) = %v %v, want %v true", got, ok, f)
	}
	if Supervised() {
		t.Fatal("SetFilesForTest alone must not make the process supervised")
	}
	restore()

	if _, ok := ExtraFD("control"); ok {
		t.Fatal("ExtraFD still sees the file after restore")
	}
}

// Apply must hand the files over in the order they were added, with each
// name at the same position, whatever mix of listeners and plain files.
func TestSetApplyKeepsFilesAndNamesInOrder(t *testing.T) {
	l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	first, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { first.Close() })
	last, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { last.Close() })

	set := &Set{}
	set.AddFile("control", first)
	if err := set.AddListener("acme-secure", l); err != nil {
		t.Fatal(err)
	}
	set.AddFile("acme-spare", last)
	closeDups(t, set)
	cmd := exec.Command("true")
	cmd.Env = []string{"KEEP=1"}
	set.Apply(cmd)

	if len(cmd.ExtraFiles) != 3 || cmd.ExtraFiles[0] != first || cmd.ExtraFiles[2] != last {
		t.Fatalf("ExtraFiles = %v, want control, the listener's dup, acme-spare", cmd.ExtraFiles)
	}
	want := []string{"KEEP=1", EnvFDs + "=3", EnvFDNames + "=control:acme-secure:acme-spare"}
	if strings.Join(cmd.Env, " ") != strings.Join(want, " ") {
		t.Fatalf("Env = %q, want %q", cmd.Env, want)
	}
	if set.Addr("control") != nil {
		t.Fatal("a plain file has an address")
	}
}

// A file added with AddFile must reach the child as the same open file,
// under its name, beside a listener.
func TestExtraFDCarriesTheFileItWasGiven(t *testing.T) {
	l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	childEnd := os.NewFile(uintptr(fds[0]), "control-child")
	parentEnd := os.NewFile(uintptr(fds[1]), "control-parent")
	t.Cleanup(func() { childEnd.Close(); parentEnd.Close() })

	set := &Set{}
	if err := set.AddListener("acme-secure", l); err != nil {
		t.Fatal(err)
	}
	set.AddFile("control", childEnd)
	closeDups(t, set)
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "LISTENERS_TEST_EXTRA=1")
	set.Apply(cmd)

	out, err := cmd.Output()
	if err != nil || string(out) != "ok\n" {
		t.Fatalf("child said %q, err %v", out, err)
	}
	// With the child gone and this copy closed, a read that finds nothing
	// ends at EOF rather than waiting.
	childEnd.Close()
	parentEnd.SetReadDeadline(time.Now().Add(5 * time.Second))
	got, err := bufio.NewReader(parentEnd).ReadString('\n')
	if err != nil || got != "hello over control\n" {
		t.Fatalf("read %q, err %v from the parent's end", got, err)
	}
}

func TestInheritedFDsAreCloseOnExecBeforeFirstUse(t *testing.T) {
	l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	childEnd := os.NewFile(uintptr(fds[0]), "control-child")
	parentEnd := os.NewFile(uintptr(fds[1]), "control-parent")
	t.Cleanup(func() { childEnd.Close(); parentEnd.Close() })

	set := &Set{}
	if err := set.AddListener("acme-secure", l); err != nil {
		t.Fatal(err)
	}
	set.AddFile("control", childEnd)
	closeDups(t, set)
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "LISTENERS_TEST_CLOEXEC=1")
	set.Apply(cmd)

	out, err := cmd.Output()
	if err != nil || string(out) != "3 closed\n4 closed\n" {
		t.Fatalf("child's own child had inherited fds: %q, err %v", out, err)
	}
}

// Closing an inherited listener must release the socket in this process at
// once. The inherited fd itself must not stay open behind the listener
// (until a garbage collection closes it), or the port keeps listening with
// nobody to accept and clients wait in its backlog.
func TestClosingAnInheritedListenerReleasesTheSocket(t *testing.T) {
	l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	set := &Set{}
	if err := set.AddListener("acme-secure", l); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "LISTENERS_TEST_CLOSE=1")
	set.Apply(cmd)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	// The parent's own handles go now: only the child can hold the port.
	l.Close()
	for _, f := range set.files {
		f.Close()
	}

	line, _ := bufio.NewReader(out).ReadString('\n')
	if line != "closed\n" {
		t.Fatalf("child said %q", line)
	}
	if c, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
		c.Close()
		t.Fatalf("%s still accepts connections after the child closed its inherited listener", addr)
	}
}
