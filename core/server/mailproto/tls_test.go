package mailproto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeCertPair generates a self-signed cert/key pair with the given common
// name and writes them as PEM files at certPath/keyPath.
func writeCertPair(t *testing.T, certPath, keyPath, commonName string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
}

func certCommonName(t *testing.T, cert *tls.Certificate) string {
	t.Helper()
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}
	return leaf.Subject.CommonName
}

func TestCertReloaderServesInitialCert(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	writeCertPair(t, certPath, keyPath, "first.example.com")

	r, err := newCertReloader(certPath, keyPath)
	if err != nil {
		t.Fatalf("newCertReloader: %v", err)
	}

	got, err := r.GetCertificate(nil)
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	if cn := certCommonName(t, got); cn != "first.example.com" {
		t.Fatalf("CN = %q, want first.example.com", cn)
	}
}

func TestCertReloaderPicksUpRenewal(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	writeCertPair(t, certPath, keyPath, "first.example.com")

	r, err := newCertReloader(certPath, keyPath)
	if err != nil {
		t.Fatalf("newCertReloader: %v", err)
	}

	// Simulate a renewal: overwrite the pair with a new cert and bump mtime
	// past the originally-recorded one (filesystem mtime resolution can be
	// coarse, so set it explicitly rather than relying on wall-clock drift).
	writeCertPair(t, certPath, keyPath, "renewed.example.com")
	future := time.Now().Add(time.Minute)
	if err := os.Chtimes(certPath, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	got, err := r.GetCertificate(nil)
	if err != nil {
		t.Fatalf("GetCertificate after renewal: %v", err)
	}
	if cn := certCommonName(t, got); cn != "renewed.example.com" {
		t.Fatalf("CN = %q, want renewed.example.com (renewal not picked up)", cn)
	}
}

func TestCertReloaderFallsBackOnBrokenReload(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	writeCertPair(t, certPath, keyPath, "good.example.com")

	r, err := newCertReloader(certPath, keyPath)
	if err != nil {
		t.Fatalf("newCertReloader: %v", err)
	}

	// Simulate a mid-renewal state: cert file rewritten with garbage and
	// mtime bumped, but it no longer parses. The reloader should keep serving
	// the last good cert rather than erroring the handshake.
	if err := os.WriteFile(certPath, []byte("not a pem cert"), 0o600); err != nil {
		t.Fatalf("write garbage: %v", err)
	}
	future := time.Now().Add(time.Minute)
	if err := os.Chtimes(certPath, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	got, err := r.GetCertificate(nil)
	if err != nil {
		t.Fatalf("GetCertificate after broken reload: %v", err)
	}
	if cn := certCommonName(t, got); cn != "good.example.com" {
		t.Fatalf("CN = %q, want good.example.com (should serve last good cert)", cn)
	}
}

func TestNewCertReloaderErrorsOnMissingFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := newCertReloader(filepath.Join(dir, "nope.pem"), filepath.Join(dir, "nope.key")); err == nil {
		t.Fatal("expected error for missing cert file, got nil")
	}
}

func TestLoadTLSConfigNilWhenUnset(t *testing.T) {
	t.Setenv("TEST_TLS_CERT_UNSET", "")
	t.Setenv("TEST_TLS_KEY_UNSET", "")

	cfg, err := loadTLSConfig("TEST_TLS_CERT_UNSET", "TEST_TLS_KEY_UNSET", "", "")
	if err != nil {
		t.Fatalf("loadTLSConfig: %v", err)
	}
	if cfg != nil {
		t.Fatal("expected nil config when cert paths unset, got non-nil")
	}
}

func TestLoadTLSConfigUsesReloader(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	writeCertPair(t, certPath, keyPath, "env.example.com")

	t.Setenv("TEST_TLS_CERT", certPath)
	t.Setenv("TEST_TLS_KEY", keyPath)

	cfg, err := loadTLSConfig("TEST_TLS_CERT", "TEST_TLS_KEY", "", "")
	if err != nil {
		t.Fatalf("loadTLSConfig: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.GetCertificate == nil {
		t.Fatal("expected GetCertificate callback (reloading), got nil")
	}
	if len(cfg.Certificates) != 0 {
		t.Fatal("expected no static Certificates (reload path uses GetCertificate)")
	}
	got, err := cfg.GetCertificate(nil)
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	if cn := certCommonName(t, got); cn != "env.example.com" {
		t.Fatalf("CN = %q, want env.example.com", cn)
	}
}
