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
// where would match a blank (ne).
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
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal"}],"sort":[{"col":"weight"}]}`, Options{ExpandAll: true}, nil)
	requireRows(t, res, "kind", "group", "detail", "detail", "group", "detail", "detail", "total")
	requireRows(t, res, "weight", int64(190), int64(190), int64(190), int64(300), int64(300), int64(300), nil)
	requireRows(t, res, "other", int64(300), int64(300), int64(300), int64(190), int64(190), int64(190), nil)
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
				{"op":"lookup","source":"zoo.animals","where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Leo"}]},"value":{"path":[{"field":"weight"}]}}]}}]}`, Options{}, nil)
	requireRows(t, res, "label", "Lions", "Total Lions", "Lions and Leo")
	requireRows(t, res, "cost", int64(3500), int64(3500), int64(3690))
}

// A lookup in a computed row reads the run's params, and refuses a report
// column: a computed row has no report row to read.
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
	requireSpecError(t, err, 400, "layout.1.formula.where.args.1", "a computed row reads")
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
