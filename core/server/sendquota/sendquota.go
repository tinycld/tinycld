// Package sendquota is core's policy seam for outbound mail ceilings.
//
// Two ceilings, and the second is the reason this package exists rather than
// a single number in the deployment's own settings.
//
// PerDay is the ordinary volume limit: how much mail a deployment may send in
// a normal day.
//
// PerHour is the THROTTLE. When outbound mail starts generating bounces or
// spam complaints at a rate that threatens the sending reputation everyone
// else shares, whoever is watching drops this to a trickle and leaves it
// there pending a human review. That is deliberately not a suspension: a
// throttled deployment keeps receiving mail, keeps its data, keeps working,
// and can still send the handful of replies a real conversation needs. It
// simply cannot blast.
//
// Both are resolved through a seam rather than read from the deployment's own
// settings, for the reason quota and sharequota both state: every deployment
// has its own superusers, so a ceiling stored inside one is a ceiling that
// deployment can raise — and a throttle its subject can lift is not a
// throttle.
package sendquota

import (
	"sync"

	"github.com/pocketbase/pocketbase/core"
)

// SendLimits are the outbound ceilings. Zero means UNLIMITED in both fields,
// matching quota.Limits and sharequota.ShareLimits — one convention across
// every ceiling in the ecosystem.
type SendLimits struct {
	// PerDay bounds ordinary volume over a UTC calendar day.
	PerDay int

	// PerHour bounds a rolling hour and is normally zero. It is set when a
	// deployment is under review, so that a send run in progress stops within
	// the hour rather than at midnight.
	PerHour int
}

// SendLimitsFunc resolves the current ceilings. Called on the send path, so
// it must be cheap — a struct read, not a query.
type SendLimitsFunc func(app core.App) SendLimits

var (
	mu      sync.RWMutex
	limits  SendLimitsFunc
	claimed bool
)

// SetLimits installs the process-wide resolver. A supervising composition
// calls it before any package registers.
//
// This CLAIMS the seam: a later call is ignored, so nothing can point these
// ceilings back at a value the deployment controls. That matters more here
// than for a storage ceiling — the throttle exists precisely for a deployment
// that is behaving badly, and one that could raise its own limit would simply
// do so.
//
// A nil resolver is ignored rather than installed: it would silently mean
// unlimited, and a caller passing nil has a bug rather than an intent.
func SetLimits(fn SendLimitsFunc) {
	if fn == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if claimed {
		return
	}
	limits = fn
	claimed = true
}

// IsClaimed reports whether a supervising composition installed a resolver.
// A standalone deployment's own wiring consults this before falling back to
// what that deployment configured for itself.
func IsClaimed() bool {
	mu.RLock()
	defer mu.RUnlock()
	return claimed
}

// Limits resolves the current ceilings, or unlimited when nobody claimed the
// seam. A negative value is normalized to zero here rather than at the call
// site: a negative ceiling reaching an enforcement comparison would refuse
// every send, and that must not depend on whoever supplies the numbers having
// validated them.
func Limits(app core.App) SendLimits {
	mu.RLock()
	fn := limits
	mu.RUnlock()

	if fn == nil {
		return SendLimits{}
	}
	l := fn(app)
	if l.PerDay < 0 {
		l.PerDay = 0
	}
	if l.PerHour < 0 {
		l.PerHour = 0
	}
	return l
}

// ResetForTesting restores the zero state: no resolver, no claim. A test that
// installs one must call it (t.Cleanup) so the claim does not leak into the
// next test and make SetLimits inert there.
func ResetForTesting() {
	mu.Lock()
	limits, claimed = nil, false
	mu.Unlock()
}
