package oauth

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

func TestScopeForRouteMapsKnownRoutes(t *testing.T) {
	cases := []struct {
		method, path, want string
	}{
		{"GET", "/api/mail/search", ScopeMailRead},
		{"POST", "/api/mail/send", ScopeMailSend},
		{"POST", "/api/mail/draft", ScopeMailSend},
		{"GET", "/api/drive/search", ScopeDriveRead},
		{"POST", "/api/drive/download-token", ScopeDriveRead},
		{"POST", "/api/drive/upload-version", ScopeDriveWrite},
		{"GET", "/api/collections/mail_messages/records", ScopeMailRead},
		{"POST", "/api/collections/drive_items/records", ScopeDriveWrite},
		{"GET", "/api/collections/contacts/records", ScopeContactsRead},
		{"PATCH", "/api/collections/calendar_events/records/abc", ScopeCalendarWrite},
	}
	for _, c := range cases {
		if got := ScopeForRoute(c.method, c.path); got != c.want {
			t.Errorf("ScopeForRoute(%s %s) = %q, want %q", c.method, c.path, got, c.want)
		}
	}
}

func TestScopeForRouteDefaultDenies(t *testing.T) {
	// Default deny: a route no rule covers must return "" so the middleware
	// refuses it for OAuth callers rather than silently allowing it.
	if got := ScopeForRoute("POST", "/api/admin/packages/install"); got != "" {
		t.Fatalf("ScopeForRoute on an uncovered admin route = %q, want \"\"", got)
	}
	if got := ScopeForRoute("GET", "/api/collections/pkg_registry/records"); got != "" {
		t.Fatalf("ScopeForRoute on an uncovered collection = %q, want \"\"", got)
	}
}

func TestScopeForRouteAllowsUnauthenticatedPublicRoutes(t *testing.T) {
	// These carry no user data and must stay reachable so a CLI can probe a
	// host and complete a login before it holds any grant.
	for _, p := range []string{
		"/api/health",
		"/api/org-info",
		"/oauth/token",
		"/oauth/device",
		"/api/cli/downloads",
		"/api/cli/download/darwin-arm64",
	} {
		if got := ScopeForRoute("GET", p); got != scopeExempt {
			t.Errorf("ScopeForRoute(GET %s) = %q, want exempt", p, got)
		}
	}
}

// TestScopeForRouteDeniesConsentEndpoints is the regression test for the
// privilege-escalation finding: exemptPaths used to list the blanket prefix
// "/oauth/", which made ScopeForRoute return scopeExempt for the consent and
// grant-management routes too. That let ANY OAuth access token — however
// narrowly scoped — reach handleApproveDevice/handleDenyDevice/handleAuthorize
// and mint or kill grants with no browser and no human consent. These routes
// must now default-deny ("") for an OAuth bearer, same as any other
// unclassified route.
func TestScopeForRouteDeniesConsentEndpoints(t *testing.T) {
	cases := []struct {
		method, path string
	}{
		{"GET", "/oauth/authorize/info"},
		{"POST", "/oauth/authorize/approve"},
		{"POST", "/oauth/authorize/deny"},
		{"POST", "/oauth/authorize"},
		{"POST", "/oauth/grants/abc123/revoke"},
	}
	for _, c := range cases {
		if got := ScopeForRoute(c.method, c.path); got != "" {
			t.Errorf("ScopeForRoute(%s %s) = %q, want \"\" (default deny) — "+
				"an OAuth bearer token must never reach a consent/management endpoint",
				c.method, c.path, got)
		}
	}
}

func TestMintAccessTokenCarriesGrantClaim(t *testing.T) {
	app := newSchemaApp(t)
	userID, clientID := seedUserAndClient(t, app)

	grant, err := NewGrant(app, userID, clientID, []string{ScopeMailRead}, "active")
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
	if token == "" {
		t.Fatal("MintAccessToken returned empty token")
	}

	// The grant id must be recoverable from the token, or the middleware has
	// nothing to look up.
	if got := grantIDFromToken(token); got != grant.GetString("jti") {
		t.Fatalf("grantIDFromToken = %q, want %q", got, grant.GetString("jti"))
	}
	if !IsOAuthToken(token) {
		t.Fatal("IsOAuthToken must recognize a minted access token")
	}
}

