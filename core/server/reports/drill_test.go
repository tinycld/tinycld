package reports

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// drillTarget is the shipped report a zoo drill opens: the reader's feedings
// by note, filtered by a period, animals (grouped by species), one species,
// a day, an as-of day and a note.
const drillTarget = `{"version":4,"source":"zoo.feedings","params":[
	{"id":"period","type":"dateRange","default":{"preset":"allDates"}},
	{"id":"animals","type":"choice","multiple":true,"optionsFrom":{"source":"zoo.animals","value":[{"field":"id"}],"label":[{"field":"name"}],"group":[{"field":"species"}]}},
	{"id":"species","type":"choice","optionsFrom":{"source":"zoo.animals","value":[{"field":"species"}],"label":[{"field":"species"}]}},
	{"id":"fedOn","type":"date"},
	{"id":"asOf","type":"asOf"},
	{"id":"note","type":"text"},{"id":"cap","type":"amount"}],
	"columns":[{"id":"note","path":[{"field":"note"}]},{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
	"filter":{"op":"and","args":[
		{"op":"between","args":[{"path":[{"field":"fed"}]},{"param":"period"}]},
		{"op":"in","args":[{"path":[{"link":"animal"},{"field":"id"}]},{"param":"animals"}]},
		{"op":"eq","args":[{"path":[{"link":"animal"},{"field":"species"}]},{"param":"species"}]},
		{"op":"lte","args":[{"path":[{"field":"fed"}]},{"param":"asOf"}]}]}}`

// shipDrillTarget registers the target; with an app it also stores it, owned
// by the first admin: published, or unpublished and shared with no one.
func shipDrillTarget(t *testing.T, s *savedZoo, unpublished bool) {
	t.Helper()
	RegisterShipped(Shipped{Key: "zoo.drill_target", Title: "Feedings by animal", Spec: json.RawMessage(drillTarget), Revision: 1, Unpublished: unpublished})
	if s == nil {
		return
	}
	if err := ReconcileShipped(s.app); err != nil {
		t.Fatal(err)
	}
}

// drilledFeedingsTmpl groups the reader's feedings by species (open, with
// subtotals), then animal; each %s is one column's drill, in column order:
// species (the first), animal, grams, share (a percent), secret (hidden).
const drilledFeedingsTmpl = `{"version":4,"source":"zoo.feedings",
	"params":[{"id":"period","type":"dateRange","default":{"preset":"allDates"}},{"id":"label","type":"text"},{"id":"cap","type":"amount"}],
	"columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]%s},
		{"id":"animal","path":[{"link":"animal"}]%s},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"%s},
		{"id":"share","formula":{"op":"percentOfTotal","args":[{"col":"grams"}],"scale":1}%s},
		{"id":"secret","path":[{"field":"cost"}],"aggregate":"sum","hidden":true%s}],
	"filter":{"op":"between","args":[{"path":[{"field":"fed"}]},{"param":"period"}]},
	"groups":[{"id":"g","col":"species","subtotals":true,"open":true},{"id":"a","col":"animal"}]}`

// drilledFeedingsOn is the report with drill on column col only.
func drilledFeedingsOn(col, drill string) string {
	at := map[string]string{col: `,"drill":` + drill}
	return fmt.Sprintf(drilledFeedingsTmpl, at["species"], at["animal"], at["grams"], at["share"], at["secret"])
}

// goodDrill opens the target from both levels.
const goodDrill = `{"report":"zoo.drill_target","levels":{
	"g":{"species":{"groupKey":"g"},"period":{"param":"period"}},
	"a":{"animals":{"groupKey":"a"},"species":{"groupKey":"g"},"period":{"param":"period"}}}}`

// columnDrill is the drill of the result column id, nil when it has none.
func columnDrill(t *testing.T, res *Result, id string) *DrillTarget {
	t.Helper()
	for _, c := range res.Columns {
		if c.ID == id {
			return c.Drill
		}
	}
	t.Fatalf("no column %s", id)
	return nil
}

