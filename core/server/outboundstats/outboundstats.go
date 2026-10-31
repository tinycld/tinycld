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

// Registry holds counters. The package-level functions use one shared default
// for the whole process; a caller that must not share it (a test, or code
// composing several apps in one process) builds its own with NewRegistry.
type Registry struct {
	mu       sync.RWMutex
	counters []registration
	bySlug   map[string]bool
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{bySlug: map[string]bool{}}
}

var defaultRegistry = NewRegistry()

// Default returns the process-wide registry the package-level functions use.
func Default() *Registry { return defaultRegistry }

// Register adds a counter. Called from a package's own Register at startup.
//
// Idempotent per slug: registering twice keeps the first, so a package
// registered by two compositions does not have its sends counted twice.
func (r *Registry) Register(slug string, c Counter) {
	if slug == "" || c == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.bySlug[slug] {
		return
	}
	r.bySlug[slug] = true
	r.counters = append(r.counters, registration{slug: slug, counter: c})
}

// Registered reports which packages have registered a counter.
func (r *Registry) Registered() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.counters))
	for _, reg := range r.counters {
		out = append(out, reg.slug)
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
func (r *Registry) Total(app core.App, since time.Time) (total int, partial bool, errs []error) {
	r.mu.RLock()
	snapshot := make([]registration, len(r.counters))
	copy(snapshot, r.counters)
	r.mu.RUnlock()

	for _, reg := range snapshot {
		n, err := reg.counter(app, since)
		if err != nil {
			partial = true
			errs = append(errs, err)
			continue
		}
		total += n
	}
	return total, partial, errs
}

// Register adds a counter to the default registry.
func Register(slug string, c Counter) { defaultRegistry.Register(slug, c) }

// Registered reports which packages registered on the default registry.
func Registered() []string { return defaultRegistry.Registered() }

// Total sums every counter on the default registry over the window.
func Total(app core.App, since time.Time) (total int, partial bool, errs []error) {
	return defaultRegistry.Total(app, since)
}

// ResetForTesting clears the default registry. A test that registers a counter
// on it must call it (t.Cleanup) so the registration does not leak into the
// next test.
func ResetForTesting() {
	defaultRegistry.mu.Lock()
	defaultRegistry.counters, defaultRegistry.bySlug = nil, map[string]bool{}
	defaultRegistry.mu.Unlock()
}
