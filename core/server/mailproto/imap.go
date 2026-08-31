package mailproto

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/pocketbase/pocketbase/core"
	"golang.org/x/crypto/acme/autocert"

	"tinycld.org/core/logging"
)

var mailLog = logging.ForPackage("mailproto")

// NewIMAPSession builds the per-connection IMAP session. The session speaks the
// feature's own schema, so the feature package supplies it; mailproto only owns
// the listener around it.
type NewIMAPSession func(app core.App, conn *imapserver.Conn) imapserver.Session

// ListenFunc opens the socket a mail service serves on, given the address the
// service would otherwise bind. mailproto's default is a plain TCP bind of
// that address; a hosting host injects its own function returning a
// listener the ROUTER opened (or handed down over an inherited fd), so a
// tenant process never binds a port — the prerequisite for IMAP/SMTP
// following CardDAV into per-org tenant processes (hosting HANDOFF §6).
//
// The returned listener carries raw (pre-TLS) connections: by default TLS
// wrapping stays in mailproto, so injection changes where the socket comes
// from, never the TLS policy around it. The one deliberate exception is
// ExternalTLS (on IMAPOptions/SMTPOptions), where the HOST terminates TLS
// before connections reach the injected listener — that mode exists so a
// hosting tenant never holds the wildcard private key.
type ListenFunc func(addr string) (net.Listener, error)

// listenWith resolves the injected listener or falls back to a TCP bind.
func listenWith(listen ListenFunc, addr string) (net.Listener, error) {
	if listen != nil {
		return listen(addr)
	}
	return net.Listen("tcp", addr)
}

// logServeExit reports why a protocol server's Serve loop returned.
//
// net.ErrClosed is the NORMAL shutdown path, not a fault: every shutdown func
// here closes the listener before the server, so Accept fails with a closed
// socket while the server still considers itself running and hands the error
// back. Logging that at error level would page on every deploy and restart —
// so the expected exit is info and only an unexpected one escalates.
func logServeExit(label, addr string, err error) {
	if err == nil || errors.Is(err, net.ErrClosed) {
		mailLog.Info("server stopped accepting", "server", label, "addr", addr)
		return
	}
	mailLog.Error("server error", "server", label, "addr", addr, "err", err)
}

// IMAPOptions configures StartIMAP beyond the app/cert wiring.
type IMAPOptions struct {
	// NewSession supplies the per-connection session (feature-owned).
	NewSession NewIMAPSession

	// Listen, when non-nil, replaces every TCP bind (both the implicit-TLS
	// and, in dev, the plain listener). Nil = bind the configured addresses.
	Listen ListenFunc

	// ExternalTLS declares that the host terminates TLS BEFORE connections
	// reach the injected listener — the hosting tenant shape: the router
	// holds the wildcard cert, handshakes on :993 to read SNI, and forwards
	// plaintext over a private per-org unix socket, so the tenant process
	// never sees the private key. mailproto then resolves no cert material
	// (a tenant's allowlist env has none), serves exactly one listener,
	// allows auth over the plaintext transport (the public hop was TLS), and
	// advertises no STARTTLS. Requires Listen: without an injected listener
	// this mode would plaintext-bind a public port, so it refuses to start.
	ExternalTLS bool
}

// IMAPCaps is the capability set advertised to clients.
var IMAPCaps = imap.CapSet{
	imap.CapIMAP4rev1:   {},
	imap.CapIMAP4rev2:   {},
	imap.CapSpecialUse:  {},
	imap.CapMove:        {},
	imap.CapIdle:        {},
	imap.CapLiteralPlus: {},
}

// StartIMAP reads configuration from environment variables, creates an IMAP
// server, starts listening, and returns a shutdown function.
//
// In production with TLS (via env vars or autocert), only an implicit TLS
// listener on :993 is started — no plain-text IMAP is exposed.
// In dev mode, a plain listener on :1143 is started with optional STARTTLS
// and an optional implicit TLS listener on :1993.
func StartIMAP(app core.App, certManager *autocert.Manager, opts IMAPOptions) (func(), error) {
	if os.Getenv("IMAP_ENABLED") == "false" {
		mailLog.Info("IMAP server disabled via IMAP_ENABLED=false")
		return func() {}, nil
	}

	if opts.ExternalTLS {
		return startIMAPExternalTLS(app, opts)
	}

	tlsConfig, err := ResolveTLSConfig("IMAP_TLS_CERT", "IMAP_TLS_KEY", "", "", certManager)
	if err != nil {
		return nil, err
	}

	// In production with TLS: only implicit TLS, no plain listener
	if !app.IsDev() && tlsConfig != nil {
		return startIMAPTLSOnly(app, tlsConfig, opts)
	}

	// Production but no TLS source: refuse to silently fall through to the dev
	// branch (which would bind plain :1143 instead of :993). That fallthrough is
	// the classic "mail port isn't listening" deploy footgun — the container
	// comes up healthy on HTTP while IMAPS is silently absent. Fail loudly so
	// the misconfiguration surfaces at deploy time. To intentionally run without
	// IMAP in production, set IMAP_ENABLED=false.
	if !app.IsDev() {
		return nil, fmt.Errorf(
			"IMAPS (:993) cannot start: no TLS configured in production. " +
				"Set IMAP_TLS_CERT and IMAP_TLS_KEY to readable cert/key files, " +
				"or enable autocert (AUTOCERT_ENABLED=true + PRIMARY_DOMAIN), " +
				"or set IMAP_ENABLED=false to run without IMAP",
		)
	}

	// Dev mode: plain listener with optional STARTTLS + optional implicit TLS
	return startIMAPDev(app, tlsConfig, opts)
}

