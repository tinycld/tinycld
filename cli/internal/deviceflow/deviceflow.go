// Package deviceflow implements the client half of the RFC 8628 device
// authorization grant against a TinyCld server's /oauth endpoints.
package deviceflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"tinycld.org/cli/client"
)

var (
	// ErrExpired means the user did not approve before the device code's TTL.
	ErrExpired = errors.New("the login request expired — run `tinycld auth login` again")
	// ErrDenied means the user explicitly rejected the request.
	ErrDenied = errors.New("the login request was denied")
)

type Flow struct {
	Origin   string
	ClientID string
	HTTP     *http.Client
	// Sleep is injectable so tests poll instantly.
	Sleep func(time.Duration)
	Now   func() time.Time
}

// DeviceAuth is the RFC 8628 §3.2 device authorization response.
type DeviceAuth struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

type tokenError struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func (f *Flow) sleep(d time.Duration) {
	if f.Sleep != nil {
		f.Sleep(d)
		return
	}
	time.Sleep(d)
}

func (f *Flow) now() time.Time {
	if f.Now != nil {
		return f.Now()
	}
	return time.Now()
}

// Start requests a device authorization (POST /oauth/device).
func (f *Flow) Start(ctx context.Context, scopes []string) (*DeviceAuth, error) {
	form := url.Values{
		"client_id": {f.ClientID},
		"scope":     {strings.Join(scopes, " ")},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		f.Origin+"/oauth/device", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := f.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("contacting %s: %w", f.Origin, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s does not accept device login (HTTP %d) — is it a TinyCld server?",
			f.Origin, resp.StatusCode)
	}
	var da DeviceAuth
	if err := json.Unmarshal(body, &da); err != nil {
		return nil, fmt.Errorf("unexpected response from %s: %w", f.Origin, err)
	}
	if da.DeviceCode == "" || da.UserCode == "" {
		return nil, fmt.Errorf("unexpected response from %s: missing device code", f.Origin)
	}
	return &da, nil
}

// Poll exchanges the device code for tokens (POST /oauth/token), waiting for
// the user's browser approval. Polling states are signalled by the JSON
// `error` field, NOT the HTTP status — the server returns slow_down as 429
// while the other RFC 8628 states are 400.
func (f *Flow) Poll(ctx context.Context, da *DeviceAuth) (client.TokenSet, error) {
	interval := time.Duration(da.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	deadline := f.now().Add(time.Duration(da.ExpiresIn) * time.Second)

	for {
		// RFC 8628 §3.5: wait interval before the first poll too.
		f.sleep(interval)
		if err := ctx.Err(); err != nil {
			return client.TokenSet{}, err
		}
		if f.now().After(deadline) {
			return client.TokenSet{}, ErrExpired
		}

		form := url.Values{
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
			"device_code": {da.DeviceCode},
			"client_id":   {f.ClientID},
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			f.Origin+"/oauth/token", strings.NewReader(form.Encode()))
		if err != nil {
			return client.TokenSet{}, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := f.HTTP.Do(req)
		if err != nil {
			return client.TokenSet{}, fmt.Errorf("contacting %s: %w", f.Origin, err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return client.TokenSet{}, err
		}

		if resp.StatusCode == http.StatusOK {
			var tok tokenResponse
			if err := json.Unmarshal(body, &tok); err != nil {
				return client.TokenSet{}, fmt.Errorf("unexpected token response: %w", err)
			}
			return client.TokenSet{
				AccessToken:  tok.AccessToken,
				RefreshToken: tok.RefreshToken,
				Scope:        tok.Scope,
				Origin:       f.Origin,
				ExpiresAt:    f.now().Add(time.Duration(tok.ExpiresIn) * time.Second),
			}, nil
		}

		var te tokenError
		if err := json.Unmarshal(body, &te); err != nil {
			return client.TokenSet{}, fmt.Errorf("unexpected response (HTTP %d) from %s",
				resp.StatusCode, f.Origin)
		}
		switch te.Error {
		case "authorization_pending":
			// normal: the user has not approved yet
		case "slow_down":
			// RFC 8628 §3.5: add 5 seconds to the interval
			interval += 5 * time.Second
		case "expired_token":
			return client.TokenSet{}, ErrExpired
		case "access_denied":
			return client.TokenSet{}, ErrDenied
		default:
			if te.ErrorDescription != "" {
				return client.TokenSet{}, fmt.Errorf("%s: %s", te.Error, te.ErrorDescription)
			}
			return client.TokenSet{}, fmt.Errorf("login failed: %s", te.Error)
		}
	}
}
