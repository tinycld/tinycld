// Package davcond enforces the HTTP conditional-request preconditions
// (If-Match / If-None-Match) for DAV PUTs.
//
// go-webdav parses the headers and hands them to each backend via PutOptions,
// delegating enforcement entirely — a backend that ignores them turns every
// concurrent edit into silent last-writer-wins data loss: the client's PUT of
// a stale copy succeeds instead of drawing the 412 that would make it
// re-fetch and merge. Shared here so CalDAV and CardDAV (whose options are
// distinct types carrying the same two fields) enforce identical semantics.
package davcond

import (
	"errors"
	"net/http"

	"github.com/emersion/go-webdav"
)

var errPrecondFailed = errors.New("precondition failed")

// Check validates a PUT's conditional headers against the resource's current
// state, per RFC 9110 §13.1.1–13.1.2. currentETag is the resource's UNQUOTED
// entity tag ("" allowed when it does not exist); exists reports whether the
// resource is already there. Returns a 412 *webdav.HTTPError on violation and
// nil when the write may proceed.
func Check(ifNoneMatch, ifMatch webdav.ConditionalMatch, currentETag string, exists bool) error {
	if ifNoneMatch.IsSet() && exists {
		// If-None-Match: * is "create only". A concrete tag refuses overwrite
		// only when it matches the stored representation.
		match, err := ifNoneMatch.MatchETag(currentETag)
		if err != nil {
			return webdav.NewHTTPError(http.StatusBadRequest, err)
		}
		if ifNoneMatch.IsWildcard() || match {
			return webdav.NewHTTPError(http.StatusPreconditionFailed, errPrecondFailed)
		}
	}

	if ifMatch.IsSet() {
		// If-Match binds the write to a specific stored representation; a
		// resource that is gone (or never existed) can match nothing.
		if !exists {
			return webdav.NewHTTPError(http.StatusPreconditionFailed, errPrecondFailed)
		}
		match, err := ifMatch.MatchETag(currentETag)
		if err != nil {
			return webdav.NewHTTPError(http.StatusBadRequest, err)
		}
		if !match {
			return webdav.NewHTTPError(http.StatusPreconditionFailed, errPrecondFailed)
		}
	}

	return nil
}
