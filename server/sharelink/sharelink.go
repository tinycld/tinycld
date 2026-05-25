// Package sharelink provides the cross-package primitive for Drive's
// public share links: minting and verifying stateless "share session"
// tokens that carry an anonymous visitor's identity, deriving stable
// "Anon <Animal>" display names, and looking up + validating the
// underlying drive_share_links record.
//
// It lives in core (the only module every member shares) precisely so
// that drive can mint sessions while calc, text, and the realtime
// broker can verify them — the members are separate Go modules and
// cannot import one another. They reach the shared drive_share_links /
// drive_items collections by name at the DB layer.
package sharelink

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/security"
)

// Collection / field names of the drive-owned tables this package reads.
// Duplicated here (rather than imported from drive) because drive is a
// separate module; the names are the stable contract between them.
const (
	shareLinksCollection = "drive_share_links"
	driveItemsCollection = "drive_items"
)

// SessionTTL is how long a minted session token stays valid. The anon
// identity itself is long-lived (the client persists anon_id in a
// cookie and re-mints), but each signed token is short-ish so a leaked
// token doesn't grant indefinite access. Re-minting is a cheap public
// call.
const SessionTTL = 12 * time.Hour

// Roles. A share link's role determines what a session can do.
const (
	RoleViewer    = "viewer"
	RoleCommentor = "commentor"
	RoleEditor    = "editor"
)

// Claims is the verified payload of a share session token. Every server
// entrypoint that accepts an anonymous visitor re-derives these from the
// signature and then re-checks the link is still live (see ResolveLink).
type Claims struct {
	ShareToken  string `json:"st"`
	AnonID      string `json:"aid"`
	DisplayName string `json:"dn"`
	Role        string `json:"role"`
	ItemID      string `json:"item"`
}

// CanComment reports whether the role may create comments.
// Viewer links are commentable by product decision (the read default is
// "anyone with the link can comment"); commentor is the explicit grant
// of the same; editor implies it.
func (c Claims) CanComment() bool {
	switch c.Role {
	case RoleViewer, RoleCommentor, RoleEditor:
		return true
	default:
		return false
	}
}

// CanEdit reports whether the role may open a writable editor / realtime
// room.
func (c Claims) CanEdit() bool {
	return c.Role == RoleEditor
}

var (
	// ErrInvalidToken is returned when a session token fails signature
	// or claim validation. Maps to 401 at the HTTP layer.
	ErrInvalidToken = errors.New("sharelink: invalid session token")
	// ErrLinkNotFound is returned when no share link matches the token.
	ErrLinkNotFound = errors.New("sharelink: share link not found")
	// ErrLinkGone is returned when the link exists but is revoked or
	// expired. Maps to 410.
	ErrLinkGone = errors.New("sharelink: share link revoked or expired")
)

// signingKey derives a dedicated HMAC key for share sessions from the
// _superusers auth-token secret. We never sign with the raw auth secret
// directly — a domain-separated subkey means a share-session signature
// can't be confused with a PocketBase auth token even though both are
// HS256. The _superusers secret is app-wide, stable across restarts,
// and shared by every member (same DB), so all modules derive the same
// key.
func signingKey(app core.App) (string, error) {
	col, err := app.FindCachedCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		return "", fmt.Errorf("sharelink: load superusers collection: %w", err)
	}
	base := col.AuthToken.Secret
	if base == "" {
		return "", errors.New("sharelink: superusers auth token secret is empty")
	}
	mac := hmac.New(sha256.New, []byte(base))
	mac.Write([]byte("tinycld:sharelink:v1"))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// MintSession signs a session token for the given claims. Called by
// drive when an anonymous visitor opens a share link.
func MintSession(app core.App, claims Claims) (string, error) {
	key, err := signingKey(app)
	if err != nil {
		return "", err
	}
	payload := jwtClaims(claims)
	return security.NewJWT(payload, key, SessionTTL)
}