func TestADrilledColumnNamesTheTargetTheReaderMayView(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, s, false)
	res := s.run(t, "ann", drilledFeedingsOn("grams", goodDrill), Options{}, nil)
	want := DrillTarget{ReportID: shippedRow(t, s, "zoo.drill_target").Id, Title: "Feedings by animal"}
	if got := columnDrill(t, res, "grams"); got == nil || *got != want {
		t.Fatalf("drill = %+v, want %+v", got, want)
	}
	if got := columnDrill(t, res, "animal"); got != nil {
		t.Fatalf("an undrilled column has drill %+v", got)
	}
	raw, err := json.Marshal(res.Columns)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), fmt.Sprintf(`"drill":{"reportId":%q,"title":"Feedings by animal"}`, want.ReportID)) {
		t.Fatalf("columns JSON: %s", raw)
	}
}

func TestADrillTargetTheReaderCannotViewGivesNoDrill(t *testing.T) {
	s := newSavedZoo(t)
	// Unpublished and shared with no one: only its owner, the admin, sees it.
	shipDrillTarget(t, s, true)
	res := s.run(t, "ann", drilledFeedingsOn("grams", goodDrill), Options{}, nil)
	if got := columnDrill(t, res, "grams"); got != nil {
		t.Fatalf("drill = %+v for a reader who cannot view the target", got)
	}
}

func TestADrillTargetNotStoredYetGivesNoDrill(t *testing.T) {
	s := newSavedZoo(t)
	// Registered, not reconciled: no reports row exists yet.
	shipDrillTarget(t, nil, false)
	res := s.run(t, "ann", drilledFeedingsOn("grams", goodDrill), Options{}, nil)
	if got := columnDrill(t, res, "grams"); got != nil {
		t.Fatalf("drill = %+v before the target is stored", got)
	}
}

func TestTwoDrilledColumnsOnOneTargetShareIt(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, s, false)
	// The template places one drill per column; give animal and grams the same key.
	spec := fmt.Sprintf(drilledFeedingsTmpl, "", `,"drill":`+goodDrill, `,"drill":`+goodDrill, "", "")
	res := s.run(t, "ann", spec, Options{}, nil)
	want := DrillTarget{ReportID: shippedRow(t, s, "zoo.drill_target").Id, Title: "Feedings by animal"}
	for _, id := range []string{"animal", "grams"} {
		if got := columnDrill(t, res, id); got == nil || *got != want {
			t.Fatalf("column %s drill = %+v, want %+v", id, got, want)
		}
	}
}

func TestDrillTargetsFailsOnALookupErrorThatIsNotNoRows(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, s, false)
	// A real storage failure (not a missing row): the reports table is gone.
	db := s.app.DB()
	if _, err := db.NewQuery("ALTER TABLE reports RENAME TO reports_moved").Execute(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.NewQuery("ALTER TABLE reports_moved RENAME TO reports").Execute(); err != nil {
			t.Error(err)
		}
	})
	got, err := drillTargets(s.app, nil, []string{"zoo.drill_target"})
	if err == nil {
		t.Fatalf("drillTargets = %+v, want an error", got)
	}
}

// validate loads raw and validates it for the zoo.
func (z *zoo) validate(t *testing.T, raw string) error {
	t.Helper()
	spec, _, err := Load([]byte(raw))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return Validate(z.app, spec)
}

func TestADrillValueKeepsItsFormThroughJSON(t *testing.T) {
	in := `{"report":"zoo.drill_target","levels":{"g":{` +
		`"animals":{"group":{"groupKey":"g"}},` +
		`"fedOn":{"op":"max","args":[{"path":[{"field":"fed"}]}]},` +
		`"period":{"range":{"from":{"pivotBound":"from"},"to":{"param":"period","bound":"to"}}},` +
		`"species":{"param":"label"}}},` +
		`"pivotCells":{"period":{"param":"period"}}}`
	var d Drill
	if err := json.Unmarshal([]byte(in), &d); err != nil {
		t.Fatal(err)
	}
	g := d.Levels["g"]
	if g["animals"].Group == nil || g["fedOn"].Formula == nil || g["period"].Range == nil || g["species"].Param != "label" {
		t.Fatalf("forms = %+v", g)
	}
	out, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != in {
		t.Fatalf("round trip\n got %s\nwant %s", out, in)
	}
}

