package oauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// postToken drives handleToken directly (no router), the way every other
// oauth package test does. Because handleToken reports every outcome —
// success and error alike — via re.JSON, which calls e.Response.WriteHeader
// itself (see tools/router/event.go), rec.Code IS reliable here: unlike
// enforceGrant (middleware.go), nothing in this file returns a bare
// *router.ApiError for router middleware to translate into a status that
// would never run in this harness. Each assertion below still says which
// signal it uses and why, since that reliability is per-path, not a package
// default.
func postToken(t *testing.T, app core.App, form url.Values) (*httptest.ResponseRecorder, TokenResponse) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/oauth/token",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rec
	_ = handleToken(app, re)

	var resp TokenResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec, resp
}

// postTokenExpectingError is postToken's counterpart for rejection paths: it
// decodes the TokenErrorResponse body instead, which is the actual signal
// handleToken produces for a 4xx (via tokenError -> re.JSON), and is what a
// real OAuth client parses.
func postTokenExpectingError(t *testing.T, app core.App, form url.Values) (*httptest.ResponseRecorder, TokenErrorResponse) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/oauth/token",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rec
	_ = handleToken(app, re)

	var errResp TokenErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error response: %v — body %s", err, rec.Body.String())
	}
	return rec, errResp
}

// approvedDeviceGrant runs the device flow up to the point of user approval.
func approvedDeviceGrant(t *testing.T, app *tests.TestApp) (deviceCode string, userID string) {
	t.Helper()
	uid, clientRecID := seedUserAndClient(t, app)

	code, err := randomToken(32)
	if err != nil {
		t.Fatalf("randomToken: %v", err)
	}
	col, err := app.FindCollectionByNameOrId(grantsCollection)
	if err != nil {
		t.Fatalf("find grants: %v", err)
	}
	jti, _ := randomToken(24)
	g := core.NewRecord(col)
	g.Set("user", uid)
	g.Set("client", clientRecID)
	g.Set("jti", jti)
	g.Set("scopes", ScopeMailRead)
	g.Set("status", "active") // approved
	g.Set("device_code", hashSecret(code))
	g.Set("expires_at", time.Now().Add(DeviceCodeTTL))
	if err := app.Save(g); err != nil {
		t.Fatalf("save grant: %v", err)
	}
	return code, uid
}

func TestTokenDeviceGrantReturnsAccessToken(t *testing.T) {
	app := newSchemaApp(t)
	deviceCode, _ := approvedDeviceGrant(t, app)

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("device_code", deviceCode)
	form.Set("client_id", "tinycld-cli")

	// Success path: handleDeviceTokenGrant's final line is
	// re.JSON(http.StatusOK, resp), which writes rec.Code itself. rec.Code is
	// a reliable signal on this path.
	rec, resp := postToken(t, app, form)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if resp.AccessToken == "" {
		t.Error("access_token must be present")
	}
	if resp.RefreshToken == "" {
		t.Error("refresh_token must be present")
	}
	if resp.TokenType != "Bearer" {
		t.Errorf("token_type = %q, want Bearer", resp.TokenType)
	}
	if resp.ExpiresIn <= 0 {
		t.Error("expires_in must be positive")
	}
	// The issued token must actually work.
	if _, err := app.FindAuthRecordByToken(resp.AccessToken, core.TokenTypeAuth); err != nil {
		t.Fatalf("issued access token does not resolve: %v", err)
	}
}

func TestTokenDeviceGrantPendingReturnsAuthorizationPending(t *testing.T) {
	app := newSchemaApp(t)
	uid, clientRecID := seedUserAndClient(t, app)

	code, _ := randomToken(32)
	col, _ := app.FindCollectionByNameOrId(grantsCollection)
	jti, _ := randomToken(24)
	g := core.NewRecord(col)
	g.Set("user", uid)
	g.Set("client", clientRecID)
	g.Set("jti", jti)
	g.Set("scopes", ScopeMailRead)
	g.Set("status", "pending") // NOT yet approved
	g.Set("device_code", hashSecret(code))
	g.Set("expires_at", time.Now().Add(DeviceCodeTTL))
	if err := app.Save(g); err != nil {
		t.Fatalf("save grant: %v", err)
	}

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("device_code", code)
	form.Set("client_id", "tinycld-cli")

	// This is not a rejection the way the harness trap warns about: the
	// "pending" branch calls tokenError -> re.JSON(400, ...), which writes
	// rec.Code itself (no router middleware involved). Still, the real signal
	// an OAuth client reads is the decoded body's Error field, per RFC 8628
	// §3.5 — so assert on that, not the numeric code.
	rec, errResp := postTokenExpectingError(t, app, form)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if errResp.Error != "authorization_pending" {
		t.Fatalf("error = %q, want authorization_pending", errResp.Error)
	}
}

