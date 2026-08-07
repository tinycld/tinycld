package oauth

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// Failed-exchange throttling for POST /oauth/token.
//
// The token endpoint is reachable with no credentials, and two of its grants
// carry a guessable secret: the device flow's user_code (RFC 8628, ~40 bits,
// typed by a human off a terminal) and device_code. PocketBase's own rate
// limiter covers its REST routes, not this bespoke endpoint, so nothing
// otherwise bounds how fast an attacker can guess.
//
// Modelled on davauth's throttle (davauth/ratelimit.go) but deliberately not
// shared with it: davauth derives its identifier from r.BasicAuth() and treats
// any credential-less request as unthrottled by design (isChallenge) — every
// OAuth request would read that way and bypass it entirely. Its throttle type
// and clientIP helper are also unexported. Rather than widen a charter that is
// deliberately scoped to the DAV protocols, this is its own small copy tuned
// for this endpoint:
//
//   - Keyed on (client IP, grant_type), never on the guessed secret itself —
//     keying on the secret would let an attacker rotate the key every attempt
//     (a fresh device_code or user_code guess each time) and never trip the
//     limiter.
//   - Only FAILED exchanges count. RFC 8628 §3.5 makes "authorization_pending"
//     the expected response to nearly every poll in a device-flow login — that
//     must never count as a failure, or every legitimate CLI login trips it.
//   - Sliding window, in memory, injectable clock for tests.
//   - Periodic sweep so the map cannot grow unbounded under attacker-chosen
//     keys.

const (
	// tokenMaxFailures bounds failed exchanges per (ip, grant_type) inside
	// tokenFailureWindow. A user_code carries ~40 bits — thin against an
	// unthrottled guesser, fine once each guess costs real wall-clock time.
	tokenMaxFailures = 10

	// tokenFailureWindow is the sliding window failures are counted over.
	tokenFailureWindow = 15 * time.Minute

	// tokenSweepInterval bounds how often expired entries are dropped.
	tokenSweepInterval = 5 * time.Minute
)

// tokenThrottle tracks recent failed exchanges per (ip, grant_type).
type tokenThrottle struct {
	mu        sync.Mutex
	failures  map[string][]time.Time
	lastSweep time.Time
	// now is injectable so tests can advance the clock rather than sleep.
	now func() time.Time
}

var defaultTokenThrottle = &tokenThrottle{
	failures: map[string][]time.Time{},
	now:      time.Now,
}

// tooManyTokenFailures reports whether this (ip, grant_type) pair has
// exhausted its attempts for the current window. Callers answer 429 without
// touching the database.
func tooManyTokenFailures(app core.App, r *http.Request, grantType string) bool {
	return defaultTokenThrottle.blocked(clientIP(app, r), grantType)
}

// noteTokenFailure records one failed exchange for this (ip, grant_type) pair.
// Only call this for an actual rejection — never for "authorization_pending",
// which is the expected, repeated response while a device-flow login is
// mid-flight (RFC 8628 §3.5).
func noteTokenFailure(app core.App, r *http.Request, grantType string) {
	defaultTokenThrottle.noteFailure(clientIP(app, r), grantType)
}

func (t *tokenThrottle) key(ip, grantType string) string {
	return ip + "\x00" + grantType
}

func (t *tokenThrottle) blocked(ip, grantType string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sweepLocked()

	cutoff := t.now().Add(-tokenFailureWindow)
	recent := 0
	for _, at := range t.failures[t.key(ip, grantType)] {
		if at.After(cutoff) {
			recent++
		}
	}
	return recent >= tokenMaxFailures
}

func (t *tokenThrottle) noteFailure(ip, grantType string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sweepLocked()

	k := t.key(ip, grantType)
	cutoff := t.now().Add(-tokenFailureWindow)
	kept := t.failures[k][:0]
	for _, at := range t.failures[k] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	t.failures[k] = append(kept, t.now())
}

// sweepLocked drops entries whose failures have all aged out. Called from the
// paths that already hold the lock, at most once per tokenSweepInterval.
func (t *tokenThrottle) sweepLocked() {
	now := t.now()
	if now.Sub(t.lastSweep) < tokenSweepInterval {
		return
	}
	t.lastSweep = now

	cutoff := now.Add(-tokenFailureWindow)
	for k, times := range t.failures {
		live := false
		for _, at := range times {
			if at.After(cutoff) {
				live = true
				break
			}
		}
		if !live {
			delete(t.failures, k)
		}
	}
}

// clientIP resolves the throttle-bucket address for a request. Trusted-proxy
// header handling mirrors davauth.clientIP; kept as its own small copy since
// that one is unexported and this package must not import davauth (an
// unrelated protocol package with its own narrow charter).
func clientIP(app core.App, r *http.Request) string {
	if app != nil {
		settings := app.Settings()
		for _, h := range settings.TrustedProxy.Headers {
			values := r.Header.Values(h)
			if len(values) == 0 {
				continue
			}
			ips := strings.Split(values[len(values)-1], ",")
			if settings.TrustedProxy.UseLeftmostIP {
				for _, ip := range ips {
					if parsed, err := netip.ParseAddr(strings.TrimSpace(ip)); err == nil {
						return parsed.StringExpanded()
					}
				}
			} else {
				for i := len(ips) - 1; i >= 0; i-- {
					if parsed, err := netip.ParseAddr(strings.TrimSpace(ips[i])); err == nil {
						return parsed.StringExpanded()
					}
				}
			}
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
