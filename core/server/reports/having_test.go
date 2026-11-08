package reports

import (
	"reflect"
	"strings"
	"testing"
)

const speciesHaving = `{"version":4,"source":"zoo.feedings","params":[{"id":"min","type":"number"}],"columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"species","having":{"op":"gte","args":[
		{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"param":"min"}]}}],
	"sort":[{"col":"species"},{"col":"note"}]}`

// Having hides groups; a condition on an unset param hides none. The
// default total still adds every base row (OQ6).
func TestHavingHidesGroupsAndAnUnsetParamHidesNone(t *testing.T) {
	z := newZoo(t)
	all := z.run(t, "ann", speciesHaving, Options{}, nil)
	requireRows(t, all, "label", "lion", "zebra", "Total")
	some := z.run(t, "ann", speciesHaving, Options{ExpandAll: true}, map[string]string{"min": "1500"})
	requireRows(t, some, "label", "lion", "lion", "lion", "Total")
	requireOrderedRows(t, speciesHaving, some, "cost", int64(3500), int64(1000), int64(2500), int64(4500))
}

const hiddenParent = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"species","having":{"op":"eq","args":[{"col":"species"},{"value":"lion"}]}},{"id":"h","col":"note"}],
	"sort":[{"col":"note"}]}`

// A hidden group hides the groups under it.
func TestAHiddenGroupHidesItsChildren(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", hiddenParent, Options{ExpandAll: true}, nil)
	// Each note group opens on its one feeding, labelled by the first column.
	requireRows(t, res, "label", "lion", "f1", "lion", "f2", "lion", "Total")
}

// With groups expanded by key, the children of an expanded hidden group stay
// hidden too.
func TestAHiddenGroupHidesItsChildrenWhenExpandedByKey(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", hiddenParent, Options{Expand: [][]any{{"lion", "USD"}, {"zebra", "USD"}}}, nil)
	requireRows(t, res, "label", "lion", "f1", "f2", "Total")
}

const lionHidden = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
	{"id":"running","formula":{"op":"runningSum","args":[{"col":"cost"}]}},
	{"id":"ofParent","formula":{"op":"percentOfParent","args":[{"col":"cost"}],"scale":1}},
	{"id":"ofTotal","formula":{"op":"percentOfTotal","args":[{"col":"cost"}],"scale":1}}],
	"groups":[{"id":"g","col":"species","having":{"op":"lt","args":[
		{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":2000}]}}],
	"sort":[{"col":"species"},{"col":"note"}]}`

// Regression guard: the rows of a hidden group take no place on a page:
// lion's two feedings sort first, and with them a page of 2 rows would hold no
// zebra row. Two filters keep them out (the detail pass reads only the open
// groups the group pass shows, and only rows under shown groups), so the test
// fails only when both stop working.
func TestHiddenRowsTakeNoPlaceOnAPage(t *testing.T) {
	z := newZoo(t)
	withLimits(t, func(l *limits) { l.pageSize = 2 })
	res := z.run(t, "ann", lionHidden, Options{ExpandAll: true}, nil)
	requireOrderedRows(t, lionHidden, res, "note", nil, "f3", "f4", nil)
	if len(res.Notices) != 0 || res.Page == nil || res.Page.Next != "" {
		t.Fatalf("notices = %+v, page = %+v", res.Notices, res.Page)
	}
}

// A running value or a percent adds the rows the reader sees: a hidden group
// adds nothing to them. The total still adds every base row (OQ6).
func TestRunningValuesReadTheShownRowsOnly(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", lionHidden, Options{ExpandAll: true}, nil)
	requireOrderedRows(t, lionHidden, res, "note", nil, "f3", "f4", nil)
	requireOrderedRows(t, lionHidden, res, "running", int64(1000), int64(700), int64(1000), int64(4500))
	requireOrderedRows(t, lionHidden, res, "ofParent", 100.0, 70.0, 30.0, 100.0)
	requireOrderedRows(t, lionHidden, res, "ofTotal", 100.0, 70.0, 30.0, 100.0)
}

