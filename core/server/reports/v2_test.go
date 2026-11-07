package reports

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// A step that changes the document still says so.
func TestAChangingStepIsNoticed(t *testing.T) {
	withZooRegistry(t)
	steps := map[int]upgradeStep{1: func(doc map[string]any) error {
		doc["columns"] = doc["cols"]
		delete(doc, "cols")
		return nil
	}}
	_, info, err := loadWith([]byte(`{"version":1,"source":"zoo.feedings","cols":[]}`), steps, 2)
	if err != nil || info.UpgradedFrom != 1 {
		t.Fatalf("info %+v, err %v", info, err)
	}
}

// every is a v2 spec that uses every new part (the zoo's "For review" spec).
const every = `{"version":2,"source":"zoo.feedings",
	"params":[{"id":"asOf","type":"asOf","label":"As of","default":{"preset":"endOfLastMonth"}},
		{"id":"rows","type":"choice","label":"Rows","default":"nonzero","options":[{"value":"nonzero","label":"Non-zero"},{"value":"all","label":"All"}]},
		{"id":"animals","type":"choice","label":"Animals","multiple":true,"optionsFrom":{"source":"zoo.animals","value":[{"field":"id"}],"label":[{"field":"name"}],"group":[{"field":"species"}],
			"where":{"op":"eq","args":[{"path":[{"field":"active"}]},{"value":true}]}}}],
	"columns":[{"id":"animal","label":"Animal","path":[{"link":"animal"}],"labelField":"name"},
		{"id":"weight","label":"Weight","formula":{"record":"byAnimal","path":[{"field":"weight"}]}},
		{"id":"cost","label":"Cost","path":[{"field":"cost"}],"aggregate":"sum","total":"rows"},
		{"id":"site","label":"Site","formula":{"op":"lookup","source":"zoo.sites","where":{"op":"eq","args":[{"path":[{"field":"region"}]},{"value":"north"}]},"value":{"path":[{"field":"name"}]},"pick":"min"}}],
	"filter":{"op":"and","args":[{"op":"lte","args":[{"path":[{"field":"due"}]},{"param":"asOf"}]},{"op":"in","args":[{"col":"animal"},{"param":"animals"}]}]},
	"groups":[{"id":"byAnimal","col":"animal","label":{"record":"byAnimal","path":[{"field":"name"}]},
		"fill":{"where":{"op":"eq","args":[{"path":[{"field":"active"}]},{"value":true}]}},
		"having":{"op":"or","args":[{"op":"eq","args":[{"param":"rows"},{"value":"all"}]},{"op":"ne","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":0}]}]},
		"open":true}],
	"sort":[{"formula":{"record":"byAnimal","path":[{"field":"weight"}]},"dir":"desc"},{"col":"cost"}],
	"detail":{"label":{"op":"concat","args":[{"path":[{"field":"fed"}]},{"value":" · "},{"path":[{"field":"note"}]}]}},
	"display":{"layouts":[{"width":"mobile","tracks":3,"cells":[{"col":"animal","line":1,"span":3},{"col":"weight","line":2},{"col":"cost","line":2,"justify":"end"},{"col":"site","line":2,"justify":"end"}]}]}}`

// pivoted is a pivot over a link named by a label field, with a display
// layout that places the pivot's cell column.
const pivoted = `{"version":2,"source":"zoo.feedings",
	"columns":[{"id":"month","path":[{"field":"fed"}]},{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
	"groups":[{"id":"m","col":"month","bucket":"month"}],
	"pivot":{"path":[{"link":"animal"}],"cell":"grams","labelField":"name"},
	"display":{"layouts":[{"width":"desktop","tracks":2,"cells":[{"col":"month","line":1},{"col":"grams","line":1}]}]}}`

