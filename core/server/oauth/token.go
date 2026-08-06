package oauth

import (
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// Grant type identifiers. OAuth 2.1 removes `password` and `implicit`; we
// implement only these three.
const (
	grantTypeDevice   = "urn:ietf:params:oauth:grant-type:device_code"
	grantTypeAuthCode = "authorization_code"
	grantTypeRefresh  = "refresh_token"
)

// TokenResponse is the RFC 6749 §5.1 successful token response.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// TokenErrorResponse is the RFC 6749 §5.2 error response. The device flow
// leans on it heavily: `authorization_pending` and `slow_down` are normal
// polling states, not failures.
type TokenErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// handleToken implements POST /oauth/token for all three supported grants.
func handleToken(app core.App, re *core.RequestEvent) error {
	if err := re.Request.ParseForm(); err != nil {
		return tokenError(re, http.StatusBadRequest, "invalid_request", "Malformed form body")
	}
	grantType := re.Request.FormValue("grant_type")

	// This endpoint is reachable with no credentials, and device_code /
	// user_code are guessable secrets — see ratelimit.go. Checked before any
	// database work so a throttled attacker cannot spend our bcrypt/DB time
	// either.
	if tooManyTokenFailures(app, re.Request, grantType) {
		return tokenError(re, http.StatusTooManyRequests, "slow_down",
			"Too many failed attempts; wait before retrying")
	}

	switch grantType {
	case grantTypeDevice:
		return handleDeviceTokenGrant(app, re)
	case grantTypeAuthCode:
		return handleAuthCodeGrant(app, re)
	case grantTypeRefresh:
		return handleRefreshGrant(app, re)
	default:
		noteTokenFailure(app, re.Request, grantType)
		return tokenError(re, http.StatusBadRequest, "unsupported_grant_type",
			"Supported grants: device_code, authorization_code, refresh_token")
	}
}

// tokenError writes an RFC 6749 §5.2 error body.
func tokenError(re *core.RequestEvent, status int, code, desc string) error {
	return re.JSON(status, TokenErrorResponse{Error: code, ErrorDescription: desc})
}

// authenticateClient resolves and (for confidential clients) authenticates the
// caller. A public client needs no secret; PKCE is what binds the exchange.
func authenticateClient(app core.App, re *core.RequestEvent) (*core.Record, error) {
	clientID := re.Request.FormValue("client_id")
	client, err := FindClientByClientID(app, clientID)
	if err != nil {
		return nil, err
	}
	if client.GetString("type") == "confidential" {
		if !VerifyClientSecret(client, re.Request.FormValue("client_secret")) {
			return nil, ErrInvalidGrant
		}
	}
	return client, nil
}

// issueTokens mints an access token plus a rotated refresh token and persists
// the refresh hash on the grant.
func issueTokens(
	app core.App,
	grant *core.Record,
	user *core.Record,
) (TokenResponse, error) {
	access, err := MintAccessToken(app, user, grant, AccessTokenTTL)
	if err != nil {
		return TokenResponse{}, err
	}
	refresh, err := randomToken(32)
	if err != nil {
		return TokenResponse{}, err
	}

	// refresh_token_hash is overwritten unconditionally, whether this call
	// originated from a first exchange or a refresh — that overwrite IS the
	// rotation: the previous hash is gone the moment this record saves, so a
	// replay of the old refresh token can never match a stored hash again.
	grant.Set("refresh_token_hash", hashSecret(refresh))
	grant.Set("status", "active")
	grant.Set("expires_at", time.Now().Add(RefreshTokenTTL))
	// Consume the one-shot codes so neither can be replayed.
	grant.Set("device_code", "")
	grant.Set("auth_code_hash", "")
	grant.Set("user_code", "")
	if err := app.Save(grant); err != nil {
		return TokenResponse{}, err
	}

	return TokenResponse{
		AccessToken:  access,
		TokenType:    "Bearer",
		ExpiresIn:    int(AccessTokenTTL.Seconds()),
		RefreshToken: refresh,
		Scope:        grant.GetString("scopes"),
	}, nil
}

// handleDeviceTokenGrant is the CLI's poll (RFC 8628 §3.4).
func handleDeviceTokenGrant(app core.App, re *core.RequestEvent) error {
	if _, err := authenticateClient(app, re); err != nil {
		noteTokenFailure(app, re.Request, grantTypeDevice)
		return tokenError(re, http.StatusUnauthorized, "invalid_client", "Unknown client")
	}
	deviceCode := re.Request.FormValue("device_code")
	if deviceCode == "" {
		noteTokenFailure(app, re.Request, grantTypeDevice)
		return tokenError(re, http.StatusBadRequest, "invalid_request", "device_code is required")
	}

	grant, err := app.FindFirstRecordByFilter(
		grantsCollection, "device_code = {:c}",
		map[string]any{"c": hashSecret(deviceCode)},
	)
	if err != nil || grant == nil {
		noteTokenFailure(app, re.Request, grantTypeDevice)
		return tokenError(re, http.StatusBadRequest, "invalid_grant", "Unknown device code")
	}
	if exp := grant.GetDateTime("expires_at"); !exp.IsZero() && exp.Time().Before(time.Now()) {
		noteTokenFailure(app, re.Request, grantTypeDevice)
		return tokenError(re, http.StatusBadRequest, "expired_token", "Device code expired")
	}
	switch grant.GetString("status") {
	case "pending":
		// Not a failure — RFC 8628 §3.5. The user simply has not approved
		// yet, and a CLI polls this every few seconds by design; counting it
		// would trip the throttle on every legitimate login.
		return tokenError(re, http.StatusBadRequest, "authorization_pending",
			"Waiting for the user to approve this device")
	case "revoked":
		noteTokenFailure(app, re.Request, grantTypeDevice)
		return tokenError(re, http.StatusBadRequest, "access_denied", "Request was denied")
	}

	user, err := app.FindRecordById("users", grant.GetString("user"))
	if err != nil {
		noteTokenFailure(app, re.Request, grantTypeDevice)
		return tokenError(re, http.StatusBadRequest, "invalid_grant", "Grant has no user")
	}
	resp, err := issueTokens(app, grant, user)
	if err != nil {
		return re.InternalServerError("Failed to issue tokens", err)
	}
	return re.JSON(http.StatusOK, resp)
}

// handleAuthCodeGrant is the Zapier path: authorization code + PKCE.
func handleAuthCodeGrant(app core.App, re *core.RequestEvent) error {
	client, err := authenticateClient(app, re)
	if err != nil {
		noteTokenFailure(app, re.Request, grantTypeAuthCode)
		return tokenError(re, http.StatusUnauthorized, "invalid_client", "Unknown client")
	}
	code := re.Request.FormValue("code")
	verifier := re.Request.FormValue("code_verifier")
	redirectURI := re.Request.FormValue("redirect_uri")
	if code == "" || verifier == "" {
		noteTokenFailure(app, re.Request, grantTypeAuthCode)
		return tokenError(re, http.StatusBadRequest, "invalid_request",
			"code and code_verifier are required")
	}

	grant, err := app.FindFirstRecordByFilter(
		grantsCollection, "auth_code_hash = {:c}",
		map[string]any{"c": hashSecret(code)},
	)
	if err != nil || grant == nil {
		noteTokenFailure(app, re.Request, grantTypeAuthCode)
		return tokenError(re, http.StatusBadRequest, "invalid_grant", "Unknown authorization code")
	}
	if exp := grant.GetDateTime("expires_at"); !exp.IsZero() && exp.Time().Before(time.Now()) {
		noteTokenFailure(app, re.Request, grantTypeAuthCode)
		return tokenError(re, http.StatusBadRequest, "invalid_grant", "Authorization code expired")
	}
	if grant.GetString("client") != client.Id {
		noteTokenFailure(app, re.Request, grantTypeAuthCode)
		return tokenError(re, http.StatusBadRequest, "invalid_grant", "Code was issued to another client")
	}
	// The redirect_uri must match the one the code was issued against.
	if grant.GetString("redirect_uri") != redirectURI {
		noteTokenFailure(app, re.Request, grantTypeAuthCode)
		return tokenError(re, http.StatusBadRequest, "invalid_grant", "redirect_uri mismatch")
	}
	if !VerifyPKCE(grant.GetString("code_challenge"), verifier) {
		noteTokenFailure(app, re.Request, grantTypeAuthCode)
		return tokenError(re, http.StatusBadRequest, "invalid_grant", "PKCE verification failed")
	}

	user, err := app.FindRecordById("users", grant.GetString("user"))
	if err != nil {
		noteTokenFailure(app, re.Request, grantTypeAuthCode)
		return tokenError(re, http.StatusBadRequest, "invalid_grant", "Grant has no user")
	}
	resp, err := issueTokens(app, grant, user)
	if err != nil {
		return re.InternalServerError("Failed to issue tokens", err)
	}
	return re.JSON(http.StatusOK, resp)
}

// handleRefreshGrant exchanges a refresh token for a new pair, rotating the
// refresh token so a leaked one has a bounded useful life.
func handleRefreshGrant(app core.App, re *core.RequestEvent) error {
	if _, err := authenticateClient(app, re); err != nil {
		noteTokenFailure(app, re.Request, grantTypeRefresh)
		return tokenError(re, http.StatusUnauthorized, "invalid_client", "Unknown client")
	}
	refresh := re.Request.FormValue("refresh_token")
	if refresh == "" {
		noteTokenFailure(app, re.Request, grantTypeRefresh)
		return tokenError(re, http.StatusBadRequest, "invalid_request", "refresh_token is required")
	}

	grant, err := app.FindFirstRecordByFilter(
		grantsCollection, "refresh_token_hash = {:h}",
		map[string]any{"h": hashSecret(refresh)},
	)
	if err != nil || grant == nil {
		noteTokenFailure(app, re.Request, grantTypeRefresh)
		return tokenError(re, http.StatusBadRequest, "invalid_grant", "Unknown refresh token")
	}
	if grant.GetString("status") == "revoked" {
		noteTokenFailure(app, re.Request, grantTypeRefresh)
		return tokenError(re, http.StatusBadRequest, "invalid_grant", "Grant was revoked")
	}

	user, err := app.FindRecordById("users", grant.GetString("user"))
	if err != nil {
		noteTokenFailure(app, re.Request, grantTypeRefresh)
		return tokenError(re, http.StatusBadRequest, "invalid_grant", "Grant has no user")
	}
	resp, err := issueTokens(app, grant, user)
	if err != nil {
		return re.InternalServerError("Failed to issue tokens", err)
	}
	return re.JSON(http.StatusOK, resp)
}
