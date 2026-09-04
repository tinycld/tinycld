package search

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
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

	RegisterSources(fakeSource("mail", 5))
	RegisterSources(fakeSource("mail", 5))
	if got := len(RegisteredSources()); got != 1 {
		t.Fatalf("registered %d sources, want 1", got)
	}
}

func TestRegisterSourcesReplacesOnReregistration(t *testing.T) {
	// Re-registration must take the NEW value: a reload that changed a label or
	// order should not leave the stale one in place.
	ResetSources()
	t.Cleanup(ResetSources)

	RegisterSources(fakeSource("mail", 5))
	updated := fakeSource("mail", 5)
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
	RegisterSources(Source{Slug: "boards"}) // no Search
	if got := len(RegisteredSources()); got != 0 {
		t.Fatalf("registered %d unusable sources, want 0", got)
	}
}

func TestRegisteredSourcesOrdersByOrderThenSlug(t *testing.T) {
	// Ordering must not depend on registration order, which is generator
	// output and can change without anyone intending it to.
	ResetSources()
	t.Cleanup(ResetSources)

	RegisterSources(fakeSource("boards", 25), fakeSource("mail", 5), fakeSource("drive", 12))
	RegisterSources(fakeSource("contacts", 5)) // ties with mail on order

	want := []string{"contacts", "mail", "drive", "boards"}
	if got := slugsOf(RegisteredSources()); !equalSlugs(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestSelectSourcesFiltersByRequestedSlugs(t *testing.T) {
	all := []Source{fakeSource("mail", 5), fakeSource("drive", 12), fakeSource("boards", 25)}

	if got := slugsOf(selectSources(all, nil, nil)); !equalSlugs(got, []string{"mail", "drive", "boards"}) {
		t.Errorf("no slugs should search everything, got %v", got)
	}
	if got := slugsOf(selectSources(all, []string{"drive"}, nil)); !equalSlugs(got, []string{"drive"}) {
		t.Errorf("selected = %v, want [drive]", got)
	}
	if got := selectSources(all, []string{"nonexistent"}, nil); len(got) != 0 {
		t.Errorf("an unknown slug should select nothing, got %v", slugsOf(got))
	}
}

func TestSelectSourcesFiltersByGrantedScopes(t *testing.T) {
	// The reason scope filtering lives here rather than in the route table: a
	// mail-only token searching everything must get mail rows, not a blanket
	// 403 that tells it nothing is searchable.
	all := []Source{
		fakeSource("mail", 5, "mail:read"),
		fakeSource("drive", 12, "drive:read"),
		fakeSource("boards", 25, "boards:read"),
	}

	got := slugsOf(selectSources(all, nil, []string{"mail:read"}))
	if !equalSlugs(got, []string{"mail"}) {
		t.Fatalf("mail:read token got %v, want [mail]", got)
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
	if got := selectSources(all, nil, []string{"mail:read", "drive:read"}); len(got) != 0 {
		t.Fatalf("scopeless source reachable by token: %v", slugsOf(got))
	}
	if got := selectSources(all, nil, nil); len(got) != 1 {
		t.Fatal("scopeless source must stay reachable by a session")
	}
}
