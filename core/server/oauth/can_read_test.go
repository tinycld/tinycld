package oauth

import "testing"

// CanReadCollection answers as the records API does: a registered collection
// whose read rule the grant satisfies.
func TestCanReadCollection(t *testing.T) {
	withEmptyRegistry(t)
	RegisterPackage(Package{
		Slug:   "notes",
		Scopes: []Scope{{ID: "notes:read", Label: "Read your notes"}, {ID: "notes:write", Label: "Change your notes"}},
		Collections: map[string]Access{
			"notes_items": {Read: []string{"notes:read"}, Write: []string{"notes:write"}},
		},
	})
	cases := []struct {
		name       string
		granted    []string
		collection string
		want       bool
	}{
		{"the read scope", []string{"notes:read"}, "notes_items", true},
		{"one of several scopes", []string{ScopeProfile, "notes:read"}, "notes_items", true},
		{"the write scope alone does not read", []string{"notes:write"}, "notes_items", false},
		{"another scope", []string{ScopeForms}, "notes_items", false},
		{"a collection nobody registered", []string{"notes:read"}, "widgets_items", false},
		{"core's own collection", []string{ScopeProfile}, "users", true},
		{"no scope", nil, "notes_items", false},
	}
	for _, c := range cases {
		if got := CanReadCollection(c.granted, c.collection); got != c.want {
			t.Errorf("%s: CanReadCollection(%v, %q) = %v, want %v", c.name, c.granted, c.collection, got, c.want)
		}
	}
}
