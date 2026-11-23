package reports

import (
	"reflect"
	"strings"
	"testing"
)

// feedingsGroupOnly is feedings grouped by animal, open, with subtotals, and
// five columns blank on detail rows: the animal's species (a text), birth
// day (a date) and site (a relation, which also sends a site@label cell), its
// weight (a number) and the cost (a sum of money). The species, the birth day
// and the site name the group, so they are group-only; the weight and the
// cost are figures. secret is a hidden blank text column.
const feedingsGroupOnly = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"animal","path":[{"link":"animal"}]},
	{"id":"species","formula":{"record":"g","path":[{"field":"species"}]},"detail":"blank"},
	{"id":"born","formula":{"record":"g","path":[{"field":"born"}]},"detail":"blank"},
	{"id":"site","path":[{"link":"animal"},{"link":"site"}],"detail":"blank"},
	{"id":"weight","formula":{"record":"g","path":[{"field":"weight"}]},"detail":"blank"},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum","detail":"blank"},
	{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"},
	{"id":"secret","formula":{"record":"g","path":[{"field":"name"}]},"detail":"blank","hidden":true}],
	"groups":[{"id":"g","col":"animal","open":true,"subtotals":true}],
	"sort":[{"col":"grams"}]}`

var (
	groupOnlyIDs = []string{"species", "born", "site"}
	figureIDs    = []string{"weight", "cost"}
)

// hasCellOf says the row has the column's cell or one of its <id>@ cells.
func hasCellOf(r Row, id string) bool {
	for key := range r.Cells {
		if key == id || strings.HasPrefix(key, id+"@") {
			return true
		}
	}
	return false
}

// sectionRows pairs each group row of the run with the subtotal row that
// ends it.
func sectionRows(t *testing.T, res *Result) map[string][2]Row {
	t.Helper()
	out := map[string][2]Row{}
	for _, r := range res.Rows {
		switch r.Kind {
		case KindGroup:
			pair := out[r.ID]
			pair[0] = r
			out[r.ID] = pair
		case KindSubtotal:
			pair := out[r.Section]
			pair[1] = r
			out[r.Section] = pair
		}
	}
	if len(out) != 2 {
		t.Fatalf("want two animals (Leo and Zed), got %d sections: %+v", len(out), res.Rows)
	}
	for id, pair := range out {
		if pair[0].ID == "" || pair[1].ID == "" {
			t.Fatalf("section %s has no group row or no subtotal row: %+v", id, pair)
		}
	}
	return out
}

func TestGroupOnly_ColumnFlag(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", feedingsGroupOnly, Options{}, nil)
	got := map[string]bool{}
	for _, c := range res.Columns {
		got[c.ID] = c.GroupOnly
	}
	want := map[string]bool{"animal": false, "species": true, "born": true, "site": true, "weight": false, "cost": false, "grams": false}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GroupOnly by column = %v, want %v", got, want)
	}
}

func TestGroupOnly_SubtotalRowDropsCells(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", feedingsGroupOnly, Options{}, nil)
	for id, pair := range sectionRows(t, res) {
		group, sub := pair[0], pair[1]
		for _, col := range groupOnlyIDs {
			if _, ok := group.Cells[col]; !ok {
				t.Fatalf("group row %s (%q) has no %s cell: %v", id, group.Label, col, group.Cells)
			}
			if hasCellOf(sub, col) {
				t.Fatalf("subtotal row of %s (%q) carries the group-only %s: %v", id, group.Label, col, sub.Cells)
			}
		}
		// The relation's label cell is the <id>@ key the subtotal row must drop too.
		if _, ok := group.Cells["site@label"]; !ok {
			t.Fatalf("group row %s (%q) has no site@label cell: %v", id, group.Label, group.Cells)
		}
		if _, ok := sub.Cells["site@label"]; ok {
			t.Fatalf("subtotal row of %s (%q) carries site@label: %v", id, group.Label, sub.Cells)
		}
		for _, col := range figureIDs {
			g, gok := group.Cells[col]
			s, sok := sub.Cells[col]
			if !gok || !sok || !reflect.DeepEqual(g, s) {
				t.Fatalf("%s of %s: group %v (%v), subtotal %v (%v), want both and equal", col, group.Label, g, gok, s, sok)
			}
		}
	}
	details := 0
	for _, r := range res.Rows {
		if r.Kind != KindDetail {
			continue
		}
		details++
		for _, col := range append(append([]string{}, groupOnlyIDs...), figureIDs...) {
			if _, ok := r.Cells[col]; ok {
				t.Fatalf("detail row %q has the blank %s cell: %v", r.Label, col, r.Cells)
			}
		}
	}
	if details != 4 {
		t.Fatalf("detail rows = %d, want 4 (f1 to f4)", details)
	}
}

func TestGroupOnly_ClosedGroupKeepsCells(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", feedingsGroupOnly, Options{Expand: [][]any{}}, nil)
	groups := 0
	for _, r := range res.Rows {
		if r.Kind == KindSubtotal || r.Kind == KindDetail {
			t.Fatalf("a closed run has a %s row: %+v", r.Kind, r)
		}
		if r.Kind != KindGroup {
			continue
		}
		groups++
		if r.Expanded {
			t.Fatalf("group row %q is open in a run that opens none", r.Label)
		}
		for _, col := range append(append([]string{}, groupOnlyIDs...), figureIDs...) {
			if _, ok := r.Cells[col]; !ok {
				t.Fatalf("closed group row %q has no %s cell: %v", r.Label, col, r.Cells)
			}
		}
	}
	if groups != 2 {
		t.Fatalf("group rows = %d, want 2", groups)
	}
}

func TestGroupOnly_TotalRow(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", feedingsGroupOnly, Options{}, nil)
	var total *Row
	for i := range res.Rows {
		if res.Rows[i].Kind == KindTotal {
			total = &res.Rows[i]
		}
	}
	if total == nil {
		t.Fatalf("no total row: %+v", res.Rows)
	}
	for _, col := range groupOnlyIDs {
		if v, ok := total.Cells[col]; ok && v != nil {
			t.Fatalf("total row has a %s value %v; a rec has no record at the total level", col, v)
		}
	}
	// Ann keeps Leo (1000 + 2500) and Zed (700 + 300).
	if got := total.Cells["cost"]; !reflect.DeepEqual(got, int64(4500)) {
		t.Fatalf("total cost = %#v, want 4500", got)
	}
}

// The ledger's No. is a rec number blank on detail rows: a figure, so not
// group-only, and its subtotal row keeps it, as before. This test checks only
// that ledger; the other existing outputs (detailblank_test.go,
// blankdetailcut_test.go, sectionpercent_test.go, the ledger_scale golden) are
// their own tests in this package and run unedited beside it.
func TestGroupOnly_FigureBlankColumnStaysOnSubtotalRows(t *testing.T) {
	z := newZoo(t)
	withLines(t, z)
	res := z.run(t, "ann", ledger(blankOnDetail), Options{ExpandAll: true}, nil)
	for _, c := range res.Columns {
		if c.GroupOnly {
			t.Fatalf("column %s is group-only in the ledger; its blank No. is a number", c.ID)
		}
	}
	subtotals := 0
	for _, r := range res.Rows {
		if r.Kind != KindSubtotal {
			continue
		}
		subtotals++
		if _, ok := r.Cells["no"]; !ok {
			t.Fatalf("subtotal row %q lost its No. cell: %v", r.Label, r.Cells)
		}
	}
	if subtotals != 3 {
		t.Fatalf("subtotal rows = %d, want 3", subtotals)
	}
}
