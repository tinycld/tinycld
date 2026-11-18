package reports

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// ann sees leo (lion: f1 1000, f2 2500) and zed (zebra: f3 700, f4 300).
const blankAboveRun = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"species","path":[{"link":"animal"},{"field":"species"}],"hidden":true},
	{"id":"animal","path":[{"link":"animal"}],"hidden":true},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum","blankAbove":"%s"},
	{"id":"twice","formula":{"op":"add","args":[{"col":"cost"},{"col":"cost"}]}},
	{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
	"groups":[{"id":"bySpecies","col":"species","subtotals":true},{"id":"byAnimal","col":"animal","subtotals":true}],
	"sort":[{"col":"note"}]}`

// blankAboveFlags maps each result column of res to its BlankAbove.
func blankAboveFlags(res *Result) map[string]bool {
	flags := map[string]bool{}
	for _, c := range res.Columns {
		flags[c.ID] = c.BlankAbove
	}
	return flags
}

func requireBlankAbove(t *testing.T, res *Result, want map[string]bool) {
	t.Helper()
	got := blankAboveFlags(res)
	for id, w := range want {
		if v, ok := got[id]; !ok || v != w {
			t.Fatalf("column %s: BlankAbove = %v (present %v), want %v; all %v", id, v, ok, w, got)
		}
	}
}

// The result column carries blankAbove: by its own setting, or because it
// reads a blank-above column; never sent as false.
func TestBlankAboveMarksTheResultColumn(t *testing.T) {
	z := newZoo(t)
	for _, level := range []string{"byAnimal", "bySpecies"} {
		res := z.run(t, "ann", fmt.Sprintf(blankAboveRun, level), Options{}, nil)
		requireBlankAbove(t, res, map[string]bool{"cost": true, "twice": true, "note": false, "grams": false})
	}

	derived := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"animal","path":[{"link":"animal"}],"hidden":true},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum","blankAbove":"byAnimal"},
		{"id":"twice","formula":{"op":"add","args":[{"col":"cost"},{"col":"cost"}]}},
		{"id":"plus","formula":{"op":"add","args":[{"col":"cost"},
			{"op":"lookup","source":"zoo.feedings","pick":"sum","value":{"path":[{"field":"grams"}]}}]}}],
		"groups":[{"id":"g","col":"species"},{"id":"byAnimal","col":"animal","subtotals":true}]}`
	res := z.run(t, "ann", derived, Options{}, nil)
	requireBlankAbove(t, res, map[string]bool{"cost": true, "twice": true, "plus": true, "species": false})

	for _, c := range res.Columns {
		raw, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		switch c.ID {
		case "cost":
			if !strings.Contains(string(raw), `"blankAbove":true`) {
				t.Fatalf("cost column json %s has no blankAbove true", raw)
			}
		case "species":
			if strings.Contains(string(raw), `"blankAbove"`) {
				t.Fatalf("species column json %s carries blankAbove", raw)
			}
		}
	}
}

// outer is a row of the levels above level: its group and subtotal rows, and
// the Total.
func outer(r Row, level int) bool {
	return r.Kind == KindTotal || ((r.Kind == KindGroup || r.Kind == KindSubtotal) && r.Level < level)
}

func requireNoCell(t *testing.T, r Row, id string) {
	t.Helper()
	for _, k := range []string{id, id + "@currency", id + "@label"} {
		if _, ok := r.Cells[k]; ok {
			t.Fatalf("row %s (%s) carries %s = %v", r.ID, r.Kind, k, r.Cells[k])
		}
	}
}

func TestBlankAboveLeavesTheOuterLevelsAndTheTotalBlank(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", fmt.Sprintf(blankAboveRun, "byAnimal"), Options{ExpandAll: true}, nil)
	var subtotals, details []any
	for _, r := range res.Rows {
		switch {
		case outer(r, 2):
			requireNoCell(t, r, "cost")
			// A {col} read of it there reads blank too (levels.go).
			if r.Cells["twice"] != nil {
				t.Fatalf("row %s: twice = %v", r.ID, r.Cells["twice"])
			}
			if r.Kind != KindGroup && r.Cells["grams"] == nil {
				t.Fatalf("row %s lost its grams", r.ID)
			}
		case r.Kind == KindSubtotal:
			subtotals = append(subtotals, r.Cells["cost"], r.Cells["cost@currency"], r.Cells["twice"])
		case r.Kind == KindDetail:
			details = append(details, r.Cells["cost"])
		}
	}
	if fmt.Sprint(subtotals) != fmt.Sprint([]any{int64(3500), "USD", int64(7000), int64(1000), "USD", int64(2000)}) {
		t.Fatalf("animal subtotals = %v", subtotals)
	}
	if fmt.Sprint(details) != fmt.Sprint([]any{int64(1000), int64(2500), int64(700), int64(300)}) {
		t.Fatalf("details = %v", details)
	}
	total := res.Rows[len(res.Rows)-1]
	if total.Kind != KindTotal || total.Cells["grams"] != int64(1700) {
		t.Fatalf("total = %+v", total)
	}
}

