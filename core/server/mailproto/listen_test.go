package mailproto

import (
	"bufio"
	"crypto/tls"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-smtp"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// The injection seam these tests pin: a host that supplies opts.Listen owns
// the socket, and mailproto must serve on exactly that listener and never
// fall back to binding the configured address. That is what lets a tenant
// process run IMAP/SMTP without binding a port (multi-org HANDOFF §6). Each
// test configures an address that CANNOT be bound — so any regression back to
// the internal bind fails loudly instead of quietly binding a port.

const unbindableAddr = "203.0.113.1:1" // TEST-NET-3: never routable/bindable locally

type nopIMAPSession struct{ imapserver.SessionIMAP4rev2 }

func newTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	return app
}

func dialAndReadLine(t *testing.T, ln net.Listener) string {
	t.Helper()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial injected listener: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("read greeting: %v", err)
	}
	return line
}

func TestStartIMAP_ServesOnInjectedListener(t *testing.T) {
	app := newTestApp(t)
	// The test app is production-mode, so this drives the TLS-only path — the
	// exact shape a router-managed tenant would use.
	dir := t.TempDir()
	certPath, keyPath := dir+"/cert.pem", dir+"/key.pem"
	writeCertPair(t, certPath, keyPath, "imap.test")
	t.Setenv("IMAP_TLS_CERT", certPath)
	t.Setenv("IMAP_TLS_KEY", keyPath)
	t.Setenv("IMAPS_ADDR", unbindableAddr)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	shutdown, err := StartIMAP(app, nil, IMAPOptions{
		NewSession: func(_ core.App, _ *imapserver.Conn) imapserver.Session {
			return &nopIMAPSession{}
		},
		Listen: func(addr string) (net.Listener, error) {
			if addr != unbindableAddr {
				t.Errorf("Listen called with %q, want the configured address %q", addr, unbindableAddr)
			}
			return ln, nil
		},
	})
	if err != nil {
		t.Fatalf("StartIMAP with an injected listener must not bind the address itself: %v", err)
	}
	t.Cleanup(shutdown)

	// TLS is still wrapped by mailproto around the injected (raw) listener.
	conn, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("tls dial injected listener: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	greeting, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("read greeting: %v", err)
	}
	if !strings.HasPrefix(greeting, "* OK") {
		t.Fatalf("IMAP greeting = %q", greeting)
	}
}

type nopSMTPBackend struct{}

func (nopSMTPBackend) NewSession(_ *smtp.Conn) (smtp.Session, error) {
	return nil, smtp.ErrAuthRequired
}

func TestStartSMTP_ServesOnInjectedListener(t *testing.T) {
	app := newTestApp(t)
	t.Setenv("TEST_SMTP_ADDR", unbindableAddr)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	shutdown, err := StartSMTP(app, nil, SMTPOptions{
		Backend:     nopSMTPBackend{},
		Label:       "test SMTP",
		AddrEnv:     "TEST_SMTP_ADDR",
		TLSAddrEnv:  "TEST_SMTPS_ADDR",
		DefaultAddr: unbindableAddr,
		Listen: func(addr string) (net.Listener, error) {
			return ln, nil
		},
	})
	if err != nil {
		t.Fatalf("StartSMTP with an injected listener must not bind the address itself: %v", err)
	}
	t.Cleanup(shutdown)

	if greeting := dialAndReadLine(t, ln); !strings.HasPrefix(greeting, "220 ") {
		t.Fatalf("SMTP greeting = %q", greeting)
	}
}

// Without injection the configured address is still bound directly — the
// single-tenant app's path is unchanged.
func TestStartSMTP_DefaultPathStillBinds(t *testing.T) {
	app := newTestApp(t)
	t.Setenv("TEST_SMTP_ADDR", "127.0.0.1:0")

	shutdown, err := StartSMTP(app, nil, SMTPOptions{
		Backend:     nopSMTPBackend{},
		Label:       "test SMTP",
		AddrEnv:     "TEST_SMTP_ADDR",
		TLSAddrEnv:  "TEST_SMTPS_ADDR",
		DefaultAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("StartSMTP default path: %v", err)
	}
	shutdown()
}
