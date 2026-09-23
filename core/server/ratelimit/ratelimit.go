// Package ratelimit is a small in-memory, per-key request limiter for the
// public (credential-less) endpoints a package exposes.
//
// Promoted from drive/server, which had the only implementation, when boards'
// public boards needed the same thing. Keeping a second copy in a second
// member was the alternative; a third was already foreseeable.
//
// WHAT THIS IS NOT: the state lives in one process's memory, so it does not
// hold across instances — two app processes behind a load balancer each
// enforce the limit separately, and a restart forgets everything. It raises
// the cost of a brute-force from one host; it is not a defence against a
// distributed one. Treat it as one layer, and make sure the endpoint's real
// protection (entropy, single-use codes, an expiry) stands on its own.
//
// The map is never evicted beyond the sliding window's own pruning, so a very
// long-lived process seeing very many distinct IPs grows it slowly. Bounded in
// practice by the fact that an entry is a key plus a short timestamp slice.
package ratelimit

import (
	"net/http"
	"sync"
	"time"

	"tinycld.org/core/logging"
)

var log = logging.ForPackage("ratelimit")

// Limiter allows up to `limit` events per `window` for each key, on a sliding
// window. The zero value is not usable — build one with New.
type Limiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
}

// New returns a limiter allowing `limit` events per `window` per key.
func New(limit int, window time.Duration) *Limiter {
	return &Limiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

// Reset clears every key's history.
//
// Intended for test isolation: a package-level limiter is a singleton whose
// state otherwise leaks between test cases, and httptest gives every request
// the same RemoteAddr — so without a reset the tenth case in a file starts
// with nine strikes against it and fails for reasons that have nothing to do
// with what it is testing.
func (l *Limiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.requests = make(map[string][]time.Time)
}

// Allow records an event for `key` and reports whether it is within the limit.
// A refused event is NOT recorded, so a caller cannot extend their own lockout
// by hammering.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window)

	// Prune in place — `valid` aliases the same backing array.
	entries := l.requests[key]
	valid := entries[:0]
	for _, t := range entries {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= l.limit {
		l.requests[key] = valid
		return false
	}

	l.requests[key] = append(valid, now)
	return true
}

// AllowOrLog is Allow, plus a record of the refusal.
//
// Prefer it at every endpoint. A 429 is one of the clearest abuse signals a
// deployment produces — a caller repeatedly bouncing off a ceiling looks
// nothing like one using the endpoint normally — and a bare Allow returns
// that signal to the caller and discards it, leaving no way to answer "who
// was hammering this?" afterwards.
//
// The logging lives here rather than inside Allow because Allow is called per
// request on hot public paths and does not know which endpoint it is guarding;
// a record without that label is noise. `endpoint` is a short stable string
// ("drive.share.download"), not a formatted message.
//
// Warn so a spike reaches Sentry without anyone querying _logs. A refused
// event is still not recorded against the key — see Allow.
//
// A caller that wants the request's Sentry hub attached (so the refusal is
// attributed to a user) should keep using Allow and log it itself with
// WarnContext — webhookin/receive.go does exactly that.
func (l *Limiter) AllowOrLog(key string, endpoint string) bool {
	if l.Allow(key) {
		return true
	}
	log.Warn("rate limit refused a request",
		"endpoint", endpoint, "key", key, "limit", l.limit, "window", l.window)
	return false
}

// ClientIP is the key most callers want.
//
// It returns X-Forwarded-For whole rather than parsing out the first hop,
// matching drive's original behaviour: a spoofable header is a spoofable key
// either way, and taking the whole string means a forged chain at least has to
// be forged consistently to reuse a budget. Falls back to RemoteAddr.
func ClientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return fwd
	}
	return r.RemoteAddr
}