func TestLoadRefusesADrillValueWithAnUnknownKey(t *testing.T) {
	for name, value := range map[string]string{
		"a range":   `{"range":{"from":{"value":"2026-01-01"},"to":{"value":"2026-01-31"},"x":1}}`,
		"a group":   `{"group":{"groupKey":"g","x":1}}`,
		"a formula": `{"op":"max","args":[],"x":1}`,
	} {
		t.Run(name, func(t *testing.T) {
			raw := drilledFeedingsOn("grams", `{"report":"zoo.drill_target","levels":{"g":{"period":`+value+`}}}`)
			if _, _, err := Load([]byte(raw)); err == nil {
				t.Fatal("Load accepted an unknown key")
			}
		})
	}
}

func TestAGoodDrillValidates(t *testing.T) {
	z := newZoo(t)
	shipDrillTarget(t, nil, false)
	if err := z.validate(t, drilledFeedingsOn("grams", goodDrill)); err != nil {
		t.Fatal(err)
	}
}

func TestADrillIsRefusedWhereItCannotOpen(t *testing.T) {
	z := newZoo(t)
	shipDrillTarget(t, nil, false)
	RegisterShipped(Shipped{Key: "other.target", Title: "Other", Spec: json.RawMessage(drillTarget), Revision: 1})
	g := func(values string) string {
		return `{"report":"zoo.drill_target","levels":{"g":{` + values + `}}}`
	}
	for name, c := range map[string]struct{ col, drill, at, contains string }{
		"a key no package ships": {"grams", `{"report":"zoo.nope","levels":{"g":{}}}`, "columns.2.drill.report", "no package ships"},
		"another package":        {"grams", `{"report":"other.target","levels":{"g":{}}}`, "columns.2.drill.report", "own package"},
		"no levels":              {"grams", `{"report":"zoo.drill_target","levels":{}}`, "columns.2.drill.levels", "at least one group level"},
		"a level not a group":    {"grams", `{"report":"zoo.drill_target","levels":{"x":{}}}`, "columns.2.drill.levels.x", "not a group"},
		"an undeclared param":    {"grams", g(`"nope":{"param":"period"}`), "columns.2.drill.levels.g.nope", "has no param nope"},
		"a range into a choice":  {"grams", g(`"species":{"range":{"from":{"value":"2026-01-01"},"to":{"value":"2026-01-31"}}}`), "columns.2.drill.levels.g.species", "does not fit param species"},
		"a group into a single":  {"grams", g(`"species":{"group":{"groupKey":"g"}}`), "columns.2.drill.levels.g.species", "does not fit param species"},
		"a formula into text":    {"grams", g(`"note":{"groupKey":"g"}`), "columns.2.drill.levels.g.note", "does not fit param note"},
		"a copy of another type": {"grams", g(`"period":{"param":"label"}`), "columns.2.drill.levels.g.period", "does not fit param period"},
		"a copy of no param":     {"grams", g(`"note":{"param":"nope"}`), "columns.2.drill.levels.g.note", "no param nope to copy"},
		"a range half":           {"grams", g(`"period":{"range":{"from":{"value":"2026-01-01"}}}`), "columns.2.drill.levels.g.period", "a range has a from and a to"},
		"the first column":       {"species", goodDrill, "columns.0.drill", "first column"},
		"a hidden column":        {"secret", goodDrill, "columns.4.drill", "hidden"},
		"a percent":              {"share", goodDrill, "columns.3.drill", "percent"},
		"pivotCells, no pivot":   {"grams", `{"report":"zoo.drill_target","levels":{"g":{}},"pivotCells":{"period":{"param":"period"}}}`, "columns.2.drill.pivotCells", "no pivot"},
		"pivotBound in a level":  {"grams", g(`"period":{"range":{"from":{"pivotBound":"from"},"to":{"pivotBound":"to"}}}`), "columns.2.drill.levels.g.period.from", "only in a drill's pivotCells"},
		"groupKey below":         {"grams", g(`"animals":{"groupKey":"a"}`), "columns.2.drill.levels.g.animals", "below the level"},
		"groupKey of no group":   {"grams", g(`"species":{"groupKey":"x"}`), "columns.2.drill.levels.g.species", "x is not a group"},
	} {
		t.Run(name, func(t *testing.T) {
			requireSpecError(t, z.validate(t, drilledFeedingsOn(c.col, c.drill)), 400, c.at, c.contains)
		})
	}
}

