package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tinycld.org/cli/client"
	"tinycld.org/cli/internal/config"
	"tinycld.org/cli/internal/keychain"
)

// authServer fakes the OAuth surface: device authorization, a token endpoint
// that answers pending once then succeeds, userinfo, and revoke.
func authServer(t *testing.T) (*httptest.Server, *struct{ tokenPolls, revokes int }) {
	t.Helper()
	counts := &struct{ tokenPolls, revokes int }{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /oauth/device", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"device_code":               "dev-code",
			"user_code":                 "WDJB-MJHT",
			"verification_uri":          "http://x/p/oauth/authorize",
			"verification_uri_complete": "http://x/p/oauth/authorize?user_code=WDJB-MJHT",
			"expires_in":                900,
			"interval":                  5,
		})
	})
	mux.HandleFunc("POST /oauth/token", func(w http.ResponseWriter, r *http.Request) {
		counts.tokenPolls++
		if counts.tokenPolls == 1 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access-1", "token_type": "Bearer",
			"expires_in": 3600, "refresh_token": "refresh-1",
			"scope": "profile mail:read",
		})
	})
	mux.HandleFunc("GET /oauth/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access-1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{
			"sub": "u1", "email": "nathan@example.com", "name": "Nathan",
		})
	})
	mux.HandleFunc("POST /oauth/revoke", func(w http.ResponseWriter, r *http.Request) {
		counts.revokes++
		r.ParseForm()
		if r.FormValue("token") != "refresh-1" {
			t.Errorf("revoke token = %q", r.FormValue("token"))
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, counts
}

func loginDeps(t *testing.T, srv *httptest.Server) (*deps, *keychain.MemStore) {
	d, store := testDeps(t)
	d.httpClient = srv.Client()
	return d, store
}

func hostOf(srv *httptest.Server) string {
	return strings.TrimPrefix(srv.URL, "http://")
}

func TestAuthLoginNonTTY(t *testing.T) {
	srv, counts := authServer(t)
	d, store := loginDeps(t, srv)

	stdout, _, err := runCLI(t, d, "auth", "login", hostOf(srv))
	if err != nil {
		t.Fatal(err)
	}

	// non-TTY: prints the verification URL instead of blocking on stdin
	if !strings.Contains(stdout, "WDJB-MJHT") {
		t.Fatalf("user code not shown:\n%s", stdout)
	}
	if !strings.Contains(stdout, "user_code=WDJB-MJHT") {
		t.Fatalf("verification URL not shown:\n%s", stdout)
	}
	if !strings.Contains(stdout, "✓ Authenticated as nathan@example.com") {
		t.Fatalf("identity not confirmed:\n%s", stdout)
	}
	if counts.tokenPolls != 2 {
		t.Fatalf("polls = %d, want pending then success", counts.tokenPolls)
	}

	// token persisted under the host-derived context name
	var tok client.TokenSet
	if err := keychain.GetJSON(store, hostOf(srv), &tok); err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "access-1" || tok.RefreshToken != "refresh-1" {
		t.Fatalf("stored token = %+v", tok)
	}

	// context saved and made current
	cfg, err := config.Load(d.configDir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Current != hostOf(srv) {
		t.Fatalf("current = %q", cfg.Current)
	}
	if cfg.Contexts[hostOf(srv)].User != "nathan@example.com" {
		t.Fatalf("context = %+v", cfg.Contexts[hostOf(srv)])
	}
}

func TestAuthStatus(t *testing.T) {
	srv, _ := authServer(t)
	d, _ := loginDeps(t, srv)
	if _, _, err := runCLI(t, d, "auth", "login", hostOf(srv)); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := runCLI(t, d, "auth", "status", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var status struct {
		Context string `json:"context"`
		Origin  string `json:"origin"`
		Email   string `json:"email"`
		Scopes  string `json:"scopes"`
	}
	if err := json.Unmarshal([]byte(stdout), &status); err != nil {
		t.Fatalf("status --json parse: %v\n%s", err, stdout)
	}
	if status.Email != "nathan@example.com" || status.Origin != srv.URL {
		t.Fatalf("status = %+v", status)
	}
	if !strings.Contains(status.Scopes, "mail:read") {
		t.Fatalf("scopes = %q", status.Scopes)
	}
}

func TestAuthStatusWithoutLogin(t *testing.T) {
	srv, _ := authServer(t)
	d, _ := loginDeps(t, srv)
	// context exists but has no credential
	if _, _, err := runCLI(t, d, "context", "add", "dev", srv.URL); err != nil {
		t.Fatal(err)
	}
	_, _, err := runCLI(t, d, "auth", "status")
	if !errors.Is(err, client.ErrAuthExpired) {
		t.Fatalf("err = %v, want ErrAuthExpired", err)
	}
}

func TestAuthLogoutRevokesAndClears(t *testing.T) {
	srv, counts := authServer(t)
	d, store := loginDeps(t, srv)
	if _, _, err := runCLI(t, d, "auth", "login", hostOf(srv)); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := runCLI(t, d, "auth", "logout")
	if err != nil {
		t.Fatal(err)
	}
	if counts.revokes != 1 {
		t.Fatalf("revokes = %d", counts.revokes)
	}
	if _, err := store.Get(hostOf(srv)); !errors.Is(err, keychain.ErrNotFound) {
		t.Fatal("credential should be cleared")
	}
	if !strings.Contains(stdout, "✓ Logged out") {
		t.Fatalf("stdout = %q", stdout)
	}

	// context row survives logout for an easy re-login
	cfg, _ := config.Load(d.configDir)
	if _, ok := cfg.Contexts[hostOf(srv)]; !ok {
		t.Fatal("context should survive logout")
	}
}

func TestAuthLogoutSurvivesRevokeFailure(t *testing.T) {
	srv, _ := authServer(t)
	d, store := loginDeps(t, srv)
	if _, _, err := runCLI(t, d, "auth", "login", hostOf(srv)); err != nil {
		t.Fatal(err)
	}
	srv.Close() // server gone: revocation cannot succeed

	_, stderr, err := runCLI(t, d, "auth", "logout")
	if err != nil {
		t.Fatalf("logout must still succeed locally: %v", err)
	}
	if !strings.Contains(stderr, "could not revoke") {
		t.Fatalf("stderr = %q", stderr)
	}
	if _, err := store.Get(hostOf(srv)); !errors.Is(err, keychain.ErrNotFound) {
		t.Fatal("credential should be cleared even when revocation fails")
	}
}

// The credential is stored before the identity is known, so a userinfo failure
// past that point must still leave a context naming it. Otherwise the token
// sits in the keychain with nothing referencing it: invisible to `context
// list` and unreachable by `auth logout`.
func TestAuthLoginLeavesNoOrphanedCredential(t *testing.T) {
	counts := &struct{ tokenPolls int }{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /oauth/device", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"device_code": "dev-code", "user_code": "WDJB-MJHT",
			"verification_uri":          "http://x/p/oauth/authorize",
			"verification_uri_complete": "http://x/p/oauth/authorize?user_code=WDJB-MJHT",
			"expires_in":                900, "interval": 5,
		})
	})
	mux.HandleFunc("POST /oauth/token", func(w http.ResponseWriter, _ *http.Request) {
		counts.tokenPolls++
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access-1", "token_type": "Bearer",
			"expires_in": 3600, "refresh_token": "refresh-1", "scope": "profile",
		})
	})
	// The grant is live, but identity cannot be read (a transient 500, a
	// revoked scope, a proxy hiccup).
	mux.HandleFunc("GET /oauth/userinfo", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	d, store := loginDeps(t, srv)
	host := hostOf(srv)

	if _, _, err := runCLI(t, d, "auth", "login", host); err == nil {
		t.Fatal("a failed userinfo must surface as an error")
	}

	// The token was saved, so a context must name it.
	if _, err := store.Get(host); err != nil {
		t.Fatalf("the credential should still be stored: %v", err)
	}
	cfg, err := config.Load(d.configDir)
	if err != nil {
		t.Fatal(err)
	}
	ctx, ok := cfg.Contexts[host]
	if !ok {
		t.Fatal("a stored credential with no context is orphaned — unreachable by logout")
	}
	if ctx.Origin != "http://"+host {
		t.Errorf("context origin = %q", ctx.Origin)
	}
}
