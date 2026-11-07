package reports

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

const siteNorth = `{"op":"lookup","source":"zoo.sites",
	"where":{"op":"eq","args":[{"path":[{"field":"region"}]},{"value":"north"}]},
	"value":{"path":[{"field":"name"}]}}`

// A lookup reads one value from another source's rows the reader may list.
func TestALookupReadsAnotherSourceUnderTheReadersRules(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"site","formula":` + siteNorth + `}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	// A lookup that reads no report value is one value for the whole report,
	// as a formula of values is: the total row shows it too.
	requireRows(t, res, "site", "North", "North", "North", "North", "North")
	if !slices.Contains(res.Topics, "collection/zoo_sites") {
		t.Fatalf("topics = %v", res.Topics)
	}
	// zoo_sites lists only members: a guest's lookup finds nothing.
	guest := z.person(t, "gus", "guest")
	z.rec(t, "kg", "zoo_keepers", map[string]any{"user": guest.Id, "animal": z.id("leo")})
	res = z.run(t, "gus", spec, Options{}, nil)
	requireRows(t, res, "site", nil, nil, nil)
}

// A lookup that reads a report column is correlated: each row looks up its own.
func TestALookupMayReadTheRowsColumns(t *testing.T) {
	z := newZoo(t)
	const spec = `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"weight","formula":{"op":"lookup","source":"zoo.animals",
			"where":{"op":"eq","args":[{"path":[{"field":"id"}]},{"col":"animal"}]},
			"value":{"path":[{"field":"weight"}]}}}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireOrderedRows(t, spec, res, "weight", int64(190), int64(190), int64(300), int64(300), nil)
}

func TestALookupInTheFilterPicksMinOrMaxAndIsBlankWithNoMatch(t *testing.T) {
	z := newZoo(t)
	zedID := `{"op":"lookup","source":"zoo.animals","where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Zed"}]},"value":{"path":[{"field":"id"}]}}`
	zeds := `{"version":2,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]}],
		"filter":{"op":"eq","args":[{"path":[{"link":"animal"}]},` + zedID + `]},"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", zeds, Options{}, nil)
	requireOrderedRows(t, zeds, res, "note", "f3", "f4", nil)

	heaviest := `{"op":"lookup","source":"zoo.animals","value":{"path":[{"field":"weight"}]},"pick":"max"}`
	none := `{"op":"lookup","source":"zoo.animals","where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Nobody"}]},"value":{"path":[{"field":"weight"}]}}`
	res = z.run(t, "ann", `{"version":2,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]},
		{"id":"heaviest","formula":`+heaviest+`},{"id":"none","formula":`+none+`}],"sort":[{"col":"name"}]}`, Options{}, nil)
	requireRows(t, res, "heaviest", int64(300), int64(300), int64(300))
	requireRows(t, res, "none", nil, nil, nil)
}

// zoo_animals lists only an animal's owner and keepers: ann cannot see Mo, so
// her lookup of Mo finds nothing, and Mo's weight (the lightest but Kit's)
// never reaches her pick.
func TestALookupNeverReadsARowTheReaderCannotList(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":2,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]},
		{"id":"mo","formula":{"op":"lookup","source":"zoo.animals","where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Mo"}]},"value":{"path":[{"field":"weight"}]}}},
		{"id":"lightest","formula":{"op":"lookup","source":"zoo.animals","where":{"op":"ne","args":[{"path":[{"field":"name"}]},{"value":"Kit"}]},"value":{"path":[{"field":"weight"}]}}}],
		"sort":[{"col":"name"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireRows(t, res, "mo", nil, nil, nil)
	requireRows(t, res, "lightest", int64(190), int64(190), int64(190))
	res = z.run(t, "bob", spec, Options{}, nil)
	requireRows(t, res, "mo", int64(12), int64(12), int64(12))
	requireRows(t, res, "lightest", int64(12), int64(12), int64(12))
}

// On a group row a correlated lookup reads the group's key; on the total row,
// where the column it reads has no single value, it is blank, even when its
// where would match a blank (ne). lighter reads another lookup column: the
// heaviest animal ann may list that is lighter than the group's (Mo, 12, is
// lighter than Leo but hidden from her, so Leo's group reads Kit's 4).
func TestACorrelatedLookupOnAGroupRowReadsTheGroupsKey(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"weight","formula":{"op":"lookup","source":"zoo.animals",
			"where":{"op":"eq","args":[{"path":[{"field":"id"}]},{"col":"animal"}]},
			"value":{"path":[{"field":"weight"}]}}},
		{"id":"other","formula":{"op":"lookup","source":"zoo.animals","pick":"max",
			"where":{"op":"ne","args":[{"path":[{"field":"id"}]},{"col":"animal"}]},
			"value":{"path":[{"field":"weight"}]}}},
		{"id":"lighter","formula":{"op":"lookup","source":"zoo.animals","pick":"max",
			"where":{"op":"lt","args":[{"path":[{"field":"weight"}]},{"col":"weight"}]},
			"value":{"path":[{"field":"weight"}]}}},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal"}],"sort":[{"col":"weight"}]}`, Options{ExpandAll: true}, nil)
	requireRows(t, res, "kind", "group", "detail", "detail", "group", "detail", "detail", "total")
	requireRows(t, res, "weight", int64(190), int64(190), int64(190), int64(300), int64(300), int64(300), nil)
	requireRows(t, res, "other", int64(300), int64(300), int64(300), int64(190), int64(190), int64(190), nil)
	requireRows(t, res, "lighter", int64(4), int64(4), int64(4), int64(190), int64(190), int64(190), nil)
}