// drilledPivotTmpl groups the reader's feedings by species, then animal,
// with a pivot by fed (pivot is its JSON) on grams; %s is the drill of
// column drillCol: grams (2) or cost (3).
const drilledPivotTmpl = `{"version":4,"source":"zoo.feedings",
	"params":[{"id":"period","type":"dateRange","default":{"preset":"allDates"}}],
	"columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"%s},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"%s}],
	"filter":{"op":"between","args":[{"path":[{"field":"fed"}]},{"param":"period"}]},
	"groups":[{"id":"g","col":"species"},{"id":"a","col":"animal"}],
	"pivot":%s}`

const monthPivot = `{"path":[{"field":"fed"}],"bucket":"month","cell":"grams"}`

// drilledPivotOn is the pivot report with drill on column col (grams or
// cost) and the pivot pivot.
func drilledPivotOn(col, drill, pivot string) string {
	at := map[string]string{col: `,"drill":` + drill}
	return fmt.Sprintf(drilledPivotTmpl, at["grams"], at["cost"], pivot)
}

// pivotDrill opens the target from level g, its pivot cells over period.
func pivotDrill(from, to string) string {
	return `{"report":"zoo.drill_target","levels":{"g":{"species":{"groupKey":"g"}}},` +
		`"pivotCells":{"period":{"range":{"from":{"pivotBound":"` + from + `"},"to":{"pivotBound":"` + to + `"}}}}}`
}

func TestADrillIsRefusedWhereItsPivotOrFormDoesNotFit(t *testing.T) {
	z := newZoo(t)
	shipDrillTarget(t, nil, false)
	// A multiple choice whose options have no group.
	RegisterShipped(Shipped{Key: "zoo.drill_ungrouped", Title: "Ungrouped", Revision: 1, Spec: json.RawMessage(`{"version":4,"source":"zoo.feedings",
		"params":[{"id":"animals","type":"choice","multiple":true,"optionsFrom":{"source":"zoo.animals","value":[{"field":"id"}],"label":[{"field":"name"}]}}],
		"columns":[{"id":"note","path":[{"field":"note"}]}]}`)})
	for name, c := range map[string]struct{ raw, at, contains string }{
		"pivotCells on another column": {drilledPivotOn("cost", pivotDrill("from", "to"), monthPivot),
			"columns.3.drill.pivotCells", "pivotCells goes on the pivot's cell column"},
		"a pivotBound not from or to": {drilledPivotOn("grams", pivotDrill("from", "mid"), monthPivot),
			"columns.2.drill.pivotCells.period.to", `pivotBound is "from" or "to"`},
		"a pivotBound without a bucket": {drilledPivotOn("grams", pivotDrill("from", "to"), `{"path":[{"field":"fed"}],"cell":"grams"}`),
			"columns.2.drill.pivotCells.period.from", "pivotBound needs a pivot by a bucket of a date"},
		"a group into options with no group": {drilledFeedingsOn("grams", `{"report":"zoo.drill_ungrouped","levels":{"g":{"animals":{"group":{"groupKey":"g"}}}}}`),
			"columns.2.drill.levels.g.animals", "does not fit param animals (choice)"},
		"a formula into a dateRange": {drilledFeedingsOn("grams", `{"report":"zoo.drill_target","levels":{"g":{"period":{"groupKey":"g"}}}}`),
			"columns.2.drill.levels.g.period", "does not fit param period (dateRange)"},
	} {
		t.Run(name, func(t *testing.T) {
			requireSpecError(t, z.validate(t, c.raw), 400, c.at, c.contains)
		})
	}
}

