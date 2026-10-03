//go:build unix

package listeners

import (
	"bufio"
	"net"
	"os"
	"os/exec"
	"strings"
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
	// an unexpected "invalid TINYCLD_LISTEN_FDS" warning would show up
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
