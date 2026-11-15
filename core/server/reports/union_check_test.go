package reports

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func validUnion() Source {
	return Source{
		Key: "zoo.u", Label: "U",
		Parts: []SourcePart{
			{Collection: "zoo_feedings", Columns: map[string]string{"note": "note", "animal": "animal"}, Values: map[string]any{"kind": "feeding"}},
			{Collection: "zoo_treats", Columns: map[string]string{"note": "label", "animal": "pet"}, Values: map[string]any{"kind": "treat"}},
		},
		Fields: []Field{
			{Key: "note", Label: "Note", Type: Text},
			{Key: "kind", Label: "Kind", Type: Select, Options: []Option{{Value: "feeding", Label: "Feeding"}, {Value: "treat", Label: "Treat"}}},
		},
		Links: []Link{{Key: "animal", Label: "Animal", Target: "zoo.animals", Cardinality: ToOne, Field: "animal"}},
	}
}

func TestRegisterSourceRefusesAMalformedUnion(t *testing.T) {
	cases := map[string]struct {
		edit func(*Source)
		want string
	}{
		"collection and parts":       {func(s *Source) { s.Collection = "zoo_feedings" }, "names its collections in its parts"},
		"one part":                   {func(s *Source) { s.Parts = s.Parts[:1] }, "at least two parts"},
		"part with no collection":    {func(s *Source) { s.Parts[1].Collection = "" }, "needs a collection"},
		"a collection twice":         {func(s *Source) { s.Parts[1].Collection = "zoo_feedings" }, "is a part twice"},
		"field with a column":        {func(s *Source) { s.Fields[0].Column = "label" }, "parts map its columns"},
		"part maps id":               {func(s *Source) { s.Parts[0].Columns["id"] = "id" }, `"id" is not a key a part maps`},
		"part maps a blank column":   {func(s *Source) { s.Parts[0].Columns["note"] = "" }, "maps no column"},
		"value of no field":          {func(s *Source) { s.Parts[0].Values["shade"] = "red" }, "is not a field of the source"},
		"value and column":           {func(s *Source) { s.Parts[0].Columns["kind"] = "note" }, "both a column and a value"},
		"value of the wrong type":    {func(s *Source) { s.Parts[0].Values["kind"] = 42 }, "does not fit"},
		"select value not an option": {func(s *Source) { s.Parts[0].Values["kind"] = "snack" }, "does not fit"},
		"field no part reads":        {func(s *Source) { s.Fields = append(s.Fields, Field{Key: "grams", Label: "Grams", Type: Number}) }, `field "grams": no part maps it`},
		"to-one link no part maps": {func(s *Source) {
			s.Links = append(s.Links, Link{Key: "site", Label: "Site", Target: "zoo.sites", Cardinality: ToOne, Field: "site"})
		}, `link "site": no part maps its field "site"`},
		"to-one link field given only as a value": {func(s *Source) {
			s.Links = append(s.Links, Link{Key: "k", Label: "K", Target: "zoo.animals", Cardinality: ToOne, Field: "kind"})
		}, `link "k": no part maps its field "kind"`},
		"a NaN for a number field": {func(s *Source) {
			s.Fields = append(s.Fields, Field{Key: "grams", Label: "Grams", Type: Number})
			s.Parts[0].Values["grams"] = math.NaN()
		}, "does not fit"},
		"an infinity for a number field": {func(s *Source) {
			s.Fields = append(s.Fields, Field{Key: "grams", Label: "Grams", Type: Number})
			s.Parts[0].Values["grams"] = math.Inf(-1)
		}, "does not fit"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			ResetForTesting()
			t.Cleanup(ResetForTesting)
			src := validUnion()
			src.Fields = append([]Field(nil), src.Fields...)
			src.Parts = append([]SourcePart(nil), src.Parts...)
			for i := range src.Parts {
				src.Parts[i].Columns = maps.Clone(src.Parts[i].Columns)
				src.Parts[i].Values = maps.Clone(src.Parts[i].Values)
			}
			c.edit(&src)
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("RegisterSource did not panic")
				}
				if msg, _ := r.(string); !strings.Contains(msg, c.want) {
					t.Fatalf("panic %q does not mention %q", r, c.want)
				}
			}()
			RegisterSource(src)
		})
	}
}

