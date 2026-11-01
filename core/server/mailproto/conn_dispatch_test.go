package mailproto

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-smtp"
	"github.com/pocketbase/pocketbase/core"
)

// loginSession accepts LOGIN for a user of the app it was built for, and
// nothing else. It stands in for mail's session, which speaks mail's schema.
type loginSession struct {
	imapserver.SessionIMAP4rev2
	app core.App
}

func (s *loginSession) Login(username, password string) error {
	user, err := s.app.FindAuthRecordByEmail("users", username)
	if err != nil || !user.ValidatePassword(password) {
		return imapserver.ErrAuthFailed
	}
	return nil
}

func (s *loginSession) Close() error { return nil }

func startDispatchedIMAP(t *testing.T, app core.App) func() {
	t.Helper()
	shutdown, err := StartIMAP(app, nil, IMAPOptions{
		NewSession: func(app core.App, _ *imapserver.Conn) imapserver.Session {
			return &loginSession{app: app}
		},
		Listen:      Dispatched,
		ExternalTLS: true,
	})
	if err != nil {
		t.Fatalf("StartIMAP: %v", err)
	}
	t.Cleanup(shutdown)
	return shutdown
}

func createUser(t *testing.T, app core.App, email, password string) {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	u := core.NewRecord(users)
	u.SetEmail(email)
	u.SetPassword(password)
	if err := app.Save(u); err != nil {
		t.Fatalf("save user: %v", err)
	}
}

// connPair returns both ends of one loopback TCP connection: the client end
// for the test, and the server end as an embedder's dispatcher holds it.
func connPair(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			close(accepted)
			return
		}
		accepted <- c
	}()
	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	server, ok := <-accepted
	if !ok {
		t.Fatal("loopback accept failed")
	}
	t.Cleanup(func() {
		client.Close()
		server.Close()
	})
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	return client, server
}

// dispatchedIMAPLogin hands a new connection to app's IMAP server, sends LOGIN
// and returns the tagged reply.
func dispatchedIMAPLogin(t *testing.T, app core.App, user, password string) string {
	t.Helper()
	client, server := connPair(t)
	if err := ServeIMAPConn(app, server); err != nil {
		t.Fatalf("ServeIMAPConn: %v", err)
	}
	r := bufio.NewReader(client)
	if greeting, err := r.ReadString('\n'); err != nil || !strings.HasPrefix(greeting, "* OK") {
		t.Fatalf("greeting = %q, %v", greeting, err)
	}
	if _, err := fmt.Fprintf(client, "a1 LOGIN %q %q\r\n", user, password); err != nil {
		t.Fatal(err)
	}
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read LOGIN reply: %v", err)
		}
		if strings.HasPrefix(line, "a1 ") {
			return line
		}
	}
}

func TestServeIMAPConn_LoginReachesOnlyTheGivenApp(t *testing.T) {
	appA, appB := newTestApp(t), newTestApp(t)
	createUser(t, appA, "only-a@example.com", "password-of-a")
	startDispatchedIMAP(t, appA)
	startDispatchedIMAP(t, appB)

	if reply := dispatchedIMAPLogin(t, appA, "only-a@example.com", "password-of-a"); !strings.HasPrefix(reply, "a1 OK") {
		t.Fatalf("LOGIN on app A = %q, want OK", reply)
	}
	if reply := dispatchedIMAPLogin(t, appB, "only-a@example.com", "password-of-a"); !strings.HasPrefix(reply, "a1 NO") {
		t.Fatalf("LOGIN on app B = %q, want NO: app A's user reached app B", reply)
	}
}

func TestServeIMAPConn_WithoutAServerIsRefused(t *testing.T) {
	app := newTestApp(t)
	_, server := connPair(t)
	if err := ServeIMAPConn(app, server); !errors.Is(err, ErrNotServing) {
		t.Fatalf("ServeIMAPConn on an app with no IMAP server = %v, want ErrNotServing", err)
	}
}

