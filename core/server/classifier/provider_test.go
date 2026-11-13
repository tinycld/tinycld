package classifier

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenRouter_SendsKeyAndPayload(t *testing.T) {
	var gotAuth, gotType string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotType = r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(429)
	}))
	t.Cleanup(srv.Close)
	p := &OpenRouter{URL: srv.URL, Client: srv.Client()}
	resp, err := p.Decide(context.Background(), "k1", []byte(`{"a":1}`))
	if err != nil || resp.Status != 429 || resp.Header.Get("Retry-After") != "2" {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	if gotAuth != "Bearer k1" || gotType != "application/json" || string(gotBody) != `{"a":1}` {
		t.Fatalf("auth=%q type=%q body=%q", gotAuth, gotType, gotBody)
	}
}
