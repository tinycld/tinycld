package reports

import (
	"fmt"
	"slices"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/changesignal"
)

const mealsReport = `{"version":4,"source":"zoo.meals","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"type","path":[{"field":"kind"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
	{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
	"sort":[{"col":"note"}]}`

// The column of the kind field has the id "type": rowsOf (run_test.go) reads
// "kind" as the row's own kind (detail, group, total), not as a cell.

// A union reads the rows of both parts; a key a part does not map (a treat's
// grams) reads blank, and a part's constant (kind) reads on every row of it.
func TestAUnionRunReadsTheRowsOfEveryPart(t *testing.T) {
	z := newUnionZoo(t)
	res := z.run(t, "ann", mealsReport, Options{}, nil)
	requireOrderedRows(t, mealsReport, res, "label", "f1", "f2", "f3", "f4", "t1", "t3", "Total")
	requireRows(t, res, "type", "feeding", "feeding", "feeding", "feeding", "treat", "treat", nil)
	requireRows(t, res, "cost", int64(1000), int64(2500), int64(700), int64(300), int64(40), int64(25), int64(4565))
	requireRows(t, res, "cost@currency", "USD", "USD", "USD", "USD", "USD", "USD", "USD")
	requireRows(t, res, "grams", int64(500), int64(800), int64(300), int64(100), nil, nil, int64(1700))
}

// Each part is read under its own list rule, exactly as the list API lists
// it: feedings by keeping the animal, treats by owning it.
func TestAUnionReadsEachPartUnderItsOwnRule(t *testing.T) {
	z := newUnionZoo(t)
	mux := z.mux(t)
	ids := `{"version":4,"source":"zoo.meals","columns":[{"id":"id","path":[{"field":"id"}]}],"sort":[{"col":"id"}]}`
	for _, user := range []string{"ann", "bob", "dan", "eve"} {
		res := z.run(t, user, ids, Options{}, nil)
		var got []string
		for _, r := range res.Rows {
			if r.Kind == KindDetail {
				got = append(got, r.Cells["id"].(string))
			}
		}
		// The id column is a record: the report sorts it by the record's label.
		slices.Sort(got)
		want := append(z.listIDs(t, mux, "zoo_feedings", user, ""), z.listIDs(t, mux, "zoo_treats", user, "")...)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Fatalf("%s reads %v, the list API lists %v", user, got, want)
		}
	}
	requireOrderedRows(t, mealsReport, z.run(t, "dan", mealsReport, Options{}, nil), "label", "t4", "Total")
	requireOrderedRows(t, mealsReport, z.run(t, "bob", mealsReport, Options{}, nil), "label", "f3", "f4", "f5", "t2", "Total")
}

// Review Focus 1: a reader who may list no row of either part gets an empty
// report in the source's currency, not an error.
func TestAUnionReaderWithNoRowsSeesAnEmptyReport(t *testing.T) {
	z := newUnionZoo(t)
	res := z.run(t, "eve", mealsReport, Options{}, nil)
	requireRows(t, res, "kind", KindTotal)
	requireRows(t, res, "label", "Total")
	requireRows(t, res, "cost", nil) // a total over no rows stays blank (emptycurrency_test.go)
	// The source's currency of an empty total is in its row id (as in
	// TestATotalOverNoRowsIsInTheSourceCurrency), not in a cell.
	if res.Rows[0].ID != `total:["USD"]` {
		t.Fatalf("total id = %q", res.Rows[0].ID)
	}
}

// A run's topics name every part's collection, and the collections the
// source's Watch names (no meals report reads zoo_sites otherwise).
func TestAUnionRunNamesEveryPartsTopic(t *testing.T) {
	z := newUnionZoo(t)
	res := z.run(t, "ann", mealsReport, Options{}, nil)
	for _, c := range []string{"zoo_feedings", "zoo_treats", "zoo_sites"} {
		if !slices.Contains(res.Topics, changesignal.Topic(c)) {
			t.Fatalf("topics %v miss %s", res.Topics, c)
		}
	}
}

// Review Focus 5: a group by a part's constant totals each part apart.
func TestAGroupByAUnionConstantTotalsEachPart(t *testing.T) {
	z := newUnionZoo(t)
	spec := `{"version":4,"source":"zoo.meals","columns":[
		{"id":"type","path":[{"field":"kind"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"type"}],"sort":[{"col":"type"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireRows(t, res, "type", "feeding", "treat", nil)
	requireRows(t, res, "cost", int64(4500), int64(65), int64(4565))
}

// A link from a union to a one-collection source, and a to-many link from a
// one-collection source into a union (each part maps the back relation to its
// own column: feedings' "animal", treats' "pet"). The
// into-union total counts: a money sum across a to-many link is refused by
// the engine for any source (bind.go), union or not; the money sum across the
// parts is pinned by TestALookupSumsAcrossTheParts.
func TestLinksFromAndIntoAUnion(t *testing.T) {
	z := newUnionZoo(t)
	from := `{"version":4,"source":"zoo.meals","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"animal","path":[{"link":"animal"},{"field":"name"}]}],
		"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", from, Options{}, nil)
	requireOrderedRows(t, from, res, "animal", "Leo", "Leo", "Zed", "Zed", "Kit", "Leo", nil)

	into := `{"version":4,"source":"zoo.pets","columns":[
		{"id":"name","path":[{"field":"name"}]},
		{"id":"meals","path":[{"link":"meals","mode":"aggregate"},{"field":"id"}],"aggregate":"count"}],
		"sort":[{"col":"name"}]}`
	res = z.run(t, "ann", into, Options{}, nil)
	requireOrderedRows(t, into, res, "label", "Kit", "Leo", "Zed", "Total")
	requireRows(t, res, "meals", int64(1), int64(3), int64(2), int64(6))
}

