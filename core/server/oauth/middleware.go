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

// grantedScopesKey is the request-store key holding the verified grant's
// scopes. Set only by enforceGrant, so its presence also distinguishes an
// OAuth-authenticated request from a session one.
const grantedScopesKey = "tinycldOAuthScopes"

// GrantedScopes returns the scopes of the OAuth grant that authenticated this
// request, or nil when the caller is a signed-in session rather than a token.
//
// nil and empty mean different things to a caller narrowing its behavior: nil is
// "no scope ceiling applies", an empty non-nil slice would be "a token granting
// nothing". Only a token ever carries a ceiling, so a session must read as nil.
func GrantedScopes(re *core.RequestEvent) []string {
	scopes, ok := re.Get(grantedScopesKey).([]string)
	if !ok {
		return nil
	}
	return scopes
}

// scopeExempt marks routes reachable without any scope check — public probes
// and the OAuth endpoints themselves, which must work before a grant exists.
const scopeExempt = "-"

// middlewarePriority runs this check BEFORE PocketBase's own loadAuthToken.
// Lower number = earlier. We need to run first so that, for an OAuth token, we
// are the one who validates the grant; PB's middleware then finds e.Auth
// already set and no-ops (see apis/middlewares.go:190).
var middlewarePriority = apis.DefaultLoadAuthTokenMiddlewarePriority - 10

// scopeRule is the set of scopes that satisfy a route. A caller passes when it
// holds ANY of them (see satisfiedBy): a collection shared between packages
// — `labels` is used by both mail and contacts — must be reachable by either
// package's grant rather than demanding both. An empty rule denies.
type scopeRule []string

// collectionAccess pairs the read and write rules for one collection. An empty
// write rule makes the collection read-only for OAuth callers.
type collectionAccess struct {
	read  scopeRule
	write scopeRule
}

