package oauth

import (
	"crypto/subtle"
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// FindClientByClientID resolves a registered client. An unknown client_id is
// an error: this is the registry that decides who may ask for access at all.
//
// A disabled client resolves as if it were unknown. This is the front half of
// the kill switch — it stops a decommissioned or compromised client from
// obtaining any NEW authorization, since every entry point that mints or
// exchanges credentials (handleAuthorize, handleDeviceAuthorization,
// handleToken) resolves its client through here. The back half is in
// VerifyGrant, which cuts off the access such a client already holds.
//
// Reported as "unknown" rather than a distinct "disabled" error on purpose:
// the caller is an unauthenticated party naming a client_id, and telling it
// "that client exists but is switched off" turns this into an oracle for
// which client_ids are registered. Same reasoning as VerifyGrant refusing to
// distinguish disabled from revoked.
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
	if rec.GetBool("disabled") {
		return nil, fmt.Errorf("oauth: unknown client %q", clientID)
	}
	return rec, nil
}

// VerifyClientSecret checks a confidential client's secret. A public client
// (the CLI) has no secret — PKCE protects it — so this always reports true
// for one, and callers must not require a secret from a public client.
//
// The empty-stored-hash branch compares against a dummy rather than returning
// early, so a confidential client with no secret set costs the same as one
// with a wrong secret.
func VerifyClientSecret(client *core.Record, secret string) bool {
	if client.GetString("type") == "public" {
		return true
	}
	stored := client.GetString("client_secret_hash")
	if stored == "" {
		compareAgainstDummyHash(secret)
		return false
	}
	return subtle.ConstantTimeCompare([]byte(hashSecret(secret)), []byte(stored)) == 1
}

// dummyClientSecretHash is the hex SHA-256 of a value no caller can supply.
// Computed at init from the same hashSecret used for real secrets, so it is
// always the right length for a constant-time compare against a real hash —
// a hard-coded literal could drift from the hash function and make the miss
// path measurably different again.
var dummyClientSecretHash = hashSecret("oauth-dummy-client-secret")

// compareAgainstDummyHash spends a client-secret verification and discards the
// result, so a path that has no real hash to check against still does the work
// one would.
//
// This is deliberately weaker than davauth's namesake, and for a different
// reason. There the mitigation exists to spend BCRYPT COST: a miss returning
// in microseconds against a hit paying a ~700x KDF was a loud username oracle.
// Client secrets are stored as plain SHA-256 (hashSecret), which is fast
// enough that the absolute timing difference here is near the noise floor of
// any remote measurement. What this removes is the BRANCH, not a cost gap: the
// miss path now performs the same hash-and-compare the hit path does, so the
// shape of the work no longer depends on whether the client_id was real.
//
// Kept because "the signal is small" is not "the signal is absent", and the
// cost of closing it is one hash. It is not load-bearing the way the bcrypt
// version is — the real defense for client_id enumeration is the rate limiter
// (ratelimit.go), which bounds how many probes an attacker gets to average
// over, and averaging is exactly what extracting a signal this small requires.
func compareAgainstDummyHash(secret string) {
	_ = subtle.ConstantTimeCompare(
		[]byte(hashSecret(secret)), []byte(dummyClientSecretHash),
	)
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