// A lookup may read a column that totals rows: on a detail row it is that
// row's amount, on a group or total row the level's total. Here: the heaviest
// animal ann may list whose weight times 10 fits the amount (Mo's 120 would
// fit 300, 700 and 1000 but she cannot list Mo).
func TestALookupReadsATotalColumnOnEveryRow(t *testing.T) {
	z := newZoo(t)
	const spec = `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"spent","formula":{"op":"sum","args":[{"path":[{"field":"cost"}]}]}},
		{"id":"fits","formula":{"op":"lookup","source":"zoo.animals","pick":"max",
			"where":{"op":"lte","args":[{"op":"mul","args":[{"path":[{"field":"weight"}]},{"value":10}]},{"col":"spent"}]},
			"value":{"path":[{"field":"weight"}]}}}],
		"groups":[{"id":"g","col":"species"}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
	requireRows(t, res, "kind", "group", "detail", "detail", "group", "detail", "detail", "total")
	requireOrderedRows(t, spec, res, "spent", int64(3500), int64(1000), int64(2500), int64(1000), int64(700), int64(300), int64(4500))
	requireOrderedRows(t, spec, res, "fits", int64(300), int64(4), int64(190), int64(4), int64(4), int64(4), int64(300))
}

// A lookup may read a running value: the outer select computes it before the
// lookup's subquery reads it, on detail rows (running within the group) and
// on group and total rows (running across the level's rows). fits is the
// heaviest animal ann may list whose weight times 15 is at most the running
// cost (Mo's 180 would fit 700 and 1000, but she cannot list Mo); lighter
// is the heaviest one lighter than fits.
func TestALookupReadsARunningValue(t *testing.T) {
	z := newZoo(t)
	const spec = `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
		{"id":"run","formula":{"op":"runningSum","args":[{"col":"cost"}]}},
		{"id":"fits","formula":{"op":"lookup","source":"zoo.animals","pick":"max",
			"where":{"op":"lte","args":[{"op":"mul","args":[{"path":[{"field":"weight"}]},{"value":15}]},{"col":"run"}]},
			"value":{"path":[{"field":"weight"}]}}},
		{"id":"lighter","formula":{"op":"lookup","source":"zoo.animals","pick":"max",
			"where":{"op":"lt","args":[{"path":[{"field":"weight"}]},{"col":"fits"}]},
			"value":{"path":[{"field":"weight"}]}}}],
		"groups":[{"id":"g","col":"species"}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
	requireRows(t, res, "kind", "group", "detail", "detail", "group", "detail", "detail", "total")
	requireOrderedRows(t, spec, res, "run", int64(3500), int64(1000), int64(3500), int64(4500), int64(700), int64(1000), int64(4500))
	requireOrderedRows(t, spec, res, "fits", int64(190), int64(4), int64(190), int64(300), int64(4), int64(4), int64(300))
	// A lookup of that lookup reads it computed too.
	requireOrderedRows(t, spec, res, "lighter", int64(4), nil, int64(4), int64(190), nil, nil, int64(190))
}