// collectionScopes maps a PocketBase collection to the scopes governing it.
// Anything absent is denied for OAuth callers by default.
var collectionScopes = map[string]collectionAccess{
	"mail_messages":      {read: scopeRule{ScopeMailRead}, write: scopeRule{ScopeMailSend}},
	"mail_threads":       {read: scopeRule{ScopeMailRead}, write: scopeRule{ScopeMailSend}},
	"mail_thread_state":  {read: scopeRule{ScopeMailRead}, write: scopeRule{ScopeMailSend}},
	"mail_mailboxes":     {read: scopeRule{ScopeMailRead}, write: scopeRule{ScopeMailSend}},
	"drive_items":        {read: scopeRule{ScopeDriveRead}, write: scopeRule{ScopeDriveWrite}},
	"drive_shares":       {read: scopeRule{ScopeDriveRead}, write: scopeRule{ScopeDriveWrite}},
	"drive_item_state":   {read: scopeRule{ScopeDriveRead}, write: scopeRule{ScopeDriveWrite}},
	"contacts":           {read: scopeRule{ScopeContactsRead}, write: scopeRule{ScopeContactsWrite}},
	"calendar_events":    {read: scopeRule{ScopeCalendarRead}, write: scopeRule{ScopeCalendarWrite}},
	"calendar_calendars": {read: scopeRule{ScopeCalendarRead}, write: scopeRule{ScopeCalendarWrite}},
	"users":              {read: scopeRule{ScopeProfile}},

	// Read-only surfaces the CLI needs: per-folder unread counts (a view), the
	// caller's mailbox memberships, and the mailbox aliases a send can pick a
	// From identity from. Aliases are administered in the app, so no write.
	//
	// mail_domains belongs here for a non-obvious reason: mail_mailboxes.address
	// stores only the LOCAL PART, so every full address the CLI prints or matches
	// on has to join the domain row. Without it `mail mailboxes`, `mail send`,
	// and `--mailbox <address>` all fail closed on the domain read, even holding
	// mail:read and mail:send. Domains are administered in the app, so no write.
	"mail_folder_counts":   {read: scopeRule{ScopeMailRead}},
	"mail_mailbox_members": {read: scopeRule{ScopeMailRead}},
	"mail_mailbox_aliases": {read: scopeRule{ScopeMailRead}},
	"mail_domains":         {read: scopeRule{ScopeMailRead}},

	// Labels are CORE collections shared across packages (mail threads and
	// contacts both get labelled through label_assignments), so either
	// package's scope grants access — requiring both would make labelling
	// mail impossible for a mail-only grant.
	"labels":            {read: scopeRule{ScopeMailRead, ScopeContactsRead}, write: scopeRule{ScopeMailSend, ScopeContactsWrite}},
	"label_assignments": {read: scopeRule{ScopeMailRead, ScopeContactsRead}, write: scopeRule{ScopeMailSend, ScopeContactsWrite}},

	"drive_item_versions": {read: scopeRule{ScopeDriveRead}, write: scopeRule{ScopeDriveWrite}},

	// Cards' board content. Every one of these carries a `project` relation
	// (denormalized onto the content rows precisely so a rule can reach it),
	// and the access rules resolve membership through cards_project_members —
	// so a grant here widens WHICH ROWS a token may touch not at all. It only
	// decides whether an OAuth caller may use the collection at all, on top of
	// the membership the rules already demand.
	"cards_projects":        {read: scopeRule{ScopeCardsRead}, write: scopeRule{ScopeCardsWrite}},
	"cards_lists":           {read: scopeRule{ScopeCardsRead}, write: scopeRule{ScopeCardsWrite}},
	"cards_cards":           {read: scopeRule{ScopeCardsRead}, write: scopeRule{ScopeCardsWrite}},
	"cards_labels":          {read: scopeRule{ScopeCardsRead}, write: scopeRule{ScopeCardsWrite}},
	"cards_checklist_items": {read: scopeRule{ScopeCardsRead}, write: scopeRule{ScopeCardsWrite}},
	"cards_comments":        {read: scopeRule{ScopeCardsRead}, write: scopeRule{ScopeCardsWrite}},
	"cards_attachments":     {read: scopeRule{ScopeCardsRead}, write: scopeRule{ScopeCardsWrite}},

	// READ-ONLY for OAuth callers, deliberately, and NOT because the rules are
	// weak — they already confine both to owners. These two are the SHARING
	// surface: a write to cards_project_members adds a person to a board, and a
	// write to cards_share_links mints a URL that opens the board to anyone
	// holding it. That is a categorically larger grant than editing cards, and
	// `cards:write` reads to a user consenting on the OAuth screen as "change my
	// cards" — not "give other people my boards".
	//
	// So a leaked or over-broad CLI token cannot reshare a board or publish a
	// public link; those stay in the app, where the Share dialog shows exactly
	// who gains access. Relaxing this later is one line and is backward
	// compatible; the reverse would silently revoke a capability integrations
	// had already built on, so start closed.
	"cards_project_members": {read: scopeRule{ScopeCardsRead}},
	"cards_share_links":     {read: scopeRule{ScopeCardsRead}},

	// calendar_members is the same shape of surface, and was simply missing —
	// so it default-denied and took `calendar list` down with it, since the
	// ROLE column reads membership. Read-only for the same reason as
	// cards_project_members: a write here grants another person access to a
	// calendar, which is not what "change my calendar events" means on the
	// consent screen. Read is what the CLI needs — a viewer learns their role
	// from a column rather than from a failed write.
	"calendar_members": {read: scopeRule{ScopeCalendarRead}},

	// text and calc own ONLY their comment collections; the documents and
	// spreadsheets they comment on are drive_items, governed by drive:*.
	//
	// A grant here widens which rows a token may touch not at all: both rules
	// reach through `drive_item` and admit only the document's creator or
	// someone it is shared with, and create additionally demands the caller be
	// the comment's own author. So this decides whether an OAuth caller may use
	// the collection at all, on top of the document access the rules demand —
	// which in practice means a useful token also holds drive:read.
	"text_comments": {read: scopeRule{ScopeTextRead}, write: scopeRule{ScopeTextWrite}},
	"calc_comments": {read: scopeRule{ScopeCalcRead}, write: scopeRule{ScopeCalcWrite}},
}

// endpointScopes maps a bespoke Go endpoint to its required scope.
var endpointScopes = map[string]scopeRule{
	"GET /api/mail/search":              {ScopeMailRead},
	"POST /api/mail/send":               {ScopeMailSend},
	"POST /api/mail/draft":              {ScopeMailSend},
	"GET /api/drive/search":             {ScopeDriveRead},
	"POST /api/drive/download-token":    {ScopeDriveRead},
	"POST /api/drive/export-token":      {ScopeDriveRead},
	"GET /api/drive/storage-usage":      {ScopeDriveRead},
	"POST /api/drive/upload-version":    {ScopeDriveWrite},
	"POST /api/drive/share":             {ScopeDriveWrite},
	"POST /api/drive/share-link":        {ScopeDriveWrite},
	"GET /api/drive/share-links":        {ScopeDriveRead},
	"POST /api/drive/versions/restore":  {ScopeDriveWrite},
	"POST /api/drive/versions/snapshot": {ScopeDriveWrite},
	"GET /api/contacts/export":          {ScopeContactsRead},
	"POST /api/contacts/import":         {ScopeContactsWrite},
	"GET /api/calendar/export":          {ScopeCalendarRead},
	"POST /api/calendar/import":         {ScopeCalendarWrite},
	"GET /api/cards/search":             {ScopeCardsRead},

	// The federated search narrows itself: it drops the sources a caller's
	// grant does not cover and returns the rest. So ANY read scope admits the
	// request — demanding one specific scope would 403 a contacts-only token
	// outright instead of handing it the contacts results it may see.
	"GET /api/search": {
		ScopeMailRead, ScopeDriveRead, ScopeContactsRead,
		ScopeCalendarRead, ScopeCardsRead,
	},

	// Advertised in the discovery document as userinfo_endpoint, so an
	// integration following the well-known metadata calls it with an ordinary
	// access token. It needs an explicit entry: it lives under /oauth/ but is
	// deliberately NOT in exemptPaths (only the credential-less endpoints are),
	// so without this it would fall into default-deny and 403 the very call the
	// server tells clients to make.
	"GET /oauth/userinfo": {ScopeProfile},
}