func TestMintedTokenResolvesThroughPocketBase(t *testing.T) {
	// The load-bearing claim of the whole design: an OAuth access token is an
	// ordinary PB auth token, so PB's own resolver accepts it and every
	// existing endpoint keeps working with no per-endpoint change.
	app := newSchemaApp(t)
	userID, clientID := seedUserAndClient(t, app)

	grant, err := NewGrant(app, userID, clientID, []string{ScopeMailRead}, "active")
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

	resolved, err := app.FindAuthRecordByToken(token, core.TokenTypeAuth)
	if err != nil {
		t.Fatalf("PocketBase rejected an OAuth access token: %v", err)
	}
	if resolved.Id != userID {
		t.Fatalf("resolved user %s, want %s", resolved.Id, userID)
	}
}

func TestIsOAuthTokenRejectsPlainAuthToken(t *testing.T) {
	app := newSchemaApp(t)
	userID, _ := seedUserAndClient(t, app)
	user, err := app.FindRecordById("users", userID)
	if err != nil {
		t.Fatalf("find user: %v", err)
	}
	// A normal web-session token carries no grant claim; the middleware must
	// leave it entirely alone.
	plain, err := user.NewAuthToken()
	if err != nil {
		t.Fatalf("NewAuthToken: %v", err)
	}
	if IsOAuthToken(plain) {
		t.Fatal("a plain web-session token must not be treated as an OAuth token")
	}
}

func TestAccessTokenTTLIsShorterThanRefresh(t *testing.T) {
	if AccessTokenTTL >= RefreshTokenTTL {
		t.Fatalf("AccessTokenTTL (%v) must be shorter than RefreshTokenTTL (%v)",
			AccessTokenTTL, RefreshTokenTTL)
	}
	if AccessTokenTTL > 24*time.Hour {
		t.Fatalf("AccessTokenTTL (%v) is too long for a bearer token", AccessTokenTTL)
	}
}

// TestMiddlewarePriorityRunsBeforePocketBase is the assertion that catches a
// priority flip. PocketBase's loadAuthToken short-circuits on e.Auth != nil
// (apis/middlewares.go), so if our priority number were not strictly less
// than PocketBase's, PB's middleware would run first, populate e.Auth from
// the raw signature alone, and our grant/scope check below would never run —
// silently disabling revocation for every OAuth access token.
func TestMiddlewarePriorityRunsBeforePocketBase(t *testing.T) {
	if middlewarePriority >= apis.DefaultLoadAuthTokenMiddlewarePriority {
		t.Fatalf("middlewarePriority %d must be < PocketBase's loadAuthToken priority %d, "+
			"or PB sets e.Auth first and the grant check is bypassed",
			middlewarePriority, apis.DefaultLoadAuthTokenMiddlewarePriority)
	}
}