// A computed layout row may look a value up (F16): here a section's cost
// plus the weight of Leo.
func TestALayoutRowLooksAValueUp(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"}],
		"layout":[{"section":"l","title":"Lions","key":"lion"},
			{"row":"r","label":"Lions and Leo","formula":{"op":"add","args":[{"subtotal":"l"},
				{"op":"lookup","source":"zoo.animals","where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Leo"}]},"value":{"path":[{"field":"weight"}]}}]}},
			{"row":"m","label":"Lions and Mo","formula":{"op":"add","args":[{"subtotal":"l"},
				{"op":"lookup","source":"zoo.animals","where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Mo"}]},"value":{"path":[{"field":"weight"}]}}]}}]}`, Options{}, nil)
	// ann cannot list Mo: his lookup is blank and adds nothing.
	requireRows(t, res, "label", "Lions", "Total Lions", "Lions and Leo", "Lions and Mo")
	requireRows(t, res, "cost", int64(3500), int64(3500), int64(3690), int64(3500))
}

// A lookup in a computed row reads the run's params, and refuses a report
// column: the row's cell in that column is its own formula.
func TestALayoutLookupReadsParamsButNoColumn(t *testing.T) {
	z := newZoo(t)
	layout := func(where string) string {
		return `{"version":2,"source":"zoo.feedings","params":[{"id":"who","type":"text"}],"columns":[
			{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
			{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
			"groups":[{"id":"g","col":"species"}],
			"layout":[{"section":"l","title":"Lions","key":"lion"},
				{"row":"r","label":"Weight","formula":{"op":"lookup","source":"zoo.animals",
					"where":` + where + `,"value":{"path":[{"field":"weight"}]}}}]}`
	}
	byParam := `{"op":"eq","args":[{"path":[{"field":"name"}]},{"param":"who"}]}`
	res := z.run(t, "ann", layout(byParam), Options{}, map[string]string{"who": `"Zed"`})
	requireRows(t, res, "cost", int64(3500), int64(3500), int64(300))
	_, err := z.try(t, "ann", layout(`{"op":"eq","args":[{"path":[{"field":"name"}]},{"col":"species"}]}`), Options{}, nil)
	requireSpecError(t, err, 400, "layout.1.formula.where.args.1", "would read itself")
}

// A lookup in a computed row reads what the row's own formula reads: a
// section's subtotal and an earlier row, at the cell being computed. fits is
// the heaviest animal ann may list whose weight times 10 is at most the
// zebras' cost (1000); next is the lightest one heavier than fits. Mo (12)
// would be the answer to both, but ann cannot list him.
func TestALayoutLookupReadsSectionsAndEarlierRows(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"}],
		"layout":[{"section":"z","title":"Zebras","key":"zebra"},
			{"row":"fits","label":"Fits","formula":{"op":"lookup","source":"zoo.animals","pick":"max",
				"where":{"op":"lte","args":[{"op":"mul","args":[{"path":[{"field":"weight"}]},{"value":10}]},{"subtotal":"z"}]},
				"value":{"path":[{"field":"weight"}]}}},
			{"row":"next","label":"Next","formula":{"op":"lookup","source":"zoo.animals",
				"where":{"op":"gt","args":[{"path":[{"field":"weight"}]},{"subtotal":"fits"}]},
				"value":{"path":[{"field":"weight"}]}}}]}`, Options{}, nil)
	requireRows(t, res, "label", "Zebras", "Total Zebras", "Fits", "Next")
	requireRows(t, res, "cost", int64(1000), int64(1000), int64(4), int64(190))
}

func TestALookupIsRefusedWhereItCannotBeRead(t *testing.T) {
	app := withZooRegistry(t)
	cases := map[string]struct{ spec, at, want string }{
		"unknown source": {`{"version":2,"source":"zoo.feedings","columns":[{"id":"x","formula":{"op":"lookup","source":"zoo.gone","value":{"path":[{"field":"id"}]}}}]}`,
			"columns.0.formula.source", "does not exist"},
		"a total inside": {`{"version":2,"source":"zoo.feedings","columns":[{"id":"x","formula":{"op":"lookup","source":"zoo.animals","value":{"op":"sum","args":[{"path":[{"field":"weight"}]}]}}}]}`,
			"columns.0.formula.value", "one value per matching row"},
		"a bad pick": {`{"version":2,"source":"zoo.feedings","columns":[{"id":"x","formula":{"op":"lookup","source":"zoo.animals","value":{"path":[{"field":"id"}]},"pick":"avg"}}]}`,
			"columns.0.formula.pick", "min, max, sum or count"},
		"a source on another op": {`{"version":2,"source":"zoo.feedings","columns":[{"id":"x","formula":{"op":"add","source":"zoo.animals","args":[{"value":1},{"value":2}]}}]}`,
			"columns.0.formula", "only lookup takes a source"},
		"a leaf key too": {`{"version":2,"source":"zoo.feedings","columns":[{"id":"x","formula":{"op":"lookup","source":"zoo.animals","col":"x","value":{"path":[{"field":"id"}]}}}]}`,
			"columns.0.formula", "one of op, col, path"},
		"no source": {`{"version":2,"source":"zoo.feedings","columns":[{"id":"x","formula":{"op":"lookup","value":{"path":[{"field":"id"}]}}}]}`,
			"columns.0.formula.source", "a lookup names its source"},
		"a value that is not a formula": {`{"version":2,"source":"zoo.feedings","columns":[{"id":"x","formula":{"op":"lookup","source":"zoo.animals","value":{"field":"id"}}}]}`,
			"columns.0.formula.value", "a lookup's value is a formula"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			spec, _, err := Load([]byte(c.spec))
			if err != nil {
				t.Fatal(err)
			}
			err = Validate(app, spec)
			status := 400
			if name == "unknown source" {
				status = 422
			}
			requireSpecError(t, err, status, c.at, c.want)
		})
	}
}

// A lookup's where reads report columns, so the formula-size guard counts the
// columns it reads inlined: c<n> reads c<n-1> twice inside the lookup, and c8
// (1,531 nodes) is refused before anything compiles.
func TestALookupThatReadsAColumnTwiceCountsItTwice(t *testing.T) {
	app := withZooRegistry(t)
	step := `{"op":"lookup","source":"zoo.animals",
		"where":{"op":"eq","args":[{"path":[{"field":"weight"}]},{"op":"add","args":[{"col":"PREV"},{"col":"PREV"}]}]},
		"value":{"path":[{"field":"weight"}]}}`
	err := Validate(app, loadSpec(t, strings.Replace(chainSpec(20, step), `"version":1`, `"version":2`, 1)))
	requireSpecError(t, err, 400, "columns.8", "with the formula columns it reads")
	if _, ok := err.(*SpecError).Problems["columns.7"]; ok {
		t.Fatalf("c7 (763 nodes) was refused: %v", err)
	}
}

// sitesWithSums sums animals per site (a number), the fees of every animal
// but lions (an amount whose currency each row carries), and every price
// (an amount in the source's fixed currency).
const sitesWithSums = `{"version":3,"source":"zoo.sites","columns":[
	{"id":"name","path":[{"field":"name"}]},
	{"id":"site","path":[{"field":"id"}]},
	{"id":"weight","formula":{"op":"lookup","source":"zoo.animals","pick":"sum",
		"where":{"op":"eq","args":[{"path":[{"link":"site"}]},{"col":"site"}]},
		"value":{"path":[{"field":"weight"}]}}},
	{"id":"usdFees","formula":{"op":"lookup","source":"zoo.animals","pick":"sum",
		"where":{"op":"ne","args":[{"path":[{"field":"species"}]},{"value":"lion"}]},
		"value":{"path":[{"field":"fee"}]}}},
	{"id":"prices","formula":{"op":"lookup","source":"zoo.animals","pick":"sum","value":{"path":[{"field":"price"}]}}}],
	"sort":[{"col":"name"}]}`

