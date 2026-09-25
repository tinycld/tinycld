package format

import (
	"errors"
	"fmt"
	"net/url"
)

// redactURLError strips the URL out of Go's *url.Error.
//
// Every transfer in this package runs against a presigned URL, whose signature
// is in the query string. url.Error.Error() prints the WHOLE url — so the plain
// error from client.Do carries the signature into the ledger's error column, the
// package log, and Sentry. The failure point is the only place that can fix
// this: once the error leaves the package nothing downstream can tell a signed
// URL from any other string.
//
// The result keeps what an operator needs (the operation and the host) and
// wraps the inner error, so errors.Is/As still reach it.
func redactURLError(err error) error {
	var uerr *url.Error
	if !errors.As(err, &uerr) {
		return err
	}
	return fmt.Errorf("%s %s: %w", uerr.Op, hostPrefix(uerr.URL), uerr.Err)
}

// hostPrefix reduces a URL to "<scheme>://<host>" — no path, no query. A URL
// too malformed to parse is reported as "the target URL" rather than echoed:
// it could still be carrying a signature.
func hostPrefix(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "the target URL"
	}
	if u.Scheme == "" {
		return u.Host
	}
	return u.Scheme + "://" + u.Host
}