func TestTokenDeviceGrantRejectsUnknownDeviceCode(t *testing.T) {
	app := newSchemaApp(t)
	seedUserAndClient(t, app)

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("device_code", "totally-made-up")
	form.Set("client_id", "tinycld-cli")

	// Rejection path: assert on the decoded TokenErrorResponse.Error, the
	// actual signal handleDeviceTokenGrant produces here (tokenError ->
	// re.JSON writes rec.Code too, so both are checked, but the body is what
	// callers key behavior on).
	rec, errResp := postTokenExpectingError(t, app, form)
	if rec.Code == http.StatusOK {
		t.Fatal("an unknown device_code must not yield a token")
	}
	if errResp.Error != "invalid_grant" {
		t.Fatalf("error = %q, want invalid_grant", errResp.Error)
	}
}

func TestTokenDeviceCodeIsSingleUse(t *testing.T) {
	app := newSchemaApp(t)
	deviceCode, _ := approvedDeviceGrant(t, app)

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("device_code", deviceCode)
	form.Set("client_id", "tinycld-cli")

	if rec, _ := postToken(t, app, form); rec.Code != http.StatusOK {
		t.Fatalf("first exchange failed: %s", rec.Body.String())
	}
	// Replay must fail: the code is consumed on first use (issueTokens clears
	// device_code). This is the mutation-test target for "used device_code
	// redeemable twice" — the rejection path writes its own 400 via
	// tokenError -> re.JSON, so rec.Code is reliable, and the body confirms
	// WHY: it must again read as an unknown/invalid grant, not a fluke.
	rec, errResp := postTokenExpectingError(t, app, form)
	if rec.Code == http.StatusOK {
		t.Fatal("a device_code must not be redeemable twice")
	}
	if errResp.Error != "invalid_grant" {
		t.Fatalf("replay error = %q, want invalid_grant", errResp.Error)
	}
}

func TestTokenRefreshRotatesTheRefreshToken(t *testing.T) {
	app := newSchemaApp(t)
	deviceCode, _ := approvedDeviceGrant(t, app)

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("device_code", deviceCode)
	form.Set("client_id", "tinycld-cli")
	_, first := postToken(t, app, form)

	refreshForm := url.Values{}
	refreshForm.Set("grant_type", "refresh_token")
	refreshForm.Set("refresh_token", first.RefreshToken)
	refreshForm.Set("client_id", "tinycld-cli")

	// Success path: rec.Code is reliable (re.JSON writes it directly).
	rec, second := postToken(t, app, refreshForm)
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh failed: %s", rec.Body.String())
	}
	if second.AccessToken == "" {
		t.Fatal("refresh must return a new access token")
	}
	// Rotation: the old refresh token must stop working. This is the
	// mutation-test target for "refresh token does not rotate" — comparing
	// the token strings themselves is what actually catches that, not the
	// status code.
	if second.RefreshToken == first.RefreshToken {
		t.Fatal("refresh token must rotate on use")
	}
	// Replay of the OLD refresh token: rejection path, so assert on the
	// decoded error body, which is what handleRefreshGrant actually produces
	// on this branch (tokenError -> re.JSON with invalid_grant).
	replay, replayErr := postTokenExpectingError(t, app, refreshForm)
	if replay.Code == http.StatusOK {
		t.Fatal("a used refresh token must not be redeemable again")
	}
	if replayErr.Error != "invalid_grant" {
		t.Fatalf("replay error = %q, want invalid_grant", replayErr.Error)
	}
}