// Review Focus 4: in the default fold state the closed animal rows carry the
// figure and the species rows above them do not.
func TestBlankAboveHoldsOnACollapsedRun(t *testing.T) {
	z := newZoo(t)
	spec := fmt.Sprintf(blankAboveRun, "byAnimal")
	res := z.run(t, "ann", spec, Options{}, nil)
	requireRows(t, res, "kind", KindGroup, KindGroup, KindTotal)
	for _, r := range res.Rows {
		requireNoCell(t, r, "cost")
	}
	requireRows(t, res, "grams", int64(1300), int64(400), int64(1700))

	// Species open, animals closed by default: the closed animal rows carry the
	// figure, the species rows above them and the Total do not.
	open := strings.Replace(spec, `"col":"species","subtotals":true`, `"col":"species","subtotals":true,"open":true`, 1)
	res = z.run(t, "ann", open, Options{}, nil)
	var animals []any
	for _, r := range res.Rows {
		if outer(r, 2) {
			requireNoCell(t, r, "cost")
			continue
		}
		if r.Kind == KindGroup && r.Level == 2 {
			animals = append(animals, r.Cells["cost"], r.Cells["cost@currency"])
		}
	}
	if fmt.Sprint(animals) != fmt.Sprint([]any{int64(3500), "USD", int64(1000), "USD"}) {
		t.Fatalf("closed animal rows = %v", animals)
	}
}

// Review Focus 2: on the first group only the Total leaves the cell blank.
func TestBlankAboveTheFirstLevelBlanksOnlyTheTotal(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", fmt.Sprintf(blankAboveRun, "bySpecies"), Options{}, nil)
	requireRows(t, res, "cost", int64(3500), int64(1000), nil)
	requireRows(t, res, "cost@currency", "USD", "USD", nil)
}

// On a layout report the sections are the first level: their rows leave the
// cell blank, the levels inside keep it.
func TestBlankAboveLeavesLayoutSectionsBlank(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"animal","path":[{"link":"animal"}],"hidden":true},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum","blankAbove":"byAnimal"},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"},{"id":"byAnimal","col":"animal","subtotals":true}],
		"layout":[{"section":"cats","title":"Lions","key":"lion"},{"section":"stripes","title":"Zebras","key":"zebra"},
		{"row":"gap","label":"Lions less zebras","formula":{"op":"sub","args":[{"subtotal":"cats"},{"subtotal":"stripes"}]}},
		{"row":"again","label":"Gap again","formula":{"op":"add","args":[{"subtotal":"gap"},{"subtotal":"gap"}]}}]}`
	res := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
	var inner []any
	computed := 0
	for _, r := range res.Rows {
		// A computed row reads sections, which are above the level: its cost is
		// blank too, not a 0 computed from blanks. Its grams keep their figure.
		if r.Kind == KindComputed {
			computed++
			requireNoCell(t, r, "cost")
			if r.Cells["grams"] == nil {
				t.Fatalf("computed row %s lost its grams: %v", r.ID, r.Cells)
			}
			continue
		}
		if r.Level == 1 {
			requireNoCell(t, r, "cost")
			continue
		}
		if r.Kind == KindSubtotal {
			inner = append(inner, r.Cells["cost"])
		}
	}
	if fmt.Sprint(inner) != fmt.Sprint([]any{int64(3500), int64(1000)}) {
		t.Fatalf("animal subtotals = %v (rows %v)", inner, rowsOf(res, "id"))
	}
	if computed != 2 {
		t.Fatalf("computed rows = %d, want 2 (rows %v)", computed, rowsOf(res, "id"))
	}
}

// R8: a column that reads a blankAbove column is blank where that one is, with
// no currency either: on group, section, computed and Total rows. twice is a
// derived column (scalar reads of {col}); plus holds a lookup, so it is not
// derived and a computed row compiles it over the section cells.
func TestBlankAboveLeavesADerivedColumnWithoutACurrencyAboveTheLevel(t *testing.T) {
	z := newZoo(t)
	const columns = `"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"animal","path":[{"link":"animal"}],"hidden":true},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum","blankAbove":"byAnimal"},
		{"id":"twice","formula":{"op":"add","args":[{"col":"cost"},{"col":"cost"}]}},
		{"id":"plus","formula":{"op":"add","args":[{"col":"cost"},
			{"op":"lookup","source":"zoo.feedings","pick":"sum","value":{"path":[{"field":"grams"}]}}]}}],
		"groups":[{"id":"g","col":"species"},{"id":"byAnimal","col":"animal","subtotals":true}]`
	for _, c := range []struct{ name, spec string }{
		{"groups and total", `{` + columns + `}`},
		{"sections and computed rows", `{` + columns + `,
			"layout":[{"section":"cats","title":"Lions","key":"lion"},{"section":"stripes","title":"Zebras","key":"zebra"},
			{"row":"gap","label":"gap","formula":{"op":"sub","args":[{"subtotal":"cats"},{"subtotal":"stripes"}]}}]}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			res := z.run(t, "ann", c.spec, Options{ExpandAll: true}, nil)
			for _, id := range []string{"twice", "plus"} {
				kept := 0
				for _, r := range res.Rows {
					if outer(r, 2) || r.Kind == KindComputed || r.Level == 1 {
						requireNoCell(t, r, id)
					} else if r.Cells[id] != nil {
						kept++
					}
				}
				if kept == 0 {
					t.Fatalf("no row on the level or below shows %s: %v", id, rowsOf(res, "id"))
				}
			}
		})
	}
}

