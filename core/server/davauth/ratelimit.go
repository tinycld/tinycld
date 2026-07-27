package davauth

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
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
// stop spending bcrypt on an attacker.
func TooManyFailures(r *http.Request) bool {
	identifier, _, _ := r.BasicAuth()
	return defaultThrottle.blocked(clientIP(r), identifier)
}

// NoteFailure records one failed authentication.
func NoteFailure(r *http.Request) {
	identifier, _, _ := r.BasicAuth()
	defaultThrottle.note(clientIP(r), identifier)
}

// NoteSuccess clears the counter: a correct password means this pair is not an
// attacker, and a user who mistyped twice before getting it right should not
// carry those failures forward.
func NoteSuccess(r *http.Request) {
	identifier, _, _ := r.BasicAuth()
	defaultThrottle.clear(clientIP(r), identifier)
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

// clientIP extracts the caller's address, preferring the proxy chain.
//
// Under the multi-org router every tenant request arrives over a unix socket,
// where RemoteAddr is empty — so without the forwarded header every caller
// would share one bucket and the limiter would throttle the whole deployment
// once any single attacker tripped it. The leftmost XFF entry is the original
// client. That header is spoofable by anyone talking to the server directly,
// which is why this is a cost-raising measure and not an access control.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		client, _, _ := strings.Cut(xff, ",")
		return strings.TrimSpace(client)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
