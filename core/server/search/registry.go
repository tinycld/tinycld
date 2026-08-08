package search

import (
	"sort"
	"sync"

	"github.com/pocketbase/pocketbase/core"
)

// Source is one package's contribution to the federated search.
//
// A registry rather than a generated list, for the reason quota's source
// registry documents: a package that is not installed contributes nothing
// without anyone maintaining a list, and the answer is only correct if it spans
// every installed package — which is exactly what the registry accumulates.
type Source struct {
	// Slug identifies the package and labels its rows.
	Slug string
	// Label is the human name for chips and group headers.
	Label string
	// Order mirrors the package's nav.order, and is the ranking tie-break so
	// ordering does not depend on which source answered first.
	Order int
	// Scopes are the OAuth scopes that permit searching this package. A caller
	// holding none of them does not see this source at all — the request still
	// succeeds with whatever else it may read, rather than failing wholesale.
	// Empty means a session-authenticated caller only.
	Scopes []string
	// Search runs the package's own query. It receives core.App (not the
	// concrete app type) so it stays callable from tests, matching fts.Search.
	//
	// Returning an error is safe: the aggregator isolates it, names the package
	// in Response.Partial, and returns everything else.
	Search func(app core.App, userID string, q Query) (Result, error)
}

// The source registry.
//
// A package registers from its own Register(app) — the same place it binds
// every other hook — and core reads the accumulated set when a search runs.
// Registration order does not matter: RegisteredSources sorts by Order.
var (
	registryMu sync.RWMutex
	registry   []Source
)

// RegisterSources adds packages' sources. Idempotent per slug so a dev reload
// that re-runs Register does not double-count a package.
func RegisterSources(sources ...Source) {
	registryMu.Lock()
	defer registryMu.Unlock()
	for _, src := range sources {
		if src.Slug == "" || src.Search == nil {
			// A source with no slug cannot label rows and one with no Search
			// cannot produce them; either is a wiring mistake, and silently
			// keeping it would surface later as an empty package.
			continue
		}
		replaced := false
		for i, existing := range registry {
			if existing.Slug == src.Slug {
				registry[i] = src
				replaced = true
				break
			}
		}
		if !replaced {
			registry = append(registry, src)
		}
	}
}

// RegisteredSources returns a snapshot ordered by Order then Slug, so the
// ranking tie-break and any UI listing are stable across restarts.
func RegisteredSources() []Source {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]Source, len(registry))
	copy(out, registry)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Slug < out[j].Slug
	})
	return out
}

// ResetSources clears the registry — for tests only.
func ResetSources() {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = nil
}

// selectSources narrows the registry to what this request may search: the
// requested slugs (empty means all) intersected with what the caller's scopes
// allow.
//
// grantedScopes nil means a session-authenticated caller, which is not scope
// limited — only OAuth bearers carry a scope ceiling.
func selectSources(all []Source, slugs []string, grantedScopes []string) []Source {
	wanted := map[string]bool{}
	for _, s := range slugs {
		wanted[s] = true
	}
	var out []Source
	for _, src := range all {
		if len(wanted) > 0 && !wanted[src.Slug] {
			continue
		}
		if grantedScopes != nil && !anyScope(src.Scopes, grantedScopes) {
			continue
		}
		out = append(out, src)
	}
	return out
}

// anyScope reports whether granted covers at least one of required. A source
// requiring no scopes is unreachable by a scoped token: a bearer must be
// explicitly permitted, never permitted by omission.
func anyScope(required, granted []string) bool {
	for _, want := range required {
		for _, have := range granted {
			if want == have {
				return true
			}
		}
	}
	return false
}