func TestADrillOnThePivotCellLaysItsPartsOverTheLevels(t *testing.T) {
	z := newZoo(t)
	shipDrillTarget(t, nil, false)
	raw := drilledPivotOn("grams", pivotDrill("from", "to"), monthPivot)
	if err := z.validate(t, raw); err != nil {
		t.Fatal(err)
	}
	spec, _, err := Load([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	p, serr := bind(z.app, spec)
	if serr != nil {
		t.Fatal(serr)
	}
	if len(p.drills) != 1 {
		t.Fatalf("drills = %d, want 1", len(p.drills))
	}
	var got []string
	for _, part := range p.drills[0].pivot {
		got = append(got, part.param+"/"+part.form+"@"+part.at)
	}
	want := "period/from@columns.2.drill.pivotCells.period.from,period/to@columns.2.drill.pivotCells.period.to"
	if strings.Join(got, ",") != want {
		t.Fatalf("pivot parts = %v, want %s", got, want)
	}
}

// A groupKey outside a drill is refused where a group level compiles it.
func TestGroupKeyIsRefusedInAGroupLevelWithoutADrill(t *testing.T) {
	z := newZoo(t)
	for name, c := range map[string]struct{ group, at string }{
		"a having": {`"having":{"op":"eq","args":[{"groupKey":"g"},{"value":"x"}]}`, "groups.0.having.args.0"},
		"a label":  {`"label":{"groupKey":"g"}`, "groups.0.label"},
	} {
		t.Run(name, func(t *testing.T) {
			raw := `{"version":4,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]},{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
				"groups":[{"id":"g","col":"note",` + c.group + `}]}`
			requireSpecError(t, z.validate(t, raw), 400, c.at, errGroupKeyOutside)
		})
	}
}

// withinTenSeconds runs fn and crashes the test binary with every
// goroutine's stack when it does not return in 10 s: a deadlock holds
// shippedMu, so a failing test's cleanup (ResetForTesting) would hang too.
func withinTenSeconds(what string, fn func()) {
	timer := time.AfterFunc(10*time.Second, func() {
		panic(what + " did not return in 10 s: a deadlock")
	})
	defer timer.Stop()
	fn()
}

// drilledShipped is a shipped zoo report whose grams column drills into
// zoo.drill_target.
func drilledShipped() Shipped {
	return Shipped{Key: "zoo.drilled_feedings", Title: "Drilled feedings", Spec: json.RawMessage(drilledFeedingsOn("grams", goodDrill)), Revision: 1}
}

// A shipped report with a drill validates its target while reconciling,
// which reads the shipped registry: reconciling must not hold its lock.
func TestAShippedReportWithADrillReconciles(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, nil, false)
	RegisterShipped(drilledShipped())
	var err error
	withinTenSeconds("ReconcileShipped", func() { err = ReconcileShipped(s.app) })
	if err != nil {
		t.Fatal(err)
	}
	if shippedRow(t, s, "zoo.drilled_feedings") == nil {
		t.Fatal("the drilled report was not shipped")
	}
}

// The first admin's save ships a report with a drill (shipForAdmin).
func TestTheFirstAdminShipsAShippedReportWithADrill(t *testing.T) {
	s := newSavedZoo(t)
	s.recs["ad"].Set("role", "member")
	if err := s.app.Save(s.recs["ad"]); err != nil {
		t.Fatal(err)
	}
	shipDrillTarget(t, nil, false)
	RegisterShipped(drilledShipped())
	withinTenSeconds("the first admin's save", func() { s.person(t, "own", "owner") })
	if shippedRow(t, s, "zoo.drilled_feedings") == nil {
		t.Fatal("the first admin's save did not ship the drilled report")
	}
}

func TestGroupKeyAndPivotBoundAreReadOnlyInADrill(t *testing.T) {
	z := newZoo(t)
	for name, c := range map[string]struct{ leaf, contains string }{
		"groupKey":   {`{"groupKey":"g"}`, "groupKey is read only in a drill value"},
		"pivotBound": {`{"pivotBound":"from"}`, "pivotBound is read only in a drill's pivotCells"},
	} {
		t.Run(name, func(t *testing.T) {
			raw := `{"version":4,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]}],
				"groups":[{"id":"g","col":"note"}],
				"filter":{"op":"eq","args":[{"path":[{"field":"note"}]},` + c.leaf + `]}}`
			requireSpecError(t, z.validate(t, raw), 400, "filter.args.1", c.contains)
		})
	}
}

// drillsOf lists every row as "<label> <its col@drill JSON>" ("" for none).
func drillsOf(t *testing.T, res *Result, col string) []string {
	t.Helper()
	out := make([]string, 0, len(res.Rows))
	for _, r := range res.Rows {
		out = append(out, r.Label+" "+drillJSON(t, r, col))
	}
	return out
}

func drillJSON(t *testing.T, r Row, col string) string {
	t.Helper()
	v, ok := r.Cells[col+"@drill"]
	if !ok {
		return ""
	}
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// valueForms drills from both levels with every formula form: a single
// choice and a list item from groupKey, a day and an as-of day from a
// level's dates, a range of them, and a copied param.
const valueForms = `{"report":"zoo.drill_target","levels":{
	"g":{"species":{"groupKey":"g"},"period":{"param":"period"},
		"fedOn":{"op":"max","args":[{"path":[{"field":"fed"}]}]},
		"asOf":{"op":"min","args":[{"path":[{"field":"fed"}]}]}},
	"a":{"animals":{"groupKey":"a"},"species":{"groupKey":"g"},
		"period":{"range":{"from":{"op":"min","args":[{"path":[{"field":"fed"}]}]},"to":{"op":"max","args":[{"path":[{"field":"fed"}]}]}}}}}}`

func TestAGroupRowDrillsWithEveryValueForm(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, s, false)
	res := s.run(t, "ann", drilledFeedingsOn("grams", valueForms), Options{Today: "2026-06-30"},
		map[string]string{"period": `{"preset":"thisFiscalYearToDate"}`})
	lion := `{"asOf":"2026-01-05","fedOn":"2026-02-10","period":{"preset":"thisFiscalYearToDate"},"species":"lion"}`
	zebra := `{"asOf":"2026-01-20","fedOn":"2026-03-03","period":{"preset":"thisFiscalYearToDate"},"species":"zebra"}`
	leo := fmt.Sprintf(`{"animals":[%q],"period":{"from":"2026-01-05","to":"2026-02-10"},"species":"lion"}`, s.id("leo"))
	zed := fmt.Sprintf(`{"animals":[%q],"period":{"from":"2026-01-20","to":"2026-03-03"},"species":"zebra"}`, s.id("zed"))
	want := []string{"lion " + lion, "Leo " + leo, "Total lion " + lion, "zebra " + zebra, "Zed " + zed, "Total zebra " + zebra, "Total "}
	if got := drillsOf(t, res, "grams"); !slices.Equal(got, want) {
		t.Fatalf("drills\n got %q\nwant %q", got, want)
	}
	if got := drillsOf(t, res, "animal"); slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, "{") }) {
		t.Fatalf("an undrilled column has drill cells: %q", got)
	}
}

