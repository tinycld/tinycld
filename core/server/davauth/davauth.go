// Package davauth is the shared HTTP Basic authentication used by core's DAV
// protocol servers (CardDAV, WebDAV, and CalDAV when it lands).
//
// DAV clients — Finder, Thunderbird, davfs2 — authenticate with Basic on every
// request; there is no session. The check is feature-agnostic, so it lives here
// once rather than being reimplemented per protocol.
package davauth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	"golang.org/x/crypto/bcrypt"
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
	// One verification per request. See WithRequestCache: without it a single
	// PROPFIND runs bcrypt once per backend call.
	if cache, ok := r.Context().Value(cacheKey{}).(*cachedAuth); ok {
		if cache.settled {
			return cache.record, cache.err
		}
		defer func() { cache.settled = true }()
		record, err := authenticate(app, r)
		cache.record, cache.err = record, err
		return record, err
	}
	return authenticate(app, r)
}

func authenticate(app core.App, r *http.Request) (*core.Record, error) {
	identifier, password, ok := r.BasicAuth()
	if !ok || identifier == "" {
		return nil, ErrUnauthorized
	}
	return VerifyCredentials(app, identifier, password)
}

// VerifyCredentials is the one credential check every protocol server shares —
// DAV Basic auth here, and mail's IMAP LOGIN / SMTP AUTH, which authenticate
// against the record directly (PocketBase's auth hooks never run for them).
// Single-sourcing it is what keeps the identifier contract ("username or
// email") and the disabled cutoff identical across protocols.
//
// The identifier may be a bare username or a full email (the discriminator is
// '@'), mirroring PocketBase's identityFields for `users`. Returns
// ErrUnauthorized for a missing user, wrong password, or disabled account —
// deliberately indistinguishable.
func VerifyCredentials(app core.App, identifier, password string) (*core.Record, error) {
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
		// Spend the same work a real verification would before answering.
		//
		// Without this the miss path returns in microseconds while a hit pays
		// the full bcrypt cost — measured at ~700x on a developer machine —
		// which is a username oracle: an attacker learns which accounts exist
		// without guessing a single password. These protocols have no login
		// form to rate limit behind, so the timing IS the signal.
		compareAgainstDummyHash(password)
		return nil, ErrUnauthorized
	}

	if !record.ValidatePassword(password) {
		return nil, ErrUnauthorized
	}

	// Protocol logins are per-request or per-session — there is no token to
	// revoke — so this check is the only thing that cuts a suspended account
	// off from CardDAV, CalDAV, WebDAV, IMAP and SMTP.
	if record.GetBool("disabled") {
		return nil, ErrUnauthorized
	}

	return record, nil
}

// cacheKey is the context key under which a completed authentication is
// memoized for the life of one request.
type cacheKey struct{}

// cachedAuth is one request's settled authentication outcome, success or not.
type cachedAuth struct {
	settled bool
	record  *core.Record
	err     error
}

// WithRequestCache returns a request whose context memoizes the result of the
// first Authenticate call against it.
//
// bcrypt is deliberately expensive, and CardDAV re-authenticates inside every
// backend method — a single PROPFIND drives several, so one request paid the
// KDF cost repeatedly. That is a self-inflicted denial-of-service: an
// unauthenticated client can make the server do arbitrary bcrypt work by
// sending a request that fans out.
//
// Scoped to a request rather than a process-wide cache on purpose: caching a
// credential across requests would keep a password valid after it changed and
// would need its own invalidation story. This memo dies with the request.
func WithRequestCache(r *http.Request) *http.Request {
	cache := &cachedAuth{}
	return r.WithContext(context.WithValue(r.Context(), cacheKey{}, cache))
}

// dummyHash is a bcrypt hash of a value nothing can supply, generated once at
// init at the same cost PocketBase uses for real passwords.
//
// Generated rather than hard-coded so it tracks bcrypt.DefaultCost: a
// hard-coded hash at a stale cost would make the miss path measurably cheaper
// than a hit again, silently reopening the oracle this exists to close.
var dummyHash = func() []byte {
	h, err := bcrypt.GenerateFromPassword([]byte("davauth-dummy-password"), bcrypt.DefaultCost)
	if err != nil {
		// Impossible for a valid cost, but a nil hash would make every miss
		// instant, so fail loudly rather than degrade silently.
		panic("davauth: cannot generate dummy hash: " + err.Error())
	}
	return h
}()

// compareAgainstDummyHash spends one bcrypt verification and discards the
// result, so a lookup miss costs what a real password check costs.
func compareAgainstDummyHash(password string) {
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
}

// Challenge writes a 401 with the Basic-Auth challenge for the given realm.
func Challenge(w http.ResponseWriter, realm string) {
	w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
	http.Error(w, "Authentication required", http.StatusUnauthorized)
}
