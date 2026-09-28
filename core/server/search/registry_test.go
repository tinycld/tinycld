package search

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/oauth"
)

func fakeSource(slug string, order int, scopes ...string) Source {
	return Source{
		Slug: slug, Label: slug, Order: order, Scopes: scopes,
		Search: func(core.App, string, Query) (Result, error) { return Result{}, nil },
	}
}

func slugsOf(sources []Source) []string {
	out := make([]string, len(sources))
	for i, s := range sources {
		out[i] = s.Slug
	}
	return out
}

func equalSlugs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRegisterSourcesIsIdempotentPerSlug(t *testing.T) {
	// A dev reload re-runs every package's Register; double-counting a source
	// would double every row it contributes.
	ResetSources()
	t.Cleanup(ResetSources)

	RegisterSources(fakeSource("gizmos", 5))
	RegisterSources(fakeSource("gizmos", 5))
	if got := len(RegisteredSources()); got != 1 {
		t.Fatalf("registered %d sources, want 1", got)
	}
}

func TestRegisterSourcesReplacesOnReregistration(t *testing.T) {
	// Re-registration must take the NEW value: a reload that changed a label or
	// order should not leave the stale one in place.
	ResetSources()
	t.Cleanup(ResetSources)

	RegisterSources(fakeSource("gizmos", 5))
	updated := fakeSource("gizmos", 5)
	updated.Label = "Email"
	RegisterSources(updated)

	sources := RegisteredSources()
	if len(sources) != 1 || sources[0].Label != "Email" {
		t.Fatalf("sources = %+v, want one labelled Email", sources)
	}
}

func TestRegisterSourcesRejectsUnusableSources(t *testing.T) {
	// A source with no slug cannot label rows; one with no Search cannot make
	// them. Keeping either would surface later as a mysteriously empty package.
	ResetSources()
	t.Cleanup(ResetSources)

	RegisterSources(Source{Slug: "", Search: func(core.App, string, Query) (Result, error) {
		return Result{}, nil
	}})
	RegisterSources(Source{Slug: "gadgets"}) // no Search
	if got := len(RegisteredSources()); got != 0 {
		t.Fatalf("registered %d unusable sources, want 0", got)
	}
}

func TestRegisteredSourcesOrdersByOrderThenSlug(t *testing.T) {
	// Ordering must not depend on registration order, which is generator
	// output and can change without anyone intending it to.
	ResetSources()
	t.Cleanup(ResetSources)

	RegisterSources(fakeSource("gadgets", 25), fakeSource("gizmos", 5), fakeSource("cogs", 12))
	RegisterSources(fakeSource("doodads", 5)) // ties with gizmos on order

	want := []string{"doodads", "gizmos", "cogs", "gadgets"}
	if got := slugsOf(RegisteredSources()); !equalSlugs(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestSelectSourcesFiltersByRequestedSlugs(t *testing.T) {
	all := []Source{fakeSource("gizmos", 5), fakeSource("cogs", 12), fakeSource("gadgets", 25)}

	if got := slugsOf(selectSources(all, nil, nil)); !equalSlugs(got, []string{"gizmos", "cogs", "gadgets"}) {
		t.Errorf("no slugs should search everything, got %v", got)
	}
	if got := slugsOf(selectSources(all, []string{"cogs"}, nil)); !equalSlugs(got, []string{"cogs"}) {
		t.Errorf("selected = %v, want [cogs]", got)
	}
	if got := selectSources(all, []string{"nonexistent"}, nil); len(got) != 0 {
		t.Errorf("an unknown slug should select nothing, got %v", slugsOf(got))
	}
}

func TestSelectSourcesFiltersByGrantedScopes(t *testing.T) {
	// The reason scope filtering lives here rather than in the route table: a
	// gizmos-only token searching everything must get gizmos rows, not a blanket
	// 403 that tells it nothing is searchable.
	all := []Source{
		fakeSource("gizmos", 5, "gizmos:read"),
		fakeSource("cogs", 12, "cogs:read"),
		fakeSource("gadgets", 25, "gadgets:read"),
	}

	got := slugsOf(selectSources(all, nil, []string{"gizmos:read"}))
	if !equalSlugs(got, []string{"gizmos"}) {
		t.Fatalf("gizmos:read token got %v, want [gizmos]", got)
	}

	// A session (nil scopes) has no ceiling and sees everything.
	if got := slugsOf(selectSources(all, nil, nil)); len(got) != 3 {
		t.Fatalf("session got %v, want all three", got)
	}

	// An empty non-nil slice is a token that granted nothing — distinct from a
	// session, and it must see nothing rather than everything.
	if got := selectSources(all, nil, []string{}); len(got) != 0 {
		t.Fatalf("a token with no scopes got %v, want none", slugsOf(got))
	}
}

func TestSelectSourcesDeniesScopelessSourceToTokens(t *testing.T) {
	// A source declaring no scopes is session-only. A bearer must be
	// explicitly permitted — never permitted because nobody classified it.
	all := []Source{fakeSource("internal", 1)}
	if got := selectSources(all, nil, []string{"gizmos:read", "cogs:read"}); len(got) != 0 {
		t.Fatalf("scopeless source reachable by token: %v", slugsOf(got))
	}
	if got := selectSources(all, nil, nil); len(got) != 1 {
		t.Fatal("scopeless source must stay reachable by a session")
	}
}

// The federated route admits any scope that permits searching some registered
// source, and nothing else: a write-only or profile-only grant has nothing to
// read. Sources register from packages in no particular order, so the rule is
// derived at request time rather than fixed when the route is bound.
func TestFederatedSearchScopeRuleFollowsSources(t *testing.T) {
	ResetSources()
	t.Cleanup(ResetSources)
	oauth.ResetRegistry()
	t.Cleanup(oauth.ResetRegistry)
	oauth.RegisterSharedEndpoint("GET", "/api/search", searchScopes)

	if rule := oauth.ScopeForRoute("GET", "/api/search"); len(rule) != 0 {
		t.Fatalf("with no sources the route must default-deny, got %v", rule)
	}

	noop := func(core.App, string, Query) (Result, error) { return Result{}, nil }
	RegisterSources(
		Source{Slug: "notes", Scopes: []string{"notes:read"}, Search: noop},
		Source{Slug: "widgets", Scopes: []string{"widgets:read"}, Search: noop},
	)
	rule := oauth.ScopeForRoute("GET", "/api/search")
	for _, scope := range []string{"notes:read", "widgets:read"} {
		if !rule.SatisfiedBy([]string{scope}) {
			t.Errorf("a token holding only %q cannot reach /api/search (rule %v)", scope, rule)
		}
	}
	for _, scope := range []string{oauth.ScopeProfile, "notes:write"} {
		if rule.SatisfiedBy([]string{scope}) {
			t.Errorf("%q alone must not admit a search", scope)
		}
	}
}