// newRequestEvent builds a bare *core.RequestEvent the way vapid_admin_test.go
// does, so enforceGrant can be called directly without standing up a router.
func newRequestEvent(app core.App, method, path, bearer string) *core.RequestEvent {
	req := httptest.NewRequest(method, path, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = httptest.NewRecorder()
	return re
}

// apiStatus extracts the HTTP status enforceGrant refused with, or 0 if err
// is not the *router.ApiError enforceGrant is documented to return.
func apiStatus(err error) int {
	apiErr, ok := err.(*router.ApiError)
	if !ok {
		return 0
	}
	return apiErr.Status
}

func TestEnforceGrantLeavesPlainTokenAlone(t *testing.T) {
	// A web-session token carries no tcg claim. enforceGrant must not touch
	// e.Auth for it at all — that is PocketBase's own loadAuthToken's job,
	// which runs after us and resolves the signature normally.
	app := newSchemaApp(t)
	userID, _ := seedUserAndClient(t, app)
	user, err := app.FindRecordById("users", userID)
	if err != nil {
		t.Fatalf("find user: %v", err)
	}
	plain, err := user.NewAuthToken()
	if err != nil {
		t.Fatalf("NewAuthToken: %v", err)
	}

	re := newRequestEvent(app, "GET", "/api/mail/search", plain)
	if err := enforceGrant(re); err != nil {
		t.Fatalf("enforceGrant on a plain token returned an error: %v", err)
	}
	if re.Auth != nil {
		t.Fatal("enforceGrant must not set e.Auth for a non-OAuth token")
	}
}

func TestEnforceGrantAllowsInScopeRequest(t *testing.T) {
	app := newSchemaApp(t)
	userID, clientID := seedUserAndClient(t, app)
	grant, err := NewGrant(app, userID, clientID, []string{ScopeMailRead}, "active")
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

	re := newRequestEvent(app, "GET", "/api/mail/search", token)
	if err := enforceGrant(re); err != nil {
		t.Fatalf("enforceGrant on an in-scope request returned an error: %v", err)
	}
	if re.Auth == nil || re.Auth.Id != userID {
		t.Fatalf("enforceGrant must set e.Auth to the grant's user; got %v", re.Auth)
	}
}

func TestEnforceGrantRejectsOutOfScopeRequest(t *testing.T) {
	// This is the assertion that must fail if the scope check is ever
	// removed or short-circuited: a grant scoped to mail:read only must not
	// authorize a drive:write route.
	app := newSchemaApp(t)
	userID, clientID := seedUserAndClient(t, app)
	grant, err := NewGrant(app, userID, clientID, []string{ScopeMailRead}, "active")
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

	re := newRequestEvent(app, "POST", "/api/drive/upload-version", token)
	err = enforceGrant(re)
	if err == nil {
		t.Fatal("enforceGrant must refuse a request outside the grant's scopes")
	}
	if got := apiStatus(err); got != 403 {
		t.Fatalf("enforceGrant out-of-scope status = %d, want 403 (%v)", got, err)
	}
	if re.Auth != nil {
		t.Fatal("enforceGrant must not set e.Auth when it refuses the request")
	}
}

func TestEnforceGrantRejectsRevokedGrant(t *testing.T) {
	// The single most important property of the whole design: revoking a
	// grant must take effect on the very next request, even though the JWT
	// signature on the already-issued access token is still perfectly valid.
	app := newSchemaApp(t)
	userID, clientID := seedUserAndClient(t, app)
	grant, err := NewGrant(app, userID, clientID, []string{ScopeMailRead}, "active")
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
	if err := RevokeGrant(app, grant.Id); err != nil {
		t.Fatalf("RevokeGrant: %v", err)
	}

	re := newRequestEvent(app, "GET", "/api/mail/search", token)
	err = enforceGrant(re)
	if err == nil {
		t.Fatal("enforceGrant must refuse a revoked grant's access token")
	}
	if got := apiStatus(err); got != 401 {
		t.Fatalf("enforceGrant revoked-grant status = %d, want 401 (%v)", got, err)
	}
	if re.Auth != nil {
		t.Fatal("enforceGrant must not set e.Auth for a revoked grant")
	}
}

func TestEnforceGrantDefaultDeniesUncoveredRoute(t *testing.T) {
	// A route the scope table does not know about must be refused for OAuth
	// callers even though the grant and token are otherwise perfectly valid.
	app := newSchemaApp(t)
	userID, clientID := seedUserAndClient(t, app)
	grant, err := NewGrant(app, userID, clientID, []string{ScopeMailRead}, "active")
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

	re := newRequestEvent(app, "POST", "/api/admin/packages/install", token)
	err = enforceGrant(re)
	if err == nil {
		t.Fatal("enforceGrant must refuse a route not covered by the scope table")
	}
	if got := apiStatus(err); got != 403 {
		t.Fatalf("enforceGrant uncovered-route status = %d, want 403 (%v)", got, err)
	}
	if re.Auth != nil {
		t.Fatal("enforceGrant must not set e.Auth when it default-denies")
	}
}