// A spec with every v2 part loads, validates, runs, and is stored with
// every part unchanged.
func TestASpecWithEveryV2PartIsKeptWhole(t *testing.T) {
	z := newZoo(t)
	for _, c := range []struct {
		raw  string
		keys []string
	}{
		{every, []string{`"optionsFrom"`, `"record":"byAnimal"`, `"total":"rows"`, `"op":"lookup"`, `"pick":"min"`, `"label":{`, `"fill":{`, `"having":{`, `"open":true`, `"dir":"desc"`, `"detail":{`, `"display":{`, `"labelField":"name"`}},
		{pivoted, []string{`"pivot":{`, `"labelField":"name"`, `"display":{`}},
	} {
		spec, _, stored := mustLoad(t, c.raw)
		if err := Validate(z.app, spec); err != nil {
			t.Fatal(err)
		}
		z.run(t, "ann", c.raw, Options{Today: "2026-04-15"}, nil)
		_, _, again := mustLoad(t, string(stored))
		if !bytes.Equal(stored, again) {
			t.Fatalf("stored twice differs:\n%s\n%s", stored, again)
		}
		for _, key := range c.keys {
			if !strings.Contains(string(stored), key) {
				t.Fatalf("stored spec lost %s: %s", key, stored)
			}
		}
		// Stored as version 3, every other part as posted.
		want := jsonDoc(t, []byte(c.raw)).(map[string]any)
		want["version"] = json.Number("3")
		if !reflect.DeepEqual(want, jsonDoc(t, stored)) {
			t.Fatalf("stored spec differs from the posted one:\n%s", stored)
		}
	}
}