func TestTokenRefreshFailsAfterRevocation(t *testing.T) {
	app := newSchemaApp(t)
	deviceCode, _ := approvedDeviceGrant(t, app)

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("device_code", deviceCode)
	form.Set("client_id", "tinycld-cli")
	_, issued := postToken(t, app, form)

	grant, err := FindGrantByJTI(app, grantIDFromToken(issued.AccessToken))
	if err != nil {
		t.Fatalf("FindGrantByJTI: %v", err)
	}
	if err := RevokeGrant(app, grant.Id); err != nil {
		t.Fatalf("RevokeGrant: %v", err)
	}

	refreshForm := url.Values{}
	refreshForm.Set("grant_type", "refresh_token")
	refreshForm.Set("refresh_token", issued.RefreshToken)
	refreshForm.Set("client_id", "tinycld-cli")

	// RevokeGrant clears refresh_token_hash entirely (grants.go), so this
	// actually exercises "unknown refresh token", not the explicit
	// status=="revoked" branch — either way it's a rejection reported via
	// tokenError -> re.JSON, so assert on the decoded body.
	rec, errResp := postTokenExpectingError(t, app, refreshForm)
	if rec.Code == http.StatusOK {
		t.Fatal("a revoked grant must not be refreshable")
	}
	if errResp.Error != "invalid_grant" {
		t.Fatalf("error = %q, want invalid_grant", errResp.Error)
	}
}

func TestTokenRejectsUnsupportedGrantType(t *testing.T) {
	app := newSchemaApp(t)
	seedUserAndClient(t, app)

	form := url.Values{}
	form.Set("grant_type", "password") // removed in OAuth 2.1
	form.Set("client_id", "tinycld-cli")

	// Rejection path via the default case in handleToken's switch, reported
	// through tokenError -> re.JSON. Assert on the decoded body: it is the
	// signal that actually distinguishes "we understood and refused" from a
	// generic failure.
	rec, errResp := postTokenExpectingError(t, app, form)
	if rec.Code == http.StatusOK {
		t.Fatal("the password grant is not supported and must be refused")
	}
	if errResp.Error != "unsupported_grant_type" {
		t.Fatalf("error = %q, want unsupported_grant_type", errResp.Error)
	}
}

func TestTokenAuthCodeGrantRequiresValidPKCE(t *testing.T) {
	// Mutation-test target: "skip PKCE verification in the authorization_code
	// path". A correct code but a WRONG verifier must be refused; if PKCE
	// verification were ever skipped or short-circuited, this would silently
	// pass and turn green on a compromised endpoint.
	app := newSchemaApp(t)
	uid, clientRecID := seedUserAndClient(t, app)

	code, err := randomToken(32)
	if err != nil {
		t.Fatalf("randomToken: %v", err)
	}
	col, err := app.FindCollectionByNameOrId(grantsCollection)
	if err != nil {
		t.Fatalf("find grants: %v", err)
	}
	jti, _ := randomToken(24)
	g := core.NewRecord(col)
	g.Set("user", uid)
	g.Set("client", clientRecID)
	g.Set("jti", jti)
	g.Set("scopes", ScopeMailRead)
	g.Set("status", "active")
	g.Set("auth_code_hash", hashSecret(code))
	g.Set("redirect_uri", "https://example.com/callback")
	// challenge for a DIFFERENT verifier than the one submitted below.
	g.Set("code_challenge", "wrong-challenge-does-not-match-any-verifier")
	g.Set("expires_at", time.Now().Add(AuthCodeTTL))
	if err := app.Save(g); err != nil {
		t.Fatalf("save grant: %v", err)
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("code_verifier", "some-verifier-that-does-not-match-the-challenge")
	form.Set("redirect_uri", "https://example.com/callback")
	form.Set("client_id", "tinycld-cli")

	rec, errResp := postTokenExpectingError(t, app, form)
	if rec.Code == http.StatusOK {
		t.Fatal("a mismatched PKCE verifier must not yield a token")
	}
	if errResp.Error != "invalid_grant" {
		t.Fatalf("error = %q, want invalid_grant", errResp.Error)
	}

	// Sanity check the grant is still usable with the CORRECT verifier for a
	// verifier that actually hashes to the stored challenge, proving the
	// rejection above was really about PKCE and not some other mismatch
	// (redirect_uri, client, expiry).
	verifier, err := randomToken(32)
	if err != nil {
		t.Fatalf("randomToken: %v", err)
	}
	g.Set("code_challenge", challengeFor(verifier))
	if err := app.Save(g); err != nil {
		t.Fatalf("save grant with matching challenge: %v", err)
	}
	form.Set("code_verifier", verifier)
	rec2, resp2 := postToken(t, app, form)
	if rec2.Code != http.StatusOK {
		t.Fatalf("exchange with a correct verifier should succeed: %s", rec2.Body.String())
	}
	if resp2.AccessToken == "" {
		t.Fatal("access_token must be present on a valid PKCE exchange")
	}
}

func TestTokenExchangeThrottlesRepeatedFailures(t *testing.T) {
	app := newSchemaApp(t)
	seedUserAndClient(t, app)

	// Point the throttle at a controllable clock and a clean map so this test
	// cannot interfere with, or be interfered by, others sharing the package
	// singleton.
	restore := installTestTokenThrottle()
	defer restore()

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("device_code", "guess-me")
	form.Set("client_id", "tinycld-cli")

	var last *httptest.ResponseRecorder
	var lastErr TokenErrorResponse
	for i := 0; i < tokenMaxFailures+1; i++ {
		last, lastErr = postTokenExpectingError(t, app, form)
	}
	if last.Code != http.StatusTooManyRequests {
		t.Fatalf("after %d failed attempts, status = %d, want 429", tokenMaxFailures+1, last.Code)
	}
	if lastErr.Error != "slow_down" {
		t.Fatalf("error = %q, want slow_down", lastErr.Error)
	}
}

func TestTokenExchangeAuthorizationPendingNeverThrottles(t *testing.T) {
	// The normal shape of a device-flow login is the CLI polling every few
	// seconds while the user is off in a browser; every one of those polls
	// returns authorization_pending. If that counted as a failure, ordinary
	// logins would start 429ing before the user finishes approving.
	app := newSchemaApp(t)
	deviceCode, _ := pendingDeviceGrant(t, app)
	restore := installTestTokenThrottle()
	defer restore()

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("device_code", deviceCode)
	form.Set("client_id", "tinycld-cli")

	for i := 0; i < tokenMaxFailures*3; i++ {
		rec, errResp := postTokenExpectingError(t, app, form)
		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("authorization_pending tripped the throttle after %d polls", i+1)
		}
		if errResp.Error != "authorization_pending" {
			t.Fatalf("poll %d: error = %q, want authorization_pending", i+1, errResp.Error)
		}
	}
}

