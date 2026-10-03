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

// SetForTest is the seam mail's server tests use in place of a real
// supervisor. It must make Inherited return the given listeners and
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
