package davauth

import (
	"net/http"
	"testing"
	"time"
)

// challenge_throttle_test.go covers F4.
//
// Every DAV client's FIRST request carries no credentials — it asks for the
// challenge and re-sends with Basic. That request has an empty identifier, so it
// landed in the (ip, "") bucket. NoteSuccess keys on the identifier too, so a
// successful login cleared (ip, "alice") and never (ip, ""): the empty bucket
// only ever accumulated. Roughly ten mounts behind one NAT and the challenge
// itself started answering 429 — nothing could authenticate from that address
// until the window passed, and every client retries, so it kept refilling.
//
// A credential-less request is not a failed attempt: there is nothing to guess
// with, and no bcrypt is spent on it. Excluding it from the limiter is what
// closes the loop; carddav/register.go already sidestepped this by challenging
// before consulting the limiter, which is why only CalDAV and WebDAV showed it.

// swapThrottle installs a throttle with a controllable clock for the duration
// of one test, so these exercise the exported entry points (which is where the
// identifier is read off the request) without leaking state between tests.
func swapThrottle(t *testing.T) {
	t.Helper()
	prev := defaultThrottle
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	defaultThrottle = &throttle{
		failures: map[string][]time.Time{},
		now:      func() time.Time { return now },
	}
	t.Cleanup(func() { defaultThrottle = prev })
}

func davRequest(t *testing.T, ip, user, pass string) *http.Request {
	t.Helper()
	r, err := http.NewRequest("PROPFIND", "/dav/drive/", nil)
	if err != nil {
		t.Fatal(err)
	}
	r.RemoteAddr = ip + ":51234"
	if user != "" {
		r.SetBasicAuth(user, pass)
	}
	return r
}

// The regression: the challenge round-trip must never be throttled, however
// many clients behind one address make it.
func TestCredentiallessRequestsAreNotThrottled(t *testing.T) {
	swapThrottle(t)

	for i := 0; i < maxFailures*5; i++ {
		r := davRequest(t, "198.51.100.4", "", "")
		if TooManyFailures(r) {
			t.Fatalf("challenge request %d was throttled — clients behind one NAT "+
				"cannot authenticate at all once this trips", i+1)
		}
		NoteFailure(r)
	}

	// The bucket must be empty, not merely under the limit: a recorded failure
	// with no identifier is never cleared by any success.
	if n := len(defaultThrottle.failures); n != 0 {
		t.Fatalf("credential-less attempts recorded %d throttle entries, want 0", n)
	}
}

// A blank username with a password IS a guess (PocketBase resolves no identity,
// but the caller supplied credentials), so it must still be counted — otherwise
// the exclusion above becomes a trivial bypass of the whole limiter.
func TestBlankIdentifierWithPasswordStillCounts(t *testing.T) {
	swapThrottle(t)

	for i := 0; i < maxFailures; i++ {
		r := davRequest(t, "198.51.100.6", "", "")
		r.Header.Set("Authorization", "Basic OnBhc3N3b3Jk") // ":password"
		NoteFailure(r)
	}
	probe := davRequest(t, "198.51.100.6", "", "")
	probe.Header.Set("Authorization", "Basic OnBhc3N3b3Jk")
	if !TooManyFailures(probe) {
		t.Fatal("an empty username with a supplied password evades the limiter")
	}
}

// The limiter must still bite on real credentials from the same address, or this
// fix would trade the lockout for an open guessing oracle.
func TestCredentialedFailuresStillThrottle(t *testing.T) {
	swapThrottle(t)

	for i := 0; i < maxFailures; i++ {
		NoteFailure(davRequest(t, "198.51.100.5", "victim@example.com", "guess"))
	}
	if !TooManyFailures(davRequest(t, "198.51.100.5", "victim@example.com", "guess")) {
		t.Fatal("credentialed guessing is no longer throttled")
	}
	// A different account from the same host keeps its own budget.
	if TooManyFailures(davRequest(t, "198.51.100.5", "other@example.com", "guess")) {
		t.Fatal("an unrelated account was throttled")
	}
}
