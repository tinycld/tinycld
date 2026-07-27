package davauth

import (
	"net/http"
	"testing"
	"time"
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

// Under the multi-org router a tenant request arrives over a unix socket where
// RemoteAddr is empty. Without reading the forwarded chain every caller would
// share one bucket, so a single attacker would throttle the whole deployment.
func TestClientIP_PrefersForwardedChain(t *testing.T) {
	cases := []struct {
		name       string
		remoteAddr string
		xff        string
		want       string
	}{
		{"direct connection", "192.0.2.7:51234", "", "192.0.2.7"},
		{"behind a proxy", "", "203.0.113.9", "203.0.113.9"},
		{"proxy chain uses the original client", "", "203.0.113.9, 70.41.3.18", "203.0.113.9"},
		{"chain with spaces", "", "  203.0.113.9 , 70.41.3.18", "203.0.113.9"},
		{"unix socket, no chain", "", "", ""},
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
			if got := clientIP(r); got != tc.want {
				t.Errorf("clientIP() = %q, want %q", got, tc.want)
			}
		})
	}
}
