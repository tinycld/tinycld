package oauth

import (
	"net/url"
	"testing"
)

// A filter or a sort on a field of a related collection does not return that
// collection's records, but the rows that match, and their order, tell the
// caller its values. Each collection such a path reads thus needs the same
// check as a direct read.
func TestEnforceGrantChecksFilterAndSortPaths(t *testing.T) {
	const list = "/api/collections/notes_items/records"

	notes := []string{scopeNotesRead}
	both := []string{scopeNotesRead, scopeWidgetsRead}

	cases := []struct {
		name   string
		scopes []string
		param  string
		value  string
		want   int
	}{
		{"own field", notes, "filter", `title ~ "a"`, 0},
		{"relation id, no join", notes, "filter", `widget = "abc"`, 0},
		{"relation id, written as widget.id", notes, "filter", `widget.id = "abc"`, 0},
		{"target in scope", notes, "filter", `label.name = "a"`, 0},
		{"target out of scope", notes, "filter", `widget.name ~ "a"`, 403},
		{"path on the right side", notes, "filter", `"a" = widget.name`, 403},
		{"path in a group", notes, "filter", `title = "a" && (title = "b" || widget.name = "c")`, 403},
		{"path with a modifier", notes, "filter", `widget.name:lower = "a"`, 403},
		{"path as a function argument", notes, "filter", `geoDistance(widget.name, 1, 2, 3) < 4`, 403},
		{"back-relation out of scope", notes, "filter", `widgets_items_via_note.name ~ "a"`, 403},
		{"users without the profile scope", notes, "filter", `author.name ~ "a"`, 403},
		{"users with the profile scope", []string{scopeNotesRead, ScopeProfile}, "filter", `author.name ~ "a"`, 0},
		{"text that looks like a path", notes, "filter", `title = "widget.name"`, 0},
		{"chain in scope", both, "filter", `widget.note.label.name = "a"`, 0},
		{"chain leaves scope at depth 2", both, "filter", `widget.owner.name = "a"`, 403},
		{"chain reaches an unclassified collection", both, "filter", `widget.grant.status = "active"`, 403},
		{"filter PocketBase refuses", notes, "filter", `title = `, 0},

		{"sort by own field", notes, "sort", "-title,+id", 0},
		{"sort by a special key", notes, "sort", "@random", 0},
		{"sort by a target in scope", notes, "sort", "label.name", 0},
		{"sort by a target out of scope", notes, "sort", "-widget.name", 403},
		{"second sort key out of scope", notes, "sort", "title, +widget.name", 403},
		{"sort by a back-relation out of scope", notes, "sort", "widgets_items_via_note.name", 403},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			app := newExpandApp(t)
			target := list + "?" + url.Values{c.param: {c.value}}.Encode()
			re := newRequestEvent(app, "GET", target, mintToken(t, app, c.scopes...))
			err := enforceGrant(re)
			if c.want == 0 {
				if err != nil {
					t.Fatalf("enforceGrant refused %s=%s: %v", c.param, c.value, err)
				}
				return
			}
			if got := apiStatus(err); got != c.want {
				t.Fatalf("enforceGrant status = %d, want %d (%v)", got, c.want, err)
			}
			if re.Auth != nil {
				t.Fatal("enforceGrant must not set e.Auth when it refuses the request")
			}
		})
	}
}
