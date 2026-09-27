package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPostStreamReturnsBodyAndSendsJSON(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access-1" {
			t.Errorf("auth header %q", r.Header.Get("Authorization"))
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type %q", ct)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("chunk-1"))
		w.(http.Flusher).Flush()
		_, _ = w.Write([]byte("chunk-2"))
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, validStore("access-1"), srv.Client())

	body, res, err := c.PostStream(context.Background(), "/api/x", map[string]any{"stream": true})
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	b, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "chunk-1chunk-2" || res.StatusCode != http.StatusOK || got["stream"] != true {
		t.Fatalf("body %q status %d sent %v", b, res.StatusCode, got)
	}
}

func TestPostStreamNon2xxIsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"The daily limit for manual backups has been reached."}`))
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, validStore("access-1"), srv.Client())

	_, _, err := c.PostStream(context.Background(), "/api/x", map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "HTTP 429") || !strings.Contains(err.Error(), "daily limit") {
		t.Fatalf("err %v", err)
	}
}

// TestPostStreamRetriesAfter401 pins the rewindable body: a stream POST whose
// token expired mid-flight must be replayed whole, not with a drained reader.
func TestPostStreamRetriesAfter401(t *testing.T) {
	var bodies []string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/x", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(raw))
		if r.Header.Get("Authorization") != "Bearer access-2" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"expired"}`))
			return
		}
		_, _ = w.Write([]byte("streamed"))
	})
	mux.HandleFunc("POST /oauth/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access-2", "token_type": "Bearer",
			"expires_in": 3600, "refresh_token": "refresh-2",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL, validStore("access-1"), srv.Client())

	body, _, err := c.PostStream(context.Background(), "/api/x", map[string]any{"k": "v"})
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	b, _ := io.ReadAll(body)
	if string(b) != "streamed" {
		t.Fatalf("body %q", b)
	}
	if len(bodies) != 2 || bodies[0] != bodies[1] {
		t.Fatalf("request bodies %q", bodies)
	}
}
