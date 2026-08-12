package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestAllowsUpToTheLimitThenRefuses(t *testing.T) {
	l := New(3, time.Minute)

	for i := range 3 {
		if !l.Allow("ip") {
			t.Fatalf("event %d refused inside the limit", i+1)
		}
	}
	if l.Allow("ip") {
		t.Fatal("the fourth event was allowed past a limit of 3")
	}
}

// A refused event must not be recorded, or a caller hammering a locked-out key
// keeps pushing their own window forward and can never recover.
func TestRefusedEventsDoNotExtendTheLockout(t *testing.T) {
	l := New(1, 40*time.Millisecond)

	if !l.Allow("ip") {
		t.Fatal("first event refused")
	}
	for range 20 {
		l.Allow("ip")
	}

	time.Sleep(60 * time.Millisecond)

	if !l.Allow("ip") {
		t.Fatal("still locked out after the window elapsed — refusals were recorded")
	}
}

func TestWindowSlides(t *testing.T) {
	l := New(2, 40*time.Millisecond)

	if !l.Allow("ip") || !l.Allow("ip") {
		t.Fatal("the first two events should be allowed")
	}
	if l.Allow("ip") {
		t.Fatal("the third event inside the window should be refused")
	}

	time.Sleep(60 * time.Millisecond)

	if !l.Allow("ip") {
		t.Fatal("the window did not slide")
	}
}

func TestKeysAreIndependent(t *testing.T) {
	l := New(1, time.Minute)

	if !l.Allow("a") {
		t.Fatal("first key refused")
	}
	if !l.Allow("b") {
		t.Fatal("a second key was charged for the first key's budget")
	}
	if l.Allow("a") {
		t.Fatal("the first key's limit was not enforced")
	}
}

func TestResetClearsEveryKey(t *testing.T) {
	l := New(1, time.Minute)
	l.Allow("a")
	l.Allow("b")

	l.Reset()

	if !l.Allow("a") || !l.Allow("b") {
		t.Fatal("Reset did not clear the history")
	}
}

// The limiter is a package-level singleton behind an HTTP handler, so
// concurrent use is the normal case rather than an edge one. Run with -race.
func TestConcurrentAllowIsSafeAndExact(t *testing.T) {
	const limit = 50
	l := New(limit, time.Minute)

	var wg sync.WaitGroup
	allowed := make(chan bool, 200)
	for range 200 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed <- l.Allow("ip")
		}()
	}
	wg.Wait()
	close(allowed)

	n := 0
	for ok := range allowed {
		if ok {
			n++
		}
	}
	if n != limit {
		t.Fatalf("allowed %d of 200 concurrent events, want exactly %d", n, limit)
	}
}

func TestClientIPPrefersForwardedFor(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.7")

	if got := ClientIP(r); got != "203.0.113.7" {
		t.Fatalf("ClientIP = %q, want the forwarded address", got)
	}
}

func TestClientIPFallsBackToRemoteAddr(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"

	if got := ClientIP(r); got != "10.0.0.1:1234" {
		t.Fatalf("ClientIP = %q, want the remote address", got)
	}
}
