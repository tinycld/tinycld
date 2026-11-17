package reports

import (
	"maps"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// The zoo's slips: a union of zoo_receipts and zoo_refunds whose to-one link
// "meal" points into the zoo.meals union. A relation cannot point at both
// zoo_feedings and zoo_treats, so each part stores the meal's id in a text
// column (receipts' "meal", refunds' "meal_id"). Any signed-in user may list
// both parts; the meals keep their own rules.
//
//	r1 → f1 (a feeding of leo)    r2 → t1 (a treat of kit)
//	d1 → t3 (a treat of leo)      d2 → t4 (a treat of pip)
//
// ann keeps leo and owns leo and kit: she sees f1, t1, t3, not t4.
// dan keeps nothing and owns pip: he sees no feeding at all, and t4.
func addZooSlips(t testing.TB, z *zoo) {
	t.Helper()
	receipts := core.NewBaseCollection("zoo_receipts")
	receipts.ListRule = rule(`@request.auth.id != ""`)
	receipts.ViewRule = receipts.ListRule
	receipts.Fields.Add(
		&core.TextField{Name: "memo"},
		&core.TextField{Name: "meal"},
	)
	z.save(t, receipts)
	refunds := core.NewBaseCollection("zoo_refunds")
	refunds.ListRule = rule(`@request.auth.id != ""`)
	refunds.ViewRule = refunds.ListRule
	refunds.Fields.Add(
		&core.TextField{Name: "memo"},
		&core.TextField{Name: "meal_id"},
		&core.TextField{Name: "hidden_meal", Hidden: true},
		&core.NumberField{Name: "amount"},
	)
	z.save(t, refunds)
	z.rec(t, "r1", "zoo_receipts", map[string]any{"memo": "r1", "meal": z.id("f1")})
	z.rec(t, "r2", "zoo_receipts", map[string]any{"memo": "r2", "meal": z.id("t1")})
	z.rec(t, "d1", "zoo_refunds", map[string]any{"memo": "d1", "meal_id": z.id("t3"), "hidden_meal": z.id("t3"), "amount": 3})
	z.rec(t, "d2", "zoo_refunds", map[string]any{"memo": "d2", "meal_id": z.id("t4"), "hidden_meal": z.id("t4"), "amount": 4})
}

func zooSlips() Source {
	return Source{
		Key: "zoo.slips", Label: "Slips",
		Parts: []SourcePart{
			{Collection: "zoo_receipts", Columns: map[string]string{"memo": "memo", "meal": "meal"}},
			{Collection: "zoo_refunds", Columns: map[string]string{"memo": "memo", "meal": "meal_id"}},
		},
		Fields: []Field{{Key: "memo", Label: "Memo", Type: Text, Aggregates: MinMax}},
		Links:  []Link{{Key: "meal", Label: "Meal", Target: "zoo.meals", Cardinality: ToOne, Field: "meal"}},
	}
}

// newSlipsZoo is the union zoo with its slips, checked.
func newSlipsZoo(t *testing.T) *zoo {
	t.Helper()
	z := newZoo(t)
	addZooTreats(t, z)
	addZooSlips(t, z)
	registerZooMeals()
	RegisterSource(zooSlips())
	if err := CheckSources(z.app); err != nil {
		t.Fatalf("CheckSources: %v", err)
	}
	return z
}

// A union part may link by a text column that holds the id of a row of one of
// the link target's parts.
func TestCheckSourcesAcceptsATextIDLinkOnAUnionPart(t *testing.T) {
	z := newZoo(t)
	addZooTreats(t, z)
	addZooSlips(t, z)
	registerZooMeals()
	RegisterSource(zooSlips())
	if err := CheckSources(z.app); err != nil {
		t.Fatalf("CheckSources = %v", err)
	}
	if lookupSource(z.app, "zoo.slips") == nil {
		t.Fatalf("zoo.slips is refused: %v", Refused(z.app))
	}
}

// The parts of one union may map a link differently: zoo_tags by its relation
// "feeding" to zoo_feedings, zoo_receipts by its text column "meal".
func TestCheckSourcesAcceptsARelationAndATextIDLinkInOneUnion(t *testing.T) {
	z := newZoo(t)
	addZooTreats(t, z)
	addZooSlips(t, z)
	registerZooMeals()
	RegisterSource(Source{
		Key: "zoo.notes", Label: "Notes",
		Parts: []SourcePart{
			{Collection: "zoo_tags", Columns: map[string]string{"memo": "tag", "meal": "feeding"}},
			{Collection: "zoo_receipts", Columns: map[string]string{"memo": "memo", "meal": "meal"}},
		},
		Fields: []Field{{Key: "memo", Label: "Memo", Type: Text}},
		Links:  []Link{{Key: "meal", Label: "Meal", Target: "zoo.meals", Cardinality: ToOne, Field: "meal"}},
	})
	if err := CheckSources(z.app); err != nil {
		t.Fatalf("CheckSources = %v", err)
	}
	if lookupSource(z.app, "zoo.notes") == nil {
		t.Fatalf("zoo.notes is refused: %v", Refused(z.app))
	}
}

// A text link column is refused where it is not a union part's, where the
// link's target is not a union, as a to-many link's BackField, where its
// column does not exist or is hidden, and a column that is neither text nor a
// relation is refused. Only the broken source is refused: zoo.slips, which
// links by text the same way, stays usable beside it.
func TestCheckSourcesRefusesABrokenTextIDLink(t *testing.T) {
	cases := map[string]struct {
		bad  func() Source
		want string
	}{
		"a text column on a one-collection source": {func() Source {
			return Source{
				Key: "zoo.bad", Collection: "zoo_receipts", Label: "Bad",
				Fields: []Field{{Key: "memo", Label: "Memo", Type: Text}},
				Links:  []Link{{Key: "meal", Label: "Meal", Target: "zoo.meals", Cardinality: ToOne, Field: "meal"}},
			}
		}, `link "meal": "meal" is not a relation field of "zoo_receipts"`},
		"a text column whose target is not a union": {func() Source {
			s := badSlips()
			s.Links = []Link{{Key: "meal", Label: "Meal", Target: "zoo.feedings", Cardinality: ToOne, Field: "meal"}}
			return s
		}, `link "meal": "meal" is not a relation field of "zoo_receipts"`},
		"a text column as a to-many link's BackField": {func() Source {
			// zoo.meals under another key, so both sides are unions and only
			// the link's cardinality refuses the text column.
			s := *sources["zoo.meals"]
			s.Key, s.Label = "zoo.bad", "Bad"
			s.Links = []Link{{Key: "slips", Label: "Slips", Target: "zoo.slips", Cardinality: ToMany, BackField: "meal"}}
			return s
		}, `link "slips": "meal" is not a relation field of "zoo_receipts"`},
		"a column that does not exist": {func() Source {
			s := badSlips()
			s.Parts[1].Columns["meal"] = "ghost"
			return s
		}, `link "meal": column "ghost" of "zoo_refunds" does not exist`},
		"a hidden text column": {func() Source {
			s := badSlips()
			s.Parts[1].Columns["meal"] = "hidden_meal"
			return s
		}, `link "meal": text "hidden_meal" of "zoo_refunds" is hidden from the API`},
		"a number column": {func() Source {
			s := badSlips()
			s.Parts[1].Columns["meal"] = "amount"
			return s
		}, `link "meal": "amount" is not a relation or text field of "zoo_refunds"`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			z := newZoo(t)
			addZooTreats(t, z)
			addZooSlips(t, z)
			registerZooMeals()
			RegisterSource(zooSlips())
			RegisterSource(c.bad())
			err := CheckSources(z.app)
			if err == nil {
				t.Fatal("CheckSources accepted the broken source")
			}
			reason := Refused(z.app)["zoo.bad"]
			if !strings.Contains(reason, c.want) {
				t.Fatalf("zoo.bad is refused for %q, want it to contain %q", reason, c.want)
			}
			if lookupSource(z.app, "zoo.bad") != nil {
				t.Fatal("the broken source is still usable")
			}
			if lookupSource(z.app, "zoo.slips") == nil {
				t.Fatalf("zoo.slips was refused beside it: %v", Refused(z.app))
			}
		})
	}
}

