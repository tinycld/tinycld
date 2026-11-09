package reports

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// Species › site region (blank: skip) › animal. Kit (a cat) has no site, so
// its region is blank and Kit sits directly under "cat".
const zooRagged = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"animal","path":[{"link":"animal"}]},
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"region","path":[{"link":"animal"},{"link":"site"},{"field":"region"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
	{"id":"shown","path":[{"field":"cost"}],"aggregate":"sum","total":"rows"}],
	"groups":[{"id":"s","col":"species","subtotals":true,"open":true},
		{"id":"r","col":"region","blank":"skip","subtotals":true,"open":true%s},
		{"id":"a","col":"animal","subtotals":true}],
	"sort":[{"col":"species"}]}`

// raggedZoo lets Ann keep Kit, so Ann reads f6 (Kit, 99, no site).
func raggedZoo(t *testing.T) *zoo {
	z := newZoo(t)
	z.rec(t, "k5", "zoo_keepers", map[string]any{"user": z.id("ann"), "animal": z.id("kit")})
	return z
}

func TestRaggedLevelSkipsABlankKey(t *testing.T) {
	z := raggedZoo(t)
	res := z.run(t, "ann", fmt.Sprintf(zooRagged, ""), Options{}, nil)
	requireRows(t, res, "label", "cat", "Kit", "Total cat", "lion", "North", "Leo", "Total North", "Total lion", "zebra", "South", "Zed", "Total South", "Total zebra", "Total")
	requireRows(t, res, "kind", "group", "group", "subtotal", "group", "group", "group", "subtotal", "subtotal", "group", "group", "group", "subtotal", "subtotal", "total")
	// Kit is one depth less than Leo and Zed; its level stays 3.
	requireRows(t, res, "depth", 0, 1, 0, 0, 1, 2, 1, 0, 0, 1, 2, 1, 0, 0)
	requireRows(t, res, "level", 1, 3, 1, 1, 2, 3, 2, 1, 1, 2, 3, 2, 1, 0)
	// The parent adds the passed row, in both total modes.
	// Group rows carry their figures too (the client hides those a subtotal repeats).
	requireRows(t, res, "cost", int64(99), int64(99), int64(99), int64(3500), int64(3500), int64(3500), int64(3500), int64(3500), int64(1000), int64(1000), int64(1000), int64(1000), int64(1000), int64(4599))
	requireRows(t, res, "shown", int64(99), int64(99), int64(99), int64(3500), int64(3500), int64(3500), int64(3500), int64(3500), int64(1000), int64(1000), int64(1000), int64(1000), int64(1000), int64(4599))
	// No (none) group, and Kit keeps the blank region in its key.
	if res.Rows[1].ID != fmt.Sprintf(`g:["cat",null,%q,"USD"]`, z.id("kit")) {
		t.Fatalf("Kit's id = %s", res.Rows[1].ID)
	}
}

// A text key that is "" (not NULL) at a skip level is blank too: f9, Leo's
// feeding with no note, sits directly under "lion" with no "(none)" group and
// no subtotal of its own, and "lion" adds it.
func TestRaggedLevelSkipsAnEmptyTextKey(t *testing.T) {
	z := raggedZoo(t)
	z.rec(t, "f9", "zoo_feedings", map[string]any{
		"animal": z.id("leo"), "cost": 40, "currency": "USD", "fed": "2026-03-31 00:00:00.000Z", "due": "2026-03-31", "grams": 10, "note": "",
	})
	raw := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"s","col":"species","subtotals":true,"open":true},
			{"id":"n","col":"note","blank":"skip","subtotals":true,"open":true},
			{"id":"a","col":"animal","subtotals":true}],
		"sort":[{"col":"species"},{"col":"note"}]}`
	res := z.run(t, "ann", raw, Options{}, nil)
	for _, r := range res.Rows {
		if strings.Contains(r.Label, "(none)") {
			t.Fatalf("a blank text key formed a group: row %q (%s)", r.Label, r.ID)
		}
	}
	start, end := -1, -1
	for i, r := range res.Rows {
		switch r.Label {
		case "lion":
			start = i
		case "Total lion":
			end = i
		}
	}
	if start < 0 || end < start {
		t.Fatalf("no lion group: %v", rowsOf(res, "label"))
	}
	// Under lion: the note groups f1 and f2 (each with Leo under it at depth 2)
	// and f9's Leo directly under lion at depth 1, level 3.
	var direct, notes []string
	for _, r := range res.Rows[start+1 : end] {
		if r.Depth != 1 {
			continue
		}
		switch r.Kind {
		case "subtotal":
			notes = append(notes, r.Label)
		case "group":
			if r.Level == 3 {
				direct = append(direct, r.Label)
			}
		}
	}
	if !reflect.DeepEqual(direct, []string{"Leo"}) {
		t.Fatalf("rows directly under lion = %v, want [Leo]\nrows %v\ndepths %v\nlevels %v", direct, rowsOf(res, "label"), rowsOf(res, "depth"), rowsOf(res, "level"))
	}
	if !reflect.DeepEqual(notes, []string{"Total f1", "Total f2"}) {
		t.Fatalf("note subtotals under lion = %v, want [Total f1 Total f2]", notes)
	}
	if got := res.Rows[end].Cells["cost"]; got != int64(3540) {
		t.Fatalf("Total lion cost = %#v, want 3540 (1000 + 2500 + the passed 40)", got)
	}
}

