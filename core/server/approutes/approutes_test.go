package approutes

import "testing"

func TestHref(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"boards", "/a/boards"},
		{"settings/personal", "/a/settings/personal"},
		// The workspace root is the bare prefix — a trailing slash would be a
		// different path from the route it needs to match.
		{"", "/a"},
	}
	for _, c := range cases {
		if got := Href(c.path); got != c.want {
			t.Errorf("Href(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

// The TS side (APP_PREFIX in core/lib/org-routes.ts) must agree with this.
func TestPrefixHasNoTrailingSlash(t *testing.T) {
	if Prefix != "/a" {
		t.Errorf("Prefix = %q, want %q", Prefix, "/a")
	}
}