// ann may list leo (north, 700 EUR), zed (south, 300 USD) and kit (no site,
// 50 USD); mo (north) is hidden from her.
func TestASumLookupAddsTheMatchingRows(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", sitesWithSums, Options{}, nil)
	requireOrderedRows(t, sitesWithSums, res, "label", "North", "South", "Total")
	// Correlated: each site adds its own animals; the total has no site.
	requireOrderedRows(t, sitesWithSums, res, "weight", int64(190), int64(300), nil)
	// One currency among the matched rows: the sum keeps it.
	requireRows(t, res, "usdFees", int64(350), int64(350), int64(350))
	requireRows(t, res, "usdFees@currency", "USD", "USD", "USD")
	requireRows(t, res, "prices", int64(625000), int64(625000), int64(625000))
	for _, c := range res.Columns {
		if c.ID == "prices" && (c.Type != Money || c.Currency != "USD") {
			t.Fatalf("prices column = %+v", c)
		}
		if c.ID == "weight" && c.Type != Number {
			t.Fatalf("weight column = %+v", c)
		}
	}
	if len(res.Notices) != 0 {
		t.Fatalf("notices = %+v", res.Notices)
	}
}

// bob's animals but lions are zed (300 USD) and mo (20 GBP): their fees
// cannot be added, so the cell is blank and a notice names the column. When
// the totals' notice is due too, the two are one notice.
func TestASumLookupOverTwoCurrenciesHasNoValueAndANotice(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "bob", sitesWithSums, Options{}, nil)
	requireRows(t, res, "usdFees", nil, nil, nil)
	want := []Notice{{Code: "mixedCurrency", Message: "A looked-up total mixes currencies, so it shows no value.", Columns: []string{"usdFees"}}}
	if !reflect.DeepEqual(res.Notices, want) {
		t.Fatalf("notices = %+v", res.Notices)
	}

	const animals = `{"version":3,"source":"zoo.animals","columns":[
		{"id":"name","path":[{"field":"name"}]},
		{"id":"fee","path":[{"field":"fee"}],"aggregate":"sum"},
		{"id":"allFees","formula":{"op":"lookup","source":"zoo.animals","pick":"sum","value":{"path":[{"field":"fee"}]}}}],
		"sort":[{"col":"name"}]}`
	res = z.run(t, "ann", animals, Options{}, nil)
	for i, v := range rowsOf(res, "allFees") {
		if v != nil {
			t.Fatalf("row %d allFees = %#v, want blank", i, v)
		}
	}
	want = []Notice{{Code: "mixedCurrency", Message: "Amounts in different currencies are totalled apart, one row per currency.", Columns: []string{"fee", "allFees"}}}
	if !reflect.DeepEqual(res.Notices, want) {
		t.Fatalf("notices = %+v", res.Notices)
	}
}

// An aggregate over a sum lookup that met two currencies adds rows that have
// no value: the group and total rows are blank too, and the notice names the
// column even when no detail row is shown.
func TestAnAggregateOverAMixedSumLookupHasNoValueAndANotice(t *testing.T) {
	z := newZoo(t)
	const lookup = `{"op":"lookup","source":"zoo.animals","pick":"sum",
		"where":{"op":"ne","args":[{"path":[{"field":"species"}]},{"value":"lion"}]},
		"value":{"path":[{"field":"fee"}]}}`
	byName := `"groups":[{"id":"g","col":"name"}]`
	byRegionAndName := `"groups":[{"id":"r","col":"region"},{"id":"g","col":"name"}]`
	specs := map[string]string{
		"aggregate":            `{"id":"usdFees","aggregate":"sum","formula":` + lookup + `}],` + byName,
		"sum op":               `{"id":"usdFees","formula":{"op":"sum","args":[` + lookup + `]}}],` + byName,
		"rows total":           `{"id":"usdFees","total":"rows","formula":` + lookup + `}],` + byName,
		"rows total, 2 levels": `{"id":"usdFees","total":"rows","formula":` + lookup + `}],` + byRegionAndName,
	}
	want := []Notice{{Code: "mixedCurrency", Message: "A looked-up total mixes currencies, so it shows no value.", Columns: []string{"usdFees"}}}
	for name, cols := range specs {
		spec := `{"version":3,"source":"zoo.sites","columns":[
			{"id":"name","path":[{"field":"name"}]},{"id":"region","path":[{"field":"region"}]},` + cols + `,
			"sort":[{"col":"name"}]}`
		for _, opts := range []Options{{}, {ExpandAll: true}} {
			res := z.run(t, "bob", spec, opts, nil)
			for i, v := range rowsOf(res, "usdFees") {
				if v != nil {
					t.Errorf("%s, expand %v: row %d usdFees = %#v, want blank", name, opts.ExpandAll, i, v)
				}
			}
			if !reflect.DeepEqual(res.Notices, want) {
				t.Errorf("%s, expand %v: notices = %+v", name, opts.ExpandAll, res.Notices)
			}
		}
	}
}