// startIMAPExternalTLS serves plaintext IMAP on the injected listener with the
// TLS-terminated posture ExternalTLS documents: auth allowed, no STARTTLS.
func startIMAPExternalTLS(app core.App, opts IMAPOptions) (func(), error) {
	if opts.Listen == nil {
		return nil, fmt.Errorf("IMAP ExternalTLS requires an injected listener (IMAPOptions.Listen)")
	}

	imapsAddr := os.Getenv("IMAPS_ADDR")
	if imapsAddr == "" {
		imapsAddr = ":993"
	}

	server := newIMAPServerInstance(app, nil, true, opts.NewSession)

	ln, err := opts.Listen(imapsAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", imapsAddr, err)
	}
	mailLog.Info("IMAP server listening (external TLS termination)", "addr", imapsAddr)
	go func() {
		logServeExit("IMAP", imapsAddr, server.Serve(ln))
	}()

	return func() {
		mailLog.Info("Shutting down IMAP server")
		ln.Close()
		server.Close()
	}, nil
}

func startIMAPTLSOnly(app core.App, tlsConfig *tls.Config, opts IMAPOptions) (func(), error) {
	imapsAddr := os.Getenv("IMAPS_ADDR")
	if imapsAddr == "" {
		imapsAddr = ":993"
	}

	server := newIMAPServerInstance(app, tlsConfig, false, opts.NewSession)

	rawLn, err := listenWith(opts.Listen, imapsAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", imapsAddr, err)
	}
	tlsLn := tls.NewListener(rawLn, tlsConfig)
	mailLog.Info("IMAPS server listening (implicit TLS, no plain listener)", "addr", imapsAddr)
	go func() {
		logServeExit("IMAPS", imapsAddr, server.Serve(tlsLn))
	}()

	return func() {
		mailLog.Info("Shutting down IMAP server")
		tlsLn.Close()
		server.Close()
	}, nil
}

func startIMAPDev(app core.App, tlsConfig *tls.Config, opts IMAPOptions) (func(), error) {
	addr := os.Getenv("IMAP_ADDR")
	if addr == "" {
		addr = ":1143"
	}

	insecureAuth := os.Getenv("IMAP_INSECURE_AUTH") == "true" || app.IsDev()
	server := newIMAPServerInstance(app, tlsConfig, insecureAuth, opts.NewSession)

	plainLn, err := listenWith(opts.Listen, addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	mailLog.Info("IMAP server listening", "addr", addr, "starttls", tlsConfig != nil)
	go func() {
		logServeExit("IMAP", addr, server.Serve(plainLn))
	}()

	var tlsLn net.Listener
	if tlsConfig != nil {
		imapsAddr := os.Getenv("IMAPS_ADDR")
		if imapsAddr == "" {
			imapsAddr = ":1993"
		}
		rawLn, err := listenWith(opts.Listen, imapsAddr)
		if err != nil {
			// Dev only, and deliberately not fatal: the implicit-TLS listener
			// is a convenience here, while the PLAIN one on IMAP_ADDR is what
			// clients and the e2e suite actually use. Tearing the plain
			// listener down because an optional port is taken is how a dev
			// server already holding :1993 made every IMAP e2e fail with
			// ECONNREFUSED on a port that had just bound successfully —
			// IMAP_ADDR is parameterized per-run, IMAPS_ADDR was not.
			//
			// Production never reaches this branch: it returns earlier via
			// startIMAPTLSOnly, or refuses to boot without a TLS source.
			// Info, not warn: this branch is dev-only (production returns via
			// startIMAPTLSOnly or refuses to boot), and the usual cause is another
			// local server already holding the optional port.
			mailLog.Info(
				"IMAPS listener unavailable; continuing with plain IMAP only",
				"addr", imapsAddr,
				"err", err,
			)
		} else {
			tlsLn = tls.NewListener(rawLn, tlsConfig)
			mailLog.Info("IMAPS server listening (implicit TLS)", "addr", imapsAddr)
			go func() {
				logServeExit("IMAPS", imapsAddr, server.Serve(tlsLn))
			}()
		}
	}

	return func() {
		mailLog.Info("Shutting down IMAP server")
		plainLn.Close()
		if tlsLn != nil {
			tlsLn.Close()
		}
		server.Close()
	}, nil
}

func newIMAPServerInstance(
	app core.App,
	tlsConfig *tls.Config,
	insecureAuth bool,
	newSession NewIMAPSession,
) *imapserver.Server {
	return imapserver.New(&imapserver.Options{
		NewSession: func(conn *imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return newSession(app, conn), nil, nil
		},
		Caps:         IMAPCaps,
		TLSConfig:    tlsConfig,
		InsecureAuth: insecureAuth,
	})
}