// pendingDeviceGrant is approvedDeviceGrant's counterpart for a device grant
// nobody has approved yet, so its poll response is authorization_pending.
func pendingDeviceGrant(t *testing.T, app *tests.TestApp) (deviceCode string, userID string) {
	t.Helper()
	uid, clientRecID := seedUserAndClient(t, app)

	code, err := randomToken(32)
	if err != nil {
		t.Fatalf("randomToken: %v", err)
	}
	col, err := app.FindCollectionByNameOrId(grantsCollection)
	if err != nil {
		t.Fatalf("find grants: %v", err)
	}
	jti, _ := randomToken(24)
	g := core.NewRecord(col)
	g.Set("user", uid)
	g.Set("client", clientRecID)
	g.Set("jti", jti)
	g.Set("scopes", ScopeMailRead)
	g.Set("status", "pending")
	g.Set("device_code", hashSecret(code))
	g.Set("expires_at", time.Now().Add(DeviceCodeTTL))
	if err := app.Save(g); err != nil {
		t.Fatalf("save grant: %v", err)
	}
	return code, uid
}

// installTestTokenThrottle swaps the package-singleton throttle for a fresh
// one with a controllable clock, and returns a func to restore the original.
// Tests that exercise the throttle must not share state with each other or
// with every other test in this file that calls postToken/postTokenExpectingError
// on a rejection path — those would otherwise silently accumulate failures
// against defaultTokenThrottle across the whole test binary run.
func installTestTokenThrottle() func() {
	original := defaultTokenThrottle
	now := time.Now()
	defaultTokenThrottle = &tokenThrottle{
		failures: map[string][]time.Time{},
		now:      func() time.Time { return now },
	}
	return func() { defaultTokenThrottle = original }
}

// seedSecondClient adds a second registered public client, "tinycld-cli-2",
// distinct from the one seedUserAndClient creates ("tinycld-cli"). Deliberately
// a local helper rather than a change to the shared seedUserAndClient — most
// tests in this package only need one client, and widening that helper's
// signature would touch every existing caller.
func seedSecondClient(t *testing.T, app *tests.TestApp) (clientRecID string) {
	t.Helper()
	clients, err := app.FindCollectionByNameOrId(clientsCollection)
	if err != nil {
		t.Fatalf("find clients: %v", err)
	}
	c := core.NewRecord(clients)
	c.Set("client_id", "tinycld-cli-2")
	c.Set("name", "A Different Client")
	c.Set("type", "public")
	c.Set("is_first_party", false)
	if err := app.Save(c); err != nil {
		t.Fatalf("save second client: %v", err)
	}
	return c.Id
}