// mixedFees is bob's sum lookup of every fee but lions': zed's 300 USD and
// mo's 20 GBP, which cannot be added.
const mixedFees = `{"op":"lookup","source":"zoo.animals","pick":"sum",
	"where":{"op":"ne","args":[{"path":[{"field":"species"}]},{"value":"lion"}]},
	"value":{"path":[{"field":"fee"}]}}`

var mixedFeesNotice = []Notice{{Code: "mixedCurrency", Message: "A looked-up total mixes currencies, so it shows no value.", Columns: []string{"usdFees"}}}

// requireMixedFeesBlank runs spec as bob, collapsed and expanded, and wants
// every usdFees cell (a pivot's cells too) blank, with the one notice.
func requireMixedFeesBlank(t *testing.T, z *zoo, name, spec string) {
	t.Helper()
	for _, opts := range []Options{{}, {ExpandAll: true}} {
		res := z.run(t, "bob", spec, opts, nil)
		cells := 0
		for i, r := range res.Rows {
			for k, v := range r.Cells {
				if k != "usdFees" && !strings.HasPrefix(k, "usdFees.p") {
					continue
				}
				cells++
				if pivotCellOfNoSite(r, k) {
					continue
				}
				if v != nil {
					t.Errorf("%s, expand %v: row %d (%s %v) %s = %#v, want blank", name, opts.ExpandAll, i, r.Kind, r.Label, k, v)
				}
			}
		}
		if cells == 0 {
			t.Errorf("%s, expand %v: no usdFees cells", name, opts.ExpandAll)
		}
		if !reflect.DeepEqual(res.Notices, mixedFeesNotice) {
			t.Errorf("%s, expand %v: notices = %+v", name, opts.ExpandAll, res.Notices)
		}
	}
}

// pivotCellOfNoSite is true for a pivot cell by region (p0 north, p1 south)
// on a row of the other region's site: it adds no row, so it is 0, not
// blank. The sites are named as their regions.
func pivotCellOfNoSite(r Row, k string) bool {
	label := r.Label
	return (strings.Contains(label, "North") && k == "usdFees.p1") || (strings.Contains(label, "South") && k == "usdFees.p0")
}

// A pivot cell over a mixed sum lookup is blank, not 0, on the innermost
// group, above it ("rows"), and on the total.
func TestAPivotCellOverAMixedSumLookupHasNoValue(t *testing.T) {
	z := newZoo(t)
	specs := map[string]string{
		"aggregate": `{"id":"usdFees","aggregate":"sum","formula":` + mixedFees + `}],
			"groups":[{"id":"g","col":"name"}]`,
		"rows total, 2 levels": `{"id":"usdFees","aggregate":"sum","total":"rows","formula":` + mixedFees + `}],
			"groups":[{"id":"r","col":"region"},{"id":"g","col":"name"}]`,
	}
	for name, cols := range specs {
		spec := `{"version":3,"source":"zoo.sites","columns":[
			{"id":"name","path":[{"field":"name"}]},{"id":"region","path":[{"field":"region"}]},` + cols + `,
			"pivot":{"path":[{"field":"region"}],"cell":"usdFees"},"sort":[{"col":"name"}]}`
		requireMixedFeesBlank(t, z, name, spec)
	}
}

// A computed layout row that reads a mixed (blank) section is blank, in the
// column's cell and in each pivot cell, not the section counted as 0.
func TestALayoutRowOverAMixedSectionHasNoValue(t *testing.T) {
	z := newZoo(t)
	const layout = `"layout":[{"section":"n","title":"North","key":"north"},
		{"row":"p","label":"North plus one","formula":{"op":"add","args":[{"subtotal":"n"},{"value":1}]}}]`
	cols := `{"id":"region","path":[{"field":"region"}]},{"id":"usdFees","aggregate":"sum","formula":` + mixedFees + `}],
		"groups":[{"id":"r","col":"region"}],`
	requireMixedFeesBlank(t, z, "layout", `{"version":3,"source":"zoo.sites","columns":[`+cols+layout+`}`)
	requireMixedFeesBlank(t, z, "layout with a pivot", `{"version":3,"source":"zoo.sites","columns":[`+cols+
		`"pivot":{"path":[{"field":"name"}],"cell":"usdFees"},`+layout+`}`)
}

// A formula that wraps a mixed sum lookup (coalesce, if) does not turn its
// blank into a value: the cell is blank on detail, group and total rows.
func TestAFormulaOverAMixedSumLookupHasNoValue(t *testing.T) {
	z := newZoo(t)
	formulas := map[string]string{
		"coalesce": `{"op":"coalesce","args":[` + mixedFees + `,{"value":0}]}`,
		"if":       `{"op":"if","args":[{"value":true},{"op":"coalesce","args":[` + mixedFees + `,{"value":0}]},{"value":0}]}`,
	}
	for name, f := range formulas {
		for _, groups := range []string{``, `,"groups":[{"id":"g","col":"name"}]`} {
			for _, agg := range []string{``, `"aggregate":"sum",`} {
				spec := `{"version":3,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]},
					{"id":"usdFees",` + agg + `"formula":` + f + `}]` + groups + `,"sort":[{"col":"name"}]}`
				requireMixedFeesBlank(t, z, name+groups+agg, spec)
			}
		}
	}
}