func TestADrillGroupPickWritesTheGroupReference(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, s, false)
	res := s.run(t, "ann", drilledFeedingsOn("grams", `{"report":"zoo.drill_target","levels":{"g":{"animals":{"group":{"groupKey":"g"}}}}}`), Options{}, nil)
	if got := drillJSON(t, res.Rows[0], "grams"); got != `{"animals":[{"group":"lion"}]}` {
		t.Fatalf("lion drill = %s", got)
	}
}

// A relation group level's record drills into a multiple choice as the one
// item of its list: the record's id.
func TestARelationLevelsRecordDrillsAsTheOneItemOfAList(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, s, false)
	drill := `{"report":"zoo.drill_target","levels":{"a":{"animals":{"record":"a","path":[{"field":"id"}]}}}}`
	res := s.run(t, "ann", drilledFeedingsOn("grams", drill), Options{}, nil)
	want := map[string]string{
		"Leo": fmt.Sprintf(`{"animals":[%q]}`, s.id("leo")),
		"Zed": fmt.Sprintf(`{"animals":[%q]}`, s.id("zed")),
	}
	seen := 0
	for _, r := range res.Rows {
		w, ok := want[r.Label]
		if !ok || r.Kind != KindGroup {
			continue
		}
		seen++
		if got := drillJSON(t, r, "grams"); got != w {
			t.Fatalf("%s drill = %s, want %s", r.Label, got, w)
		}
	}
	if seen != 2 {
		t.Fatalf("animal rows = %d, want 2", seen)
	}
}

