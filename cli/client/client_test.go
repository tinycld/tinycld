package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type memTokenStore struct {
	mu    sync.Mutex
	tok   TokenSet
	empty bool
	saves int
}

func (s *memTokenStore) Load() (TokenSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.empty {
		return TokenSet{}, ErrAuthExpired
	}
	return s.tok, nil
}

func (s *memTokenStore) Save(tok TokenSet) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tok = tok
	s.empty = false
	s.saves++
	return nil
}

func validStore(access string) *memTokenStore {
	return &memTokenStore{tok: TokenSet{
		AccessToken:  access,
		RefreshToken: "refresh-1",
		ExpiresAt:    time.Now().Add(time.Hour),
	}}
}

// apiServer answers /api/ping, accepting only the given bearer token, and
// refreshes refresh-1 → (access-2, refresh-2) at /oauth/token.
func apiServer(t *testing.T, acceptToken string) (*httptest.Server, *int, *int) {
	t.Helper()
	pings, refreshes := 0, 0
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/ping", func(w http.ResponseWriter, r *http.Request) {
		pings++
		if r.Header.Get("Authorization") != "Bearer "+acceptToken {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"message": "expired"})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"pong": "ok"})
	})
	mux.HandleFunc("POST /oauth/token", func(w http.ResponseWriter, r *http.Request) {
		refreshes++
		r.ParseForm()
		if r.FormValue("grant_type") != "refresh_token" || r.FormValue("client_id") != DefaultClientID {
			t.Errorf("bad refresh form: %v", r.Form)
		}
		if r.FormValue("refresh_token") != "refresh-1" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access-2", "token_type": "Bearer",
			"expires_in": 3600, "refresh_token": "refresh-2", "scope": "profile",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &pings, &refreshes
}

func TestGetJSONHappyPath(t *testing.T) {
	srv, pings, refreshes := apiServer(t, "access-1")
	c := New(srv.URL, validStore("access-1"), srv.Client())

	var out map[string]string
	if err := c.GetJSON(context.Background(), "/api/ping", &out); err != nil {
		t.Fatal(err)
	}
	if out["pong"] != "ok" || *pings != 1 || *refreshes != 0 {
		t.Fatalf("out=%v pings=%d refreshes=%d", out, *pings, *refreshes)
	}
}

func TestRefreshOn401PersistsRotationBeforeRetry(t *testing.T) {
	srv, pings, refreshes := apiServer(t, "access-2")
	store := validStore("access-1") // stale: server only accepts access-2
	c := New(srv.URL, store, srv.Client())

	var out map[string]string
	if err := c.GetJSON(context.Background(), "/api/ping", &out); err != nil {
		t.Fatal(err)
	}
	if out["pong"] != "ok" {
		t.Fatalf("out = %v", out)
	}
	if *pings != 2 || *refreshes != 1 {
		t.Fatalf("pings=%d refreshes=%d, want 2/1", *pings, *refreshes)
	}
	if store.tok.RefreshToken != "refresh-2" || store.tok.AccessToken != "access-2" {
		t.Fatalf("rotated tokens not persisted: %+v", store.tok)
	}
	if store.saves != 1 {
		t.Fatalf("saves = %d", store.saves)
	}
}

func TestProactiveRefreshNearExpiry(t *testing.T) {
	srv, pings, refreshes := apiServer(t, "access-2")
	store := &memTokenStore{tok: TokenSet{
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		ExpiresAt:    time.Now().Add(10 * time.Second), // inside the 60s window
	}}
	c := New(srv.URL, store, srv.Client())

	var out map[string]string
	if err := c.GetJSON(context.Background(), "/api/ping", &out); err != nil {
		t.Fatal(err)
	}
	// refreshed BEFORE the request: exactly one ping, with the new token
	if *pings != 1 || *refreshes != 1 {
		t.Fatalf("pings=%d refreshes=%d, want 1/1", *pings, *refreshes)
	}
}

func TestSecond401DoesNotLoop(t *testing.T) {
	// Server never accepts any token: expect exactly one refresh and one
	// retry, then an error.
	srv, pings, refreshes := apiServer(t, "never-valid")
	c := New(srv.URL, validStore("access-1"), srv.Client())

	err := c.GetJSON(context.Background(), "/api/ping", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if *pings != 2 || *refreshes != 1 {
		t.Fatalf("pings=%d refreshes=%d, want 2/1", *pings, *refreshes)
	}
}

func TestRefreshInvalidGrantIsAuthExpired(t *testing.T) {
	srv, _, _ := apiServer(t, "access-2")
	store := &memTokenStore{tok: TokenSet{
		AccessToken:  "access-1",
		RefreshToken: "revoked-token", // server answers invalid_grant
		ExpiresAt:    time.Now().Add(-time.Minute),
	}}
	c := New(srv.URL, store, srv.Client())

	err := c.GetJSON(context.Background(), "/api/ping", nil)
	if !errors.Is(err, ErrAuthExpired) {
		t.Fatalf("err = %v, want ErrAuthExpired", err)
	}
}

func TestNoStoredCredential(t *testing.T) {
	srv, pings, _ := apiServer(t, "x")
	c := New(srv.URL, &memTokenStore{empty: true}, srv.Client())
	err := c.GetJSON(context.Background(), "/api/ping", nil)
	if !errors.Is(err, ErrAuthExpired) {
		t.Fatalf("err = %v", err)
	}
	if *pings != 0 {
		t.Fatal("no request should be sent without a credential")
	}
}

func TestAPIErrorSurfacesMessage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/boom", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"message": "Insufficient scope"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := New(srv.URL, validStore("t"), srv.Client())
	err := c.GetJSON(context.Background(), "/api/boom", nil)
	if err == nil || err.Error() != "server error (HTTP 403): Insufficient scope" {
		t.Fatalf("err = %v", err)
	}
}
