package mailproto

import (
	"bufio"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/pocketbase/pocketbase/core"
)

// The external-TLS contract these tests pin: when the host declares it
// terminates TLS itself (the hosting router holds the wildcard cert and
// hands the tenant plaintext over a private unix socket), mailproto must
//   - start with NO cert material configured, even in production mode,
//   - serve exactly the injected listener,
//   - allow authentication over the plaintext transport (the public hop was
//     TLS; the plaintext hop is a router-owned unix socket), and
//   - advertise no STARTTLS (there is no TLS to start).
// A regression to the TLS-only production path would refuse to boot every
// router-managed tenant; a regression to LOGINDISABLED/no-AUTH would lock
// every mail client out.

// extTLSIMAPSession is a nop session with a real Close, so tearing the
// connection down doesn't panic through the nil embedded interface.
type extTLSIMAPSession struct{ imapserver.SessionIMAP4rev2 }

func (*extTLSIMAPSession) Close() error { return nil }

// authStubSMTPSession advertises PLAIN auth — go-smtp only includes AUTH in
// the EHLO capability list when the session implements AuthSession, and the
// AUTH advertisement is exactly what these tests assert.
type authStubSMTPSession struct{}

func (authStubSMTPSession) Reset()                               {}
func (authStubSMTPSession) Logout() error                        { return nil }
func (authStubSMTPSession) Mail(string, *smtp.MailOptions) error { return smtp.ErrAuthRequired }
func (authStubSMTPSession) Rcpt(string, *smtp.RcptOptions) error { return smtp.ErrAuthRequired }
func (authStubSMTPSession) Data(io.Reader) error                 { return smtp.ErrAuthRequired }
func (authStubSMTPSession) AuthMechanisms() []string             { return []string{sasl.Plain} }
func (authStubSMTPSession) Auth(string) (sasl.Server, error) {
	return nil, smtp.ErrAuthUnknownMechanism
}

type authStubSMTPBackend struct{}

func (authStubSMTPBackend) NewSession(*smtp.Conn) (smtp.Session, error) {
	return authStubSMTPSession{}, nil
}

// readIMAPUntilTagged reads lines until the response tagged `tag` arrives,
// returning everything read.
func readIMAPUntilTagged(t *testing.T, r *bufio.Reader, tag string) string {
	t.Helper()
	var b strings.Builder
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read IMAP response: %v (got so far: %q)", err, b.String())
		}
		b.WriteString(line)
		if strings.HasPrefix(line, tag+" ") {
			return b.String()
		}
	}
}

func TestStartIMAP_ExternalTLSAllowsAuthWithoutCerts(t *testing.T) {
	app := newTestApp(t) // production mode: without ExternalTLS this path refuses to start
	t.Setenv("IMAPS_ADDR", unbindableAddr)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	shutdown, err := StartIMAP(app, nil, IMAPOptions{
		NewSession: func(_ core.App, _ *imapserver.Conn) imapserver.Session {
			return &extTLSIMAPSession{}
		},
		Listen:      func(string) (net.Listener, error) { return ln, nil },
		ExternalTLS: true,
	})
	if err != nil {
		t.Fatalf("StartIMAP with ExternalTLS must start without cert material: %v", err)
	}
	t.Cleanup(shutdown)

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial injected listener: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	r := bufio.NewReader(conn)

	greeting, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("read greeting: %v", err)
	}
	if !strings.HasPrefix(greeting, "* OK") {
		t.Fatalf("IMAP greeting over plaintext = %q", greeting)
	}

	if _, err := conn.Write([]byte("a1 CAPABILITY\r\n")); err != nil {
		t.Fatal(err)
	}
	caps := readIMAPUntilTagged(t, r, "a1")
	if strings.Contains(caps, "LOGINDISABLED") {
		t.Fatalf("external-TLS listener must allow auth over plaintext, got %q", caps)
	}
	if strings.Contains(caps, "STARTTLS") {
		t.Fatalf("external-TLS listener must not advertise STARTTLS, got %q", caps)
	}
}

func TestStartIMAP_ExternalTLSRequiresInjectedListener(t *testing.T) {
	app := newTestApp(t)
	_, err := StartIMAP(app, nil, IMAPOptions{
		NewSession: func(_ core.App, _ *imapserver.Conn) imapserver.Session {
			return &nopIMAPSession{}
		},
		ExternalTLS: true,
	})
	if err == nil {
		t.Fatal("ExternalTLS without an injected listener would plaintext-bind a public port; must refuse")
	}
}

func TestStartSMTP_ExternalTLSAllowsAuthWithoutCerts(t *testing.T) {
	app := newTestApp(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	shutdown, err := StartSMTP(app, nil, SMTPOptions{
		Backend:            authStubSMTPBackend{},
		Label:              "test SMTP",
		AddrEnv:            "TEST_SMTP_ADDR",
		TLSAddrEnv:         "TEST_SMTPS_ADDR",
		DefaultTLSAddr:     unbindableAddr,
		DefaultAddr:        unbindableAddr,
		ProductionTLSError: "no TLS in production", // would refuse without ExternalTLS
		Listen:             func(string) (net.Listener, error) { return ln, nil },
		ExternalTLS:        true,
	})
	if err != nil {
		t.Fatalf("StartSMTP with ExternalTLS must start without cert material: %v", err)
	}
	t.Cleanup(shutdown)

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial injected listener: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	r := bufio.NewReader(conn)

	greeting, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("read greeting: %v", err)
	}
	if !strings.HasPrefix(greeting, "220 ") {
		t.Fatalf("SMTP greeting over plaintext = %q", greeting)
	}

	if _, err := conn.Write([]byte("EHLO client.example\r\n")); err != nil {
		t.Fatal(err)
	}
	var ehlo strings.Builder
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read EHLO response: %v (got so far: %q)", err, ehlo.String())
		}
		ehlo.WriteString(line)
		if strings.HasPrefix(line, "250 ") || !strings.HasPrefix(line, "250") {
			break
		}
	}
	resp := ehlo.String()
	if !strings.Contains(resp, "AUTH") {
		t.Fatalf("external-TLS listener must offer AUTH over plaintext, got %q", resp)
	}
	if strings.Contains(resp, "STARTTLS") {
		t.Fatalf("external-TLS listener must not advertise STARTTLS, got %q", resp)
	}
}

func TestStartSMTP_ExternalTLSRequiresInjectedListener(t *testing.T) {
	app := newTestApp(t)
	_, err := StartSMTP(app, nil, SMTPOptions{
		Backend:     nopSMTPBackend{},
		Label:       "test SMTP",
		AddrEnv:     "TEST_SMTP_ADDR",
		TLSAddrEnv:  "TEST_SMTPS_ADDR",
		DefaultAddr: unbindableAddr,
		ExternalTLS: true,
	})
	if err == nil {
		t.Fatal("ExternalTLS without an injected listener would plaintext-bind a public port; must refuse")
	}
}
