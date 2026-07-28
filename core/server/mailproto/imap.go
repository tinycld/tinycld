package mailproto

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/pocketbase/pocketbase/core"
	"golang.org/x/crypto/acme/autocert"
)

// NewIMAPSession builds the per-connection IMAP session. The session speaks the
// feature's own schema, so the feature package supplies it; mailproto only owns
// the listener around it.
type NewIMAPSession func(app core.App, conn *imapserver.Conn) imapserver.Session

// ListenFunc opens the socket a mail service serves on, given the address the
// service would otherwise bind. mailproto's default is a plain TCP bind of
// that address; a multi-org host injects its own function returning a
// listener the ROUTER opened (or handed down over an inherited fd), so a
// tenant process never binds a port — the prerequisite for IMAP/SMTP
// following CardDAV into per-org tenant processes (multi-org HANDOFF §6).
//
// The returned listener must carry raw (pre-TLS) connections: TLS wrapping
// stays in mailproto either way, so injection changes where the socket comes
// from, never the TLS policy around it.
type ListenFunc func(addr string) (net.Listener, error)

// listenWith resolves the injected listener or falls back to a TCP bind.
func listenWith(listen ListenFunc, addr string) (net.Listener, error) {
	if listen != nil {
		return listen(addr)
	}
	return net.Listen("tcp", addr)
}

// IMAPOptions configures StartIMAP beyond the app/cert wiring.
type IMAPOptions struct {
	// NewSession supplies the per-connection session (feature-owned).
	NewSession NewIMAPSession

	// Listen, when non-nil, replaces every TCP bind (both the implicit-TLS
	// and, in dev, the plain listener). Nil = bind the configured addresses.
	Listen ListenFunc
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
		app.Logger().Info("IMAP server disabled via IMAP_ENABLED=false")
		return func() {}, nil
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
	app.Logger().Info("IMAPS server listening (implicit TLS, no plain listener)", "addr", imapsAddr)
	go func() {
		if err := server.Serve(tlsLn); err != nil {
			app.Logger().Error("IMAPS server error", "addr", imapsAddr, "error", err)
		}
	}()

	return func() {
		app.Logger().Info("Shutting down IMAP server")
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
	app.Logger().Info("IMAP server listening", "addr", addr, "starttls", tlsConfig != nil)
	go func() {
		if err := server.Serve(plainLn); err != nil {
			app.Logger().Error("IMAP server error", "addr", addr, "error", err)
		}
	}()

	var tlsLn net.Listener
	if tlsConfig != nil {
		imapsAddr := os.Getenv("IMAPS_ADDR")
		if imapsAddr == "" {
			imapsAddr = ":1993"
		}
		rawLn, err := listenWith(opts.Listen, imapsAddr)
		if err != nil {
			plainLn.Close()
			return nil, fmt.Errorf("failed to listen on %s: %w", imapsAddr, err)
		}
		tlsLn = tls.NewListener(rawLn, tlsConfig)
		app.Logger().Info("IMAPS server listening (implicit TLS)", "addr", imapsAddr)
		go func() {
			if err := server.Serve(tlsLn); err != nil {
				app.Logger().Error("IMAPS server error", "addr", imapsAddr, "error", err)
			}
		}()
	}

	return func() {
		app.Logger().Info("Shutting down IMAP server")
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