func TestRaggedRowsFollowTheirParentsFold(t *testing.T) {
	z := raggedZoo(t)
	shut := z.run(t, "ann", fmt.Sprintf(zooRagged, ""), Options{Expand: [][]any{}}, nil)
	requireRows(t, shut, "label", "cat", "lion", "zebra", "Total")
	cat := shut.Rows[0].GroupKey
	open := z.run(t, "ann", fmt.Sprintf(zooRagged, ""), Options{Expand: [][]any{cat}}, nil)
	requireRows(t, open, "label", "cat", "Kit", "Total cat", "lion", "zebra", "Total")
}

func TestRaggedLevelRefusals(t *testing.T) {
	z := newZoo(t)
	cases := map[string]struct{ groups, at, want string }{
		"last level":  {`{"id":"s","col":"species"},{"id":"a","col":"animal","blank":"skip"}`, "groups.1.blank", "the last level"},
		"first level": {`{"id":"r","col":"region","blank":"skip"},{"id":"a","col":"animal"}`, "groups.0.blank", "no group above"},
		"bucketed":    {`{"id":"s","col":"species"},{"id":"m","col":"fed","bucket":"month","blank":"skip"},{"id":"a","col":"animal"}`, "groups.1.blank", "bucket"},
		"other value": {`{"id":"s","col":"species"},{"id":"r","col":"region","blank":"keep"},{"id":"a","col":"animal"}`, "groups.1.blank", `"skip"`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			raw := `{"version":4,"source":"zoo.feedings","columns":[
				{"id":"animal","path":[{"link":"animal"}]},
				{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
				{"id":"region","path":[{"link":"animal"},{"link":"site"},{"field":"region"}]},
				{"id":"fed","path":[{"field":"fed"}]},
				{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],"groups":[` + c.groups + `]}`
			spec, _, err := Load([]byte(raw))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			requireSpecError(t, Validate(z.app, spec), 400, c.at, c.want)
		})
	}
}

func raggedHaving(level string) string {
	return fmt.Sprintf(`,"having":{"op":"gte","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":%s}]}`, level)
}

// A skip level's having is a rule for groups: it hides South (1000) but does
// not apply to the blank key, which passes while Kit shows under it.
func TestRaggedHavingDoesNotApplyToTheBlankKey(t *testing.T) {
	z := raggedZoo(t)
	res := z.run(t, "ann", fmt.Sprintf(zooRagged, raggedHaving("2000")), Options{}, nil)
	requireRows(t, res, "label", "cat", "Kit", "Total cat", "lion", "North", "Leo", "Total North", "Total lion", "zebra", "Total")
	requireRows(t, res, "shown", int64(99), int64(99), int64(99), int64(3500), int64(3500), int64(3500), int64(3500), int64(3500), int64(0), int64(3599))
}

// A having on the level below hides Kit (99), so the blank key has nothing
// shown under it and goes too.
func TestRaggedBlankKeyNeedsAShownChild(t *testing.T) {
	z := raggedZoo(t)
	spec := strings.Replace(fmt.Sprintf(zooRagged, ""), `{"id":"a","col":"animal","subtotals":true}`, `{"id":"a","col":"animal","subtotals":true`+raggedHaving("100")+`}`, 1)
	res := z.run(t, "ann", spec, Options{}, nil)
	requireRows(t, res, "label", "cat", "lion", "North", "Leo", "Total North", "Total lion", "zebra", "South", "Zed", "Total South", "Total zebra", "Total")
	requireRows(t, res, "shown", int64(0), int64(3500), int64(3500), int64(3500), int64(3500), int64(3500), int64(1000), int64(1000), int64(1000), int64(1000), int64(1000), int64(4500))
}