// A lookup sums one amount across both parts: one source, one currency.
func TestALookupSumsAcrossTheParts(t *testing.T) {
	z := newUnionZoo(t)
	spec := `{"version":4,"source":"zoo.pets","columns":[
		{"id":"name","path":[{"field":"name"}]},
		{"id":"pet","path":[{"field":"id"}]},
		{"id":"meals","formula":{"op":"lookup","source":"zoo.meals","pick":"sum",
			"where":{"op":"eq","args":[{"path":[{"link":"animal"}]},{"col":"pet"}]},
			"value":{"path":[{"field":"cost"}]}}}],
		"sort":[{"col":"name"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireOrderedRows(t, spec, res, "label", "Kit", "Leo", "Zed", "Total")
	requireRows(t, res, "meals", int64(40), int64(3525), int64(1000), nil)
	requireRows(t, res, "meals@currency", "USD", "USD", "USD", nil)
}

// A record group on a link into a union is labelled by the union record, and
// its fill adds the union records with no rows.
func TestARecordGroupOnAUnionLinkAndItsFill(t *testing.T) {
	z := newUnionZoo(t)
	spec := `{"version":4,"source":"zoo.tags","columns":[
		{"id":"meal","path":[{"link":"meal"}],"labelField":"note"},
		{"id":"tags","path":[{"field":"tag"}],"aggregate":"count"}],
		"groups":[{"id":"g","col":"meal"%s}]}`
	res := z.run(t, "ann", fmt.Sprintf(spec, ""), Options{}, nil)
	requireRows(t, res, "label", "f1", "f3", "Total")
	requireRows(t, res, "tags", int64(1), int64(1), int64(2))

	filled := z.run(t, "ann", fmt.Sprintf(spec, `,"fill":{}`), Options{}, nil)
	requireRows(t, filled, "label", "f1", "f2", "f3", "f4", "t1", "t3", "Total")
}

// A source-backed param over a union lists the union's records the reader
// may see, from both parts.
func TestAParamListsItsOptionsFromAUnion(t *testing.T) {
	z := newUnionZoo(t)
	spec := `{"version":4,"source":"zoo.meals",
		"params":[{"id":"meal","type":"choice","multiple":true,
			"optionsFrom":{"source":"zoo.meals","value":[{"field":"id"}],"label":[{"field":"note"}]}}],
		"columns":[{"id":"note","path":[{"field":"note"}]}],
		"filter":{"op":"in","args":[{"path":[{"field":"id"}]},{"param":"meal"}]}}`
	labels := func(user string) []string {
		got := optionLabels(z.options(t, user, spec, OptionsRequest{Param: "meal"}))
		slices.Sort(got)
		return got
	}
	if got := labels("ann"); !slices.Equal(got, []string{"f1", "f2", "f3", "f4", "t1", "t3"}) {
		t.Fatalf("ann's options = %v", got)
	}
	if got := labels("dan"); !slices.Equal(got, []string{"t4"}) {
		t.Fatalf("dan's options = %v", got)
	}
}

// Two parts whose list rules carry different literals build their params
// apart; the union reads each part under its own literal, for every reader.
func TestAUnionOfPartsWithLiteralRulesRuns(t *testing.T) {
	z := newZoo(t)
	for _, c := range []struct{ name, literal string }{{"zoo_alphas", "alpha"}, {"zoo_betas", "beta"}} {
		col := core.NewBaseCollection(c.name)
		col.ListRule = rule(`word = "` + c.literal + `"`)
		col.ViewRule = col.ListRule
		col.Fields.Add(&core.TextField{Name: "word"})
		z.save(t, col)
		z.rec(t, c.name+"_hit", c.name, map[string]any{"word": c.literal})
		z.rec(t, c.name+"_miss", c.name, map[string]any{"word": "other"})
	}
	RegisterSource(Source{
		Key: "zoo.words", Label: "Words",
		Parts: []SourcePart{
			{Collection: "zoo_alphas", Columns: map[string]string{"word": "word"}},
			{Collection: "zoo_betas", Columns: map[string]string{"word": "word"}},
		},
		Fields: []Field{{Key: "word", Label: "Word", Type: Text, Aggregates: MinMax}},
	})
	if err := CheckSources(z.app); err != nil {
		t.Fatalf("CheckSources: %v", err)
	}
	spec := `{"version":4,"source":"zoo.words","columns":[{"id":"word","path":[{"field":"word"}]}],"sort":[{"col":"word"}]}`
	for _, user := range []string{"ann", "bob"} {
		res := z.run(t, user, spec, Options{}, nil)
		requireOrderedRows(t, spec, res, "word", "alpha", "beta", nil)
	}
}