// A count is a number, never blank for currencies: a count lookup counts
// amounts in two currencies, and a count of a mixed sum lookup counts its
// rows, with no notice.
func TestACountIsNeverBlankForCurrencies(t *testing.T) {
	z := newZoo(t)
	const fees = `{"op":"lookup","source":"zoo.animals","pick":"count",
		"where":{"op":"ne","args":[{"path":[{"field":"species"}]},{"value":"lion"}]},
		"value":{"path":[{"field":"fee"}]}}`
	spec := `{"version":3,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]},
		{"id":"n","formula":` + fees + `}],"sort":[{"col":"name"}]}`
	res := z.run(t, "bob", spec, Options{}, nil)
	requireRows(t, res, "n", int64(2), int64(2), int64(2))
	if len(res.Notices) != 0 {
		t.Fatalf("count lookup: notices = %+v", res.Notices)
	}

	byName := `"groups":[{"id":"g","col":"name"}]`
	specs := map[string]string{
		"aggregate": `{"id":"n","aggregate":"count","formula":` + mixedFees + `}],` + byName,
		"count op":  `{"id":"n","formula":{"op":"count","args":[` + mixedFees + `]}}],` + byName,
		"pivot":     `{"id":"n","aggregate":"count","formula":` + mixedFees + `}],` + byName + `,"pivot":{"path":[{"field":"region"}],"cell":"n"}`,
	}
	for name, cols := range specs {
		spec := `{"version":3,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]},` + cols + `,"sort":[{"col":"name"}]}`
		for _, opts := range []Options{{}, {ExpandAll: true}} {
			res := z.run(t, "bob", spec, opts, nil)
			// A detail row of an aggregate column shows the looked-up amount
			// itself, which is blank with the notice; only its count is a
			// number.
			amountShown := name == "aggregate" && opts.ExpandAll
			for i, r := range res.Rows {
				var want any = int64(1)
				switch {
				case r.Kind == "total":
					want = int64(2)
				case r.Kind == "detail" && amountShown:
					want = nil
				}
				if got := r.Cells["n"]; got != want {
					t.Errorf("%s, expand %v: row %d (%s) n = %#v, want %d", name, opts.ExpandAll, i, r.Kind, got, want)
				}
				// The pivot cells count the row's sites by region.
				var pivoted any = int64(0)
				for k, v := range r.Cells {
					if strings.HasPrefix(k, "n.p") {
						if v == nil {
							pivoted = nil
							break
						}
						pivoted = pivoted.(int64) + v.(int64)
					}
				}
				if name == "pivot" && pivoted != want {
					t.Errorf("%s, expand %v: row %d pivot cells add to %#v, want %d", name, opts.ExpandAll, i, pivoted, want)
				}
			}
			wantNotices := 0
			if amountShown {
				wantNotices = 1
			}
			if len(res.Notices) != wantNotices {
				t.Errorf("%s, expand %v: notices = %+v", name, opts.ExpandAll, res.Notices)
			}
		}
	}
}

