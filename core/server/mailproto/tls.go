// Package mailproto is core's shared mail-protocol transport layer: the TLS
// plumbing and the IMAP/SMTP listener lifecycle (bind, serve, graceful
// shutdown, dev-vs-production TLS policy).
//
// It owns only the transport. The protocol SESSIONS — which speak a specific
// application's schema (threads, folders, UIDs, mailbox membership) — stay in
// the package that owns that schema and are injected via Config. That split is
// deliberate: the listener half is genuinely generic, while a session is
// saturated with its feature's data model and would only be "configurable" by
// reimplementing that model as configuration.
//
// Two hosts drive this: the single-tenant app (via the mail package's
// Register) and, once per-org process isolation lands, the hosting router.
// Because everything here takes core.App, neither needs a concrete
// *pocketbase.PocketBase.

package mailproto

import (
	"crypto/tls"
	"fmt"
	"os"
	"sync"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

// certReloader serves a TLS certificate loaded from disk, transparently
// re-reading the cert/key pair when the cert file's modification time changes.
// This lets externally-renewed certs (e.g. a weekly ACME renewal writing new
// files) take effect without restarting the process — the next TLS handshake
// picks up the fresh cert. GetCertificate runs on every handshake, so the
// result is cached and only reloaded when the on-disk cert file changes.
type certReloader struct {
	certPath, keyPath string

	mu      sync.RWMutex
	cached  *tls.Certificate
	modTime time.Time
}

func newCertReloader(certPath, keyPath string) (*certReloader, error) {
	r := &certReloader{certPath: certPath, keyPath: keyPath}
	// Load eagerly so a misconfigured cert/key path fails at startup rather
	// than on the first client connection.
	if _, err := r.load(); err != nil {
		return nil, err
	}
	return r, nil
}

// load reads the cert/key pair from disk and updates the cache, recording the
// cert file's modification time so future calls can skip re-reading.
func (r *certReloader) load() (*tls.Certificate, error) {
	fi, err := os.Stat(r.certPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat TLS cert %s: %w", r.certPath, err)
	}

	cert, err := tls.LoadX509KeyPair(r.certPath, r.keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS cert (%s/%s): %w", r.certPath, r.keyPath, err)
	}

	r.mu.Lock()
	r.cached = &cert
	r.modTime = fi.ModTime()
	r.mu.Unlock()
	return &cert, nil
}

// GetCertificate returns the cached certificate, reloading from disk first if
// the cert file's modification time has advanced since the last load. On a
// reload error it falls back to the last good cert so a transient half-written
// file (mid-renewal) doesn't take down TLS.
func (r *certReloader) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	r.mu.RLock()
	cached, modTime := r.cached, r.modTime
	r.mu.RUnlock()

	fi, err := os.Stat(r.certPath)
	if err == nil && fi.ModTime().After(modTime) {
		if reloaded, err := r.load(); err == nil {
			return reloaded, nil
		}
		// Reload failed (e.g. cert written but key not yet) — keep serving
		// the last good cert; the next handshake will retry.
	}
	return cached, nil
}

// loadTLSConfig builds a TLS config from the given environment variable names.
// It tries the primary env vars first, then the fallback env vars.
// Returns (nil, nil) if no cert paths are configured — TLS is optional in dev.
// The returned config reloads its cert from disk on renewal (see certReloader).
func loadTLSConfig(certEnv, keyEnv, fallbackCertEnv, fallbackKeyEnv string) (*tls.Config, error) {
	certPath := os.Getenv(certEnv)
	keyPath := os.Getenv(keyEnv)
	if certPath == "" && fallbackCertEnv != "" {
		certPath = os.Getenv(fallbackCertEnv)
	}
	if keyPath == "" && fallbackKeyEnv != "" {
		keyPath = os.Getenv(fallbackKeyEnv)
	}
	if certPath == "" || keyPath == "" {
		return nil, nil
	}

	reloader, err := newCertReloader(certPath, keyPath)
	if err != nil {
		return nil, err
	}

	return HardenedTLSConfig(&tls.Config{
		GetCertificate: reloader.GetCertificate,
	}), nil
}

// hardenedTLSConfig applies hardened TLS settings to the given config.
var hardenedCipherSuites = []uint16{
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
	tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
}

func HardenedTLSConfig(cfg *tls.Config) *tls.Config {
	cfg.MinVersion = tls.VersionTLS12
	cfg.CipherSuites = hardenedCipherSuites
	return cfg
}

// tlsConfigFromCertManager creates a hardened TLS config using the autocert manager.
func tlsConfigFromCertManager(certManager *autocert.Manager) *tls.Config {
	return HardenedTLSConfig(&tls.Config{
		GetCertificate: certManager.GetCertificate,
	})
}

// resolveTLSConfig returns a TLS config using the first available source:
// (1) cert/key files from environment variables, (2) autocert manager, (3) nil.
func ResolveTLSConfig(certEnv, keyEnv, fallbackCertEnv, fallbackKeyEnv string, certManager *autocert.Manager) (*tls.Config, error) {
	cfg, err := loadTLSConfig(certEnv, keyEnv, fallbackCertEnv, fallbackKeyEnv)
	if err != nil {
		return nil, err
	}
	if cfg != nil {
		return cfg, nil
	}
	if certManager != nil {
		return tlsConfigFromCertManager(certManager), nil
	}
	return nil, nil
}
