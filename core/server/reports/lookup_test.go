package reports

import (
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
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"weight","formula":{"op":"lookup","source":"zoo.animals",
			"where":{"op":"eq","args":[{"path":[{"field":"id"}]},{"col":"animal"}]},
			"value":{"path":[{"field":"weight"}]}}}],"sort":[{"col":"note"}]}`, Options{}, nil)
	requireRows(t, res, "weight", int64(190), int64(190), int64(300), int64(300), nil)
}

func TestALookupInTheFilterPicksMinOrMaxAndIsBlankWithNoMatch(t *testing.T) {
	z := newZoo(t)
	zedID := `{"op":"lookup","source":"zoo.animals","where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Zed"}]},"value":{"path":[{"field":"id"}]}}`
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]}],
		"filter":{"op":"eq","args":[{"path":[{"link":"animal"}]},`+zedID+`]},"sort":[{"col":"note"}]}`, Options{}, nil)
	requireRows(t, res, "note", "f3", "f4", nil)

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
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"spent","formula":{"op":"sum","args":[{"path":[{"field":"cost"}]}]}},
		{"id":"fits","formula":{"op":"lookup","source":"zoo.animals","pick":"max",
			"where":{"op":"lte","args":[{"op":"mul","args":[{"path":[{"field":"weight"}]},{"value":10}]},{"col":"spent"}]},
			"value":{"path":[{"field":"weight"}]}}}],
		"groups":[{"id":"g","col":"species"}],"sort":[{"col":"note"}]}`, Options{ExpandAll: true}, nil)
	requireRows(t, res, "kind", "group", "detail", "detail", "group", "detail", "detail", "total")
	requireRows(t, res, "spent", int64(3500), int64(1000), int64(2500), int64(1000), int64(700), int64(300), int64(4500))
	requireRows(t, res, "fits", int64(300), int64(4), int64(190), int64(4), int64(4), int64(4), int64(300))
}

// A lookup may read a running value: the outer select computes it before the
// lookup's subquery reads it, on detail rows (running within the group) and
// on group and total rows (running across the level's rows). fits is the
// heaviest animal ann may list whose weight times 15 is at most the running
// cost (Mo's 180 would fit 700 and 1000, but she cannot list Mo); lighter
// is the heaviest one lighter than fits.
func TestALookupReadsARunningValue(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
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
		"groups":[{"id":"g","col":"species"}],"sort":[{"col":"note"}]}`, Options{ExpandAll: true}, nil)
	requireRows(t, res, "kind", "group", "detail", "detail", "group", "detail", "detail", "total")
	requireRows(t, res, "run", int64(3500), int64(1000), int64(3500), int64(4500), int64(700), int64(1000), int64(4500))
	requireRows(t, res, "fits", int64(190), int64(4), int64(190), int64(300), int64(4), int64(4), int64(300))
	// A lookup of that lookup reads it computed too.
	requireRows(t, res, "lighter", int64(4), nil, int64(4), int64(190), nil, nil, int64(190))
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
		// Task 5a builds this (Q5) and deletes the case.
		"an amount with a row currency": {`{"version":2,"source":"zoo.animals","columns":[{"id":"x","formula":{"op":"lookup","source":"zoo.feedings","value":{"path":[{"field":"cost"}]}}}]}`,
			"columns.0.formula.value.path", "read it through a column"},
		"a bad pick": {`{"version":2,"source":"zoo.feedings","columns":[{"id":"x","formula":{"op":"lookup","source":"zoo.animals","value":{"path":[{"field":"id"}]},"pick":"avg"}}]}`,
			"columns.0.formula.pick", "min or max"},
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
