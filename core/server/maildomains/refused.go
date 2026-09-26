package maildomains

import "errors"

// ErrDomainRefused matches every *RefusedError via errors.Is, so a caller that
// only needs to know "the registrar said no on purpose" does not have to
// unpack the code.
var ErrDomainRefused = errors.New("maildomains: domain refused by the registrar")

// RefusedError is what a registrar returns when it deliberately declines a
// domain name — as opposed to failing to reach the provider. The two need
// different answers: a failure is retried or reported as an outage, while a
// refusal can only be fixed by the org's admin choosing another name.
//
// Code is a stable machine-readable reason a client may branch on. Message is
// shown verbatim to the org's admin, so a registrar must write it for that
// reader: say what is wrong and what to do, and name no provider or internal
// detail.
type RefusedError struct {
	Code    string
	Message string
}

// Refused builds a *RefusedError.
func Refused(code, message string) *RefusedError {
	return &RefusedError{Code: code, Message: message}
}

func (e *RefusedError) Error() string {
	return "maildomains: domain refused (" + e.Code + "): " + e.Message
}

// Is lets errors.Is(err, ErrDomainRefused) match any refusal regardless of code.
func (e *RefusedError) Is(target error) bool {
	return target == ErrDomainRefused
}
