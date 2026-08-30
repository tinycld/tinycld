package davauth

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// newAuthApp builds a test app whose users collection carries the `disabled`
// flag the real schema has (NewTestApp ships the stock collection without it).
func newAuthApp(t testing.TB) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(app.Cleanup)

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users: %v", err)
	}
	users.Fields.Add(&core.BoolField{Name: "disabled"})
	if err := app.Save(users); err != nil {
		t.Fatalf("add disabled field: %v", err)
	}
	return app
}

func newAuthUser(t testing.TB, app core.App, email, password string, disabled bool) *core.Record {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users: %v", err)
	}
	u := core.NewRecord(users)
	u.Set("email", email)
	u.Set("password", password)
	u.Set("disabled", disabled)
	if err := app.Save(u); err != nil {
		t.Fatalf("save user: %v", err)
	}
	return u
}

func basicAuthRequest(t testing.TB, identifier, password string) *http.Request {
	t.Helper()
	r, err := http.NewRequest(http.MethodGet, "/carddav/", nil)
	if err != nil {
		t.Fatal(err)
	}
	r.SetBasicAuth(identifier, password)
	return r
}

func TestAuthenticate_EnabledUserSucceeds(t *testing.T) {
	app := newAuthApp(t)
	u := newAuthUser(t, app, "alice@example.com", "Password123!", false)

	got, err := Authenticate(app, basicAuthRequest(t, "alice@example.com", "Password123!"))
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if got.Id != u.Id {
		t.Fatalf("authenticated as %q, want %q", got.Id, u.Id)
	}
}

func TestAuthenticate_DisabledUserIsRefused(t *testing.T) {
	app := newAuthApp(t)
	newAuthUser(t, app, "mallory@example.com", "Password123!", true)

	// DAV is Basic-per-request: there is no token to revoke, so the disabled
	// flag is the ONLY thing standing between a suspended account and its
	// CardDAV/CalDAV/WebDAV data.
	got, err := Authenticate(app, basicAuthRequest(t, "mallory@example.com", "Password123!"))
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("disabled user authenticated (record=%v, err=%v), want ErrUnauthorized", got, err)
	}
}

func TestAuthenticate_WrongPasswordIsRefused(t *testing.T) {
	app := newAuthApp(t)
	newAuthUser(t, app, "alice@example.com", "Password123!", false)

	if _, err := Authenticate(app, basicAuthRequest(t, "alice@example.com", "wrong-password")); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("wrong password accepted, err=%v, want ErrUnauthorized", err)
	}
}

// Authenticate short-circuits when no user matches the identifier, so a miss
// returns without ever running bcrypt while a hit pays the full KDF cost. The
// gap is large — bcrypt is deliberately slow — and measurable over the network,
// which turns the endpoint into a username oracle: an attacker learns which
// accounts exist without ever guessing a password.
//
// The fix is to verify against a dummy hash on the miss path so both answers
// cost the same. This test measures the ratio rather than an absolute, so it
// does not depend on the machine's speed.
func TestAuthenticate_MissAndHitCostTheSame(t *testing.T) {
	app := newAuthApp(t)
	newAuthUser(t, app, "real@example.com", "Password123!", false)

	const rounds = 6

	measure := func(identifier, password string) time.Duration {
		var total time.Duration
		for i := 0; i < rounds; i++ {
			start := time.Now()
			_, _ = Authenticate(app, basicAuthRequest(t, identifier, password))
			total += time.Since(start)
		}
		return total / rounds
	}

	// Wrong password on a real account: pays for bcrypt.
	hit := measure("real@example.com", "wrong-password")
	// No such account: must also pay for bcrypt, or the difference is an oracle.
	miss := measure("nosuchuser@example.com", "wrong-password")

	if hit <= 0 {
		t.Skip("timer resolution too coarse to measure")
	}
	ratio := float64(hit) / float64(miss)
	// A miss that skips bcrypt entirely runs orders of magnitude faster. 8x is
	// far above any scheduling noise while still catching the real bug.
	if ratio > 8 {
		t.Fatalf("a failed lookup is %.0fx faster than a failed password "+
			"(hit=%v miss=%v): the miss path skips bcrypt and leaks which "+
			"accounts exist", ratio, hit, miss)
	}
}

// A disabled user is refused, but the refusal must not be free either: the
// disabled check runs after ValidatePassword, so it already costs the same as
// any other wrong-credential answer. Pinned so a future reordering — checking
// `disabled` before the password, which looks like an optimization — does not
// reintroduce an oracle for "is this account suspended".
func TestAuthenticate_DisabledCostsTheSameAsWrongPassword(t *testing.T) {
	app := newAuthApp(t)
	newAuthUser(t, app, "enabled@example.com", "Password123!", false)
	newAuthUser(t, app, "suspended@example.com", "Password123!", true)

	const rounds = 6
	measure := func(identifier, password string) time.Duration {
		var total time.Duration
		for i := 0; i < rounds; i++ {
			start := time.Now()
			_, _ = Authenticate(app, basicAuthRequest(t, identifier, password))
			total += time.Since(start)
		}
		return total / rounds
	}

	wrongPass := measure("enabled@example.com", "wrong-password")
	disabled := measure("suspended@example.com", "Password123!")

	if wrongPass <= 0 || disabled <= 0 {
		t.Skip("timer resolution too coarse to measure")
	}
	ratio := float64(wrongPass) / float64(disabled)
	if ratio > 8 || ratio < 0.125 {
		t.Fatalf("a disabled-account refusal is distinguishable by timing "+
			"from a wrong password (wrongPass=%v disabled=%v)", wrongPass, disabled)
	}
}