func TestADrillCopiesTheParamAsTheRunWasGivenIt(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, s, false)
	copyOf := func(target, source string) string {
		return `{"report":"zoo.drill_target","levels":{"g":{"` + target + `":{"param":"` + source + `"}}}}`
	}
	drill := copyOf("period", "period")
	for name, c := range map[string]struct {
		drill  string
		params map[string]string
		want   string
	}{
		"a preset":  {drill, map[string]string{"period": `{"preset":"thisFiscalYearToDate"}`}, `{"period":{"preset":"thisFiscalYearToDate"}}`},
		"a custom":  {drill, map[string]string{"period": `{"from":"2026-01-01","to":"2026-01-31"}`}, `{"period":{"from":"2026-01-01","to":"2026-01-31"}}`},
		"all dates": {drill, map[string]string{"period": `{"preset":"allDates"}`}, `{"period":{"preset":"allDates"}}`},
		"unposted":  {drill, nil, `{"period":{"preset":"allDates"}}`},
		// The URL reads a bare number as units, so bare cents are written as an object.
		"an amount":      {copyOf("cap", "cap"), map[string]string{"cap": `12000`}, `{"cap":{"amount":12000,"currency":""}}`},
		"an unset param": {copyOf("note", "label"), nil, `{"note":null}`},
	} {
		t.Run(name, func(t *testing.T) {
			res := s.run(t, "ann", drilledFeedingsOn("grams", c.drill), Options{Today: "2026-06-30"}, c.params)
			if got := drillJSON(t, res.Rows[0], "grams"); got != c.want {
				t.Fatalf("drill = %s, want %s", got, c.want)
			}
		})
	}
}

func TestADrillValueThatIsBlankLeavesTheCellWithNoTarget(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, s, false)
	// Nub has no birth day: its species row's fedOn is blank.
	nub := s.rec(t, "nub", "zoo_animals", map[string]any{"name": "Nub", "species": "newt", "owner": s.id("ann")})
	s.rec(t, "k9", "zoo_keepers", map[string]any{"user": s.id("ann"), "animal": nub.Id})
	s.feeding(t, "f7", nub, 10, "2026-02-01", 50)
	drill := `{"report":"zoo.drill_target","levels":{"g":{"species":{"groupKey":"g"},
		"fedOn":{"op":"max","args":[{"path":[{"link":"animal"},{"field":"born"}]}]}}}}`
	res := s.run(t, "ann", drilledFeedingsOn("grams", drill), Options{}, nil)
	species := map[string]bool{"lion": true, "zebra": true, "newt": true}
	for _, r := range res.Rows {
		// Only level g drills; the open animal rows below it never do.
		if r.Kind != KindGroup || !species[r.Label] {
			continue
		}
		got := drillJSON(t, r, "grams")
		if (r.Label == "newt") != (got == "") {
			t.Fatalf("%s drill = %q", r.Label, got)
		}
	}
}

