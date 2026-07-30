package davauth

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// Failed-authentication throttling for the DAV protocols.
//
// DAV has no login form and no session: every request carries Basic
// credentials, and PocketBase's own rate limiter covers its REST routes, not
// these. So an attacker can guess passwords against /carddav, /caldav or
// /dav/drive as fast as the server will answer, with nothing recording that it is
// happening. bcrypt bounds the rate, but bounding is not preventing — an
// online guessing attack against a weak password succeeds in hours.
//
// The limiter is deliberately narrow:
//
//   - Only FAILED attempts count. A client syncing legitimately makes many
//     requests per minute and must never be throttled.
//   - Keyed on client IP + identifier, so one attacker cannot lock out an
//     unrelated user by guessing at their account from elsewhere, and cannot
//     evade by rotating the username from one host.
//   - Sliding window, in memory. A restart forgives, which is acceptable: this
//     raises the cost of guessing, it is not an audit trail.
//
// It is NOT a lockout: there is no state a victim can be trapped in. Once the
// window passes, attempts resume.

const (
	// maxFailures is how many failed attempts one (ip, identifier) pair may
	// make inside failureWindow before being refused outright.
	//
	// Ten is generous for a human retyping a password and useless for a
	// dictionary: at 10 per 15 minutes, a thousand-word list takes a day.
	maxFailures = 10

	// failureWindow is the sliding window failures are counted over.
	failureWindow = 15 * time.Minute

	// sweepInterval is how often expired entries are dropped. Without it the
	// map grows once per distinct (ip, identifier) an attacker sends — which is
	// unbounded and attacker-controlled.
	sweepInterval = 5 * time.Minute
)

// throttle tracks recent failures per (ip, identifier).
type throttle struct {
	mu        sync.Mutex
	failures  map[string][]time.Time
	lastSweep time.Time
	// now is injectable so tests can advance the clock rather than sleep.
	now func() time.Time
}

var defaultThrottle = &throttle{
	failures: map[string][]time.Time{},
	now:      time.Now,
}

// TooManyFailures reports whether this (ip, identifier) pair has exhausted its
// attempts. Callers answer 429 without touching the database — the point is to
// stop spending bcrypt on an attacker. The app supplies the TrustedProxy
// settings that decide whether forwarded headers may name the client; nil is
// valid and means "trust nothing but the socket peer".
//
// A request carrying no credentials at all is never throttled: see isChallenge.
func TooManyFailures(app core.App, r *http.Request) bool {
	if isChallenge(r) {
		return false
	}
	identifier, _, _ := r.BasicAuth()
	return defaultThrottle.blocked(clientIP(app, r), identifier)
}

// NoteFailure records one failed authentication. A credential-less request is
// not one — see isChallenge.
func NoteFailure(app core.App, r *http.Request) {
	if isChallenge(r) {
		return
	}
	identifier, _, _ := r.BasicAuth()
	defaultThrottle.note(clientIP(app, r), identifier)
}

// isChallenge reports whether the request supplied no Basic credentials at all.
//
// Every DAV client's first request is exactly this: it asks for the challenge,
// then re-sends with credentials. Counted as a failure it lands in the (ip, "")
// bucket — which NoteSuccess can never clear, because success clears
// (ip, identifier) — so the bucket only accumulates. Around ten mounts behind
// one NAT and the challenge itself starts answering 429, locking out every
// client from that address; since clients retry, it refills as fast as it ages
// out.
//
// Excluding it costs nothing: with no credentials there is nothing to guess
// with, no identity to resolve, and no bcrypt to spend. A supplied-but-empty
// username (`Authorization: Basic OnBhc3N3b3Jk`) is still a guess and still
// counts — the discriminator is whether the header parsed, not whether the
// username is non-empty.
func isChallenge(r *http.Request) bool {
	_, _, ok := r.BasicAuth()
	return !ok
}

// NoteSuccess clears the counter: a correct password means this pair is not an
// attacker, and a user who mistyped twice before getting it right should not
// carry those failures forward.
func NoteSuccess(app core.App, r *http.Request) {
	identifier, _, _ := r.BasicAuth()
	defaultThrottle.clear(clientIP(app, r), identifier)
}

func (t *throttle) key(ip, identifier string) string {
	return ip + "\x00" + identifier
}

func (t *throttle) blocked(ip, identifier string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sweepLocked()

	cutoff := t.now().Add(-failureWindow)
	recent := 0
	for _, at := range t.failures[t.key(ip, identifier)] {
		if at.After(cutoff) {
			recent++
		}
	}
	return recent >= maxFailures
}

func (t *throttle) note(ip, identifier string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sweepLocked()

	k := t.key(ip, identifier)
	cutoff := t.now().Add(-failureWindow)
	kept := t.failures[k][:0]
	for _, at := range t.failures[k] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	t.failures[k] = append(kept, t.now())
}

func (t *throttle) clear(ip, identifier string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.failures, t.key(ip, identifier))
}

// sweepLocked drops entries whose failures have all aged out. Called from the
// paths that already hold the lock, at most once per sweepInterval.
func (t *throttle) sweepLocked() {
	now := t.now()
	if now.Sub(t.lastSweep) < sweepInterval {
		return
	}
	t.lastSweep = now

	cutoff := now.Add(-failureWindow)
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

// clientIP resolves the throttle-bucket address for a request.
//
// Forwarded headers are honored ONLY when the app's TrustedProxy settings name
// them — the same switch PocketBase's own RealIP() uses. The multi-org router
// materializes it for every tenant (whose requests arrive over a unix socket,
// where RemoteAddr identifies nobody), and a standalone operator sets it when
// deploying behind a proxy. Trusting the header unconditionally handed it to
// the CLIENT on any direct connection: rotating it minted a fresh bucket per
// request (throttle bypass), and naming a victim's egress IP filled the
// victim's bucket remotely (targeted, refreshable lockout).
//
// When a trusted header is read, the RIGHTMOST parseable entry of its LAST
// value wins (unless UseLeftmostIP) — that is the entry the proxy appended;
// leftmost values are whatever the client sent. Mirrors core's RealIP.
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
