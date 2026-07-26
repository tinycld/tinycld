// Package davauth is the shared HTTP Basic authentication used by core's DAV
// protocol servers (CardDAV, WebDAV, and CalDAV when it lands).
//
// DAV clients — Finder, Thunderbird, davfs2 — authenticate with Basic on every
// request; there is no session. The check is feature-agnostic, so it lives here
// once rather than being reimplemented per protocol.
package davauth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// ErrUnauthorized is returned when credentials are missing, the identifier
// matches no user, or the password is wrong. Callers translate it to a 401 with
// a WWW-Authenticate challenge; the distinction between "no such user" and
// "wrong password" is deliberately not surfaced.
var ErrUnauthorized = errors.New("davauth: unauthorized")

// Authenticate validates HTTP Basic credentials against the users auth
// collection. The identifier may be a bare username or a full email (the
// discriminator is '@'), mirroring PocketBase's identityFields for `users`.
func Authenticate(app core.App, r *http.Request) (*core.Record, error) {
	identifier, password, ok := r.BasicAuth()
	if !ok || identifier == "" {
		return nil, ErrUnauthorized
	}

	var record *core.Record
	var err error
	if strings.Contains(identifier, "@") {
		record, err = app.FindAuthRecordByEmail("users", identifier)
	} else {
		record, err = app.FindFirstRecordByFilter(
			"users",
			"username = {:u}",
			map[string]any{"u": identifier},
		)
	}
	if err != nil || record == nil {
		return nil, ErrUnauthorized
	}

	if !record.ValidatePassword(password) {
		return nil, ErrUnauthorized
	}

	return record, nil
}

// Challenge writes a 401 with the Basic-Auth challenge for the given realm.
func Challenge(w http.ResponseWriter, realm string) {
	w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
	http.Error(w, "Authentication required", http.StatusUnauthorized)
}
