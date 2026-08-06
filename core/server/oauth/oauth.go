// Package oauth is TinyCld's OAuth 2.1 authorization server.
//
// PocketBase is an OAuth2 *client* (social login); it issues nothing to third
// parties. This package supplies the other half: the Device Authorization
// Grant (RFC 8628) that the tinycld CLI uses, and Authorization Code + PKCE
// (RFC 7636) for integrations such as Zapier.
//
// Access tokens are ordinary PocketBase `auth`-type static tokens. That is
// deliberate: PocketBase's own loadAuthToken middleware resolves them and
// populates e.Auth, so every existing endpoint and collection rule keeps
// working with no per-endpoint changes. What OAuth adds on top is a grant
// record consulted on every request — which is the only way to revoke one
// device without rotating tokenKey and killing the user's web session too.
package oauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// Collection names this package owns.
const (
	clientsCollection = "oauth_clients"
	grantsCollection  = "oauth_grants"
)

// Scopes. Named <package>:<capability> so the set grows naturally as packages
// are installed. `profile` is the baseline identity scope every grant gets.
const (
	ScopeProfile        = "profile"
	ScopeMailRead       = "mail:read"
	ScopeMailSend       = "mail:send"
	ScopeDriveRead      = "drive:read"
	ScopeDriveWrite     = "drive:write"
	ScopeContactsRead   = "contacts:read"
	ScopeContactsWrite  = "contacts:write"
	ScopeCalendarRead   = "calendar:read"
	ScopeCalendarWrite  = "calendar:write"
)

// AllScopes is the full catalog, used to validate a requested scope string and
// to render the consent screen.
var AllScopes = []string{
	ScopeProfile,
	ScopeMailRead, ScopeMailSend,
	ScopeDriveRead, ScopeDriveWrite,
	ScopeContactsRead, ScopeContactsWrite,
	ScopeCalendarRead, ScopeCalendarWrite,
}

// Claims is the verified payload carried by an OAuth access token, over and
// above the standard PocketBase auth claims. GrantID is the jti: the row this
// token is bound to, re-read on every request so revocation is immediate.
type Claims struct {
	GrantID  string
	ClientID string
	Scopes   []string
}

var (
	// ErrInvalidGrant covers a missing, malformed, or expired grant. Maps to 401.
	ErrInvalidGrant = errors.New("oauth: invalid grant")
	// ErrGrantRevoked is a grant explicitly revoked by the user. Maps to 401.
	ErrGrantRevoked = errors.New("oauth: grant revoked")
	// ErrInsufficientScope is a valid grant lacking the scope for this route.
	// Maps to 403 (RFC 6750 §3.1).
	ErrInsufficientScope = errors.New("oauth: insufficient scope")
)

// ParseScopes splits a space-delimited scope string, dropping empties so
// "  a   b  " and "a b" parse identically.
func ParseScopes(s string) []string {
	return strings.Fields(s)
}

// HasScope reports whether granted contains want.
func HasScope(granted []string, want string) bool {
	for _, g := range granted {
		if g == want {
			return true
		}
	}
	return false
}

// signingKey derives a dedicated HMAC key for OAuth artifacts (device codes,
// authorization codes) from the _superusers auth-token secret.
//
// We never sign with the raw auth secret directly. A domain-separated subkey
// means an OAuth artifact's signature cannot be confused with a PocketBase
// auth token even though both are HS256. The _superusers secret is app-wide
// and stable across restarts, so every module derives the same key. The `:v1`
// suffix is the rotation seam: bump it to invalidate every outstanding
// artifact at once. Mirrors sharelink.signingKey.
func signingKey(app core.App) (string, error) {
	col, err := app.FindCachedCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		return "", fmt.Errorf("oauth: load superusers collection: %w", err)
	}
	base := col.AuthToken.Secret
	if base == "" {
		return "", errors.New("oauth: superusers auth token secret is empty")
	}
	mac := hmac.New(sha256.New, []byte(base))
	mac.Write([]byte("tinycld:oauth:v1"))
	return hex.EncodeToString(mac.Sum(nil)), nil
}