// endpointPrefixScopes classifies routes whose path carries a record id, which
// the exact-match table above cannot express. Kept deliberately short: a
// prefix is broader than it looks, so each entry must end at a path segment
// boundary and name a route family, never a bare namespace.
var endpointPrefixScopes = []struct {
	method, prefix string
	scopes         scopeRule
}{
	{"DELETE", "/api/drive/share-link/", scopeRule{ScopeDriveWrite}},
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

// ScopeForRoute returns the scopes that satisfy a request — the caller needs
// ANY one of them — or scopeExempt for public routes. An empty result means
// deny.
//
// Default deny is deliberate: a route nobody has classified must not be
// reachable with a third-party token just because someone added it.
func ScopeForRoute(method, path string) scopeRule {
	for _, p := range exemptPaths {
		if strings.HasPrefix(path, p) {
			return scopeRule{scopeExempt}
		}
	}
	if s, ok := endpointScopes[method+" "+path]; ok {
		return s
	}
	for _, r := range endpointPrefixScopes {
		// The remainder must be non-empty: the prefix ends in "/" and names a
		// route family, so the bare prefix itself is a different route.
		if method == r.method && strings.HasPrefix(path, r.prefix) && len(path) > len(r.prefix) {
			return r.scopes
		}
	}
	if name, ok := collectionFromPath(path); ok {
		access, known := collectionScopes[name]
		if !known {
			return nil
		}
		if writeMethods[method] {
			return access.write // empty for read-only collections => deny writes
		}
		return access.read
	}
	if name, ok := fileCollectionFromPath(path); ok {
		// A stored file (mail body, attachment, drive content) is governed by
		// its collection's READ scope. No field is `protected` today, so these
		// URLs answer a bare unauthenticated GET — but the CLI attaches its
		// bearer to every request, and without this classification the file
		// fetch would 403 for OAuth callers only. Writes never go through
		// /api/files/, so any write verb is denied outright.
		//
		// POST /api/files/token deliberately stays default-denied: file-token
		// requests are never scope-checked, so a file token minted by a bearer
		// would be a credential that bypasses the scope table.
		if method != http.MethodGet && method != http.MethodHead {
			return nil
		}
		access, known := collectionScopes[name]
		if !known {
			return nil
		}
		return access.read
	}
	return nil
}

// isExempt reports whether a rule marks the route as needing no scope.
func (r scopeRule) isExempt() bool {
	return len(r) == 1 && r[0] == scopeExempt
}

// describe renders the rule for an error message: a single scope reads as
// itself, several as an any-of list.
func (r scopeRule) describe() string {
	if len(r) == 1 {
		return fmt.Sprintf("%q", r[0])
	}
	quoted := make([]string, len(r))
	for i, s := range r {
		quoted[i] = fmt.Sprintf("%q", s)
	}
	return "one of " + strings.Join(quoted, ", ")
}

// satisfiedBy reports whether the granted scopes cover this rule.
func (r scopeRule) satisfiedBy(granted []string) bool {
	for _, want := range r {
		if HasScope(granted, want) {
			return true
		}
	}
	return false
}

// fileCollectionFromPath extracts "drive_items" from
// /api/files/drive_items/{recordId}/{filename}. All three segments must be
// present and non-empty — "/api/files/token" (2 segments) is not a file path.
func fileCollectionFromPath(path string) (string, bool) {
	const prefix = "/api/files/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(path, prefix), "/", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", false
	}
	return parts[0], true
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
	if len(required) == 0 {
		return re.ForbiddenError(
			"This endpoint is not available to API tokens", nil)
	}
	if !required.isExempt() {
		if !required.satisfiedBy(ParseScopes(grant.GetString("scopes"))) {
			return re.ForbiddenError(
				fmt.Sprintf("Requires the %s scope", required.describe()), nil)
		}
	}

	if err := TouchGrant(re.App, grant); err != nil {
		// Non-fatal: last_used_at is cosmetic.
		re.App.Logger().Warn("oauth: touch grant", "error", err)
	}

	// Publish the grant's scopes for handlers that must narrow their OWN
	// behavior rather than pass or fail wholesale. The route-level check above
	// answers "may this token call this endpoint"; an endpoint that federates
	// over several packages also needs "which of them may it see", so that a
	// mail-only token searching everything gets mail results instead of a 403.
	re.Set(grantedScopesKey, ParseScopes(grant.GetString("scopes")))

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