// The cats' feedings meet two currencies (Kit has a EUR one), so a having that
// sums a group's species costs is unknown on the cat rows. The only cat row of
// the region level is the blank key, to which the having does not apply, so the
// run names no mixed having.
func TestRaggedBlankKeyIsNotProbedForAMixedHaving(t *testing.T) {
	z := raggedZoo(t)
	z.rec(t, "f8", "zoo_feedings", map[string]any{
		"animal": z.id("kit"), "cost": 40, "currency": "EUR", "fed": "2026-03-31 00:00:00.000Z", "due": "2026-03-31", "grams": 10, "note": "f8",
	})
	speciesCosts := `{"op":"lookup","source":"zoo.feedings",
		"where":{"op":"eq","args":[{"path":[{"link":"animal"},{"field":"species"}]},{"col":"species"}]},
		"value":{"path":[{"field":"cost"}]},"pick":"sum"}`
	having := `,"having":{"op":"gte","args":[` + speciesCosts + `,{"value":0}]}`
	res := z.run(t, "ann", fmt.Sprintf(zooRagged, having), Options{}, nil)
	if n := conditionNotice(res, "groups.1.having"); n != nil {
		t.Fatalf("the blank key was probed: %+v", n)
	}
}

func TestRaggedPagesEqualTheWholeRun(t *testing.T) {
	z := raggedZoo(t)
	raw := fmt.Sprintf(zooRagged, "")
	whole := z.run(t, "ann", raw, Options{ExpandAll: true, Limit: 1000}, nil)
	pages := groupedPages(t, z, "ann", raw, Options{ExpandAll: true, Limit: 1})
	if len(pages) < 3 {
		t.Fatalf("%d pages: the run must page across the skipped level", len(pages))
	}
	if merged := mergedRows(t, pages); !reflect.DeepEqual(merged, whole.Rows) {
		t.Fatalf("pages\n%v\nwhole\n%v", merged, whole.Rows)
	}
}

// Paged group rows (J1) over a skipped level: the skipped blank key draws no
// row, so the limit does not count it (a page of it would end early and lose
// the rows after it), the cursor still reads its order values as an enclosing
// group of Kit, and a page that starts under it starts after "cat". Every
// limit, with the open levels, every group open, and "lion" folded.
func TestRaggedGroupRowsPageAtEveryLimit(t *testing.T) {
	z := raggedZoo(t)
	raw := fmt.Sprintf(zooRagged, "")
	for _, opts := range []Options{{}, {ExpandAll: true}, {Collapse: [][]any{{"lion", "USD"}}}} {
		whole := z.run(t, "ann", raw, Options{ExpandAll: opts.ExpandAll, Collapse: opts.Collapse, Limit: 1000}, nil)
		for limit := 1; limit <= 12; limit++ {
			opts.Limit = limit
			pages := groupedPages(t, z, "ann", raw, opts)
			if merged := mergedRows(t, pages); !reflect.DeepEqual(merged, whole.Rows) {
				t.Fatalf("%+v: pages\n%v\nwhole\n%v", opts, rowsOf(&Result{Rows: merged}, "id"), rowsOf(whole, "id"))
			}
			requireCut(t, pages, whole, limit)
		}
	}
}

func TestRaggedCursorFromAnotherGroupingIsRefused(t *testing.T) {
	z := raggedZoo(t)
	page := z.run(t, "ann", fmt.Sprintf(zooRagged, ""), Options{ExpandAll: true, Limit: 1}, nil)
	plain := strings.Replace(fmt.Sprintf(zooRagged, ""), `"blank":"skip",`, "", 1)
	_, err := z.try(t, "ann", plain, Options{ExpandAll: true, Limit: 1, Cursor: page.Page.Next}, nil)
	requireSpecError(t, err, 400, "options.cursor", "not one this report gave")
}

// A fill on the animal level below the skipped region: the animals Ann may list
// with no feeding under "cat" fill in under "cat" itself, where Kit's rows go.
func TestRaggedFillFillsUnderTheParentTheRowsReach(t *testing.T) {
	z := raggedZoo(t)
	raw := strings.Replace(fmt.Sprintf(zooRagged, ""), `{"id":"a","col":"animal","subtotals":true}`, `{"id":"a","col":"animal","subtotals":true,"fill":{}}`, 1)
	res := z.run(t, "ann", raw, Options{}, nil)
	var underCat []any
	for _, r := range res.Rows[1:] {
		if r.Label == "Total cat" {
			break
		}
		if r.Depth != 1 || r.Level != 3 {
			t.Fatalf("row %q under cat at depth %d level %d", r.Label, r.Depth, r.Level)
		}
		underCat = append(underCat, r.Label)
	}
	// Ann lists Leo and Kit (owner) and Zed (keeper); Leo and Zed have no cat
	// feeding, so they fill in beside Kit.
	if !reflect.DeepEqual(underCat, []any{"Kit", "Leo", "Zed"}) {
		t.Fatalf("under cat: %v", underCat)
	}
}
