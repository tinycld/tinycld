// Package sharequota is core's policy seam for per-share-link download
// ceilings, and the hook that lets a package charge against them.
//
// A public share link is an open invitation: anyone holding the URL may fetch
// the bytes, forever, as many times as they like. That is the feature. It is
// also how one link becomes a file-distribution host — and the package that
// owns the link cannot decide how much is too much, because "too much" is a
// property of the deployment, not of the file.
//
// So the shape is the one quota already uses, for the same reason: core
// defines the ceiling, whoever composed the app supplies the value, and a
// deployment that supplies none gets unlimited. Core names no package and no
// collection; a package says which of its collections to meter and how to
// charge, and core binds the hook.
package sharequota

import (
	"fmt"
	"sync"

	"github.com/pocketbase/pocketbase/core"
)

// ShareLimits are the per-link download ceilings.
//
// Zero means UNLIMITED, matching quota.Limits — deliberately, so an operator
// who has learned one convention does not have to learn a second. A negative
// value is nonsense and is treated as zero by the resolver's caller.
type ShareLimits struct {
	// Lifetime caps the downloads one link may ever serve. A link that
	// reaches it is finished: nothing resets, and the owner must mint a new
	// link. That is a blunt instrument — see PerDay.
	Lifetime int

	// PerDay caps the downloads one link may serve within a UTC calendar day,
	// and is the better abuse control of the two. It throttles a link that is
	// being redistributed without killing one that is merely popular, and it
	// heals itself at midnight without anyone intervening.
	PerDay int
}

// ShareLimitsFunc resolves the current ceilings.
//
// Called on the download path, so it must be cheap — a struct read, not a
// query. The default returns the zero value, which is unlimited.
type ShareLimitsFunc func(app core.App) ShareLimits

// Meter charges one file-download request against whatever per-link counters
// the registering package keeps.
//
// Call e.Next() to serve the file, or return an error to REFUSE it: the file
// route calls fsys.Serve INSIDE this callback, so an error returned before
// e.Next() means nothing was written.
//
// What a download is charged to is entirely the package's business. Core does
// not know that a share token exists, let alone which header carries one.
type Meter func(app core.App, e *core.FileDownloadRequestEvent) error

// MeteredCollection is a package's declaration that files in one of its
// collections are served to the public and should be charged.
type MeteredCollection struct {
	// Slug is the owning package, for log context only.
	Slug string

	// Collection is the collection whose files are metered. It is used as the
	// hook's TAG, so core never compares it to anything — the tag is the
	// dispatch.
	Collection string

	// Meter charges the download. Required.
	Meter Meter
}

var (
	mu       sync.RWMutex
	limits   ShareLimitsFunc
	claimed  bool
	metered  []MeteredCollection
	byColumn = map[string]bool{}
)

// SetLimits installs the process-wide ceiling resolver. A supervising
// composition calls it BEFORE any package registers, so a package's first read
// already resolves through it.
//
// This CLAIMS the seam: a later call is ignored, so core's own wiring cannot
// point these ceilings back at something the deployment controls. The reason
// is the one syscfg states — every deployment has its own superusers, so a
// ceiling stored inside the deployment is a ceiling that deployment can
// raise, and a ceiling you can raise is not a ceiling.
//
// A nil resolver is ignored rather than installed: it would silently mean
// unlimited, and a caller passing nil has a bug rather than an intent.
func SetLimits(fn ShareLimitsFunc) {
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
func IsClaimed() bool {
	mu.RLock()
	defer mu.RUnlock()
	return claimed
}

// Limits resolves the current ceilings, or unlimited when nobody claimed the
// seam. Negative values are normalized to zero (unlimited) here rather than at
// each call site: a negative ceiling reaching an enforcement expression would
// refuse every download. Whoever supplies the ceilings may validate them,
// but this must not depend on their having done so.
func Limits(app core.App) ShareLimits {
	mu.RLock()
	fn := limits
	mu.RUnlock()

	if fn == nil {
		return ShareLimits{}
	}
	l := fn(app)
	if l.Lifetime < 0 {
		l.Lifetime = 0
	}
	if l.PerDay < 0 {
		l.PerDay = 0
	}
	return l
}

// RegisterMetered records that a package's collection should be metered.
// Called from the package's Register at startup, before BindMeters.
//
// Idempotent per collection: registering the same one twice keeps the first,
// matching quota.RegisterSources, so a package registered by two compositions
// does not bind two hooks and charge every download twice.
func RegisterMetered(cols ...MeteredCollection) error {
	mu.Lock()
	defer mu.Unlock()

	for _, c := range cols {
		if c.Collection == "" {
			return fmt.Errorf("sharequota: a metered collection needs a name")
		}
		if c.Meter == nil {
			return fmt.Errorf("sharequota: %s has no meter", c.Collection)
		}
		if byColumn[c.Collection] {
			continue
		}
		byColumn[c.Collection] = true
		metered = append(metered, c)
	}
	return nil
}

// RegisteredMetered returns the registered collections.
func RegisteredMetered() []MeteredCollection {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]MeteredCollection, len(metered))
	copy(out, metered)
	return out
}

// ResetForTesting restores the zero state: no resolver, no claim, nothing
// metered. A test that installs either must call it (t.Cleanup) so the claim
// does not leak into the next test and make SetLimits inert there.
func ResetForTesting() {
	mu.Lock()
	limits, claimed = nil, false
	metered, byColumn = nil, map[string]bool{}
	mu.Unlock()
}
