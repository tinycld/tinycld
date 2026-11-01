package reports

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestCheckSourcesPassesTheZoo(t *testing.T) {
	z := newZoo(t)
	if err := CheckSources(z.app); err != nil {
		t.Fatalf("CheckSources: %v", err)
	}
	if got := len(Sources(z.app)); got != 6 {
		t.Fatalf("%d sources, want 6", got)
	}
}

func TestCheckSourcesRefusesASourceThatDoesNotMatchTheSchema(t *testing.T) {
	cases := map[string]struct {
		src  Source
		want string
	}{
		"missing collection": {
			Source{Key: "zoo.ghosts", Collection: "zoo_ghosts", Label: "Ghosts"},
			`collection "zoo_ghosts" does not exist`,
		},
		"missing column": {
			Source{Key: "zoo.x", Collection: "zoo_animals", Label: "X", Fields: []Field{{Key: "tail", Label: "Tail", Type: Text}}},
			`column "tail" does not exist`,
		},
		"hidden column": {
			Source{Key: "zoo.x", Collection: "zoo_animals", Label: "X", Fields: []Field{{Key: "secret", Label: "Secret", Type: Text}}},
			`column "secret" is hidden from the API`,
		},
		"wrong type": {
			Source{Key: "zoo.x", Collection: "zoo_animals", Label: "X", Fields: []Field{{Key: "name", Label: "Name", Type: Money}},
				Currency: func(core.App) string { return "USD" }},
			`column "name" is a text field, which a money field cannot read`,
		},
		"select option not in the schema": {
			Source{Key: "zoo.x", Collection: "zoo_sites", Label: "X", Fields: []Field{{Key: "region", Label: "Region", Type: Select, Options: []Option{{Value: "east", Label: "East"}}}}},
			`option "east" is not a value`,
		},
		"to-one link through the wrong relation": {
			Source{Key: "zoo.x", Collection: "zoo_animals", Label: "X", Links: []Link{{Key: "site", Label: "Site", Target: "zoo.people", Cardinality: ToOne, Field: "site"}}},
			`relation "site" of "zoo_animals" does not point at "users"`,
		},
		"to-many link without a back relation": {
			Source{Key: "zoo.x", Collection: "zoo_sites", Label: "X", Links: []Link{{Key: "feedings", Label: "Feedings", Target: "zoo.feedings", Cardinality: ToMany, BackField: "note"}}},
			`"note" is not a relation field of "zoo_feedings"`,
		},
		"unregistered target": {
			Source{Key: "zoo.x", Collection: "zoo_animals", Label: "X", Links: []Link{{Key: "pen", Label: "Pen", Target: "zoo.pens", Cardinality: ToOne, Field: "site"}}},
			`target source "zoo.pens" is not registered`,
		},
		"auth email": {
			Source{Key: "zoo.x", Collection: "users", Label: "X", Fields: []Field{{Key: "email", Label: "Email", Type: Text}}},
			`column "email" is visible only by emailVisibility`,
		},
		"system collection": {
			Source{Key: "zoo.x", Collection: "_superusers", Label: "X"},
			`is a system collection`,
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			z := newZoo(t)
			RegisterSource(c.src)
			err := CheckSources(z.app)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("CheckSources = %v, want it to contain %q", err, c.want)
			}
			if _, bad := Refused(z.app)[c.src.Key]; !bad {
				t.Fatalf("%s not refused", c.src.Key)
			}
			if lookupSource(z.app, c.src.Key) != nil {
				t.Fatal("a refused source is still usable")
			}
			if lookupSource(z.app, "zoo.animals") == nil {
				t.Fatal("a sound source was refused")
			}
		})
	}
}

func TestCheckSourcesRefusesARuleThatCannotBeBuiltAndEveryLinkToIt(t *testing.T) {
	z := newZoo(t)
	// PocketBase validates a rule when the collection is saved, so a broken
	// rule can only arrive by a raw write; reload the cache so it is read.
	if _, err := z.app.DB().NewQuery("UPDATE _collections SET listRule = 'nope = 1' WHERE name = 'zoo_sites'").Execute(); err != nil {
		t.Fatal(err)
	}
	if err := z.app.ReloadCachedCollections(); err != nil {
		t.Fatal(err)
	}
	err := CheckSources(z.app)
	if err == nil || !strings.Contains(err.Error(), `the list rule of "zoo_sites" cannot be built`) {
		t.Fatalf("CheckSources = %v", err)
	}
	refusedNow := Refused(z.app)
	for _, key := range []string{"zoo.sites", "zoo.animals", "zoo.feedings", "zoo.keepers", "zoo.vault"} {
		if _, bad := refusedNow[key]; !bad {
			t.Errorf("%s is not refused", key)
		}
	}
	if !strings.Contains(refusedNow["zoo.animals"], `link "site" targets source "zoo.sites", which is refused`) {
		t.Errorf("zoo.animals reason = %q", refusedNow["zoo.animals"])
	}
	if _, bad := refusedNow["zoo.people"]; bad {
		t.Error("zoo.people links nowhere and must stay usable")
	}
}

func TestCheckSourcesRefusesARuleThatBuildsButDoesNotCompile(t *testing.T) {
	z := newZoo(t)
	sites, err := z.app.FindCollectionByNameOrId("zoo_sites")
	if err != nil {
		t.Fatal(err)
	}
	sites.ListRule = rule(`name != ""`)
	z.save(t, sites)
	// The collection still lists the column, so the rule builds, but the
	// table no longer has it: only SQLite can tell.
	if _, err := z.app.DB().NewQuery("ALTER TABLE zoo_sites DROP COLUMN name").Execute(); err != nil {
		t.Fatal(err)
	}
	err = CheckSources(z.app)
	if err == nil || !strings.Contains(err.Error(), `the list rule of "zoo_sites" does not compile`) {
		t.Fatalf("CheckSources = %v", err)
	}
}

func TestCheckSourcesAgainClearsAFixedRefusal(t *testing.T) {
	z := newZoo(t)
	RegisterSource(Source{Key: "zoo.ghosts", Collection: "zoo_ghosts", Label: "Ghosts"})
	if CheckSources(z.app) == nil {
		t.Fatal("expected a refusal")
	}
	RegisterSource(Source{Key: "zoo.ghosts", Collection: "zoo_sites", Label: "Ghosts"})
	if err := CheckSources(z.app); err != nil {
		t.Fatalf("CheckSources after the fix: %v", err)
	}
}
