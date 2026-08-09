package mailproto

import (
	"crypto/tls"
	"net"
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-smtp"
	"github.com/pocketbase/pocketbase/core"
)

// In DEV, the implicit-TLS listener is a convenience and the plain one is what
// clients actually use. A dev machine commonly has another local server
// already holding :1993 / :1587 — those two ports are not parameterized the
// way IMAP_ADDR is — and the old code closed the plain listener and returned
// an error when the TLS bind failed. The symptom was baffling: the log said
// "IMAP server listening" and every client still got ECONNREFUSED on a port
// that HAD just bound. It made all eight mail IMAP e2e specs fail whenever a
// dev server was running.
//
// These call the dev entry points directly: IsDev comes from unexported
// PocketBase config, so a tests.TestApp cannot be flipped into dev mode.

// listenOnceThenFail hands out `ln` for the first address and refuses every
// later one — standing in for a TLS port another process already holds.
func listenOnceThenFail(ln net.Listener) (ListenFunc, *int) {
	calls := 0
	return func(addr string) (net.Listener, error) {
		calls++
		if calls == 1 {
			return ln, nil
		}
		return nil, &net.OpError{
			Op:   "listen",
			Net:  "tcp",
			Addr: nil,
			Err:  net.UnknownNetworkError("address already in use"),
		}
	}, &calls
}

func TestStartIMAPDev_KeepsPlainListenerWhenTLSPortIsTaken(t *testing.T) {
	app := newTestApp(t)

	dir := t.TempDir()
	certPath, keyPath := dir+"/cert.pem", dir+"/key.pem"
	writeCertPair(t, certPath, keyPath, "imap.test")
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	listen, calls := listenOnceThenFail(ln)
	shutdown, err := startIMAPDev(app, tlsConfig, IMAPOptions{
		NewSession: func(_ core.App, _ *imapserver.Conn) imapserver.Session {
			return &nopIMAPSession{}
		},
		Listen: listen,
	})
	if err != nil {
		t.Fatalf("a taken IMAPS port must not fail the whole IMAP server: %v", err)
	}
	t.Cleanup(shutdown)

	if *calls != 2 {
		t.Fatalf("Listen called %d times, want 2 (plain, then the failing TLS port)", *calls)
	}

	// The plain listener is still serving — this is the assertion the old
	// code failed, having closed it on the way out.
	if greeting := dialAndReadLine(t, ln); !strings.HasPrefix(greeting, "* OK") {
		t.Fatalf("IMAP greeting = %q, want a plain listener still serving", greeting)
	}
}

func TestStartSMTPDev_KeepsPlainListenerWhenTLSPortIsTaken(t *testing.T) {
	app := newTestApp(t)
	t.Setenv("TEST_SMTP_ADDR", unbindableAddr)

	dir := t.TempDir()
	certPath, keyPath := dir+"/cert.pem", dir+"/key.pem"
	writeCertPair(t, certPath, keyPath, "smtp.test")
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	listen, calls := listenOnceThenFail(ln)
	shutdown, err := startSMTPDev(app, tlsConfig, SMTPOptions{
		Backend:           nopSMTPBackend{},
		Label:             "test SMTP",
		AddrEnv:           "TEST_SMTP_ADDR",
		TLSAddrEnv:        "TEST_SMTPS_ADDR",
		DefaultAddr:       unbindableAddr,
		DefaultDevTLSAddr: unbindableAddr,
		Listen:            listen,
	})
	if err != nil {
		t.Fatalf("a taken submission-TLS port must not fail the whole SMTP server: %v", err)
	}
	t.Cleanup(shutdown)

	if *calls != 2 {
		t.Fatalf("Listen called %d times, want 2 (plain, then the failing TLS port)", *calls)
	}

	if greeting := dialAndReadLine(t, ln); !strings.HasPrefix(greeting, "220") {
		t.Fatalf("SMTP greeting = %q, want a plain listener still serving", greeting)
	}
}

// Guard rail: the dev leniency above must not leak into production. A missing
// TLS source there still refuses to boot rather than quietly serving plain.
func TestStartIMAP_ProductionWithoutTLSStillRefuses(t *testing.T) {
	app := newTestApp(t) // tests.TestApp is production-mode
	t.Setenv("IMAP_TLS_CERT", "")
	t.Setenv("IMAP_TLS_KEY", "")

	_, err := StartIMAP(app, nil, IMAPOptions{
		NewSession: func(_ core.App, _ *imapserver.Conn) imapserver.Session {
			return &nopIMAPSession{}
		},
		Listen: func(string) (net.Listener, error) {
			t.Fatal("production without TLS must not reach a listener at all")
			return nil, nil
		},
	})
	if err == nil {
		t.Fatal("production without a TLS source must refuse to start IMAP")
	}
	if !strings.Contains(err.Error(), "IMAPS") {
		t.Fatalf("error should name the misconfiguration, got: %v", err)
	}
}

var _ = smtp.ErrAuthRequired
