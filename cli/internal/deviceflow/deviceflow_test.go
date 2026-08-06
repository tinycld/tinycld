package deviceflow

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type pollStep struct {
	status int
	body   any
}

func newFlowServer(t *testing.T, steps []pollStep) (*Flow, *httptest.Server, *[]time.Duration) {
	t.Helper()
	var polls int
	mux := http.NewServeMux()
	mux.HandleFunc("POST /oauth/device", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.FormValue("client_id") != "tinycld-cli" {
			t.Errorf("client_id = %q", r.FormValue("client_id"))
		}
		json.NewEncoder(w).Encode(DeviceAuth{
			DeviceCode:              "dev-code",
			UserCode:                "WDJB-MJHT",
			VerificationURI:         "http://x/p/oauth/authorize",
			VerificationURIComplete: "http://x/p/oauth/authorize?user_code=WDJB-MJHT",
			ExpiresIn:               900,
			Interval:                5,
		})
	})
	mux.HandleFunc("POST /oauth/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if got := r.FormValue("grant_type"); got != "urn:ietf:params:oauth:grant-type:device_code" {
			t.Errorf("grant_type = %q", got)
		}
		if got := r.FormValue("device_code"); got != "dev-code" {
			t.Errorf("device_code = %q", got)
		}
		if polls >= len(steps) {
			t.Fatalf("unexpected poll #%d", polls+1)
		}
		step := steps[polls]
		polls++
		w.WriteHeader(step.status)
		json.NewEncoder(w).Encode(step.body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	sleeps := &[]time.Duration{}
	flow := &Flow{
		Origin:   srv.URL,
		ClientID: "tinycld-cli",
		HTTP:     srv.Client(),
		Sleep:    func(d time.Duration) { *sleeps = append(*sleeps, d) },
	}
	return flow, srv, sleeps
}

func start(t *testing.T, f *Flow) *DeviceAuth {
	t.Helper()
	da, err := f.Start(context.Background(), []string{"profile"})
	if err != nil {
		t.Fatal(err)
	}
	return da
}

func TestPollPendingThenSuccess(t *testing.T) {
	flow, _, sleeps := newFlowServer(t, []pollStep{
		{400, map[string]string{"error": "authorization_pending"}},
		{400, map[string]string{"error": "authorization_pending"}},
		{200, map[string]any{
			"access_token": "at", "token_type": "Bearer",
			"expires_in": 3600, "refresh_token": "rt", "scope": "profile",
		}},
	})
	da := start(t, flow)
	if da.UserCode != "WDJB-MJHT" {
		t.Fatalf("user code = %q", da.UserCode)
	}

	tok, err := flow.Poll(context.Background(), da)
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "at" || tok.RefreshToken != "rt" || tok.Scope != "profile" {
		t.Fatalf("token = %+v", tok)
	}
	if tok.Origin == "" {
		t.Fatal("token origin not set")
	}
	if until := time.Until(tok.ExpiresAt); until < 59*time.Minute || until > 61*time.Minute {
		t.Fatalf("expires_at not derived from expires_in: %v", tok.ExpiresAt)
	}
	if len(*sleeps) != 3 {
		t.Fatalf("sleeps = %v (want one wait before each poll)", *sleeps)
	}
	for _, s := range *sleeps {
		if s != 5*time.Second {
			t.Fatalf("sleep = %v, want 5s", s)
		}
	}
}

func TestPollSlowDownGrowsInterval(t *testing.T) {
	// slow_down arrives as HTTP 429 — the client must branch on the error
	// body, not the status code.
	flow, _, sleeps := newFlowServer(t, []pollStep{
		{429, map[string]string{"error": "slow_down"}},
		{400, map[string]string{"error": "authorization_pending"}},
		{200, map[string]any{"access_token": "at", "expires_in": 3600}},
	})
	if _, err := flow.Poll(context.Background(), start(t, flow)); err != nil {
		t.Fatal(err)
	}
	want := []time.Duration{5 * time.Second, 10 * time.Second, 10 * time.Second}
	if len(*sleeps) != len(want) {
		t.Fatalf("sleeps = %v", *sleeps)
	}
	for i, s := range *sleeps {
		if s != want[i] {
			t.Fatalf("sleeps = %v, want %v", *sleeps, want)
		}
	}
}

func TestPollExpiredToken(t *testing.T) {
	flow, _, _ := newFlowServer(t, []pollStep{
		{400, map[string]string{"error": "expired_token"}},
	})
	_, err := flow.Poll(context.Background(), start(t, flow))
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("err = %v, want ErrExpired", err)
	}
}

func TestPollAccessDenied(t *testing.T) {
	flow, _, _ := newFlowServer(t, []pollStep{
		{400, map[string]string{"error": "access_denied"}},
	})
	_, err := flow.Poll(context.Background(), start(t, flow))
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("err = %v, want ErrDenied", err)
	}
}

func TestPollDeadline(t *testing.T) {
	flow, _, _ := newFlowServer(t, nil)
	da := start(t, flow)
	da.ExpiresIn = 1

	now := time.Now()
	flow.Now = func() time.Time { return now }
	flow.Sleep = func(time.Duration) { now = now.Add(2 * time.Second) }

	_, err := flow.Poll(context.Background(), da)
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("err = %v, want ErrExpired", err)
	}
}

func TestPollUnknownErrorSurfacesDescription(t *testing.T) {
	flow, _, _ := newFlowServer(t, []pollStep{
		{400, map[string]string{"error": "invalid_grant", "error_description": "Unknown device code"}},
	})
	_, err := flow.Poll(context.Background(), start(t, flow))
	if err == nil || err.Error() != "invalid_grant: Unknown device code" {
		t.Fatalf("err = %v", err)
	}
}

func TestStartRejectsNonTinyCldServer(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	flow := &Flow{Origin: srv.URL, ClientID: "tinycld-cli", HTTP: srv.Client()}
	if _, err := flow.Start(context.Background(), nil); err == nil {
		t.Fatal("expected error for a non-TinyCld server")
	}
}
