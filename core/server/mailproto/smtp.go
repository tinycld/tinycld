package mailproto

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/pocketbase/pocketbase/core"
	"golang.org/x/crypto/acme/autocert"
)

// SMTPOptions describes one SMTP listener. Submission (authenticated, :465) and
// inbound MX (unauthenticated, :25) differ only in their env-var names, ports
// and backend, so both drive this one implementation.
type SMTPOptions struct {
	// Backend supplies the per-connection session. Feature-owned, like the IMAP
	// session — it speaks the feature's schema.
	Backend smtp.Backend

	// Label appears in log lines ("SMTP", "inbound SMTP").
	Label string

	// EnabledEnv gates startup: setting it to "false" disables the listener.
	EnabledEnv string
	// AddrEnv / TLSAddrEnv name the env vars holding the listen addresses.
	AddrEnv, TLSAddrEnv string
	// DefaultTLSAddr is the implicit-TLS address in production (e.g. ":465").
	// DefaultAddr / DefaultDevTLSAddr are the dev plain / implicit-TLS addresses.
	DefaultTLSAddr, DefaultAddr, DefaultDevTLSAddr string

	// CertEnv/KeyEnv name the primary cert env vars; FallbackCertEnv/
	// FallbackKeyEnv are consulted when the primary pair is unset.
	CertEnv, KeyEnv, FallbackCertEnv, FallbackKeyEnv string

	// InsecureAuthEnv, when "true", permits AUTH without TLS (also implied by dev).
	InsecureAuthEnv string

	// ProductionTLSError is returned when production has no TLS source. It must
	// name the concrete remedies — a container that comes up healthy on HTTP
	// with no mail listener is the failure this guards against.
	ProductionTLSError string

	// MaxMessageBytes caps an accepted message (0 → 25 MiB).
	MaxMessageBytes int64
	// AuthDisabled advertises no AUTH mechanisms (inbound MX).
	AuthDisabled bool

	// Listen, when non-nil, replaces every TCP bind for this service (see
	// ListenFunc — the multi-org injection seam). Nil = bind the configured
	// addresses.
	Listen ListenFunc

	// ExternalTLS declares that the host terminates TLS before connections
	// reach the injected listener — same contract and rationale as
	// IMAPOptions.ExternalTLS: no cert resolution, one listener, auth over
	// plaintext allowed, no STARTTLS. Requires Listen.
	ExternalTLS bool
}

// StartSMTP builds an SMTP listener from opts, starts it, and returns a
// shutdown function. It mirrors StartIMAP's policy: implicit TLS only in
// production, plain + optional STARTTLS in dev, and a loud failure rather than
// a silent fallthrough when production has no TLS.
func StartSMTP(app core.App, certManager *autocert.Manager, opts SMTPOptions) (func(), error) {
	if opts.EnabledEnv != "" && os.Getenv(opts.EnabledEnv) == "false" {
		app.Logger().Info(opts.Label + " server disabled via " + opts.EnabledEnv + "=false")
		return func() {}, nil
	}

	if opts.ExternalTLS {
		return startSMTPExternalTLS(app, opts)
	}

	tlsConfig, err := ResolveTLSConfig(
		opts.CertEnv, opts.KeyEnv, opts.FallbackCertEnv, opts.FallbackKeyEnv, certManager,
	)
	if err != nil {
		return nil, err
	}

	if !app.IsDev() && tlsConfig != nil {
		return startSMTPTLSOnly(app, tlsConfig, opts)
	}

	if !app.IsDev() && opts.ProductionTLSError != "" {
		return nil, fmt.Errorf("%s", opts.ProductionTLSError)
	}

	return startSMTPDev(app, tlsConfig, opts)
}

func newSMTPServerInstance(tlsConfig *tls.Config, insecureAuth bool, opts SMTPOptions) *smtp.Server {
	domain := os.Getenv("SMTP_DOMAIN")
	if domain == "" {
		domain = "localhost"
	}

	maxBytes := opts.MaxMessageBytes
	if maxBytes == 0 {
		maxBytes = 25 << 20
	}

	server := smtp.NewServer(opts.Backend)
	server.Domain = domain
	server.AllowInsecureAuth = insecureAuth
	server.MaxMessageBytes = maxBytes
	server.EnableSMTPUTF8 = true
	server.ReadTimeout = 60 * time.Second
	server.WriteTimeout = 60 * time.Second
	if tlsConfig != nil {
		server.TLSConfig = tlsConfig
	}
	return server
}