func TestOnlyTheDrilledLevelsRowsDrill(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, s, false)
	res := s.run(t, "ann", `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum","drill":{"report":"zoo.drill_target","levels":{"g":{"species":{"groupKey":"g"}}}}}],
		"groups":[{"id":"g","col":"species","subtotals":true,"open":true}]}`, Options{}, nil)
	drilled := 0
	for _, r := range res.Rows {
		got := drillJSON(t, r, "grams")
		switch {
		case (r.Kind == KindGroup || r.Kind == KindSubtotal) && got != "":
			drilled++
		case r.Kind != KindGroup && r.Kind != KindSubtotal && got != "":
			t.Fatalf("%s row %s drills: %s", r.Kind, r.Label, got)
		}
	}
	if drilled != 4 {
		t.Fatalf("drilled group and subtotal rows = %d, want 4", drilled)
	}
}

// A layout's computed row is a leg of the group statement too: it carries
// the drill cells blank, and draws none.
func TestALayoutSectionDrillsAndItsComputedRowDoesNot(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, s, false)
	res := s.run(t, "ann", `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum","drill":{"report":"zoo.drill_target","levels":{"g":{"species":{"groupKey":"g"}}}}}],
		"groups":[{"id":"g","col":"species"}],
		"layout":[{"section":"cats","title":"Lions","key":"lion"},{"section":"stripes","title":"Zebras","key":"zebra"},
			{"row":"both","label":"Both","formula":{"op":"add","args":[{"subtotal":"cats"},{"subtotal":"stripes"}]}}]}`, Options{}, nil)
	want := []string{`Lions {"species":"lion"}`, `Total Lions {"species":"lion"}`, `Zebras {"species":"zebra"}`, `Total Zebras {"species":"zebra"}`, "Both "}
	if got := drillsOf(t, res, "grams"); !slices.Equal(got, want) {
		t.Fatalf("drills\n got %q\nwant %q", got, want)
	}
}

func TestNoDrillCellsWithoutAccessToTheTarget(t *testing.T) {
	s := newSavedZoo(t)
	shipDrillTarget(t, s, true)
	res := s.run(t, "ann", drilledFeedingsOn("grams", valueForms), Options{}, nil)
	for _, r := range res.Rows {
		if got := drillJSON(t, r, "grams"); got != "" {
			t.Fatalf("row %s drills %s for a reader who cannot view the target", r.Label, got)
		}
	}
}

func TestAnAmountCopyKeepsItsCents(t *testing.T) {
	for raw, want := range map[string]string{
		`12000`:                             `{"amount":12000,"currency":""}`,
		`{"amount":12000,"currency":"USD"}`: `{"amount":12000,"currency":"USD"}`,
		`null`:                              `null`,
	} {
		if got := string(amountJSON(json.RawMessage(raw))); got != want {
			t.Errorf("amountJSON(%s) = %s, want %s", raw, got, want)
		}
	}
}

func TestAFormulaOfTheWrongTypeIsRefused(t *testing.T) {
	z := newZoo(t)
	shipDrillTarget(t, nil, false)
	for name, c := range map[string]struct{ values, at string }{
		"text into a day":     {`"fedOn":{"groupKey":"g"}`, "columns.2.drill.levels.g.fedOn"},
		"a number to choose":  {`"species":{"op":"sum","args":[{"path":[{"field":"grams"}]}]}`, "columns.2.drill.levels.g.species"},
		"text into a range":   {`"period":{"range":{"from":{"groupKey":"g"},"to":{"groupKey":"g"}}}`, "columns.2.drill.levels.g.period.from"},
		"a number to a group": {`"animals":{"group":{"op":"sum","args":[{"path":[{"field":"grams"}]}]}}`, "columns.2.drill.levels.g.animals.group"},
	} {
		t.Run(name, func(t *testing.T) {
			raw := drilledFeedingsOn("grams", `{"report":"zoo.drill_target","levels":{"g":{`+c.values+`}}}`)
			requireSpecError(t, z.validate(t, raw), 400, c.at, "does not fit")
		})
	}
}