func TestServeIMAPConn_RefusedOnceTheServerStops(t *testing.T) {
	app := newTestApp(t)
	shutdown := startDispatchedIMAP(t, app)
	shutdown()

	_, server := connPair(t)
	if err := ServeIMAPConn(app, server); !errors.Is(err, ErrNotServing) {
		t.Fatalf("ServeIMAPConn after the stop = %v, want ErrNotServing", err)
	}
}

// A process that holds many apps opens and closes them at any time, so a stop
// can come before the server's serve loop has started. go-imap's Close waits
// for the serve loops it counts, and Serve counts itself after it registers
// its listener, with no lock between them: closing the server before the loop
// has returned races (run with -race).
func TestStartIMAP_StopRightAfterStart(t *testing.T) {
	app := newTestApp(t)
	for range 50 {
		shutdown, err := StartIMAP(app, nil, IMAPOptions{
			NewSession: func(app core.App, _ *imapserver.Conn) imapserver.Session {
				return &loginSession{app: app}
			},
			Listen:      Dispatched,
			ExternalTLS: true,
		})
		if err != nil {
			t.Fatalf("StartIMAP: %v", err)
		}
		shutdown()
	}
}

// namedBackend refuses every session with its own name, so a reply tells
// which server answered.
type namedBackend string

func (b namedBackend) NewSession(_ *smtp.Conn) (smtp.Session, error) {
	return nil, &smtp.SMTPError{Code: 554, EnhancedCode: smtp.EnhancedCode{5, 0, 0}, Message: "served by " + string(b)}
}

func startDispatchedSMTP(t *testing.T, app core.App, name string) {
	t.Helper()
	shutdown, err := StartSMTP(app, nil, SMTPOptions{
		Backend:        namedBackend("submission " + name),
		Label:          "test SMTP",
		TLSAddrEnv:     "TEST_SMTPS_ADDR",
		DefaultTLSAddr: ":465",
		Listen:         Dispatched,
		ExternalTLS:    true,
	})
	if err != nil {
		t.Fatalf("StartSMTP: %v", err)
	}
	t.Cleanup(shutdown)

	ln, err := Dispatched(":25")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ServeSMTP(app, ServiceMX, smtp.NewServer(namedBackend("mx "+name)), ln, "test MX"))
}

// dispatchedEHLO hands a new connection over through serve, sends EHLO and
// returns the reply line.
func dispatchedEHLO(t *testing.T, serve func(net.Conn) error) string {
	t.Helper()
	client, server := connPair(t)
	if err := serve(server); err != nil {
		t.Fatalf("hand-off: %v", err)
	}
	r := bufio.NewReader(client)
	if greeting, err := r.ReadString('\n'); err != nil || !strings.HasPrefix(greeting, "220 ") {
		t.Fatalf("greeting = %q, %v", greeting, err)
	}
	if _, err := fmt.Fprintf(client, "EHLO client.test\r\n"); err != nil {
		t.Fatal(err)
	}
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("read EHLO reply: %v", err)
	}
	return line
}

func TestServeSubmissionConnAndServeMXConn_ReachOnlyTheGivenApp(t *testing.T) {
	appA, appB := newTestApp(t), newTestApp(t)
	startDispatchedSMTP(t, appA, "A")
	startDispatchedSMTP(t, appB, "B")

	for _, tc := range []struct {
		app   core.App
		serve func(core.App, net.Conn) error
		want  string
	}{
		{appA, ServeSubmissionConn, "served by submission A"},
		{appB, ServeSubmissionConn, "served by submission B"},
		{appA, ServeMXConn, "served by mx A"},
		{appB, ServeMXConn, "served by mx B"},
	} {
		reply := dispatchedEHLO(t, func(c net.Conn) error { return tc.serve(tc.app, c) })
		if !strings.Contains(reply, tc.want) {
			t.Errorf("EHLO reply = %q, want it from %q", reply, tc.want)
		}
	}
}
