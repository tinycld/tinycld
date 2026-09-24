// Package outboundstats lets a package report how much mail this deployment
// has sent, without knowing who is asking or why.
//
// Something outside a deployment may need to judge its sending — a rate needs
// a denominator, and only the deployment knows how many messages it actually
// handed to a provider. But a package that reports the number must not learn
// what is decided with it, or the reporting and the judging become one thing
// and the package ends up knowing about a supervisor it should not.
//
// So this is a registry of counters and nothing else. A package registers a
// function that counts its own sends over a window; whoever composed the app
// collects them. Core names no package, and a package names no consumer.
package outboundstats

import (
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// Counter reports how many messages the registering package handed to an
// outbound provider at or after `since`.
//
// It counts MESSAGES, not recipients. A rate compared against a provider's
// own account limits has to use the same denominator that provider does, and
// providers count messages. One message to a hundred recipients is one send
// here and can still produce a hundred failures, which means a computed rate
// may exceed 1 — that is correct, and it makes wide fan-out trip sooner,
// which is the desired behaviour rather than an artefact to normalise away.
//
// Called from a supervisor's poll, so it should be one indexed query.
type Counter func(app core.App, since time.Time) (int, error)

type registration struct {
	slug    string
	counter Counter
}

var (
	mu       sync.RWMutex
	counters []registration
	bySlug   = map[string]bool{}
)

// Register adds a counter. Called from a package's own Register at startup.
//
// Idempotent per slug: registering twice keeps the first, so a package
// registered by two compositions does not have its sends counted twice.
func Register(slug string, c Counter) {
	if slug == "" || c == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if bySlug[slug] {
		return
	}
	bySlug[slug] = true
	counters = append(counters, registration{slug: slug, counter: c})
}

// Registered reports which packages have registered a counter.
func Registered() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(counters))
	for _, r := range counters {
		out = append(out, r.slug)
	}
	return out
}

// Total sums every registered counter over the window.
//
// A failing counter does not fail the total. It contributes nothing and is
// reported in `partial`, because the difference between "this deployment sent
// nothing" and "this deployment could not say" is the difference between a
// rate of zero and no rate at all — and a caller that cannot tell them apart
// will compute a rate of infinity from a numerator over a denominator that
// was never really zero.
func Total(app core.App, since time.Time) (total int, partial bool, errs []error) {
	mu.RLock()
	snapshot := make([]registration, len(counters))
	copy(snapshot, counters)
	mu.RUnlock()

	for _, r := range snapshot {
		n, err := r.counter(app, since)
		if err != nil {
			partial = true
			errs = append(errs, err)
			continue
		}
		total += n
	}
	return total, partial, errs
}

// ResetForTesting clears the registry. A test that registers a counter must
// call it (t.Cleanup) so the registration does not leak into the next test.
func ResetForTesting() {
	mu.Lock()
	counters, bySlug = nil, map[string]bool{}
	mu.Unlock()
}
