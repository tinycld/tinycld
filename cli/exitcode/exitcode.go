// Package exitcode is the exit-code contract of the tinycld CLI, for the
// shell and for the command modules of packages
// (tinycld.org/packages/<slug>/cli).
//
// main exits with the code of the first *Error in the chain of the error a
// command returns, and with Failed for any other error. A command that wants
// a script or an agent to tell "sign in again" from "the server refused the
// data" returns Classify(err), which reads the class from the error itself.
//
// The numbers are a public contract: scripts test them. Add a class at the
// end; never renumber one.
package exitcode

import (
	"context"
	"errors"
	"net"

	"tinycld.org/cli/client"
)

const (
	// OK: the command did what was asked.
	OK = 0
	// Failed: any failure with no class of its own.
	Failed = 1
	// Usage: the call itself is wrong (flags, arguments, input document).
	// Nothing was sent to the server.
	Usage = 2
	// Auth: there is no usable credential (HTTP 401). Sign in again.
	Auth = 3
	// Forbidden: the server knows the caller and refuses the action
	// (HTTP 403): a missing OAuth scope or a missing permission.
	Forbidden = 4
	// NotFound: the record or route does not exist, or the caller may not
	// see it (HTTP 404).
	NotFound = 5
	// Refused: the server refused the data (any other HTTP 4xx). Change the
	// input; the same call fails again.
	Refused = 6
	// Unavailable: the server did not answer, or answered HTTP 408, 429 or
	// 5xx. The same call can succeed later.
	Unavailable = 7
)

// Error carries the exit code main uses for Err.
type Error struct {
	Code int
	Err  error
}

func (e *Error) Error() string {
	if e.Err == nil {
		return Name(e.Code)
	}
	return e.Err.Error()
}

func (e *Error) Unwrap() error { return e.Err }

// New gives err the exit code. A nil err stays nil.
func New(code int, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Code: code, Err: err}
}

// Of is the exit code for err: OK for nil, the code of the first *Error in
// its chain, or Failed.
func Of(err error) int {
	if err == nil {
		return OK
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return Failed
}

// Name is the stable, lower-case name of a code, for a machine-readable error
// ("usage", "not_found"). An unknown code is "failed".
func Name(code int) string {
	switch code {
	case OK:
		return "ok"
	case Usage:
		return "usage"
	case Auth:
		return "auth"
	case Forbidden:
		return "forbidden"
	case NotFound:
		return "not_found"
	case Refused:
		return "refused"
	case Unavailable:
		return "unavailable"
	default:
		return "failed"
	}
}

// Classify gives err the exit code of its class. An error that already
// carries a code keeps it, so a command can set Usage itself and classify
// everything else in one place.
func Classify(err error) error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return err
	}
	return &Error{Code: classOf(err), Err: err}
}

func classOf(err error) int {
	if errors.Is(err, client.ErrAuthExpired) {
		return Auth
	}
	var api *client.APIError
	if errors.As(err, &api) {
		return statusClass(api.Status)
	}
	// context.Canceled stays Failed: it is the caller's own cancel (Ctrl-C),
	// not a server that is down, so "retry later" would be wrong advice.
	// *url.Error, which every failed http.Client call returns, is a net.Error.
	var netErr net.Error
	if errors.As(err, &netErr) || errors.Is(err, context.DeadlineExceeded) {
		return Unavailable
	}
	return Failed
}

func statusClass(status int) int {
	switch {
	case status == 401:
		return Auth
	case status == 403:
		return Forbidden
	case status == 404:
		return NotFound
	case status == 408, status == 429, status >= 500:
		return Unavailable
	case status >= 400:
		return Refused
	default:
		return Failed
	}
}