// A having on a deeper group hides its rows, but its parent's subtotal still
// adds every base row under the parent: lion's 3500 adds the hidden f1. Zebra,
// whose note groups are all hidden, stays closed and shows its own figures.
func TestADeeperHavingLeavesTheParentSubtotal(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species","subtotals":true},{"id":"h","col":"note","having":{"op":"gt","args":[{"col":"cost"},{"value":1500}]}}],
		"sort":[{"col":"species"},{"col":"note"}]}`, Options{Expand: [][]any{{"lion", "USD"}, {"zebra", "USD"}}}, nil)
	requireRows(t, res, "kind", KindGroup, KindGroup, KindSubtotal, KindGroup, KindTotal)
	requireRows(t, res, "label", "lion", "f2", "Total lion", "zebra", "Total")
	requireRows(t, res, "cost", int64(3500), int64(2500), int64(3500), int64(1000), int64(4500))
	if zebra := res.Rows[3]; zebra.Expanded || zebra.CanExpand {
		t.Fatalf("zebra = %+v, want a closed group that cannot open", zebra)
	}
}

func TestHavingReadsARecordField(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal","having":{"op":"gt","args":[{"record":"g","path":[{"field":"weight"}]},{"value":200}]}}]}`, Options{}, nil)
	requireRows(t, res, "label", "Zed", "Total")
}

// A having that totals an amount no column shows, in two currencies, still
// groups by the amount's currency: its key is known before the levels (F5).
func TestHavingOnAnAmountNoColumnShows(t *testing.T) {
	z := newZoo(t)
	z.rec(t, "f7", "zoo_feedings", map[string]any{"animal": z.id("leo"), "cost": 100, "currency": "EUR",
		"fed": "2026-01-07 00:00:00.000Z", "grams": 10, "note": "f7", "due": "2026-01-07"})
	res := z.run(t, "ann", `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"n","path":[{"field":"id"}],"aggregate":"count"}],
		"groups":[{"id":"g","col":"species","having":{"op":"gt","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":150}]}}],
		"sort":[{"col":"species"}]}`, Options{}, nil)
	requireRows(t, res, "label", "Total", "lion", "zebra", "Total")
	requireRows(t, res, "n", int64(1), int64(2), int64(2), int64(4))
}

// A having that totals a record's amount, which carries its record's currency,
// groups by that currency though no column reads it (valueCurrencies): leo's
// fees are 2 × 700 EUR, zed's 2 × 300 USD.
func TestHavingOnACarriedAmountGroupsByItsCurrency(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"n","path":[{"field":"id"}],"aggregate":"count"}],
		"groups":[{"id":"g","col":"animal","having":{"op":"gt","args":[
			{"op":"sum","args":[{"record":"g","path":[{"field":"fee"}]}]},{"value":1000}]}}]}`, Options{}, nil)
	requireRows(t, res, "label", "Leo", "Total", "Total")
	requireRows(t, res, "n", int64(2), int64(2), int64(2))
}

// A fill record's totals are those of no rows (a sum 0), so a having reads
// them as it reads any group's.
func TestHavingOnFillRows(t *testing.T) {
	z := newZoo(t)
	withEmptyAnimals(t, z)
	res := z.run(t, "ann", `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal",
			"fill":{"where":{"op":"eq","args":[{"path":[{"field":"active"}]},{"value":true}]}},
			"having":{"op":"lt","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":1}]}}],
		"sort":[{"formula":{"record":"g","path":[{"field":"name"}]}}]}`, Options{}, nil)
	requireRows(t, res, "label", "Eli", "Kit", "Lia", "Total")
	requireRows(t, res, "cost", int64(0), int64(0), int64(0), int64(4500))
}

// A having that reads a formula column reads the value its cell shows: the
// column's condition on an unset param is blank, as it is in the cell (and as
// a filter reads a column), so no group passes.
func TestHavingReadsAColumnAsItsCellShowsIt(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":4,"source":"zoo.feedings","params":[{"id":"min","type":"number"}],"columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
		{"id":"big","formula":{"op":"gte","args":[{"col":"cost"},{"param":"min"}]}}],
		"groups":[{"id":"g","col":"species","having":{"col":"big"}}],"sort":[{"col":"species"}]}`
	set := z.run(t, "ann", spec, Options{}, map[string]string{"min": "1500"})
	requireRows(t, set, "label", "lion", "Total")
	requireRows(t, set, "big", true, true)
	unset := z.run(t, "ann", spec, Options{}, nil)
	requireRows(t, unset, "label", "Total")
}