// badSlips is zooSlips under another key, with its own copy of the parts.
func badSlips() Source {
	s := zooSlips()
	s.Key, s.Label = "zoo.bad", "Bad"
	s.Parts = append([]SourcePart(nil), s.Parts...)
	for i := range s.Parts {
		s.Parts[i].Columns = maps.Clone(s.Parts[i].Columns)
	}
	return s
}

const slipsReport = `{"version":4,"source":"zoo.slips","columns":[
	{"id":"memo","path":[{"field":"memo"}]},
	{"id":"meal","path":[{"link":"meal"}],"labelField":"note"},
	{"id":"type","path":[{"link":"meal"},{"field":"kind"}]},
	{"id":"cost","path":[{"link":"meal"},{"field":"cost"}]}],
	"sort":[{"col":"memo"}]}`

// A run follows each slip's text id to its own meal: a receipt's to a feeding
// or a treat, a refund's to a treat, whichever part holds that id.
func TestATextIDLinkOpensEachRowsOwnTarget(t *testing.T) {
	z := newSlipsZoo(t)
	res := z.run(t, "ann", slipsReport, Options{}, nil)
	// cost is money read through the link, so core totals per currency: d2's
	// meal (t4) is not ann's to see, its currency is null, and adds a total.
	requireOrderedRows(t, slipsReport, res, "id",
		"d:"+z.id("d1"), "d:"+z.id("d2"), "d:"+z.id("r1"), "d:"+z.id("r2"), `total:["USD"]`, `total:[null]`)
	requireOrderedRows(t, slipsReport, res, "memo", "d1", "d2", "r1", "r2", nil, nil)
	requireRows(t, res, "meal", z.id("t3"), nil, z.id("f1"), z.id("t1"), nil, nil)
	requireRows(t, res, "meal@label", "t3", nil, "f1", "t1", nil, nil)
	requireRows(t, res, "type", "treat", nil, "feeding", "treat", nil, nil)
	requireRows(t, res, "cost", int64(25), nil, int64(1000), int64(40), nil, nil)
}

// A reader who may list no row of one target collection (dan keeps no animal,
// so he sees no feeding) gets no link for the rows that name one, as with a
// relation to a row he cannot see; his treat still links.
func TestATextIDLinkToARowTheReaderCannotSeeReadsBlank(t *testing.T) {
	z := newSlipsZoo(t)
	res := z.run(t, "dan", slipsReport, Options{}, nil)
	// cost is money read through the link, so core totals per currency: the
	// rows whose meal dan cannot see have a null currency and add a total.
	requireOrderedRows(t, slipsReport, res, "id",
		"d:"+z.id("d1"), "d:"+z.id("d2"), "d:"+z.id("r1"), "d:"+z.id("r2"), `total:["USD"]`, `total:[null]`)
	requireOrderedRows(t, slipsReport, res, "memo", "d1", "d2", "r1", "r2", nil, nil)
	requireRows(t, res, "meal", nil, z.id("t4"), nil, nil, nil, nil)
	requireRows(t, res, "meal@label", nil, "t4", nil, nil, nil, nil)
	requireRows(t, res, "type", nil, "treat", nil, nil, nil, nil)
	requireRows(t, res, "cost", nil, int64(10), nil, nil, nil, nil)
}