// VerifySession verifies a session token's signature and expiry and
// returns its claims. It does NOT check whether the underlying link is
// still active — call ResolveLink (or VerifyAndResolve) for that.
func VerifySession(app core.App, token string) (Claims, error) {
	key, err := signingKey(app)
	if err != nil {
		return Claims{}, err
	}
	mapClaims, err := security.ParseJWT(token, key)
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	c := Claims{
		ShareToken:  asString(mapClaims["st"]),
		AnonID:      asString(mapClaims["aid"]),
		DisplayName: asString(mapClaims["dn"]),
		Role:        asString(mapClaims["role"]),
		ItemID:      asString(mapClaims["item"]),
	}
	if c.ShareToken == "" || c.AnonID == "" || c.ItemID == "" || c.Role == "" {
		return Claims{}, ErrInvalidToken
	}
	return c, nil
}

// VerifyAndResolve verifies the session token AND re-checks that the
// share link is still active and non-expired, returning the live link
// and drive_item records. This is what gates real actions (render,
// comment, edit): revoking a link takes effect immediately because the
// signature alone is not enough.
func VerifyAndResolve(app core.App, token string) (Claims, *core.Record, *core.Record, error) {
	claims, err := VerifySession(app, token)
	if err != nil {
		return Claims{}, nil, nil, err
	}
	link, item, err := ResolveLink(app, claims.ShareToken)
	if err != nil {
		return Claims{}, nil, nil, err
	}
	// Guard against a token minted for a different item than the link
	// currently points at (link edited, or a crafted token).
	if item.Id != claims.ItemID {
		return Claims{}, nil, nil, ErrInvalidToken
	}
	// The session's role must not exceed the link's current role (a link
	// downgraded from editor to viewer must immediately lose edit).
	if !roleAtMost(claims.Role, link.GetString("role")) {
		return Claims{}, nil, nil, ErrInvalidToken
	}
	return claims, link, item, nil
}

// ResolveLink loads a share link by its public token and validates it is
// active and not expired, returning the link + drive_item records.
// Shared by every public endpoint (it is the single source of truth for
// "is this link still usable").
func ResolveLink(app core.App, shareToken string) (*core.Record, *core.Record, error) {
	if len(shareToken) != 64 {
		return nil, nil, ErrLinkNotFound
	}
	link, err := app.FindFirstRecordByFilter(
		shareLinksCollection,
		"token = {:token}",
		map[string]any{"token": shareToken},
	)
	if err != nil || link == nil {
		return nil, nil, ErrLinkNotFound
	}
	if !link.GetBool("is_active") {
		return nil, nil, ErrLinkGone
	}
	expiresAt := link.GetDateTime("expires_at")
	if !expiresAt.IsZero() && expiresAt.Time().Before(time.Now()) {
		return nil, nil, ErrLinkGone
	}
	item, err := app.FindRecordById(driveItemsCollection, link.GetString("item"))
	if err != nil {
		return nil, nil, ErrLinkNotFound
	}
	return link, item, nil
}

// HTTPStatus maps a package error to an HTTP status code so callers
// across modules report consistent responses.
func HTTPStatus(err error) int {
	switch {
	case errors.Is(err, ErrLinkGone):
		return http.StatusGone
	case errors.Is(err, ErrLinkNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrInvalidToken):
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}

func jwtClaims(c Claims) map[string]any {
	return map[string]any{
		"st":   c.ShareToken,
		"aid":  c.AnonID,
		"dn":   c.DisplayName,
		"role": c.Role,
		"item": c.ItemID,
	}
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

// roleRank orders roles by privilege so a session role can be checked
// against the link's current role.
func roleRank(role string) int {
	switch role {
	case RoleEditor:
		return 3
	case RoleCommentor:
		return 2
	case RoleViewer:
		return 1
	default:
		return 0
	}
}

func roleAtMost(have, max string) bool {
	return roleRank(have) <= roleRank(max) && roleRank(have) > 0
}
