package oauth

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
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
	for _, p := range []string{"/api/health", "/api/org-info", "/oauth/token", "/oauth/device"} {
		if got := ScopeForRoute("GET", p); got != scopeExempt {
			t.Errorf("ScopeForRoute(GET %s) = %q, want exempt", p, got)
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