// With two malformed keys in one part, the refusal names the same one every
// run: the keys are checked in sorted order.
func TestRegisterSourceRefusesMalformedKeysInOrder(t *testing.T) {
	for _, bad := range []string{"Columns", "Values"} {
		messages := map[string]bool{}
		for range 30 {
			ResetForTesting()
			src := validUnion()
			src.Parts[0].Columns = map[string]string{"note": "note", "animal": "animal"}
			src.Parts[0].Values = map[string]any{"kind": "feeding"}
			if bad == "Columns" {
				src.Parts[0].Columns["Bad One"] = "x"
				src.Parts[0].Columns["Bad Two"] = "x"
			} else {
				src.Parts[0].Values["ghost_a"] = "x"
				src.Parts[0].Values["ghost_b"] = "x"
			}
			func() {
				defer func() { messages[fmt.Sprint(recover())] = true }()
				RegisterSource(src)
			}()
		}
		ResetForTesting()
		if len(messages) != 1 {
			t.Fatalf("%s: the refusal varies between runs: %v", bad, messages)
		}
	}
}

// A union registers with a Money field and a Bool field that only constants
// give.
func TestRegisterSourceAcceptsValuesOnlyMoneyAndBool(t *testing.T) {
	ResetForTesting()
	t.Cleanup(ResetForTesting)
	src := validUnion()
	src.Currency = func(core.App, *core.RequestInfo) string { return "USD" }
	src.Fields = append(src.Fields,
		Field{Key: "fee", Label: "Fee", Type: Money},
		Field{Key: "paid", Label: "Paid", Type: Bool},
	)
	src.Parts[0].Values = map[string]any{"kind": "feeding", "fee": 5, "paid": true}
	src.Parts[1].Values = map[string]any{"kind": "treat", "fee": 2.5, "paid": false}
	RegisterSource(src)
	if lookupRegistered("zoo.u") == nil {
		t.Fatal("the union is not registered")
	}
}

// The registry keeps its own copy of the parts: a caller that changes its maps
// after registering changes nothing.
func TestRegisterSourceCopiesTheParts(t *testing.T) {
	ResetForTesting()
	t.Cleanup(ResetForTesting)
	src := validUnion()
	RegisterSource(src)
	src.Parts[0].Columns["note"] = "changed"
	src.Parts[0].Values["kind"] = "treat"
	got := lookupRegistered("zoo.u").Parts[0]
	if got.Columns["note"] != "note" || got.Values["kind"] != "feeding" {
		t.Fatalf("registered part = %+v", got)
	}
}

func TestCheckSourcesPassesAUnionAndLinksToAndFromIt(t *testing.T) {
	z := newUnionZoo(t)
	for _, key := range []string{"zoo.meals", "zoo.pets", "zoo.tags"} {
		if lookupSource(z.app, key) == nil {
			t.Fatalf("%s is not usable: %v", key, Refused(z.app))
		}
	}
}

func TestCheckSourcesRefusesABrokenUnion(t *testing.T) {
	cases := map[string]struct {
		edit func(*Source)
		want string
	}{
		"unknown part collection": {func(s *Source) { s.Parts[1].Collection = "zoo_ghosts" }, `collection "zoo_ghosts" does not exist`},
		"unknown part column":     {func(s *Source) { s.Parts[1].Columns["note"] = "tail" }, `part "zoo_treats": column "tail" does not exist`},
		"unknown column of a key that is no field": {func(s *Source) { s.Parts[1].Columns["extra"] = "tail" },
			`key "extra": part "zoo_treats": column "tail" does not exist`},
		"part column of another type": {func(s *Source) { s.Parts[1].Columns["note"] = "remark" },
			`column "remark" of "zoo_treats" is of type editor, but column "note" of "zoo_feedings" is of type text`},
		"part with no list rule": {func(s *Source) {
			s.Parts[1] = SourcePart{Collection: "zoo_vault", Columns: map[string]string{"note": "name", "animal": "animal"}, Values: map[string]any{"kind": "treat"}}
		}, `part collection "zoo_vault" has no list rule`},
		"to-many link from a union whose back relation points elsewhere": {func(s *Source) {
			s.Links = append(s.Links, Link{Key: "pets", Label: "Pets", Target: "zoo.animals", Cardinality: ToMany, BackField: "site"})
		}, `relation "site" of "zoo_animals" does not point at "zoo_feedings" or "zoo_treats"`},
		"to-one link through a relation to elsewhere": {func(s *Source) { s.Links[0].Target = "zoo.sites" },
			`relation "animal" of "zoo_feedings" does not point at "zoo_sites"`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			z := newZoo(t)
			addZooTreats(t, z)
			src := validUnion()
			for i := range src.Parts {
				src.Parts[i].Columns = maps.Clone(src.Parts[i].Columns)
			}
			c.edit(&src)
			RegisterSource(src)
			err := CheckSources(z.app)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("CheckSources = %v, want it to contain %q", err, c.want)
			}
			if lookupSource(z.app, "zoo.u") != nil {
				t.Fatal("a refused union is still usable")
			}
			if lookupSource(z.app, "zoo.feedings") == nil {
				t.Fatal("a sound source was refused")
			}
		})
	}
}