func TestAnEmptyMatchIsBlankForSumAndZeroForCount(t *testing.T) {
	z := newZoo(t)
	nobody := `"where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Nobody"}]},"value":{"path":[{"field":"weight"}]}`
	spec := `{"version":3,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]},
		{"id":"sum","formula":{"op":"lookup","source":"zoo.animals","pick":"sum",` + nobody + `}},
		{"id":"count","formula":{"op":"lookup","source":"zoo.animals","pick":"count",` + nobody + `}}],
		"sort":[{"col":"name"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireRows(t, res, "sum", nil, nil, nil)
	requireRows(t, res, "count", int64(0), int64(0), int64(0))
}

// count counts the matching rows whose value is not blank: kit has no birth
// day, so ann's animals with one are leo and zed.
func TestACountLookupCountsRowsWithAValue(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":3,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]},
		{"id":"site","path":[{"field":"id"}]},
		{"id":"born","formula":{"op":"lookup","source":"zoo.animals","pick":"count","value":{"path":[{"field":"born"}]}}},
		{"id":"here","formula":{"op":"lookup","source":"zoo.animals","pick":"count",
			"where":{"op":"eq","args":[{"path":[{"link":"site"}]},{"col":"site"}]},"value":{"path":[{"field":"name"}]}}}],
		"sort":[{"col":"name"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireRows(t, res, "born", int64(2), int64(2), int64(2))
	requireOrderedRows(t, spec, res, "here", int64(1), int64(1), nil)
	for _, c := range res.Columns {
		if (c.ID == "born" || c.ID == "here") && c.Type != Number {
			t.Fatalf("%s column = %+v", c.ID, c)
		}
	}
}

func TestPickSumNeedsANumberOrAnAmount(t *testing.T) {
	app := withZooRegistry(t)
	lookup := func(pick, field string) string {
		return `{"version":3,"source":"zoo.sites","columns":[{"id":"x","formula":{"op":"lookup","source":"zoo.animals",
			"pick":"` + pick + `","value":{"path":[{"field":"` + field + `"}]}}}]}`
	}
	for _, field := range []string{"name", "born", "active"} {
		err := Validate(app, loadSpec(t, lookup("sum", field)))
		requireSpecError(t, err, 400, "columns.0.formula.value", "pick sum adds numbers or amounts")
	}
	if err := Validate(app, loadSpec(t, lookup("count", "name"))); err != nil {
		t.Fatalf("a count of names: %v", err)
	}
}

// feedingsWithOuter reads the report's row from inside lookups: earlier
// counts ann's feedings fed before the row's (f1 01-05, f3 01-20, f2 02-10,
// f4 03-03), other is the other feeding of the row's animal, site the name of
// the row's animal's site, and nested counts the earlier feedings inside a
// lookup of another source.
const feedingsWithOuter = `{"version":3,"source":"zoo.feedings","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"earlier","formula":{"op":"lookup","source":"zoo.feedings","pick":"count",
		"where":{"op":"lt","args":[{"path":[{"field":"fed"}]},{"outer":[{"field":"fed"}]}]},
		"value":{"path":[{"field":"id"}]}}},
	{"id":"other","formula":{"op":"lookup","source":"zoo.feedings",
		"where":{"op":"and","args":[
			{"op":"eq","args":[{"path":[{"link":"animal"}]},{"outer":[{"link":"animal"}]}]},
			{"op":"ne","args":[{"path":[{"field":"note"}]},{"outer":[{"field":"note"}]}]}]},
		"value":{"path":[{"field":"note"}]}}},
	{"id":"site","formula":{"op":"lookup","source":"zoo.sites",
		"where":{"op":"eq","args":[{"path":[{"field":"id"}]},{"outer":[{"link":"animal"},{"link":"site"}]}]},
		"value":{"path":[{"field":"name"}]}}},
	{"id":"nested","formula":{"op":"lookup","source":"zoo.sites",
		"where":{"op":"eq","args":[{"path":[{"field":"region"}]},{"value":"north"}]},
		"value":{"op":"lookup","source":"zoo.feedings","pick":"count",
			"where":{"op":"lt","args":[{"path":[{"field":"fed"}]},{"outer":[{"field":"fed"}]}]},
			"value":{"path":[{"field":"id"}]}}}}],
	"sort":[{"col":"note"}]}`

// Each detail row looks up its own answer; the total row has no one row,
// so a lookup that reads it is blank there.
func TestAnOuterLeafCorrelatesALookupToItsRow(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", feedingsWithOuter, Options{}, nil)
	requireOrderedRows(t, feedingsWithOuter, res, "note", "f1", "f2", "f3", "f4", nil)
	requireOrderedRows(t, feedingsWithOuter, res, "earlier", int64(0), int64(2), int64(1), int64(3), nil)
	requireOrderedRows(t, feedingsWithOuter, res, "other", "f2", "f1", "f4", "f3", nil)
	requireOrderedRows(t, feedingsWithOuter, res, "site", "North", "North", "South", "South", nil)
}

// An outer leaf in a lookup inside a lookup reads the report's row, not the
// rows of the lookup around it (zoo.sites has no fed field).
func TestAnOuterLeafInANestedLookupReadsTheReportRow(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", feedingsWithOuter, Options{}, nil)
	requireOrderedRows(t, feedingsWithOuter, res, "nested", int64(0), int64(2), int64(1), int64(3), nil)
}

func TestAnOuterLeafIsBlankOnGroupRows(t *testing.T) {
	z := newZoo(t)
	const spec = `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"earlier","formula":{"op":"lookup","source":"zoo.feedings","pick":"count",
			"where":{"op":"lt","args":[{"path":[{"field":"fed"}]},{"outer":[{"field":"fed"}]}]},
			"value":{"path":[{"field":"id"}]}}}],
		"groups":[{"id":"g","col":"animal"}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
	requireRows(t, res, "kind", "group", "detail", "detail", "group", "detail", "detail", "total")
	requireOrderedRows(t, spec, res, "earlier", nil, int64(0), int64(2), nil, int64(1), int64(3), nil)
}

func TestAnOuterLeafIsRefusedOutsideALookup(t *testing.T) {
	app := withZooRegistry(t)
	outer := `{"outer":[{"field":"note"}]}`
	grouped := func(part string) string {
		return `{"version":3,"source":"zoo.feedings","columns":[{"id":"animal","path":[{"link":"animal"}]},{"id":"note","path":[{"field":"note"}]}],` + part + `}`
	}
	cases := map[string]struct{ spec, at string }{
		"a column":      {`{"version":3,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]},{"id":"x","formula":` + outer + `}]}`, "columns.1.formula"},
		"the filter":    {grouped(`"filter":{"op":"eq","args":[` + outer + `,{"value":"f1"}]}`), "filter.args.0"},
		"a having":      {grouped(`"groups":[{"id":"g","col":"animal","having":{"op":"eq","args":[` + outer + `,{"value":"f1"}]}}]`), "groups.0.having.args.0"},
		"a group label": {grouped(`"groups":[{"id":"g","col":"animal","label":` + outer + `}]`), "groups.0.label"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			err := Validate(app, loadSpec(t, c.spec))
			requireSpecError(t, err, 400, c.at, "outer reads the report's row: use it inside a lookup")
		})
	}
}

// A feeding's animal has many feedings: the row has no one value there.
func TestAnOuterLeafIsRefusedOnAToManyPath(t *testing.T) {
	app := withZooRegistry(t)
	err := Validate(app, loadSpec(t, `{"version":3,"source":"zoo.feedings","columns":[{"id":"x","formula":{"op":"lookup","source":"zoo.animals",
		"where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"outer":[{"link":"animal"},{"link":"feedings"},{"field":"note"}]}]},
		"value":{"path":[{"field":"weight"}]}}}]}`))
	requireSpecError(t, err, 400, "columns.0.formula.where.args.1.outer", "outer reads one value of the report's row: a to-many link is not one")
}

