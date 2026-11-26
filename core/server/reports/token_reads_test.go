package reports

import (
	"slices"
	"strconv"
	"testing"

	"tinycld.org/core/oauth"
)

// feedingsAtASite is feedingsFlat with a choice param whose labels come from
// zoo.sites, a source no column of the report reads.
const feedingsAtASite = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"params":[{"id":"site","type":"choice","label":"Site","header":"Site: {label}",
		"optionsFrom":{"source":"zoo.sites","value":[{"field":"id"}],"label":[{"field":"name"}]}}],
	"sort":[{"col":"note"}]}`

// registerZooScopes registers a package that gives the feedings to zoo:read,
// the animals to zoo:animals and the sites to zoo:sites. No scope reads the
// keepers, which only the feedings' list rule reads.
func registerZooScopes(t *testing.T) {
	t.Helper()
	oauth.RegisterPackage(oauth.Package{
		Slug: "zoo",
		Scopes: []oauth.Scope{
			{ID: "zoo:read", Label: "Read the feedings"},
			{ID: "zoo:animals", Label: "Read the animals"},
			{ID: "zoo:sites", Label: "Read the sites"},
		},
		Collections: map[string]oauth.Access{
			"zoo_feedings": {Read: []string{"zoo:read"}},
			"zoo_animals":  {Read: []string{"zoo:animals"}},
			"zoo_sites":    {Read: []string{"zoo:sites"}},
		},
	})
	t.Cleanup(oauth.ResetRegistry)
}

func TestClosedToScopes(t *testing.T) {
	z := newZoo(t)
	registerZooScopes(t)
	watchVault(t)
	cases := []struct {
		name, spec string
		params     map[string]string
		granted    []string
		want       []string
	}{
		{"every collection is in the scopes", feedingsByDay, nil, []string{"zoo:read", "zoo:animals"}, nil},
		{"a column's collection is outside", feedingsByDay, nil, []string{"zoo:read"}, []string{"zoo_animals"}},
		{"no data scope", feedingsByDay, nil, []string{"profile"}, []string{"zoo_animals", "zoo_feedings"}},
		{"a param's source is outside", feedingsAtASite, nil, []string{"zoo:read"}, []string{"zoo_sites"}},
		{"a param's source is inside", feedingsAtASite, nil, []string{"zoo:read", "zoo:sites"}, nil},
		{"a pick's label crosses a link", feedingsOfOne, map[string]string{"one": strconv.Quote(z.id("f1"))}, []string{"zoo:read"}, []string{"zoo_animals"}},
		{"no pick, no label read", feedingsOfOne, nil, []string{"zoo:read"}, nil},
		// The label's currency comes from a hook of the animals, which reads the vault.
		{"a hook's collection is outside", pricedAnimal, map[string]string{"animal": strconv.Quote(z.id("leo"))}, []string{"zoo:read", "zoo:animals"}, []string{"zoo_vault"}},
	}
	for _, c := range cases {
		spec, _, err := Load([]byte(c.spec))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		res := z.run(t, "ann", c.spec, Options{}, c.params)
		if got := closedToScopes(z.app, c.granted, spec, res); !slices.Equal(got, c.want) {
			t.Errorf("%s: closedToScopes = %v, want %v", c.name, got, c.want)
		}
	}
}
