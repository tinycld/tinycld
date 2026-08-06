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
