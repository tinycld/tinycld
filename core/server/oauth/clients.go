package oauth

import (
	"crypto/subtle"
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// FindClientByClientID resolves a registered client. An unknown client_id is
// an error: this is the registry that decides who may ask for access at all.
func FindClientByClientID(app core.App, clientID string) (*core.Record, error) {
	if clientID == "" {
		return nil, fmt.Errorf("oauth: empty client_id")
	}
	rec, err := app.FindFirstRecordByFilter(
		clientsCollection, "client_id = {:id}", map[string]any{"id": clientID},
	)
	if err != nil || rec == nil {
		return nil, fmt.Errorf("oauth: unknown client %q", clientID)
	}
	return rec, nil
}

// VerifyClientSecret checks a confidential client's secret. A public client
// (the CLI) has no secret — PKCE protects it — so this always reports true
// for one, and callers must not require a secret from a public client.
func VerifyClientSecret(client *core.Record, secret string) bool {
	if client.GetString("type") == "public" {
		return true
	}
	stored := client.GetString("client_secret_hash")
	if stored == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(hashSecret(secret)), []byte(stored)) == 1
}

// ValidateScopes rejects any scope outside the catalog. Silently dropping an
// unknown scope would let a client believe it holds access it does not.
func ValidateScopes(requested []string) error {
	for _, s := range requested {
		if !HasScope(AllScopes, s) {
			return fmt.Errorf("oauth: unknown scope %q", s)
		}
	}
	return nil
}

// ValidateClientScopes enforces the client's own registration as a ceiling,
// on top of ValidateScopes' catalog check. AllScopes says what the SERVER
// knows how to grant at all; oauth_clients.scopes says what THIS client was
// registered to ask for. Both must hold, or a client registered for
// `profile` only could request `mail:send drive:write` and receive it, since
// nothing else in the request path ever reads the client's own scopes column
// — it is set at registration time and otherwise silently unenforced.
//
// An unset/empty scopes field denies every scope rather than allowing every
// scope. "Allow all" would make the column decorative for exactly the
// clients most likely to leave it blank — quick manual registrations — which
// is backwards: the ceiling matters most for a client nobody has reviewed
// yet. The seeded first-party CLI client lists its full scope set
// explicitly (1980000001_seed_cli_oauth_client.js), so deny-by-default costs
// it nothing.
//
// ScopeProfile is always allowed regardless of registration: it is the
// baseline identity scope every grant gets (see its doc comment in
// oauth.go), and both callers default an empty request to exactly this
// scope — that default must never itself be able to fail the ceiling it is
// falling back to satisfy.
func ValidateClientScopes(client *core.Record, requested []string) error {
	if err := ValidateScopes(requested); err != nil {
		return err
	}
	allowed := ParseScopes(client.GetString("scopes"))
	for _, s := range requested {
		if s == ScopeProfile {
			continue
		}
		if !HasScope(allowed, s) {
			return fmt.Errorf("oauth: scope %q is not registered for this client", s)
		}
	}
	return nil
}

// RedirectURIAllowed reports whether uri exactly matches one of the client's
// registered redirect URIs. Exact match only — prefix matching is a well-known
// open-redirect vector.
func RedirectURIAllowed(client *core.Record, uri string) bool {
	if uri == "" {
		return false
	}
	var registered []string
	if err := client.UnmarshalJSONField("redirect_uris", &registered); err != nil {
		return false
	}
	for _, r := range registered {
		if r == uri {
			return true
		}
	}
	return false
}