// R8 on a report with a skip level: cost is blank above the animal level (3),
// the skipped region level (2) included. Kit has no region, so it sits directly
// under "cat" with level 3 and keeps the figure; every row of a level above 3
// leaves cost out, and the Total too.
func TestBlankAboveOnAReportWithASkipLevel(t *testing.T) {
	z := raggedZoo(t)
	res := z.run(t, "ann", `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"region","path":[{"link":"animal"},{"link":"site"},{"field":"region"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum","blankAbove":"a"},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"groups":[{"id":"s","col":"species","subtotals":true,"open":true},
			{"id":"r","col":"region","blank":"skip","subtotals":true,"open":true},
			{"id":"a","col":"animal","subtotals":true}],
		"sort":[{"col":"species"}]}`, Options{}, nil)
	requireRows(t, res, "label", "cat", "Kit", "Total cat", "lion", "North", "Leo", "Total North", "Total lion", "zebra", "South", "Zed", "Total South", "Total zebra", "Total")
	requireRows(t, res, "level", 1, 3, 1, 1, 2, 3, 2, 1, 1, 2, 3, 2, 1, 0)
	for _, r := range res.Rows {
		if r.Level < 3 {
			requireNoCell(t, r, "cost")
			if r.Kind != KindGroup && r.Cells["grams"] == nil {
				t.Fatalf("row %s (%s) lost its grams", r.ID, r.Kind)
			}
			continue
		}
		if r.Cells["cost@currency"] != "USD" {
			t.Fatalf("row %s (%s) lost its currency: %v", r.ID, r.Kind, r.Cells)
		}
	}
	requireRows(t, res, "cost", nil, int64(99), nil, nil, nil, int64(3500), nil, nil, nil, nil, int64(1000), nil, nil, nil)
}

// R8: a rollup column that reads a blankAbove column is blank where that one is,
// currency included: its rollup runs on group, subtotal and Total rows.
func TestBlankAboveLeavesARollupThatReadsItWithoutACurrency(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"species","path":[{"link":"animal"},{"field":"species"}],"hidden":true},
		{"id":"animal","path":[{"link":"animal"}],"hidden":true},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum","blankAbove":"byAnimal"},
		{"id":"again","path":[{"field":"cost"}],"aggregate":"sum","rollup":{"col":"cost"}}],
		"groups":[{"id":"bySpecies","col":"species","subtotals":true},{"id":"byAnimal","col":"animal","subtotals":true}],
		"sort":[{"col":"note"}]}`, Options{ExpandAll: true}, nil)
	kept := 0
	for _, r := range res.Rows {
		if outer(r, 2) {
			requireNoCell(t, r, "again")
		} else if r.Cells["again"] != nil {
			kept++
		}
	}
	if kept == 0 {
		t.Fatalf("no row on the level or below shows again: %v", rowsOf(res, "id"))
	}
}

// R8: a column's record label goes with its value. who repeats the animal
// group's key, so its outer rows would show the animal and its label; above the
// note level the cell has neither, on the level and below it keeps both.
func TestBlankAboveLeavesALabelledColumnWithoutItsLabelAboveTheLevel(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}],"labelField":"species"},
		{"id":"note","path":[{"field":"note"}],"hidden":true},
		{"id":"who","formula":{"col":"animal"},"blankAbove":"byNote"},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"groups":[{"id":"byAnimal","col":"animal","subtotals":true},{"id":"byNote","col":"note","subtotals":true}]}`, Options{ExpandAll: true}, nil)
	kept := 0
	for _, r := range res.Rows {
		if outer(r, 2) {
			requireNoCell(t, r, "who")
			continue
		}
		if r.Cells["who"] != nil {
			kept++
			if r.Cells["who@label"] == nil {
				t.Fatalf("row %s (%s) keeps who = %v but lost its label", r.ID, r.Kind, r.Cells["who"])
			}
		}
	}
	if kept == 0 {
		t.Fatalf("no row on the level or below shows who: %v", rowsOf(res, "id"))
	}
}