func TestHavingIsRefusedWhereItCannotWork(t *testing.T) {
	app := withZooRegistry(t)
	for _, c := range []struct{ spec, at, want string }{
		{`{"version":4,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]}],"groups":[{"id":"g","col":"n","having":{"value":3}}]}`,
			"groups.0.having", "having is a condition"},
		{`{"version":4,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]},{"id":"c","path":[{"field":"cost"}],"aggregate":"sum"}],
			"groups":[{"id":"g","col":"n","having":{"op":"gt","args":[{"op":"percentOfTotal","scale":2,"args":[{"col":"c"}]},{"value":10}]}}]}`,
			"groups.0.having", "not a condition"},
		{`{"version":4,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]},{"id":"c","path":[{"field":"cost"}],"aggregate":"sum"}],
			"groups":[{"id":"g","col":"n","having":{"op":"gt","args":[{"col":"c"},{"value":0}]}}],"layout":[{"section":"s","title":"S","key":"f1"}]}`,
			"groups.0.having", "a layout's sections are fixed rows"},
		{`{"version":4,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]}],
			"groups":[{"id":"g","col":"n","having":{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"f1"}]}}]}`,
			"groups.0.having", "not a value of one row"},
	} {
		spec, _, err := Load([]byte(c.spec))
		if err != nil {
			t.Fatal(err)
		}
		requireSpecError(t, Validate(app, spec), 400, c.at, c.want)
	}
}

// Validate checks a having as it checks a group label: its size with the
// columns it reads, and its lookups' values.
func TestValidateChecksAHaving(t *testing.T) {
	app := withZooRegistry(t)
	big := strings.TrimSuffix(chainSpec(8, doubling), "}") + `,"groups":[{"id":"g","col":"c0","having":
		{"op":"eq","args":[{"op":"concat","args":[{"col":"c8"},{"col":"c8"}]},{"value":"x"}]}}]}`
	requireSpecError(t, Validate(app, loadSpec(t, big)), 400, "groups.0.having", "with the formula columns it reads")
	bad := `{"version":4,"source":"zoo.feedings","columns":[{"id":"a","path":[{"link":"animal"}]}],
		"groups":[{"id":"g","col":"a","having":{"op":"lookup","source":"zoo.sites","value":{"nope":1}}}]}`
	requireSpecError(t, Validate(app, loadSpec(t, bad)), 400, "groups.0.having.value", "a lookup's value is a formula")
}

// A group whose every child group its having hides opens onto nothing, so it
// is not expandable; a group with one shown child is. Lion's feedings cost
// 1000 and 2500, zebra's 500 each: only lion keeps a note group over 2000.
func TestAGroupWhoseChildrenAreAllHiddenDoesNotExpand(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"},{"id":"h","col":"note","having":{"op":"gte","args":[
			{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":2000}]}}],
		"sort":[{"col":"species"},{"col":"note"}]}`
	for _, opts := range []Options{{}, {ExpandAll: true}} {
		res := z.run(t, "ann", spec, opts, nil)
		expandable := map[string]bool{}
		for _, row := range res.Rows {
			if row.Kind == KindGroup {
				expandable[row.Label] = row.CanExpand
			}
		}
		want := map[string]bool{"lion": true, "zebra": false}
		if opts.ExpandAll {
			// A note group opens onto its feeding.
			want["f2"] = true
		}
		if !reflect.DeepEqual(expandable, want) {
			t.Errorf("expandAll %v: canExpand = %v, want %v", opts.ExpandAll, expandable, want)
		}
	}
}

// The shown groups of a level are computed once: the detail rows under them
// do not total the level again for each row.
func TestShownGroupsAreComputedOnce(t *testing.T) {
	z := newZoo(t)
	withPlainFeedingsRule(t, z)
	seen := observeReads(t)
	z.run(t, "ann", speciesHaving, Options{ExpandAll: true}, map[string]string{"min": "2000"})
	if reads := perRowReadsOf(t, z, *seen, "zoo_feedings"); len(reads) != 0 {
		t.Fatalf("a subquery reads the feedings for each row: %v", reads)
	}
}
