package davauth

import (
	"fmt"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

// ratelimit_spoof_test.go pins WHO the throttle believes about the client's
// address. X-Forwarded-For is client-writable on a direct connection, so
// trusting it unconditionally gave an attacker two moves:
//
//   - rotate the header per request → a fresh bucket every time → the
//     throttle never trips (full bypass);
//   - set it to a victim's egress IP → fill the victim's (ip, identifier)
//     bucket remotely → a refreshable targeted lockout of their DAV sync.
//
// The header must count only when the app's TrustedProxy settings name it —
// the same trust switch PocketBase's own RealIP honors, which the hosting
// router materializes for every tenant and a standalone operator sets when
// they deploy behind a proxy.

// A direct attacker rotating XFF must stay in the RemoteAddr bucket.
func TestSpoofedXFF_DoesNotEvadeThrottle(t *testing.T) {
	swapThrottle(t)

	for i := 0; i < maxFailures; i++ {
		r := davRequest(t, "203.0.113.9", "victim", "wrong")
		r.Header.Set("X-Forwarded-For", fmt.Sprintf("10.0.0.%d", i))
		NoteFailure(nil, r)
	}

	r := davRequest(t, "203.0.113.9", "victim", "wrong")
	r.Header.Set("X-Forwarded-For", "10.0.0.250")
	if !TooManyFailures(nil, r) {
		t.Fatal("rotating X-Forwarded-For evaded the throttle — unlimited guessing from one host")
	}
}

// A direct attacker naming the victim's IP must not poison the victim's bucket.
func TestSpoofedXFF_CannotLockOutVictim(t *testing.T) {
	swapThrottle(t)

	for i := 0; i < maxFailures*2; i++ {
		r := davRequest(t, "203.0.113.9", "victim", "wrong")
		r.Header.Set("X-Forwarded-For", "198.51.100.7") // the victim's egress IP
		NoteFailure(nil, r)
	}

	victim := davRequest(t, "198.51.100.7", "victim", "right-password")
	if TooManyFailures(nil, victim) {
		t.Fatal("an attacker filled the victim's bucket by spoofing their IP")
	}
}

// Behind a configured trusted proxy the forwarded header IS the client: two
// clients sharing the proxy's RemoteAddr must land in distinct buckets, keyed
// by the RIGHTMOST forwarded entry (the one the proxy appended — leftmost
// values are whatever the client sent).
func TestTrustedProxy_UsesForwardedClient(t *testing.T) {
	swapThrottle(t)

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()
	app.Settings().TrustedProxy.Headers = []string{"X-Forwarded-For"}

	for i := 0; i < maxFailures; i++ {
		r := davRequest(t, "127.0.0.1", "alice", "wrong")
		r.Header.Set("X-Forwarded-For", "198.51.100.7")
		NoteFailure(app, r)
	}

	blocked := davRequest(t, "127.0.0.1", "alice", "wrong")
	blocked.Header.Set("X-Forwarded-For", "198.51.100.7")
	if !TooManyFailures(app, blocked) {
		t.Fatal("the guessing client behind the proxy should be throttled")
	}

	other := davRequest(t, "127.0.0.1", "alice", "wrong")
	other.Header.Set("X-Forwarded-For", "203.0.113.44")
	if TooManyFailures(app, other) {
		t.Fatal("an unrelated client behind the same proxy shares the attacker's bucket")
	}

	// The rightmost entry is the proxy's word; leftmost values are whatever
	// the client itself sent. The throttled client prepending a fresh IP
	// (proxy appends the real one) must stay in its bucket.
	prepended := davRequest(t, "127.0.0.1", "alice", "wrong")
	prepended.Header.Set("X-Forwarded-For", "10.9.9.9, 198.51.100.7")
	if !TooManyFailures(app, prepended) {
		t.Fatal("client-prepended forwarded entry evaded the throttle — the bucket must key on the proxy-appended (rightmost) entry")
	}
}
