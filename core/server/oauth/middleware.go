package oauth

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/security"
)

// Token lifetimes. The access token is deliberately short: it is a bearer
// credential that travels on every request, and the refresh token (stored
// only as a hash) is what gives a CLI or integration long-lived access.
const (
	AccessTokenTTL  = 1 * time.Hour
	RefreshTokenTTL = 90 * 24 * time.Hour
	// DeviceCodeTTL bounds how long a user has to approve a device login.
	DeviceCodeTTL = 15 * time.Minute
	// AuthCodeTTL is short by design — the code is exchanged immediately.
	AuthCodeTTL = 60 * time.Second
)

// grantClaim is the private claim carrying the grant's jti. PocketBase ignores
// claims it does not know, so adding this keeps the token a valid PB auth
// token while letting us find the grant row.
const grantClaim = "tcg"

// scopeExempt marks routes reachable without any scope check — public probes
// and the OAuth endpoints themselves, which must work before a grant exists.
const scopeExempt = "-"

// middlewarePriority runs this check BEFORE PocketBase's own loadAuthToken.
// Lower number = earlier. We need to run first so that, for an OAuth token, we
// are the one who validates the grant; PB's middleware then finds e.Auth
// already set and no-ops (see apis/middlewares.go:190).
var middlewarePriority = apis.DefaultLoadAuthTokenMiddlewarePriority - 10

// collectionScopes maps a PocketBase collection to the scopes governing it.
// Anything absent is denied for OAuth callers by default.
var collectionScopes = map[string][2]string{
	// collection: {read scope, write scope}
	"mail_messages":      {ScopeMailRead, ScopeMailSend},
	"mail_threads":       {ScopeMailRead, ScopeMailSend},
	"mail_thread_state":  {ScopeMailRead, ScopeMailSend},
	"mail_mailboxes":     {ScopeMailRead, ScopeMailSend},
	"drive_items":        {ScopeDriveRead, ScopeDriveWrite},
	"drive_shares":       {ScopeDriveRead, ScopeDriveWrite},
	"drive_item_state":   {ScopeDriveRead, ScopeDriveWrite},
	"contacts":           {ScopeContactsRead, ScopeContactsWrite},
	"calendar_events":    {ScopeCalendarRead, ScopeCalendarWrite},
	"calendar_calendars": {ScopeCalendarRead, ScopeCalendarWrite},
	"users":              {ScopeProfile, ""},
}

// endpointScopes maps a bespoke Go endpoint to its required scope.
var endpointScopes = map[string]string{
	"GET /api/mail/search":           ScopeMailRead,
	"POST /api/mail/send":            ScopeMailSend,
	"POST /api/mail/draft":           ScopeMailSend,
	"GET /api/drive/search":          ScopeDriveRead,
	"POST /api/drive/download-token": ScopeDriveRead,
	"POST /api/drive/export-token":   ScopeDriveRead,
	"GET /api/drive/storage-usage":   ScopeDriveRead,
	"POST /api/drive/upload-version": ScopeDriveWrite,
	"POST /api/drive/share":          ScopeDriveWrite,
	"GET /api/contacts/search":       ScopeContactsRead,

	// Advertised in the discovery document as userinfo_endpoint, so an
	// integration following the well-known metadata calls it with an ordinary
	// access token. It needs an explicit entry: it lives under /oauth/ but is
	// deliberately NOT in exemptPaths (only the credential-less endpoints are),
	// so without this it would fall into default-deny and 403 the very call the
	// server tells clients to make.
	"GET /oauth/userinfo": ScopeProfile,
}

// exemptPaths need no scope: public probes, and the OAuth endpoints a client
// must reach before it holds any grant.
//
// This list must stay narrow. It used to be the blanket prefix "/oauth/",
// which made every route under it — including the consent endpoints
// (/oauth/authorize*, /oauth/grants/{id}/revoke) — scopeExempt for an OAuth
// bearer token. That let a stolen profile-only access token approve or deny
// ANY device login and mint itself a fully-scoped grant, because
// enforceGrant never even reached the scope check for those paths. Only the
// genuinely credential-less endpoints belong here: a device/CLI has no
// token yet when it calls them. Everything else under /oauth/ must fall
// through to default-deny ("") for an OAuth caller, and is separately
// required to be a real session (see rejectOAuthToken in authorize.go and
// grants_endpoint.go) — never merely "some scope reachable this route".
var exemptPaths = []string{
	"/api/health",
	"/api/org-info",
	"/api/version",
	"/api/release",
	// Public credential-less binary downloads, same bucket as /api/version
	// and /api/release (and a future `tinycld update` needs them reachable
	// with a bearer attached).
	"/api/cli/",
	"/oauth/device",
	"/oauth/token",
	"/oauth/revoke",
	"/.well-known/",
}

// writeMethods are the HTTP verbs treated as writes for scope selection.
var writeMethods = map[string]bool{
	"POST": true, "PUT": true, "PATCH": true, "DELETE": true,
}

// ScopeForRoute returns the scope required for a request, scopeExempt for
// public routes, or "" meaning deny.
//
// Default deny is deliberate: a route nobody has classified must not be
// reachable with a third-party token just because someone added it.
func ScopeForRoute(method, path string) string {
	for _, p := range exemptPaths {
		if strings.HasPrefix(path, p) {
			return scopeExempt
		}
	}
	if s, ok := endpointScopes[method+" "+path]; ok {
		return s
	}
	if name, ok := collectionFromPath(path); ok {
		pair, known := collectionScopes[name]
		if !known {
			return ""
		}
		if writeMethods[method] {
			return pair[1] // "" for read-only collections => deny writes
		}
		return pair[0]
	}
	return ""
}

