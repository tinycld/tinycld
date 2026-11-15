package reports

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// The zoo's union: zoo_treats has the shape of zoo_feedings with other column
// names (its animal relation is "pet", so a link into the union names the back
// relation per part) and another list rule (the animal's owner, where feedings take its
// keepers), so a reader sees feedings by keeping an animal and treats by
// owning one. zoo.meals is the union of both.
//
//	ann owns leo and kit, keeps leo and zed: feedings f1-f4, treats t1 (kit), t3 (leo)
//	bob owns zed and mo, keeps zed and mo:   feedings f3, f4, f5, treat t2 (zed)
//	dan owns pip and keeps nothing:          treat t4 (pip), no feeding
//	eve owns and keeps nothing:              no meal
//
// zoo_tags tags feedings (list: any signed-in user): tag1 → f1, tag2 → f3.
// zoo.pets is zoo_animals with a to-many link into the union; zoo.tags has a
// to-one link into it.
func addZooTreats(t testing.TB, z *zoo) {
	t.Helper()
	animals, err := z.app.FindCollectionByNameOrId("zoo_animals")
	if err != nil {
		t.Fatal(err)
	}
	feedings, err := z.app.FindCollectionByNameOrId("zoo_feedings")
	if err != nil {
		t.Fatal(err)
	}
	treats := core.NewBaseCollection("zoo_treats")
	treats.ListRule = rule(`pet.owner = @request.auth.id`)
	treats.ViewRule = treats.ListRule
	treats.Fields.Add(
		&core.RelationField{Name: "pet", CollectionId: animals.Id, MaxSelect: 1},
		&core.NumberField{Name: "price"},
		&core.TextField{Name: "currency"},
		&core.DateField{Name: "given"},
		&core.TextField{Name: "label"},
		&core.EditorField{Name: "remark"},
	)
	z.save(t, treats)
	tags := core.NewBaseCollection("zoo_tags")
	tags.ListRule = rule(`@request.auth.id != ""`)
	tags.ViewRule = tags.ListRule
	tags.Fields.Add(
		&core.TextField{Name: "tag"},
		&core.RelationField{Name: "feeding", CollectionId: feedings.Id, MaxSelect: 1},
	)
	z.save(t, tags)

	dan := z.user(t, "dan")
	z.user(t, "eve")
	pip := z.rec(t, "pip", "zoo_animals", map[string]any{"name": "Pip", "species": "parrot", "owner": dan.Id, "weight": 1})
	z.treat(t, "t1", z.id("kit"), 40, "2026-01-07")
	z.treat(t, "t2", z.id("zed"), 60, "2026-02-02")
	z.treat(t, "t3", z.id("leo"), 25, "2026-03-01")
	z.treat(t, "t4", pip.Id, 10, "2026-03-15")
	z.rec(t, "tag1", "zoo_tags", map[string]any{"tag": "tasty", "feeding": z.id("f1")})
	z.rec(t, "tag2", "zoo_tags", map[string]any{"tag": "fresh", "feeding": z.id("f3")})
}

func (z *zoo) treat(t testing.TB, key, animal string, price int, day string) {
	t.Helper()
	z.rec(t, key, "zoo_treats", map[string]any{
		"pet": animal, "price": price, "currency": "USD", "given": day + " 00:00:00.000Z", "label": key,
	})
}

func registerZooMeals() {
	RegisterSource(Source{
		Key: "zoo.meals", Label: "Meals",
		Parts: []SourcePart{
			{
				Collection: "zoo_feedings",
				Columns:    map[string]string{"cost": "cost", "currency": "currency", "grams": "grams", "fed": "fed", "note": "note", "animal": "animal"},
				Values:     map[string]any{"kind": "feeding"},
			},
			{
				Collection: "zoo_treats",
				Columns:    map[string]string{"cost": "price", "currency": "currency", "fed": "given", "note": "label", "animal": "pet"},
				Values:     map[string]any{"kind": "treat"},
			},
		},
		Fields: []Field{
			{Key: "note", Label: "Note", Type: Text, Aggregates: MinMax},
			{Key: "kind", Label: "Kind", Type: Select, Options: []Option{{Value: "feeding", Label: "Feeding"}, {Value: "treat", Label: "Treat"}}},
			{Key: "cost", Label: "Cost", Type: Money, Aggregates: SumAll, Currency: "currency"},
			{Key: "currency", Label: "Currency", Type: Text},
			{Key: "grams", Label: "Grams", Type: Number, Aggregates: SumAll},
			{Key: "fed", Label: "Fed", Type: Date, Aggregates: MinMax},
		},
		Links:    []Link{{Key: "animal", Label: "Animal", Target: "zoo.animals", Cardinality: ToOne, Field: "animal"}},
		Currency: func(core.App, *core.RequestInfo) string { return "USD" },
		// No meals report reads sites, so a run's zoo_sites topic comes from Watch alone.
		Watch: []string{"zoo_sites"},
	})
	RegisterSource(Source{
		Key: "zoo.pets", Collection: "zoo_animals", Label: "Pets",
		Fields: []Field{{Key: "name", Label: "Name", Type: Text, Aggregates: MinMax}},
		Links:  []Link{{Key: "meals", Label: "Meals", Target: "zoo.meals", Cardinality: ToMany, BackField: "animal"}},
	})
	RegisterSource(Source{
		Key: "zoo.tags", Collection: "zoo_tags", Label: "Tags",
		Fields: []Field{{Key: "tag", Label: "Tag", Type: Text}},
		Links:  []Link{{Key: "meal", Label: "Meal", Target: "zoo.meals", Cardinality: ToOne, Field: "feeding"}},
	})
}

// newUnionZoo is the zoo with its union, checked.
func newUnionZoo(t *testing.T) *zoo {
	t.Helper()
	z := newZoo(t)
	addZooTreats(t, z)
	registerZooMeals()
	if err := CheckSources(z.app); err != nil {
		t.Fatalf("CheckSources: %v", err)
	}
	return z
}