// TestTokenRefreshRejectsWrongClient is CRITICAL gap #1: a refresh token
// issued to client A must not be redeemable by client B presenting its own
// client_id. Without the grant.GetString("client") != client.Id check in
// handleRefreshGrant, "a valid client" was sufficient — "the client this
// grant belongs to" was never verified. Any client holding a leaked, logged,
// or intercepted refresh token belonging to a DIFFERENT client could mint
// access tokens for that other client's user.
func TestTokenRefreshRejectsWrongClient(t *testing.T) {
	app := newSchemaApp(t)
	seedSecondClient(t, app)
	deviceCode, _ := approvedDeviceGrant(t, app) // issued to "tinycld-cli"

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("device_code", deviceCode)
	form.Set("client_id", "tinycld-cli")
	_, issued := postToken(t, app, form)
	if issued.RefreshToken == "" {
		t.Fatal("setup: device exchange did not return a refresh token")
	}

	// Client A's refresh token, presented under client B's identity.
	refreshForm := url.Values{}
	refreshForm.Set("grant_type", "refresh_token")
	refreshForm.Set("refresh_token", issued.RefreshToken)
	refreshForm.Set("client_id", "tinycld-cli-2")

	rec, errResp := postTokenExpectingError(t, app, refreshForm)
	if rec.Code == http.StatusOK {
		t.Fatal("a refresh token must not be redeemable by a client it was not issued to")
	}
	if errResp.Error != "invalid_grant" {
		t.Fatalf("error = %q, want invalid_grant", errResp.Error)
	}
}

// TestTokenRefreshRejectsExpiredGrant is CRITICAL gap #2: handleRefreshGrant
// checked only status == "revoked", never expires_at — even though
// issueTokens deliberately repurposes expires_at as the REFRESH deadline
// (RefreshTokenTTL) the moment a grant goes active. Without an expiry check
// here, that deadline is decorative: a still-"active" grant whose refresh
// window closed months ago stays refreshable forever.
func TestTokenRefreshRejectsExpiredGrant(t *testing.T) {
	app := newSchemaApp(t)
	deviceCode, _ := approvedDeviceGrant(t, app)

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("device_code", deviceCode)
	form.Set("client_id", "tinycld-cli")
	_, issued := postToken(t, app, form)

	grant, err := FindGrantByJTI(app, grantIDFromToken(issued.AccessToken))
	if err != nil {
		t.Fatalf("FindGrantByJTI: %v", err)
	}
	// issueTokens just set expires_at to now+RefreshTokenTTL; force it into
	// the past to simulate an old refresh token past its deadline.
	grant.Set("expires_at", time.Now().Add(-time.Hour))
	if err := app.Save(grant); err != nil {
		t.Fatalf("save expired grant: %v", err)
	}

	refreshForm := url.Values{}
	refreshForm.Set("grant_type", "refresh_token")
	refreshForm.Set("refresh_token", issued.RefreshToken)
	refreshForm.Set("client_id", "tinycld-cli")

	rec, errResp := postTokenExpectingError(t, app, refreshForm)
	if rec.Code == http.StatusOK {
		t.Fatal("a refresh token past its grant's expires_at must not be redeemable")
	}
	if errResp.Error != "invalid_grant" {
		t.Fatalf("error = %q, want invalid_grant", errResp.Error)
	}
}

// TestTokenDeviceGrantRejectsWrongClient is IMPORTANT gap #3, the device-flow
// counterpart of the refresh-token client-binding gap: a device_code issued
// to client A must not be redeemable by client B. Lower severity than the
// refresh case (device_code is high-entropy and single-use, so the exposure
// window is narrower) but the same missing check.
func TestTokenDeviceGrantRejectsWrongClient(t *testing.T) {
	app := newSchemaApp(t)
	seedSecondClient(t, app)
	deviceCode, _ := approvedDeviceGrant(t, app) // issued to "tinycld-cli"

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("device_code", deviceCode)
	form.Set("client_id", "tinycld-cli-2") // a DIFFERENT, but still registered, client

	rec, errResp := postTokenExpectingError(t, app, form)
	if rec.Code == http.StatusOK {
		t.Fatal("a device_code must not be redeemable by a client it was not issued to")
	}
	if errResp.Error != "invalid_grant" {
		t.Fatalf("error = %q, want invalid_grant", errResp.Error)
	}
}