// Columns of one compatibility class agree: a date column and an autodate
// column are both days.
func TestCheckSourcesAcceptsADateAndAnAutodateColumn(t *testing.T) {
	z := newZoo(t)
	addZooTreats(t, z)
	treats, err := z.app.FindCollectionByNameOrId("zoo_treats")
	if err != nil {
		t.Fatal(err)
	}
	treats.Fields.Add(&core.AutodateField{Name: "stamped", OnCreate: true})
	z.save(t, treats)
	src := validUnion()
	src.Fields = append(src.Fields, Field{Key: "fed", Label: "Fed", Type: Date})
	src.Parts[0].Columns = map[string]string{"note": "note", "animal": "animal", "fed": "fed"}
	src.Parts[1].Columns = map[string]string{"note": "label", "animal": "pet", "fed": "stamped"}
	RegisterSource(src)
	if err := CheckSources(z.app); err != nil {
		t.Fatalf("CheckSources = %v", err)
	}
	if lookupSource(z.app, "zoo.u") == nil {
		t.Fatal("the union was refused")
	}
}

// A to-one link needs a part that maps its field; the check refuses a link
// with no owner even when registration did not.
func TestCheckSourcesRefusesAToOneLinkWithNoOwner(t *testing.T) {
	z := newUnionZoo(t)
	src := validUnion()
	l := Link{Key: "k", Label: "K", Target: "zoo.animals", Cardinality: ToOne, Field: "kind"}
	msg := checkLink(z.app, &src, l)
	if !strings.Contains(msg, `no part of "zoo.u" maps "kind"`) {
		t.Fatalf("checkLink = %q", msg)
	}
}

// A to-many link into a union needs its back relation mapped by a part, and a
// source that links to a refused union is refused with it.
func TestCheckSourcesRefusesLinksIntoABrokenUnion(t *testing.T) {
	z := newZoo(t)
	addZooTreats(t, z)
	registerZooMeals()
	RegisterSource(Source{
		Key: "zoo.owners", Collection: "zoo_animals", Label: "Owners",
		Links: []Link{{Key: "meals", Label: "Meals", Target: "zoo.meals", Cardinality: ToMany, BackField: "owner"}},
	})
	err := CheckSources(z.app)
	if err == nil || !strings.Contains(err.Error(), `no part of "zoo.meals" maps "owner"`) {
		t.Fatalf("CheckSources = %v", err)
	}

	z = newZoo(t)
	addZooTreats(t, z)
	registerZooMeals()
	broken := *lookupRegistered("zoo.meals")
	broken.Parts = append([]SourcePart(nil), broken.Parts...)
	// A column that does not exist refuses the union and leaves its collections
	// resolvable, so zoo.pets is refused by the cascade (check.go), not by a
	// missing target collection.
	broken.Parts[1].Columns = maps.Clone(broken.Parts[1].Columns)
	broken.Parts[1].Columns["note"] = "tail"
	RegisterSource(broken)
	err = CheckSources(z.app)
	if err == nil || !strings.Contains(err.Error(), `link "meals" targets source "zoo.meals", which is refused`) {
		t.Fatalf("CheckSources = %v", err)
	}
	if lookupSource(z.app, "zoo.pets") != nil {
		t.Fatal("a link to a refused union kept its source")
	}
}

// The catalog offers a union like any source: its fields and links, never its
// collections or parts.
func TestCatalogListsAUnionLikeAnySource(t *testing.T) {
	z := newUnionZoo(t)
	var meals *CatalogSource
	for _, cs := range Catalog(z.app) {
		if cs.Key == "zoo.meals" {
			meals = &cs
		}
	}
	if meals == nil {
		t.Fatal("zoo.meals is not in the catalog")
	}
	var keys []string
	for _, f := range meals.Fields {
		keys = append(keys, f.Key)
	}
	if strings.Join(keys, ",") != "id,note,kind,cost,currency,grams,fed" {
		t.Fatalf("fields = %v", keys)
	}
	if len(meals.Links) != 1 || meals.Links[0].Key != "animal" || meals.Links[0].Target != "zoo.animals" {
		t.Fatalf("links = %+v", meals.Links)
	}
	raw, err := json.Marshal(meals)
	if err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"zoo_feedings", "zoo_treats", "parts", "collection"} {
		if strings.Contains(strings.ToLower(string(raw)), word) {
			t.Fatalf("the catalog names %q: %s", word, raw)
		}
	}
}
