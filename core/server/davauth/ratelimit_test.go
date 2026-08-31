package davauth

import (
	"net/http"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// newTestThrottle returns a throttle with a controllable clock, so the sliding
// window can be tested without sleeping through it.
func newTestThrottle() (*throttle, func(time.Duration)) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	t := &throttle{
		failures: map[string][]time.Time{},
		now:      func() time.Time { return now },
	}
	return t, func(d time.Duration) { now = now.Add(d) }
}

func TestThrottle_BlocksAfterMaxFailures(t *testing.T) {
	th, _ := newTestThrottle()

	for i := 0; i < maxFailures-1; i++ {
		th.note("10.0.0.1", "victim@example.com")
		if th.blocked("10.0.0.1", "victim@example.com") {
			t.Fatalf("blocked after only %d failures, limit is %d", i+1, maxFailures)
		}
	}
	th.note("10.0.0.1", "victim@example.com")
	if !th.blocked("10.0.0.1", "victim@example.com") {
		t.Fatalf("not blocked after %d failures", maxFailures)
	}
}

// The window slides: attempts age out, so this raises the cost of guessing
// rather than locking an account permanently.
func TestThrottle_ForgivesAfterWindow(t *testing.T) {
	th, advance := newTestThrottle()

	for i := 0; i < maxFailures; i++ {
		th.note("10.0.0.1", "victim@example.com")
	}
	if !th.blocked("10.0.0.1", "victim@example.com") {
		t.Fatal("should be blocked")
	}

	advance(failureWindow + time.Second)
	if th.blocked("10.0.0.1", "victim@example.com") {
		t.Fatal("still blocked after the window elapsed; this is a lockout, not a throttle")
	}
}

// A correct password clears the counter, so someone who mistypes a few times
// and then succeeds does not carry the failures forward into their next sync.
func TestThrottle_SuccessClearsFailures(t *testing.T) {
	th, _ := newTestThrottle()

	for i := 0; i < maxFailures-1; i++ {
		th.note("10.0.0.1", "user@example.com")
	}
	th.clear("10.0.0.1", "user@example.com")

	for i := 0; i < maxFailures-1; i++ {
		th.note("10.0.0.1", "user@example.com")
		if th.blocked("10.0.0.1", "user@example.com") {
			t.Fatal("counter was not cleared by the successful auth")
		}
	}
}

// Keyed per (ip, identifier), so an attacker guessing at someone's account
// cannot lock that person out of their own client from a different address.
func TestThrottle_OneAttackerCannotLockOutTheVictim(t *testing.T) {
	th, _ := newTestThrottle()

	for i := 0; i < maxFailures*2; i++ {
		th.note("203.0.113.9", "victim@example.com") // attacker
	}
	if !th.blocked("203.0.113.9", "victim@example.com") {
		t.Fatal("the attacker should be blocked")
	}
	if th.blocked("10.0.0.5", "victim@example.com") {
		t.Fatal("the victim's own address was locked out by someone else's guessing")
	}
}

// And an attacker cannot evade the limit by rotating usernames from one host
// — each identifier is counted separately, but the per-account limit still
// bites, which is what protects any single password.
func TestThrottle_PerIdentifierCountsAreIndependent(t *testing.T) {
	th, _ := newTestThrottle()

	for i := 0; i < maxFailures; i++ {
		th.note("203.0.113.9", "alice@example.com")
	}
	if !th.blocked("203.0.113.9", "alice@example.com") {
		t.Fatal("alice's bucket should be exhausted")
	}
	if th.blocked("203.0.113.9", "bob@example.com") {
		t.Fatal("bob's bucket should be independent")
	}
}

// The map is attacker-growable — one entry per distinct (ip, identifier) — so
// entries whose failures have aged out must be dropped.
func TestThrottle_SweepsExpiredEntries(t *testing.T) {
	th, advance := newTestThrottle()

	for i := 0; i < 100; i++ {
		th.note("203.0.113.9", string(rune('a'+i%26))+"@example.com")
	}
	if len(th.failures) == 0 {
		t.Fatal("nothing recorded")
	}

	advance(failureWindow + sweepInterval + time.Second)
	th.note("10.0.0.1", "trigger-sweep@example.com")

	if len(th.failures) > 1 {
		t.Fatalf("expired entries were not swept: %d remain", len(th.failures))
	}
}

// The forwarded chain is honored only when the app's TrustedProxy settings
// name the header (see ratelimit_spoof_test.go for why); a nil app or an
// unconfigured one keys strictly on the socket peer. Under the hosting
// router the tenant's settings always carry X-Forwarded-For (materialized
// into app.json), so unix-socket requests — whose RemoteAddr identifies
// nobody — still get per-client buckets.
func TestClientIP_TrustSwitch(t *testing.T) {
	trusted, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer trusted.Cleanup()
	trusted.Settings().TrustedProxy.Headers = []string{"X-Forwarded-For"}

	cases := []struct {
		name       string
		app        core.App
		remoteAddr string
		xff        string
		want       string
	}{
		{"direct connection", nil, "192.0.2.7:51234", "", "192.0.2.7"},
		{"direct connection ignores spoofed header", nil, "192.0.2.7:51234", "203.0.113.9", "192.0.2.7"},
		{"trusted proxy reads the header", trusted, "", "203.0.113.9", "203.0.113.9"},
		{"trusted proxy uses the proxy-appended (rightmost) entry", trusted, "", "10.9.9.9, 203.0.113.9", "203.0.113.9"},
		{"trusted chain with spaces", trusted, "", " 10.9.9.9 , 203.0.113.9 ", "203.0.113.9"},
		{"unix socket, no chain", trusted, "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := http.NewRequest(http.MethodGet, "/carddav/", nil)
			if err != nil {
				t.Fatal(err)
			}
			r.RemoteAddr = tc.remoteAddr
			if tc.xff != "" {
				r.Header.Set("X-Forwarded-For", tc.xff)
			}
			if got := clientIP(tc.app, r); got != tc.want {
				t.Errorf("clientIP() = %q, want %q", got, tc.want)
			}
		})
	}
}
