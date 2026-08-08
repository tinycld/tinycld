// Package useraccount answers "may this account act?" for paths that cannot ask
// PocketBase's collection rules.
//
// It is deliberately a leaf: it imports nothing from core, so every layer —
// fts, driveshare, carddav, coreserver, and feature packages — can depend on it
// without a cycle. That is the whole reason it is its own package rather than a
// function bolted onto coreserver (which transitively imports driveshare, so
// driveshare could not have imported it back).
package useraccount

import (
	"github.com/pocketbase/pocketbase/core"
)

// usersCollection is the auth collection every account lives in.
const usersCollection = "users"

// IsSuspended reports whether an account may not act — because it is flagged
// `disabled = true`, or because we cannot prove otherwise.
//
// This is the per-REQUEST guard, and the companion to coreserver's
// RegisterDisabledUserGuard: that one refuses to ISSUE a token to a disabled
// user, this one refuses to serve a request carrying a token minted before the
// user was disabled. Neither covers the other.
//
// Use it on any path that authorizes a whole query rather than one record, and
// so cannot go through a per-record role check: raw Go routes and direct SQL
// bypass PocketBase's collection rules entirely, so without an explicit call
// here a suspended account keeps reading until its token expires. Paths that
// read records through the REST API are already covered by each collection's
// `@request.auth.disabled != true` rule.
//
// Fails CLOSED, in both senses: an empty user id and a lookup error both report
// suspended. That is deliberate — if we cannot prove the account is active, a DB
// blip must not silently restore a suspended user's reach. Reading the flag off
// an already-loaded auth record instead would be wrong: that record is a
// snapshot from token-issuance time, so a user suspended afterwards would still
// pass. This re-fetches every time for exactly that reason.
//
// A deployment predating the `disabled` field has no such column, and GetBool
// returns false, so this is a no-op there.
func IsSuspended(app core.App, userID string) bool {
	if userID == "" {
		return true
	}
	user, err := app.FindRecordById(usersCollection, userID)
	if err != nil || user == nil {
		return true
	}
	return user.GetBool("disabled")
}