// A lookup whose value is a lookup that reads only the report's row (here
// through a column) keeps every detail row: SQLite would otherwise run the
// outer lookup's aggregate over the report's rows.
func TestALookupOfALookupThatReadsTheRowKeepsEveryRow(t *testing.T) {
	z := newZoo(t)
	const spec = `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"fed","path":[{"field":"fed"}]},
		{"id":"nested","formula":{"op":"lookup","source":"zoo.sites",
			"where":{"op":"eq","args":[{"path":[{"field":"region"}]},{"value":"north"}]},
			"value":{"op":"lookup","source":"zoo.feedings","pick":"count",
				"where":{"op":"lt","args":[{"path":[{"field":"fed"}]},{"col":"fed"}]},
				"value":{"path":[{"field":"id"}]}}}}],
		"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireOrderedRows(t, spec, res, "nested", int64(0), int64(2), int64(1), int64(3), nil)
}

// A lookup whose where matches a link to the report's row bounds the stored
// relation column too, so SQLite searches an index of it for each row
// instead of reading the looked-up source whole.
func TestALookupOnALinkSearchesAnIndexOfTheRelation(t *testing.T) {
	z := newZoo(t)
	withPlainFeedingsRule(t, z)
	seen := observeReads(t)
	res := z.run(t, "ann", `{"version":3,"source":"zoo.animals","columns":[{"id":"name","path":[{"field":"name"}]},
		{"id":"fed","formula":{"op":"lookup","source":"zoo.feedings","pick":"count","value":{"path":[{"field":"id"}]},
			"where":{"op":"eq","args":[{"path":[{"link":"animal"}]},{"outer":[{"field":"id"}]}]}}}],
		"sort":[{"col":"name"}]}`, Options{}, nil)
	requireRows(t, res, "fed", int64(1), int64(2), int64(2), nil)
	reads := perRowReadsOf(t, z, *seen, "zoo_feedings")
	if len(reads) == 0 {
		t.Fatal("no subquery read the feedings for each row")
	}
	for _, step := range reads {
		if !indexSearch.MatchString(step) {
			t.Fatalf("the lookup reads the feedings by %q", step)
		}
	}
}

// A link path of two hops reads its last link through a join, blank where
// the middle row is blank (kit has no vault) or hidden (no reader may list
// the vault): a blank looked-for value matches those rows, at the top of the
// where and under or alike. Kit has no site, so its count finds every animal
// ann may list; leo's and zed's sites match no animal's vault's animal.
func TestALookupOnATwoHopLinkMatchesABlankMiddleRow(t *testing.T) {
	z := newZoo(t)
	match := `{"op":"eq","args":[{"path":[{"link":"vault"},{"link":"animal"}]},{"outer":[{"link":"site"}]}]}`
	count := func(where string) string {
		return `{"op":"lookup","source":"zoo.animals","pick":"count","value":{"path":[{"field":"id"}]},"where":` + where + `}`
	}
	spec := `{"version":3,"source":"zoo.animals","columns":[{"id":"name","path":[{"field":"name"}]},
		{"id":"top","formula":` + count(match) + `},
		{"id":"underOr","formula":` + count(`{"op":"or","args":[`+match+`,{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Nobody"}]}]}`) + `}],
		"sort":[{"col":"name"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireOrderedRows(t, spec, res, "name", "Kit", "Leo", "Zed", nil)
	requireOrderedRows(t, spec, res, "top", int64(3), int64(0), int64(0), nil)
	requireOrderedRows(t, spec, res, "underOr", int64(3), int64(0), int64(0), nil)
}

// A lookup on a detail row is computed once per row: a formula that reads a
// sum lookup's value, its mixed check and its currency many times runs each
// subquery once in the detail statement.
func TestALookupOnADetailRowIsComputedOnce(t *testing.T) {
	z := newZoo(t)
	seen := observeReads(t)
	spec := `{"version":3,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]},
		{"id":"usdFees","formula":{"op":"if","args":[{"value":true},{"op":"coalesce","args":[` + mixedFees + `,{"value":0}]},{"value":0}]}}],"sort":[{"col":"name"}]}`
	requireMixedFeesBlank(t, z, "if", spec)
	details := 0
	for _, st := range *seen {
		if strings.Contains(st.sql, "__level") {
			continue
		}
		details++
		if n := strings.Count(st.sql, "count(DISTINCT"); n != 1 {
			t.Fatalf("the detail rows compute the mixed check %d times:\n%s", n, st.sql)
		}
	}
	if details == 0 {
		t.Fatal("no detail statement was read")
	}
}

// withPlainFeedingsRule gives zoo_feedings a rule SQLite reads in place, and
// an index on animal, so a query plan shows where the feedings are read: a
// rule PocketBase compiles to a DISTINCT join is computed whole first, and
// the plan reads that copy instead.
func withPlainFeedingsRule(t *testing.T, z *zoo) {
	t.Helper()
	feedings, err := z.app.FindCollectionByNameOrId("zoo_feedings")
	if err != nil {
		t.Fatal(err)
	}
	feedings.ListRule = rule(`@request.auth.id != ""`)
	feedings.ViewRule = feedings.ListRule
	feedings.AddIndex("idx_zoo_feedings_animal", false, "animal", "")
	z.save(t, feedings)
}