// collectionFromPath extracts "mail_messages" from
// /api/collections/mail_messages/records[/id].
func collectionFromPath(path string) (string, bool) {
	const prefix = "/api/collections/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(path, prefix)
	slash := strings.IndexByte(rest, '/')
	if slash <= 0 {
		return "", false
	}
	return rest[:slash], true
}

// grantIDFromToken reads the grant claim without verifying the signature. That
// is safe here because it is only used to LOCATE the grant; the signature is
// verified by PocketBase's own resolver, and the grant row is then re-checked.
func grantIDFromToken(token string) string {
	claims, err := security.ParseUnverifiedJWT(token)
	if err != nil {
		return ""
	}
	v, _ := claims[grantClaim].(string)
	return v
}

// IsOAuthToken reports whether a token carries a grant claim.
func IsOAuthToken(token string) bool {
	return grantIDFromToken(token) != ""
}

// MintAccessToken issues a PocketBase static auth token carrying the grant
// claim. Static (non-refreshable) is correct: renewal goes through the OAuth
// refresh grant, not PB's authRefresh, so revoking the grant is the only way
// to get a new access token.
func MintAccessToken(
	app core.App,
	user *core.Record,
	grant *core.Record,
	ttl time.Duration,
) (string, error) {
	base, err := user.NewStaticAuthToken(ttl)
	if err != nil {
		return "", fmt.Errorf("oauth: mint static token: %w", err)
	}
	// Re-sign with the grant claim added. We must use the same key PocketBase
	// verifies with, or FindAuthRecordByToken would reject it.
	claims, err := security.ParseUnverifiedJWT(base)
	if err != nil {
		return "", fmt.Errorf("oauth: parse minted token: %w", err)
	}
	claims[grantClaim] = grant.GetString("jti")

	key := user.TokenKey() + user.Collection().AuthToken.Secret
	signed, err := security.NewJWT(claims, key, ttl)
	if err != nil {
		return "", fmt.Errorf("oauth: sign access token: %w", err)
	}
	return signed, nil
}

// enforceGrant is the body of the grant-enforcement middleware, split out so
// it can be exercised directly in tests without standing up a router — the
// handler this package registers just calls it under the same Id/Priority.
func enforceGrant(re *core.RequestEvent) error {
	token := bearerToken(re.Request)
	if token == "" || !IsOAuthToken(token) {
		// Not an OAuth request — leave it entirely alone.
		return re.Next()
	}

	record, err := re.App.FindAuthRecordByToken(token, core.TokenTypeAuth)
	if err != nil || record == nil {
		return re.UnauthorizedError("Invalid access token", nil)
	}

	grant, err := VerifyGrant(re.App, grantIDFromToken(token))
	if err != nil {
		return re.UnauthorizedError("Access token is no longer valid", nil)
	}
	if grant.GetString("user") != record.Id {
		// The grant belongs to a different user than the token
		// claims. Should be impossible; refuse loudly.
		return re.UnauthorizedError("Access token is no longer valid", nil)
	}

	required := ScopeForRoute(re.Request.Method, re.Request.URL.Path)
	if required == "" {
		return re.ForbiddenError(
			"This endpoint is not available to API tokens", nil)
	}
	if required != scopeExempt {
		if !HasScope(ParseScopes(grant.GetString("scopes")), required) {
			return re.ForbiddenError(
				fmt.Sprintf("Requires the %q scope", required), nil)
		}
	}

	if err := TouchGrant(re.App, grant); err != nil {
		// Non-fatal: last_used_at is cosmetic.
		re.App.Logger().Warn("oauth: touch grant", "error", err)
	}

	re.Auth = record
	return re.Next()
}

// bindGrantEnforcement installs the middleware that turns a valid signature
// into an authorized request. It runs ahead of PocketBase's loadAuthToken so
// that, for OAuth tokens, we populate e.Auth ourselves after checking the
// grant; PB's middleware then sees e.Auth != nil and no-ops.
func bindGrantEnforcement(app core.App) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.Bind(&hook.Handler[*core.RequestEvent]{
			Id:       "tinycldOAuthGrant",
			Priority: middlewarePriority,
			Func:     enforceGrant,
		})
		return e.Next()
	})
}

// bearerToken reads the Authorization header, tolerating a missing "Bearer "
// prefix the way PocketBase does.
func bearerToken(r *http.Request) string {
	v := r.Header.Get("Authorization")
	if len(v) > 7 && strings.EqualFold(v[:7], "Bearer ") {
		return v[7:]
	}
	return v
}

// rejectOAuthToken refuses a request authenticated with an OAuth access
// token, for the consent and grant-management endpoints that must only be
// reachable from a genuine interactive session.
//
// Narrowing exemptPaths (above) makes ScopeForRoute default-deny an OAuth
// bearer on these routes, but that alone is not the fix: default-deny is
// enforced by enforceGrant, and enforceGrant only runs its scope check when
// re.Auth is not already set by something else. It is this handler-level
// check — independent of the scope table — that guarantees a bearer token
// can never reach approve/deny/authorize/revoke-by-id, even if a future
// endpoint or scope-table edit accidentally reopens the route. A genuine
// browser session token carries no grant claim, so IsOAuthToken is false and
// this is a no-op for it.
func rejectOAuthToken(re *core.RequestEvent) error {
	if IsOAuthToken(bearerToken(re.Request)) {
		return re.ForbiddenError(
			"This endpoint requires a signed-in session, not an OAuth access token", nil)
	}
	return nil
}