// bcrypt is deliberately expensive, and CardDAV re-authenticates inside every
// backend method — one PROPFIND drives several, so a single request paid the
// KDF cost repeatedly. An unauthenticated client could make the server do
// arbitrary bcrypt work just by sending a request that fans out.
//
// WithRequestCache settles the answer once per request. Measured rather than
// asserted structurally: the point is the COST, so the test times it.
func TestWithRequestCache_VerifiesOncePerRequest(t *testing.T) {
	app := newAuthApp(t)
	newAuthUser(t, app, "alice@example.com", "Password123!", false)

	const calls = 8

	uncachedReq := basicAuthRequest(t, "alice@example.com", "Password123!")
	start := time.Now()
	for i := 0; i < calls; i++ {
		if _, err := Authenticate(app, uncachedReq); err != nil {
			t.Fatal(err)
		}
	}
	uncached := time.Since(start)

	cachedReq := WithRequestCache(basicAuthRequest(t, "alice@example.com", "Password123!"))
	start = time.Now()
	for i := 0; i < calls; i++ {
		if _, err := Authenticate(app, cachedReq); err != nil {
			t.Fatal(err)
		}
	}
	cached := time.Since(start)

	if cached <= 0 || uncached <= 0 {
		t.Skip("timer resolution too coarse to measure")
	}
	// N calls should cost about one verification, not N.
	if ratio := float64(uncached) / float64(cached); ratio < float64(calls)/2 {
		t.Fatalf("%d cached calls cost %v vs %v uncached (%.1fx): "+
			"the request cache is not preventing repeated bcrypt", calls, cached, uncached, ratio)
	}
}

// A failed authentication must be cached too, or a request that fans out still
// pays per-call bcrypt for the wrong password — the amplification this closes.
func TestWithRequestCache_CachesFailuresToo(t *testing.T) {
	app := newAuthApp(t)
	newAuthUser(t, app, "alice@example.com", "Password123!", false)

	const calls = 8

	// Measured as a RATIO against the same calls uncached, never as an
	// absolute duration. bcrypt's cost is a property of the build, not of
	// this code: under -race a single verification runs several times
	// slower and blows through any fixed millisecond bound, failing a
	// cache that is working perfectly. Dividing by an uncached baseline
	// measured on the same machine cancels that out, and it is what the
	// success-path test above already does.
	req := WithRequestCache(basicAuthRequest(t, "alice@example.com", "wrong-password"))
	start := time.Now()
	for i := range calls {
		if _, err := Authenticate(app, req); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("cached call %d: err = %v, want ErrUnauthorized", i, err)
		}
	}
	cached := time.Since(start)

	// A fresh request per call, so each one must pay its own verification.
	start = time.Now()
	for i := range calls {
		fresh := WithRequestCache(basicAuthRequest(t, "alice@example.com", "wrong-password"))
		if _, err := Authenticate(app, fresh); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("uncached call %d: err = %v, want ErrUnauthorized", i, err)
		}
	}
	uncached := time.Since(start)

	if cached <= 0 || uncached <= 0 {
		t.Skip("timer resolution too coarse to measure")
	}
	// N cached calls should cost about ONE verification, not N. Half the
	// ideal ratio leaves room for scheduling noise while still failing
	// decisively if every call is paying its own bcrypt.
	if ratio := float64(uncached) / float64(cached); ratio < float64(calls)/2 {
		t.Fatalf("%d failed cached calls cost %v vs %v uncached (%.1fx): "+
			"failures are not being cached", calls, cached, uncached, ratio)
	}
}

// The cache must not outlive its request: a second request re-verifies, so a
// changed or revoked password takes effect immediately.
func TestWithRequestCache_DoesNotLeakAcrossRequests(t *testing.T) {
	app := newAuthApp(t)
	user := newAuthUser(t, app, "alice@example.com", "Password123!", false)

	first := WithRequestCache(basicAuthRequest(t, "alice@example.com", "Password123!"))
	if _, err := Authenticate(app, first); err != nil {
		t.Fatal(err)
	}

	// Suspend the account, then authenticate on a NEW request.
	fresh, err := app.FindRecordById("users", user.Id)
	if err != nil {
		t.Fatal(err)
	}
	fresh.Set("disabled", true)
	if err := app.Save(fresh); err != nil {
		t.Fatal(err)
	}

	second := WithRequestCache(basicAuthRequest(t, "alice@example.com", "Password123!"))
	if _, err := Authenticate(app, second); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("a new request reused a stale cached success: err = %v", err)
	}
}
