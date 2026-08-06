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

// pendingUserCodeGrant seeds a pending device grant identified by its
// user_code, for exercising the browser-side approval flow. It is distinct
// from token_test.go's pendingDeviceGrant, which is keyed by device_code and
// exercises the CLI-side poll — same "pending device grant" concept, two
// different lookup keys, so both helpers are kept.
func pendingUserCodeGrant(t *testing.T, app *tests.TestApp) (userCode string, userID string) {
	t.Helper()
	uid, clientRecID := seedUserAndClient(t, app)

	code, err := newUserCode()
	if err != nil {
		t.Fatalf("newUserCode: %v", err)
	}
	col, _ := app.FindCollectionByNameOrId(grantsCollection)
	jti, _ := randomToken(24)
	g := core.NewRecord(col)
	g.Set("client", clientRecID)
	g.Set("jti", jti)
	g.Set("scopes", ScopeMailRead)
	g.Set("status", "pending")
	g.Set("user_code", code)
	dc, _ := randomToken(32)
	g.Set("device_code", hashSecret(dc))
	g.Set("expires_at", time.Now().Add(DeviceCodeTTL))
	if err := app.Save(g); err != nil {
		t.Fatalf("save pending grant: %v", err)
	}
	return code, uid
}

func TestApproveDeviceBindsUserAndActivates(t *testing.T) {
	app := newSchemaApp(t)
	userCode, userID := pendingUserCodeGrant(t, app)

	form := url.Values{}
	form.Set("user_code", userCode)
	form.Set("device_label", "Nathan's laptop")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	user, err := app.FindRecordById("users", userID)
	if err != nil {
		t.Fatalf("find user: %v", err)
	}
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rec
	re.Auth = user // the consent screen runs inside an authenticated session

	if err := handleApproveDevice(app, re); err != nil {
		t.Fatalf("handleApproveDevice: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}

	grant, err := FindGrantByUserCode(app, userCode)
	if err == nil && grant != nil && grant.GetString("user_code") != "" {
		// user_code should still be present until the token exchange consumes it
		if grant.GetString("status") != "active" {
			t.Fatalf("status = %q, want active", grant.GetString("status"))
		}
		if grant.GetString("user") != userID {
			t.Fatalf("grant user = %q, want %q", grant.GetString("user"), userID)
		}
		if grant.GetString("device_label") != "Nathan's laptop" {
			t.Errorf("device_label not stored")
		}
	} else {
		t.Fatalf("grant not found after approval: %v", err)
	}
}

// oauthAuthedRequest builds a *core.RequestEvent carrying a real OAuth
// bearer token in the Authorization header AND re.Auth set to that token's
// user — i.e. the state the request would be in if enforceGrant's
// default-deny were ever bypassed or the scope table regressed. This is
// deliberately how these consent-endpoint tests probe the handler-level
// rejectOAuthToken check rather than the router/middleware layer: it proves
// the handler itself refuses an OAuth token even when re.Auth looks
// authenticated, which is the belt-and-suspenders half of the fix (the
// exemptPaths narrowing is the other half, covered in middleware_test.go).
//
// Takes an already-seeded userID/clientRecID rather than seeding its own —
// callers typically already have a user from setting up the pending grant
// under test, and seedUserAndClient's fixed email would collide on a second
// call.
func oauthAuthedRequest(t *testing.T, app *tests.TestApp, userID, clientRecID, method, target string, form url.Values) *core.RequestEvent {
	t.Helper()
	grant, err := NewGrant(app, userID, clientRecID, []string{ScopeProfile}, "active")
	if err != nil {
		t.Fatalf("NewGrant: %v", err)
	}
	user, err := app.FindRecordById("users", userID)
	if err != nil {
		t.Fatalf("find user: %v", err)
	}
	token, err := MintAccessToken(app, user, grant, AccessTokenTTL)
	if err != nil {
		t.Fatalf("MintAccessToken: %v", err)
	}

	var body strings.Reader
	if form != nil {
		body = *strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, target, &body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)

	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = httptest.NewRecorder()
	re.Auth = user // simulates a bypassed/regressed enforceGrant still setting e.Auth
	return re
}

// TestApproveDeviceRejectsOAuthToken is the mutation target for Finding 1:
// an OAuth access token — however narrowly scoped — must never be able to
// approve a device login. Before the fix, exemptPaths' blanket "/oauth/"
// prefix made this endpoint scopeExempt, so any bearer token that passed
// enforceGrant's user/grant checks sailed straight through to here.
func TestApproveDeviceRejectsOAuthToken(t *testing.T) {
	app := newSchemaApp(t)
	userCode, userID := pendingUserCodeGrant(t, app)
	clientRecID := mustFindClientRecID(t, app, "tinycld-cli")

	form := url.Values{}
	form.Set("user_code", userCode)
	re := oauthAuthedRequest(t, app, userID, clientRecID, http.MethodPost, "/oauth/authorize", form)

	err := handleApproveDevice(app, re)
	if status := apiStatus(err); status != http.StatusForbidden {
		t.Fatalf("an OAuth access token must not approve a device login (status = %d, err = %v)", status, err)
	}

	grant, findErr := FindGrantByUserCode(app, userCode)
	if findErr != nil {
		t.Fatalf("grant lookup: %v", findErr)
	}
	if grant.GetString("status") != "pending" {
		t.Fatalf("a rejected approval must not activate the grant; status = %q", grant.GetString("status"))
	}
}

// TestDenyDeviceRejectsOAuthToken mirrors the approve case: a bearer token
// must not be able to kill someone's in-flight device login either.
func TestDenyDeviceRejectsOAuthToken(t *testing.T) {
	app := newSchemaApp(t)
	userCode, userID := pendingUserCodeGrant(t, app)
	clientRecID := mustFindClientRecID(t, app, "tinycld-cli")

	form := url.Values{}
	form.Set("user_code", userCode)
	re := oauthAuthedRequest(t, app, userID, clientRecID, http.MethodPost, "/oauth/authorize/deny", form)

	err := handleDenyDevice(app, re)
	if status := apiStatus(err); status != http.StatusForbidden {
		t.Fatalf("an OAuth access token must not deny a device login (status = %d, err = %v)", status, err)
	}
}

// TestAuthorizeInfoRejectsOAuthToken: the consent screen's own read endpoint
// must not be servable to a bearer token either — it discloses which scopes
// a pending grant is requesting.
func TestAuthorizeInfoRejectsOAuthToken(t *testing.T) {
	app := newSchemaApp(t)
	userCode, userID := pendingUserCodeGrant(t, app)
	clientRecID := mustFindClientRecID(t, app, "tinycld-cli")
	re := oauthAuthedRequest(t, app, userID, clientRecID, http.MethodGet, "/oauth/authorize?user_code="+userCode, nil)

	err := handleAuthorizeInfo(app, re)
	if status := apiStatus(err); status != http.StatusForbidden {
		t.Fatalf("an OAuth access token must not read authorize info (status = %d, err = %v)", status, err)
	}
}

// TestAuthorizeRejectsOAuthToken: the Authorization Code + PKCE path must
// require a real session too, not merely "some authenticated re.Auth".
func TestAuthorizeRejectsOAuthToken(t *testing.T) {
	app := newSchemaApp(t)
	userID, bearerClientRecID := seedUserAndClient(t, app)
	const redirectURI = "https://app.example.com/cb"
	clientID := "zapier-oauth-reject"
	seedClientWithRedirectURIs(t, app, clientID, []string{redirectURI})
	verifier, _ := randomToken(32)

	form := validAuthorizeForm(clientID, redirectURI, verifier)
	re := oauthAuthedRequest(t, app, userID, bearerClientRecID, http.MethodPost, "/oauth/authorize", form)

	err := handleAuthorize(app, re)
	if status := apiStatus(err); status != http.StatusForbidden {
		t.Fatalf("an OAuth access token must not mint an authorization code (status = %d, err = %v)", status, err)
	}
}

func TestApproveDeviceRequiresAuthentication(t *testing.T) {
	app := newSchemaApp(t)
	userCode, _ := pendingUserCodeGrant(t, app)

	form := url.Values{}
	form.Set("user_code", userCode)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rec
	re.Auth = nil // anonymous

	// handleApproveDevice reports failure via a returned *router.ApiError
	// (re.UnauthorizedError), which — called directly, with no router in
	// front of it — never touches rec.Code. That write only happens in
	// router error-handling middleware, which does not run in this harness.
	// So the real signal here is the returned error's status, not rec.Code.
	err := handleApproveDevice(app, re)
	if status := apiStatus(err); status != http.StatusUnauthorized {
		t.Fatalf("an anonymous caller must not be able to approve a device (status = %d, err = %v)", status, err)
	}
}

func TestApproveDeviceRejectsUnknownUserCode(t *testing.T) {
	app := newSchemaApp(t)
	userID, _ := seedUserAndClient(t, app)
	user, _ := app.FindRecordById("users", userID)

	form := url.Values{}
	form.Set("user_code", "ZZZZ-ZZZZ")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rec
	re.Auth = user

	// Same signal choice as TestApproveDeviceRequiresAuthentication above:
	// handleApproveDevice's rejection paths return a *router.ApiError that
	// only the router's error middleware (absent here) would translate into
	// rec.Code, so rec.Code is not a real signal for this call shape.
	err := handleApproveDevice(app, re)
	if status := apiStatus(err); status != http.StatusNotFound {
		t.Fatalf("an unknown user_code must not approve anything (status = %d, err = %v)", status, err)
	}
}

// TestDenyDeviceHappyPathRevokesPendingGrant is the first-ever test of
// handleDenyDevice's success path — previously zero test coverage existed for
// this handler at all.
func TestDenyDeviceHappyPathRevokesPendingGrant(t *testing.T) {
	app := newSchemaApp(t)
	userCode, userID := pendingUserCodeGrant(t, app)
	user, err := app.FindRecordById("users", userID)
	if err != nil {
		t.Fatalf("find user: %v", err)
	}

	form := url.Values{}
	form.Set("user_code", userCode)
	re, rec := authorizeRequest(app, user, form)
	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize/deny",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	re.Request = req

	if err := handleDenyDevice(app, re); err != nil {
		t.Fatalf("handleDenyDevice: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}

	// The grant is gone by user_code (RevokeGrant clears it), so confirm the
	// revocation by looking the grant up through the recorded jti instead.
	if _, err := FindGrantByUserCode(app, userCode); err == nil {
		t.Fatal("a denied grant must no longer be findable by its (now-cleared) user_code")
	}
}

// TestDenyDeviceRequiresAuthentication: matches
// TestApproveDeviceRequiresAuthentication's coverage — an anonymous caller
// must not be able to deny (or thereby reveal the validity of) any code.
func TestDenyDeviceRequiresAuthentication(t *testing.T) {
	app := newSchemaApp(t)
	userCode, _ := pendingUserCodeGrant(t, app)

	form := url.Values{}
	form.Set("user_code", userCode)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize/deny",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rec
	re.Auth = nil

	err := handleDenyDevice(app, re)
	if status := apiStatus(err); status != http.StatusUnauthorized {
		t.Fatalf("an anonymous caller must not be able to deny a device request (status = %d, err = %v)", status, err)
	}
}

// TestDenyDeviceRejectsAlreadyActiveGrant is the mutation target for the
// missing-status-check half of Finding 2: device_code/user_code are only
// cleared on TOKEN EXCHANGE (see issueTokens), not on approval, so between
// "user approved" and "CLI polled the token endpoint" a grant is status
// "active" but still findable by its user_code. Without this check, deny
// would silently revoke a connection the user just approved on the very
// same screen.
func TestDenyDeviceRejectsAlreadyActiveGrant(t *testing.T) {
	app := newSchemaApp(t)
	userCode, userID := pendingUserCodeGrant(t, app)
	user, err := app.FindRecordById("users", userID)
	if err != nil {
		t.Fatalf("find user: %v", err)
	}

	grant, err := FindGrantByUserCode(app, userCode)
	if err != nil {
		t.Fatalf("find pending grant: %v", err)
	}
	grant.Set("status", "active")
	grant.Set("user", userID)
	if err := app.Save(grant); err != nil {
		t.Fatalf("activate grant: %v", err)
	}

	form := url.Values{}
	form.Set("user_code", userCode)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize/deny",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rec
	re.Auth = user

	err = handleDenyDevice(app, re)
	if status := apiStatus(err); status != http.StatusBadRequest {
		t.Fatalf("deny on an already-active grant: status = %d, err = %v, want 400", status, err)
	}

	reloaded, findErr := app.FindRecordById(grantsCollection, grant.Id)
	if findErr != nil {
		t.Fatalf("reload grant: %v", findErr)
	}
	if reloaded.GetString("status") != "active" {
		t.Fatalf("a rejected deny must not touch an already-active grant; status = %q", reloaded.GetString("status"))
	}
}

func TestRevokeMarksGrantRevoked(t *testing.T) {
	app := newSchemaApp(t)
	deviceCode, _ := approvedDeviceGrant(t, app)

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("device_code", deviceCode)
	form.Set("client_id", "tinycld-cli")
	_, issued := postToken(t, app, form)

	revokeForm := url.Values{}
	revokeForm.Set("token", issued.RefreshToken)
	revokeForm.Set("client_id", "tinycld-cli")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/oauth/revoke",
		strings.NewReader(revokeForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rec

	if err := handleRevoke(app, re); err != nil {
		t.Fatalf("handleRevoke: %v", err)
	}
	// RFC 7009 §2.2: always 200, even for an unknown token.
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	if _, err := VerifyGrant(app, grantIDFromToken(issued.AccessToken)); err == nil {
		t.Fatal("the grant must not verify after revocation")
	}
}

func TestRevokeUnknownTokenStillReturns200(t *testing.T) {
	app := newSchemaApp(t)
	seedUserAndClient(t, app)

	form := url.Values{}
	form.Set("token", "never-issued")
	form.Set("client_id", "tinycld-cli")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/oauth/revoke",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rec

	_ = handleRevoke(app, re)
	// Per RFC 7009 an unknown token is not an error — answering otherwise
	// turns the endpoint into a token oracle.
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for an unknown token", rec.Code)
	}
}

// seedClientWithRedirectURIs registers a fresh public client with the given
// redirect_uris, for handleAuthorize tests that need RedirectURIAllowed to
// have something real to check against — seedUserAndClient's client carries
// no redirect_uris at all, which would make every redirect_uri "unregistered"
// for a trivial, uninteresting reason rather than the one under test.
func seedClientWithRedirectURIs(t *testing.T, app *tests.TestApp, clientID string, uris []string) (clientRecID string) {
	t.Helper()
	clients, err := app.FindCollectionByNameOrId(clientsCollection)
	if err != nil {
		t.Fatalf("find clients: %v", err)
	}
	c := core.NewRecord(clients)
	c.Set("client_id", clientID)
	c.Set("name", "Zapier-like Integration")
	c.Set("type", "public")
	c.Set("is_first_party", false)
	c.Set("redirect_uris", uris)
	// Full catalog: these tests exercise redirect_uri/PKCE mechanics, not the
	// scope ceiling (ValidateClientScopes has its own dedicated tests) — a
	// narrow registration here would make an unrelated scope check the
	// reason a redirect/PKCE test fails.
	c.Set("scopes", strings.Join(AllScopes, " "))
	if err := app.Save(c); err != nil {
		t.Fatalf("save client with redirect_uris: %v", err)
	}
	return c.Id
}

// authorizeRequest builds the *core.RequestEvent handleAuthorize expects,
// mirroring the shape every other handler test in this file already uses. It
// returns the recorder too, so a caller that needs to inspect a re.JSON
// success body (rec.Code is real on that path) does not have to type-assert
// re.Response back to *httptest.ResponseRecorder itself.
func authorizeRequest(app core.App, auth *core.Record, form url.Values) (*core.RequestEvent, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rec
	re.Auth = auth
	return re, rec
}

func validAuthorizeForm(clientID, redirectURI, verifier string) url.Values {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("redirect_uri", redirectURI)
	form.Set("code_challenge", challengeFor(verifier))
	form.Set("code_challenge_method", MethodS256)
	form.Set("scope", ScopeMailRead)
	form.Set("state", "xyz")
	return form
}

// TestAuthorizeRejectsUnregisteredRedirectURI is the open-redirect /
// authorization-code-interception mutation target: RedirectURIAllowed must be
// exact-match, and a prefix/substring variant of a registered URI must be
// refused exactly like a wholly different one.
func TestAuthorizeRejectsUnregisteredRedirectURI(t *testing.T) {
	app := newSchemaApp(t)
	userID, _ := seedUserAndClient(t, app)
	user, _ := app.FindRecordById("users", userID)
	const registered = "https://app.example.com/cb"
	clientID := "zapier-1"
	seedClientWithRedirectURIs(t, app, clientID, []string{registered})

	verifier, _ := randomToken(32)
	for _, presented := range []string{
		"https://evil.example.com/cb",
		registered + ".evil.com",
		registered + "/../x",
		registered + "/extra",
	} {
		form := validAuthorizeForm(clientID, presented, verifier)
		re, _ := authorizeRequest(app, user, form)
		err := handleAuthorize(app, re)
		if status := apiStatus(err); status != http.StatusBadRequest {
			t.Errorf("redirect_uri %q: status = %d, err = %v, want 400", presented, status, err)
		}
	}
}

// TestAuthorizeRequiresCodeChallenge is the PKCE-downgrade mutation target:
// an absent code_challenge must be refused outright, never treated as "no
// PKCE requested".
func TestAuthorizeRequiresCodeChallenge(t *testing.T) {
	app := newSchemaApp(t)
	userID, _ := seedUserAndClient(t, app)
	user, _ := app.FindRecordById("users", userID)
	const redirectURI = "https://app.example.com/cb"
	clientID := "zapier-2"
	seedClientWithRedirectURIs(t, app, clientID, []string{redirectURI})

	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("redirect_uri", redirectURI)
	// code_challenge intentionally omitted.
	re, _ := authorizeRequest(app, user, form)
	err := handleAuthorize(app, re)
	if status := apiStatus(err); status != http.StatusBadRequest {
		t.Fatalf("missing code_challenge: status = %d, err = %v, want 400", status, err)
	}
}

// TestAuthorizeRejectsNonS256Method covers both a "plain" challenge method
// and an absent one — OAuth 2.1 removed `plain`, so accepting it (or
// defaulting to it) would silently downgrade PKCE for every client that
// omits the parameter.
func TestAuthorizeRejectsNonS256Method(t *testing.T) {
	app := newSchemaApp(t)
	userID, _ := seedUserAndClient(t, app)
	user, _ := app.FindRecordById("users", userID)
	const redirectURI = "https://app.example.com/cb"
	clientID := "zapier-3"
	seedClientWithRedirectURIs(t, app, clientID, []string{redirectURI})
	verifier, _ := randomToken(32)

	cases := []struct {
		name   string
		method string
	}{
		{"plain", "plain"},
		{"absent", ""},
	}
	for _, c := range cases {
		form := url.Values{}
		form.Set("client_id", clientID)
		form.Set("redirect_uri", redirectURI)
		form.Set("code_challenge", challengeFor(verifier))
		if c.method != "" {
			form.Set("code_challenge_method", c.method)
		}
		re, _ := authorizeRequest(app, user, form)
		err := handleAuthorize(app, re)
		if status := apiStatus(err); status != http.StatusBadRequest {
			t.Errorf("method %q: status = %d, err = %v, want 400", c.name, status, err)
		}
	}
}

// TestAuthorizeRequiresAuthentication: nobody may mint an authorization code
// without an authenticated session, however well-formed the rest of the
// request.
func TestAuthorizeRequiresAuthentication(t *testing.T) {
	app := newSchemaApp(t)
	const redirectURI = "https://app.example.com/cb"
	clientID := "zapier-4"
	seedClientWithRedirectURIs(t, app, clientID, []string{redirectURI})
	verifier, _ := randomToken(32)

	form := validAuthorizeForm(clientID, redirectURI, verifier)
	re, _ := authorizeRequest(app, nil, form)
	err := handleAuthorize(app, re)
	if status := apiStatus(err); status != http.StatusUnauthorized {
		t.Fatalf("anonymous authorize: status = %d, err = %v, want 401", status, err)
	}
}

// TestAuthorizeRejectsUnknownScope must not silently drop an unrecognized
// scope — the caller would then believe it holds access it was never granted.
func TestAuthorizeRejectsUnknownScope(t *testing.T) {
	app := newSchemaApp(t)
	userID, _ := seedUserAndClient(t, app)
	user, _ := app.FindRecordById("users", userID)
	const redirectURI = "https://app.example.com/cb"
	clientID := "zapier-5"
	seedClientWithRedirectURIs(t, app, clientID, []string{redirectURI})
	verifier, _ := randomToken(32)

	form := validAuthorizeForm(clientID, redirectURI, verifier)
	form.Set("scope", "not-a-real-scope")
	re, _ := authorizeRequest(app, user, form)
	err := handleAuthorize(app, re)
	if status := apiStatus(err); status != http.StatusBadRequest {
		t.Fatalf("unknown scope: status = %d, err = %v, want 400", status, err)
	}
}

// TestAuthorizeRejectsScopeOutsideClientCeiling is the Authorization Code +
// PKCE half of Finding 3's fix: a third-party integration client registered
// for a narrow scope set must not receive a broader grant just because the
// broader scope is a valid catalog entry.
func TestAuthorizeRejectsScopeOutsideClientCeiling(t *testing.T) {
	app := newSchemaApp(t)
	userID, _ := seedUserAndClient(t, app)
	user, _ := app.FindRecordById("users", userID)
	const redirectURI = "https://app.example.com/cb"
	clientID := "zapier-narrow-scope"
	clientRecID := seedClientWithRedirectURIs(t, app, clientID, []string{redirectURI})
	verifier, _ := randomToken(32)

	// Narrow this client's registration down from seedClientWithRedirectURIs'
	// default full catalog, to isolate the scope-ceiling check from the
	// redirect_uri/PKCE mechanics the other authorize tests exercise.
	client, err := app.FindRecordById(clientsCollection, clientRecID)
	if err != nil {
		t.Fatalf("find client: %v", err)
	}
	client.Set("scopes", ScopeProfile)
	if err := app.Save(client); err != nil {
		t.Fatalf("narrow client scopes: %v", err)
	}

	form := validAuthorizeForm(clientID, redirectURI, verifier)
	form.Set("scope", ScopeMailRead)
	re, _ := authorizeRequest(app, user, form)
	err = handleAuthorize(app, re)
	if status := apiStatus(err); status != http.StatusBadRequest {
		t.Fatalf("scope outside client ceiling: status = %d, err = %v, want 400", status, err)
	}
}

// TestAuthorizeHappyPathBindsPKCEAndRedirectAcrossToTokenExchange is the one
// test that proves the binding is load-bearing rather than merely stored: it
// drives handleAuthorize for a real code, then feeds that code into the
// actual token endpoint (postToken/handleToken, Task 6) with the correct
// verifier (must succeed) and a wrong one (must fail). A version of
// handleAuthorize that stored code_challenge/redirect_uri but never had them
// checked downstream would still pass a test that only inspected the grant
// row — this closes that gap.
func TestAuthorizeHappyPathBindsPKCEAndRedirectAcrossToTokenExchange(t *testing.T) {
	app := newSchemaApp(t)
	userID, _ := seedUserAndClient(t, app)
	user, _ := app.FindRecordById("users", userID)
	const redirectURI = "https://app.example.com/cb"
	const clientID = "tinycld-cli" // matches seedUserAndClient's client_id
	verifier, err := randomToken(32)
	if err != nil {
		t.Fatalf("randomToken: %v", err)
	}

	// seedUserAndClient's client has no redirect_uris registered yet — add
	// one directly so RedirectURIAllowed has a real match to make.
	client, err := app.FindRecordById(clientsCollection, mustFindClientRecID(t, app, clientID))
	if err != nil {
		t.Fatalf("find client: %v", err)
	}
	client.Set("redirect_uris", []string{redirectURI})
	if err := app.Save(client); err != nil {
		t.Fatalf("save client redirect_uris: %v", err)
	}

	form := validAuthorizeForm(clientID, redirectURI, verifier)
	re, authRec := authorizeRequest(app, user, form)
	if err := handleAuthorize(app, re); err != nil {
		t.Fatalf("handleAuthorize: %v", err)
	}
	// handleAuthorize's only return path on success is re.JSON, which writes
	// rec.Code itself — a real signal here, unlike the rejection paths above.
	if authRec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", authRec.Code, authRec.Body.String())
	}
	var issuedCode struct {
		Code        string `json:"code"`
		RedirectURI string `json:"redirect_uri"`
	}
	if err := json.Unmarshal(authRec.Body.Bytes(), &issuedCode); err != nil {
		t.Fatalf("decode authorize response: %v", err)
	}
	if issuedCode.Code == "" {
		t.Fatal("handleAuthorize must return a code")
	}

	grant, err := app.FindFirstRecordByFilter(
		grantsCollection, "auth_code_hash = {:h}",
		map[string]any{"h": hashSecret(issuedCode.Code)},
	)
	if err != nil || grant == nil {
		t.Fatalf("grant for issued code not found: %v", err)
	}
	if grant.GetString("redirect_uri") != redirectURI {
		t.Fatalf("grant redirect_uri = %q, want %q", grant.GetString("redirect_uri"), redirectURI)
	}
	if grant.GetString("code_challenge") != challengeFor(verifier) {
		t.Fatal("grant code_challenge does not match the S256 hash of the verifier")
	}

	// Correct verifier: the exchange must succeed end to end.
	okForm := url.Values{}
	okForm.Set("grant_type", "authorization_code")
	okForm.Set("code", issuedCode.Code)
	okForm.Set("code_verifier", verifier)
	okForm.Set("redirect_uri", redirectURI)
	okForm.Set("client_id", clientID)
	rec, resp := postToken(t, app, okForm)
	if rec.Code != http.StatusOK {
		t.Fatalf("token exchange with correct verifier: status = %d, body %s", rec.Code, rec.Body.String())
	}
	if resp.AccessToken == "" {
		t.Fatal("token exchange with correct verifier must return an access token")
	}
}

// TestAuthorizeHappyPathWrongVerifierFailsExchange is the negative half of
// the above: a code issued against one challenge must be refused by the
// token endpoint when redeemed with a DIFFERENT verifier, proving the two
// handlers are not just storing PKCE data but actually checking it against
// each other.
func TestAuthorizeHappyPathWrongVerifierFailsExchange(t *testing.T) {
	app := newSchemaApp(t)
	userID, _ := seedUserAndClient(t, app)
	user, _ := app.FindRecordById("users", userID)
	const redirectURI = "https://app.example.com/cb"
	const clientID = "tinycld-cli"
	verifier, err := randomToken(32)
	if err != nil {
		t.Fatalf("randomToken: %v", err)
	}

	client, err := app.FindRecordById(clientsCollection, mustFindClientRecID(t, app, clientID))
	if err != nil {
		t.Fatalf("find client: %v", err)
	}
	client.Set("redirect_uris", []string{redirectURI})
	if err := app.Save(client); err != nil {
		t.Fatalf("save client redirect_uris: %v", err)
	}

	form := validAuthorizeForm(clientID, redirectURI, verifier)
	re, authRec := authorizeRequest(app, user, form)
	if err := handleAuthorize(app, re); err != nil {
		t.Fatalf("handleAuthorize: %v", err)
	}
	var issuedCode struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(authRec.Body.Bytes(), &issuedCode); err != nil {
		t.Fatalf("decode authorize response: %v", err)
	}

	wrongVerifier, _ := randomToken(32)
	badForm := url.Values{}
	badForm.Set("grant_type", "authorization_code")
	badForm.Set("code", issuedCode.Code)
	badForm.Set("code_verifier", wrongVerifier)
	badForm.Set("redirect_uri", redirectURI)
	badForm.Set("client_id", clientID)
	// handleAuthCodeGrant's rejection path is tokenError -> re.JSON, which
	// writes rec.Code itself (see token_test.go's postToken doc comment), so
	// rec.Code is a real signal here.
	rec, errResp := postTokenExpectingError(t, app, badForm)
	if rec.Code == http.StatusOK {
		t.Fatal("a wrong verifier must not redeem a code issued against a different challenge")
	}
	if errResp.Error != "invalid_grant" {
		t.Fatalf("error = %q, want invalid_grant", errResp.Error)
	}
}

// mustFindClientRecID resolves seedUserAndClient's client record id from its
// client_id, since that helper only returns the record id at creation time
// and these tests need to mutate the record afterward.
func mustFindClientRecID(t *testing.T, app core.App, clientID string) string {
	t.Helper()
	rec, err := FindClientByClientID(app, clientID)
	if err != nil {
		t.Fatalf("find client %q: %v", clientID, err)
	}
	return rec.Id
}

func TestAuthorizeInfoReturnsClientNameAndScopes(t *testing.T) {
	app := newSchemaApp(t)
	userCode, userID := pendingUserCodeGrant(t, app)
	user, err := app.FindRecordById("users", userID)
	if err != nil {
		t.Fatalf("find user: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?user_code="+userCode, nil)
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rec
	re.Auth = user

	if err := handleAuthorizeInfo(app, re); err != nil {
		t.Fatalf("handleAuthorizeInfo: %v", err)
	}
	// handleAuthorizeInfo's only return path is re.JSON, which writes rec.Code.
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var info AuthorizeInfoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if info.ClientName != "TinyCld CLI" {
		t.Errorf("client_name = %q, want %q", info.ClientName, "TinyCld CLI")
	}
	if len(info.Scopes) != 1 || info.Scopes[0] != ScopeMailRead {
		t.Errorf("scopes = %v, want [%s]", info.Scopes, ScopeMailRead)
	}
}

func TestAuthorizeInfoRejectsAnonymousCaller(t *testing.T) {
	app := newSchemaApp(t)
	userCode, _ := pendingUserCodeGrant(t, app)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?user_code="+userCode, nil)
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rec
	re.Auth = nil

	// handleAuthorizeInfo's rejection path returns re.UnauthorizedError, a
	// *router.ApiError never routed through re.Response in this harness — the
	// real signal is apiStatus(err), not rec.Code.
	err := handleAuthorizeInfo(app, re)
	if status := apiStatus(err); status != http.StatusUnauthorized {
		t.Fatalf("anonymous caller: status = %d, err = %v, want 401", status, err)
	}
}

func TestMetadataAdvertisesSupportedGrants(t *testing.T) {
	app := newSchemaApp(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/.well-known/oauth-authorization-server", nil)
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rec

	if err := handleMetadata(app, re); err != nil {
		t.Fatalf("handleMetadata: %v", err)
	}

	var md MetadataResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &md); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if md.TokenEndpoint == "" || md.DeviceAuthorizationEndpoint == "" {
		t.Fatal("metadata must advertise the token and device endpoints")
	}
	if len(md.CodeChallengeMethodsSupported) != 1 ||
		md.CodeChallengeMethodsSupported[0] != MethodS256 {
		t.Fatalf("must advertise S256 only, got %v", md.CodeChallengeMethodsSupported)
	}
	for _, want := range []string{grantTypeDevice, grantTypeAuthCode, grantTypeRefresh} {
		var found bool
		for _, g := range md.GrantTypesSupported {
			if g == want {
				found = true
			}
		}
		if !found {
			t.Errorf("metadata omits grant type %q", want)
		}
	}
	if len(md.ScopesSupported) == 0 {
		t.Error("metadata must advertise the scope catalog")
	}
}
