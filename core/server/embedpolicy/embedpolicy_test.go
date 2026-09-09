package embedpolicy

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The default is the whole point of the package: a page nobody claimed must
// not be framable. Every path through FrameAncestors has to land here rather
// than on an empty directive, which a browser ignores entirely.
func TestFrameAncestorsDefaultsToNone(t *testing.T) {
	ResetForTesting()

	if got := FrameAncestors(httptest.NewRequest("GET", "/a/anything", nil)); got != "'none'" {
		t.Errorf("FrameAncestors = %q, want 'none'", got)
	}
}

func TestFrameAncestorsJoinsAClaimedRequestsOrigins(t *testing.T) {
	ResetForTesting()
	defer ResetForTesting()

	Register(func(r *http.Request) []string {
		if r.URL.Path == "/p/x/tok" {
			return []string{"https://a.example.com", "https://b.example.com"}
		}
		return nil
	})

	got := FrameAncestors(httptest.NewRequest("GET", "/p/x/tok", nil))
	want := "https://a.example.com https://b.example.com"
	if got != want {
		t.Errorf("FrameAncestors = %q, want %q", got, want)
	}
}

// A resolver returning nil is saying "not mine", not "allow" — so an unclaimed
// path keeps the deny default even with resolvers registered.
func TestFrameAncestorsKeepsTheDefaultWhenEveryResolverDeclines(t *testing.T) {
	ResetForTesting()
	defer ResetForTesting()

	Register(func(_ *http.Request) []string { return nil })
	Register(func(_ *http.Request) []string { return nil })

	if got := FrameAncestors(httptest.NewRequest("GET", "/p/x/tok", nil)); got != "'none'" {
		t.Errorf("FrameAncestors = %q, want 'none'", got)
	}
}

// The first claim wins and later resolvers are not consulted. Two packages
// cannot own one URL, and asking the rest could only widen the first answer.
func TestFrameAncestorsStopsAtTheFirstClaim(t *testing.T) {
	ResetForTesting()
	defer ResetForTesting()

	Register(func(_ *http.Request) []string { return []string{"https://first.example.com"} })
	Register(func(t2 *http.Request) []string {
		t.Error("a second resolver was consulted after the request was claimed")
		return []string{"https://second.example.com"}
	})

	if got := FrameAncestors(httptest.NewRequest("GET", "/p/x/tok", nil)); got != "https://first.example.com" {
		t.Errorf("FrameAncestors = %q, want the first claim", got)
	}
}

// An empty slice is not a claim. A resolver that recognised the path but found
// nothing to allow (a revoked link) must fall through to the deny default.
func TestFrameAncestorsTreatsAnEmptySliceAsNoClaim(t *testing.T) {
	ResetForTesting()
	defer ResetForTesting()

	Register(func(_ *http.Request) []string { return []string{} })

	if got := FrameAncestors(httptest.NewRequest("GET", "/p/x/tok", nil)); got != "'none'" {
		t.Errorf("FrameAncestors = %q, want 'none'", got)
	}
}

func TestRegisterIgnoresNil(t *testing.T) {
	ResetForTesting()
	defer ResetForTesting()

	Register(nil)
	if got := FrameAncestors(httptest.NewRequest("GET", "/", nil)); got != "'none'" {
		t.Errorf("FrameAncestors = %q, want 'none'", got)
	}
}
