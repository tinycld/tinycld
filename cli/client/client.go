// Package client is the authenticated HTTP client every CLI command talks to
// a TinyCld server through. It attaches the OAuth access token, refreshes it
// when expired (persisting the rotated refresh token), and retries the
// original request exactly once after a mid-flight 401.
//
// It is exported (not internal/) because package command modules
// (tinycld.org/packages/<slug>/cli) receive a *Client.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DefaultClientID is the first-party public OAuth client every deployment
// seeds for the CLI.
const DefaultClientID = "tinycld-cli"

// ErrAuthExpired means there is no usable credential for the context — the
// user must log in again.
var ErrAuthExpired = errors.New("authentication expired — run `tinycld auth login`")

// TokenSet is the credential bundle stored (as JSON) in the OS keychain,
// one per context.
type TokenSet struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Scope        string    `json:"scope"`
	Origin       string    `json:"origin"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// TokenStore persists a context's TokenSet. Load returns ErrAuthExpired when
// no credential exists.
type TokenStore interface {
	Load() (TokenSet, error)
	Save(TokenSet) error
}

type Client struct {
	origin   string
	store    TokenStore
	http     *http.Client
	ClientID string

	// resolve fills origin+store on first use (see NewLazy).
	resolve     func() (origin string, store TokenStore, err error)
	resolveOnce sync.Once
	resolveErr  error

	// mu serializes refreshes so concurrent requests can't both rotate the
	// refresh token (the server invalidates the old one on every rotation).
	mu sync.Mutex

	// userMu guards userID, the memoized /oauth/userinfo subject.
	userMu sync.Mutex
	userID string
}

func New(origin string, store TokenStore, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{origin: origin, store: store, http: hc, ClientID: DefaultClientID}
}

// NewLazy defers context resolution to the first request. Package commands
// receive their *Client at Cobra wiring time — before flags are parsed and
// before any context has to exist — so commands that never touch the network
// (--help, completion) must not force one to resolve.
func NewLazy(resolve func() (string, TokenStore, error), hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{resolve: resolve, http: hc, ClientID: DefaultClientID}
}

func (c *Client) ensure() error {
	if c.resolve == nil {
		return nil
	}
	c.resolveOnce.Do(func() {
		origin, store, err := c.resolve()
		if err != nil {
			c.resolveErr = err
			return
		}
		c.origin, c.store = origin, store
	})
	return c.resolveErr
}

func (c *Client) Origin() string { return c.origin }

// Token returns the stored TokenSet, refreshing first when it is expired or
// about to expire.
func (c *Client) Token() (TokenSet, error) {
	if err := c.ensure(); err != nil {
		return TokenSet{}, err
	}
	tok, err := c.store.Load()
	if err != nil {
		return TokenSet{}, err
	}
	if time.Until(tok.ExpiresAt) < time.Minute {
		return c.refresh(tok)
	}
	return tok, nil
}

// Do sends req with authentication. On a 401 it refreshes and retries the
// request exactly once — only when the body is rewindable (GetBody set or no
// body at all).
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	tok, err := c.Token()
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}
	if req.Body != nil && req.GetBody == nil {
		return resp, nil
	}
	resp.Body.Close()

	tok, err = c.refresh(tok)
	if err != nil {
		return nil, err
	}
	retry := req.Clone(req.Context())
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		retry.Body = body
	}
	retry.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	return c.http.Do(retry)
}

// GetJSON GETs path (origin-relative) and decodes the response into out.
func (c *Client) GetJSON(ctx context.Context, path string, out any) error {
	if err := c.ensure(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.origin+path, nil)
	if err != nil {
		return err
	}
	return c.doJSON(req, out)
}

// PostJSON POSTs body (JSON-encoded) to path and decodes the response.
func (c *Client) PostJSON(ctx context.Context, path string, body, out any) error {
	return c.sendJSON(ctx, http.MethodPost, path, body, out)
}

// PatchJSON PATCHes body (JSON-encoded) to path and decodes the response.
func (c *Client) PatchJSON(ctx context.Context, path string, body, out any) error {
	return c.sendJSON(ctx, http.MethodPatch, path, body, out)
}

func (c *Client) sendJSON(ctx context.Context, method, path string, body, out any) error {
	if err := c.ensure(); err != nil {
		return err
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.origin+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	return c.doJSON(req, out)
}

// Delete sends an authenticated DELETE and discards the (typically empty)
// response body.
func (c *Client) Delete(ctx context.Context, path string) error {
	if err := c.ensure(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.origin+path, nil)
	if err != nil {
		return err
	}
	return c.doJSON(req, nil)
}

// Get performs an authenticated GET and hands back the response body for
// streaming (file content, downloads). The caller must close it. A non-2xx
// status is drained into the same error shape doJSON produces.
func (c *Client) Get(ctx context.Context, path string) (io.ReadCloser, *http.Response, error) {
	if err := c.ensure(); err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.origin+path, nil)
	if err != nil {
		return nil, nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, nil, apiError(resp.StatusCode, data)
	}
	return resp.Body, resp, nil
}

// UserID returns the authenticated user's record id (the userinfo `sub`
// claim), memoized for the process lifetime. Commands need it to filter
// per-user rows and to fill relation fields like created_by.
func (c *Client) UserID(ctx context.Context) (string, error) {
	c.userMu.Lock()
	defer c.userMu.Unlock()
	if c.userID != "" {
		return c.userID, nil
	}
	var info struct {
		Sub string `json:"sub"`
	}
	if err := c.GetJSON(ctx, "/oauth/userinfo", &info); err != nil {
		return "", fmt.Errorf("resolving current user: %w", err)
	}
	if info.Sub == "" {
		return "", errors.New("userinfo response carried no subject")
	}
	c.userID = info.Sub
	return c.userID, nil
}

func (c *Client) doJSON(req *http.Request, out any) error {
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return apiError(resp.StatusCode, data)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

// refresh exchanges the refresh token for a new pair and persists the rotated
// set BEFORE returning — the old refresh token is dead server-side the moment
// the exchange succeeds, so failing to save would strand the user.
func (c *Client) refresh(stale TokenSet) (TokenSet, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Another request may have refreshed while we waited on the lock.
	if cur, err := c.store.Load(); err == nil &&
		cur.AccessToken != stale.AccessToken && time.Until(cur.ExpiresAt) >= time.Minute {
		return cur, nil
	}

	if stale.RefreshToken == "" {
		return TokenSet{}, ErrAuthExpired
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {stale.RefreshToken},
		"client_id":     {c.ClientID},
	}
	resp, err := c.http.Post(c.origin+"/oauth/token",
		"application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return TokenSet{}, fmt.Errorf("refreshing credentials: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return TokenSet{}, err
	}
	if resp.StatusCode != http.StatusOK {
		var te struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &te) == nil && te.Error == "invalid_grant" {
			return TokenSet{}, ErrAuthExpired
		}
		return TokenSet{}, fmt.Errorf("refreshing credentials failed (HTTP %d)", resp.StatusCode)
	}
	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(data, &tok); err != nil {
		return TokenSet{}, fmt.Errorf("unexpected token response: %w", err)
	}
	fresh := TokenSet{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		Scope:        tok.Scope,
		Origin:       stale.Origin,
		ExpiresAt:    time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second),
	}
	if err := c.store.Save(fresh); err != nil {
		return TokenSet{}, fmt.Errorf("saving refreshed credentials: %w", err)
	}
	return fresh, nil
}

// apiError shapes a non-2xx response into a readable error, surfacing the
// server's message field when one exists.
func apiError(status int, body []byte) error {
	var pb struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &pb) == nil && pb.Message != "" {
		return fmt.Errorf("server error (HTTP %d): %s", status, pb.Message)
	}
	return fmt.Errorf("server error (HTTP %d)", status)
}