// startSMTPExternalTLS serves plaintext SMTP on the injected listener with the
// TLS-terminated posture ExternalTLS documents: auth allowed, no STARTTLS.
func startSMTPExternalTLS(app core.App, opts SMTPOptions) (func(), error) {
	if opts.Listen == nil {
		return nil, fmt.Errorf("%s ExternalTLS requires an injected listener (SMTPOptions.Listen)", opts.Label)
	}

	addr := os.Getenv(opts.TLSAddrEnv)
	if addr == "" {
		addr = opts.DefaultTLSAddr
	}
	if addr == "" {
		addr = opts.DefaultAddr
	}

	server := newSMTPServerInstance(nil, true, opts)
	server.Addr = addr

	ln, err := opts.Listen(addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	app.Logger().Info(opts.Label+" server listening (external TLS termination)", "addr", addr)
	go func() {
		if err := server.Serve(ln); err != nil {
			app.Logger().Error(opts.Label+" server error", "addr", addr, "error", err)
		}
	}()

	return func() {
		app.Logger().Info("Shutting down " + opts.Label + " server")
		ln.Close()
		server.Close()
	}, nil
}

func startSMTPTLSOnly(app core.App, tlsConfig *tls.Config, opts SMTPOptions) (func(), error) {
	addr := os.Getenv(opts.TLSAddrEnv)
	if addr == "" {
		addr = opts.DefaultTLSAddr
	}

	server := newSMTPServerInstance(tlsConfig, false, opts)
	server.Addr = addr

	rawLn, err := listenWith(opts.Listen, addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	tlsLn := tls.NewListener(rawLn, tlsConfig)
	app.Logger().Info(opts.Label+" server listening (implicit TLS, no plain listener)", "addr", addr)
	go func() {
		if err := server.Serve(tlsLn); err != nil {
			app.Logger().Error(opts.Label+" server error", "addr", addr, "error", err)
		}
	}()

	return func() {
		app.Logger().Info("Shutting down " + opts.Label + " server")
		tlsLn.Close()
		server.Close()
	}, nil
}

func startSMTPDev(app core.App, tlsConfig *tls.Config, opts SMTPOptions) (func(), error) {
	addr := os.Getenv(opts.AddrEnv)
	if addr == "" {
		addr = opts.DefaultAddr
	}

	insecureAuth := app.IsDev()
	if opts.InsecureAuthEnv != "" && os.Getenv(opts.InsecureAuthEnv) == "true" {
		insecureAuth = true
	}
	server := newSMTPServerInstance(tlsConfig, insecureAuth, opts)
	server.Addr = addr

	plainLn, err := listenWith(opts.Listen, addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	app.Logger().Info(opts.Label+" server listening", "addr", addr, "starttls", tlsConfig != nil)
	go func() {
		if err := server.Serve(plainLn); err != nil {
			app.Logger().Error(opts.Label+" server error", "addr", addr, "error", err)
		}
	}()

	var tlsLn net.Listener
	if tlsConfig != nil && opts.DefaultDevTLSAddr != "" {
		tlsAddr := os.Getenv(opts.TLSAddrEnv)
		if tlsAddr == "" {
			tlsAddr = opts.DefaultDevTLSAddr
		}
		rawLn, err := listenWith(opts.Listen, tlsAddr)
		if err != nil {
			// Same reasoning as the IMAPS branch in imap.go: dev only, and the
			// plain listener above is the one that matters. Closing it because
			// an optional TLS port is already held by another local server
			// turns a taken port into a total outage of the protocol.
			app.Logger().Warn(
				opts.Label+" implicit-TLS listener unavailable; continuing with plain only",
				"addr", tlsAddr,
				"error", err,
			)
		} else {
			tlsLn = tls.NewListener(rawLn, tlsConfig)
			app.Logger().Info(opts.Label+" server listening (implicit TLS)", "addr", tlsAddr)
			go func() {
				if err := server.Serve(tlsLn); err != nil {
					app.Logger().Error(opts.Label+" server error", "addr", tlsAddr, "error", err)
				}
			}()
		}
	}

	return func() {
		app.Logger().Info("Shutting down " + opts.Label + " server")
		plainLn.Close()
		if tlsLn != nil {
			tlsLn.Close()
		}
		server.Close()
	}, nil
}