func mustLoad(t *testing.T, raw string) (*Spec, *LoadInfo, []byte) {
	t.Helper()
	spec, info, err := Load([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	stored, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	return spec, info, stored
}

// jsonDoc decodes a JSON document with exact numbers, for comparing two
// documents whatever their key order and spacing.
func jsonDoc(t *testing.T, raw []byte) any {
	t.Helper()
	var doc any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

// zoo.animals renames weight from mass: every v2 part that names a field
// is resolved.
func TestAliasesResolveInEveryV2Part(t *testing.T) {
	withZooRegistry(t)
	raw := strings.NewReplacer(`"field":"weight"`, `"field":"mass"`).Replace(every)
	spec, info, _ := mustLoad(t, raw)
	if spec.Columns[1].Formula.Path[0].Field != "weight" || spec.Sort[0].Formula.Path[0].Field != "weight" {
		t.Fatalf("record paths not resolved: %+v %+v", spec.Columns[1].Formula, spec.Sort[0].Formula)
	}
	if len(info.Aliased) != 2 {
		t.Fatalf("aliased = %+v", info.Aliased)
	}
	// A lookup's where resolves against the looked-up source.
	looked := strings.NewReplacer(
		`"source":"zoo.sites"`, `"source":"zoo.animals"`,
		`{"path":[{"field":"region"}]},{"value":"north"}`, `{"path":[{"field":"mass"}]},{"value":190}`,
	).Replace(every)
	spec, _, _ = mustLoad(t, looked)
	if got := spec.Columns[3].Formula.Where.Args[0].Path[0].Field; got != "weight" {
		t.Fatalf("lookup where = %q, want weight", got)
	}
}

func TestDisplayLayoutsAreChecked(t *testing.T) {
	app := withZooRegistry(t)
	base := `{"version":2,"source":"zoo.feedings","columns":[{"id":"a","path":[{"field":"note"}]},{"id":"b","path":[{"field":"cost"}]}],"display":{"layouts":[%s]}}`
	for _, c := range []struct {
		layout, at, want string
		status           int
	}{
		{`{"width":"watch","tracks":2,"cells":[]}`, "display.layouts.0.width", "mobile, tablet or desktop", 400},
		{`{"width":"mobile","tracks":13,"cells":[]}`, "display.layouts.0.tracks", "1 to 12", 400},
		{`{"width":"mobile","tracks":2,"cells":[{"col":"zz","line":1}]}`, "display.layouts.0.cells.0.col", "has no column", 422},
		{`{"width":"mobile","tracks":2,"cells":[{"col":"a","line":9}]}`, "display.layouts.0.cells.0", "line is 1 to 8", 400},
		{`{"width":"mobile","tracks":2,"cells":[{"col":"a","line":1,"span":2},{"col":"b","line":1}]}`, "display.layouts.0.cells.1", "no room on line 1", 400},
		{`{"width":"mobile","tracks":2,"cells":[{"col":"a","line":1,"lines":2},{"col":"b","line":2,"start":1}]}`, "display.layouts.0.cells.1", "overlaps", 400},
		{`{"width":"mobile","tracks":2,"cells":[{"col":"a","line":1,"justify":"left"}]}`, "display.layouts.0.cells.0.justify", "start, center or end", 400},
		{`{"width":"mobile","tracks":2,"cells":[]},{"width":"mobile","tracks":1,"cells":[]}`, "display.layouts.1.width", "one layout per width", 400},
		{`{"width":"mobile","tracks":1,"cells":[]},{"width":"tablet","tracks":1,"cells":[]},{"width":"desktop","tracks":1,"cells":[]},{"width":"mobile","tracks":1,"cells":[]}`, "display.layouts", "at most 3 display layouts", 400},
		{`{"width":"mobile","tracks":2,"cells":[` + strings.Repeat(`{"col":"a","line":1},`, 100) + `{"col":"a","line":1}]}`, "display.layouts.0.cells", "at most 100 columns", 400},
		{`{"width":"mobile","tracks":2,"cells":[{"col":"a","line":1},{"col":"a","line":2}]}`, "display.layouts.0.cells.1.col", "placed once per layout", 400},
		{`{"width":"mobile","tracks":2,"cells":[{"col":"a","line":1,"lines":-1}]}`, "display.layouts.0.cells.0", "lines is 1 to 8", 400},
		{`{"width":"mobile","tracks":2,"cells":[{"col":"a","line":1,"span":-1}]}`, "display.layouts.0.cells.0", "span is 1 to 2", 400},
		{`{"width":"mobile","tracks":2,"cells":[{"col":"a","line":1,"span":3}]}`, "display.layouts.0.cells.0", "span is 1 to 2", 400},
		{`{"width":"mobile","tracks":2,"cells":[{"col":"a","line":1,"start":3}]}`, "display.layouts.0.cells.0", "does not fit its line", 400},
		{`{"width":"mobile","tracks":2,"cells":[{"col":"a","line":1,"start":2,"span":2}]}`, "display.layouts.0.cells.0", "does not fit its line", 400},
		{`{"width":"mobile","tracks":2,"cells":[{"col":"a","line":8,"lines":2}]}`, "display.layouts.0.cells.0", "a cell ends by line 8", 400},
		{`{"width":"mobile","tracks":2,"cells":[]}`, "display.layouts.0.cells", "at least one column", 400},
		{`{"width":"tablet","tracks":2}`, "display.layouts.0.cells", "at least one column", 400},
	} {
		spec, _, err := Load([]byte(fmt.Sprintf(base, c.layout)))
		if err != nil {
			t.Fatal(err)
		}
		requireSpecError(t, Validate(app, spec), c.status, c.at, c.want)
	}
	// A layout need not list every column; a pivot report places its cell
	// column.
	for _, raw := range []string{
		fmt.Sprintf(base, `{"width":"mobile","tracks":2,"cells":[{"col":"b","line":1,"start":2}]}`),
		pivoted,
	} {
		spec, _, err := Load([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		if err := Validate(app, spec); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
	}
}

// A pinwheel over five real columns is a 400 (not an unknown column).
func TestDisplayRefusesAPinwheel(t *testing.T) {
	app := withZooRegistry(t)
	spec, _, err := Load([]byte(`{"version":2,"source":"zoo.feedings","columns":[
		{"id":"a","path":[{"field":"note"}]},{"id":"b","path":[{"field":"cost"}]},{"id":"c","path":[{"field":"grams"}]},
		{"id":"d","path":[{"field":"fed"}]},{"id":"e","path":[{"field":"currency"}]}],
		"display":{"layouts":[{"width":"mobile","tracks":3,"cells":[{"col":"a","line":1,"start":1,"span":2},{"col":"b","line":1,"start":3,"lines":2},
			{"col":"c","line":3,"start":2,"span":2},{"col":"d","line":2,"start":1,"lines":2},{"col":"e","line":2,"start":2}]}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	requireSpecError(t, Validate(app, spec), 400, "display.layouts.0.cells.0", "can't be laid out")
}

func TestPlaceCellsFlowsIntoFreeTracks(t *testing.T) {
	got, err := placeCells(3, []DisplayCell{{Col: "a", Line: 1, Span: 3}, {Col: "b", Line: 2}, {Col: "c", Line: 2}, {Col: "d", Line: 3}})
	if err != nil {
		t.Fatal(err)
	}
	want := []placed{{1, 3, 1, 1}, {1, 1, 2, 1}, {2, 1, 2, 1}, {1, 1, 3, 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("placed = %+v", got)
	}
}

// A cell may span lines anywhere a straight cut frees it, mid-row included;
// only a pinwheel is refused (R28).
func TestPlaceCellsCutsAnyLayoutButAPinwheel(t *testing.T) {
	mid := []DisplayCell{{Col: "a", Line: 1}, {Col: "m", Line: 1, Start: 2, Lines: 2}, {Col: "b", Line: 1, Start: 3}, {Col: "c", Line: 2, Start: 1}, {Col: "d", Line: 2, Start: 3}}
	if _, err := placeCells(3, mid); err != nil {
		t.Fatal(err)
	}
	pinwheel := []DisplayCell{
		{Col: "a", Line: 1, Start: 1, Span: 2},
		{Col: "b", Line: 1, Start: 3, Lines: 2},
		{Col: "c", Line: 3, Start: 2, Span: 2},
		{Col: "d", Line: 2, Start: 1, Lines: 2},
		{Col: "e", Line: 2, Start: 2},
	}
	_, err := placeCells(3, pinwheel)
	requireSpecError(t, err, 400, "cells.0", "These cells overlap in a way that can't be laid out; move one cell.")
}

// Every part that reads a field reads its current key: group labels, having,
// fill wheres (on the group's records, through a formula column too), sort
// formulas, the detail label, layout formulas and a lookup's value.
func TestAliasesResolveInEveryWalkedPart(t *testing.T) {
	withZooRegistry(t)
	mass := `{"path":[{"link":"animal"},{"field":"mass"}]}`
	own := `{"path":[{"field":"mass"}]}`
	raw := `{"version":2,"source":"zoo.feedings",
		"columns":[{"id":"animal","path":[{"link":"animal"}]},
			{"id":"again","formula":{"op":"if","args":[{"value":true},{"col":"animal"},{"col":"animal"}]}},
			{"id":"heavy","formula":{"op":"lookup","source":"zoo.animals","where":{"op":"gt","args":[` + own + `,{"value":100}]},"value":` + own + `}}],
		"groups":[{"id":"g1","col":"animal","label":{"record":"g1","path":[{"field":"mass"}]},
				"having":{"op":"gt","args":[{"op":"sum","args":[` + mass + `]},{"value":0}]},
				"fill":{"where":{"op":"gt","args":[` + own + `,{"value":1}]}}},
			{"id":"g2","col":"again","fill":{"where":{"op":"gt","args":[` + own + `,{"value":2}]}}}],
		"sort":[{"formula":` + mass + `}],
		"detail":{"label":` + mass + `},
		"layout":[{"row":"r","label":"R","formula":{"op":"lookup","source":"zoo.animals","where":{"op":"gt","args":[` + own + `,{"value":3}]},"value":` + own + `}}]}`
	spec, info, _ := mustLoad(t, raw)
	lookupValueField := func(n *Node) string {
		v, err := lookupValue(n, "")
		if err != nil {
			t.Fatal(err)
		}
		return v.Path[0].Field
	}
	for name, got := range map[string]string{
		"lookup where": spec.Columns[2].Formula.Where.Args[0].Path[0].Field,
		"lookup value": lookupValueField(spec.Columns[2].Formula),
		"group label":  spec.Groups[0].Label.Path[0].Field,
		"having":       spec.Groups[0].Having.Args[0].Args[0].Path[1].Field,
		"fill where":   spec.Groups[0].Fill.Where.Args[0].Path[0].Field,
		"formula fill": spec.Groups[1].Fill.Where.Args[0].Path[0].Field,
		"sort formula": spec.Sort[0].Formula.Path[1].Field,
		"detail label": spec.Detail.Label.Path[1].Field,
		"layout where": spec.Layout[0].Formula.Where.Args[0].Path[0].Field,
		"layout value": lookupValueField(spec.Layout[0].Formula),
	} {
		if got != "weight" {
			t.Errorf("%s = %q, want weight", name, got)
		}
	}
	if len(info.Aliased) != 10 {
		t.Fatalf("aliased %d keys: %+v", len(info.Aliased), info.Aliased)
	}
}

// A lookup whose value holds no aliased key keeps the value as posted.
func TestALookupValueWithoutAnAliasIsKept(t *testing.T) {
	withZooRegistry(t)
	// Posted as the current version: an older document is re-encoded by the
	// upgrade, which compacts the raw value.
	raw := `{"version":3,"source":"zoo.feedings","columns":[{"id":"l","formula":{"op":"lookup","source":"zoo.animals","value":{ "path" : [{"field":"name"}] }}}]}`
	spec, _, _ := mustLoad(t, raw)
	if got := string(spec.Columns[0].Formula.Value); got != `{ "path" : [{"field":"name"}] }` {
		t.Fatalf("value = %s", got)
	}
}

// A version 2 lookup whose value holds no aliased key keeps its value through
// the 2→3 upgrade: the upgrade re-encodes the document, so the raw text is
// compacted, but the value decodes to the one posted.
func TestAV2LookupValueWithoutAnAliasIsKeptThroughTheUpgrade(t *testing.T) {
	withZooRegistry(t)
	const value = `{ "path" : [{"field":"name"}] }`
	raw := `{"version":2,"source":"zoo.feedings","columns":[{"id":"l","formula":{"op":"lookup","source":"zoo.animals","value":` + value + `}}]}`
	spec, info, _ := mustLoad(t, raw)
	if spec.Version != 3 || info.UpgradedFrom != 0 || len(info.Aliased) != 0 {
		t.Fatalf("version %d, info %+v", spec.Version, info)
	}
	if got := spec.Columns[0].Formula.Value; !reflect.DeepEqual(jsonDoc(t, got), jsonDoc(t, []byte(value))) {
		t.Fatalf("value = %s, want %s", got, value)
	}
}

// A group over a formula column whose link was renamed still finds the
// records it links to, so its fill where resolves on them.
func TestAFillWhereResolvesBehindARenamedLink(t *testing.T) {
	withZooRegistry(t)
	RegisterSource(Source{
		Key: "zoo.tours", Collection: "zoo_tours", Label: "Tours",
		Links:   []Link{{Key: "beast", Label: "Beast", Target: "zoo.animals", Cardinality: ToOne, Field: "beast"}},
		Aliases: map[string]string{"pet": "beast"},
	})
	spec, info, _ := mustLoad(t, `{"version":2,"source":"zoo.tours",
		"columns":[{"id":"b","formula":{"op":"coalesce","args":[{"path":[{"link":"pet"}]}]}}],
		"groups":[{"id":"g","col":"b","fill":{"where":{"op":"gt","args":[{"path":[{"field":"mass"}]},{"value":1}]}}}]}`)
	if got := spec.Groups[0].Fill.Where.Args[0].Path[0].Field; got != "weight" {
		t.Fatalf("fill where = %q, want weight", got)
	}
	if got := spec.Columns[0].Formula.Args[0].Path[0].Link; got != "beast" || len(info.Aliased) != 2 {
		t.Fatalf("link = %q, aliased %+v", got, info.Aliased)
	}
}
