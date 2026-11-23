package reports

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// salesByStallPct groups the sales by the to-one link stall (so the leaf cut
// reads the stored column stall), with the amount and its percent of the
// total. PCT is the percent column's detail member.
const salesByStallPct = `{"version":4,"source":"zoo.sales","columns":[
	{"id":"stall","path":[{"link":"stall"}]},
	{"id":"name","path":[{"field":"name"}]},
	{"id":"amount","path":[{"field":"amount"}],"aggregate":"sum"},
	{"id":"pct","formula":{"op":"percentOfTotal","args":[{"col":"amount"}],"scale":1}PCT}],
	"groups":[{"id":"g","col":"stall"}],"sort":[{"col":"name"}]}`

func salesPct(member string) string { return strings.Replace(salesByStallPct, "PCT", member, 1) }

// leafCutTerm is the start of the leaf cut of the first group level
// (leafCut): the base row's stored stall column in the open leaves' keys.
const leafCutTerm = "IN (SELECT g0 FROM gord"

// ann's sales by stall (withStalls): Blank 4 (S3), Food 1 (S1), Gifts 2 (S2)
// and the blank group 24 (S4 with no stall, S5 at the hidden stall Staff), of
// 31 in all. Each group's percent of the total, at one place.
var stallPercents = []string{"Blank=12.9", "Food=3.2", "Gifts=6.5", "-=77.4"}

// groupPercents lists res's group rows as "<key>=<pct>", the key as leafRows
// writes it.
func groupPercents(t *testing.T, z *zoo, res *Result) []string {
	t.Helper()
	names := map[any]string{}
	for _, r := range z.recs {
		if r.Collection().Name == "zoo_stalls" {
			names[r.Id] = r.GetString("name")
		}
	}
	var out []string
	for _, r := range res.Rows {
		if r.Kind != KindGroup {
			continue
		}
		key := "-"
		if r.GroupKey[0] != nil {
			key = names[r.GroupKey[0]]
		}
		out = append(out, fmt.Sprintf("%s=%v", key, r.Cells["pct"]))
	}
	return out
}

// A report whose only window value is a percent blank on detail rows cuts
// the opened group's read to its leaf: the detail statement reads the sales
// of the open stall through gord's keys, and computes no window value, as
// the percent is never shown on a detail row.
func TestABlankPercentColumnKeepsTheLeafCut(t *testing.T) {
	z := newZoo(t)
	withStalls(t, z, false)
	spec := salesPct(blankOnDetail)
	seen := observeReads(t)
	res := z.run(t, "ann", spec, Options{Expand: [][]any{{z.id("food")}}, Collapse: [][]any{}}, nil)
	observeRead = nil
	if got, want := detailCells(res, "name"), []any{"S1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("the opened stall shows %v, want %v", got, want)
	}
	detail := openedIn(t, *seen)
	if !strings.Contains(detail.sql, leafCutTerm) {
		t.Errorf("the detail statement has no leaf cut (%s):\n%s", leafCutTerm, detail.sql)
	}
	if strings.Contains(detail.sql, "OVER (") {
		t.Errorf("the detail statement computes a window value (OVER ():\n%s", detail.sql)
	}
}

// withoutDetailPct is rows with no pct cell on its detail rows.
func withoutDetailPct(rows []Row) []Row {
	out := make([]Row, len(rows))
	for i, r := range rows {
		if r.Kind == KindDetail {
			r.Cells = maps.Clone(r.Cells)
			delete(r.Cells, "pct")
		}
		out[i] = r
	}
	return out
}

// The blank percent changes no result: opened, closed, every group opened,
// and in pages, its rows, cells and columns are those of the same report with
// the percent shown, less the percent's detail cells, and the group rows'
// percents are each stall's share of ann's 31.
func TestABlankPercentColumnLeavesEveryResult(t *testing.T) {
	z := newZoo(t)
	withStalls(t, z, false)
	blank, shown := salesPct(blankOnDetail), salesPct("")
	food, gifts := []any{z.id("food")}, []any{z.id("gifts")}
	for _, c := range []struct {
		name    string
		opts    Options
		details []any
	}{
		{"one stall opened", Options{Expand: [][]any{food}, Collapse: [][]any{}}, []any{"S1"}},
		{"closed", Options{}, []any{}},
		{"every stall opened", Options{ExpandAll: true}, []any{"S3", "S1", "S2", "S4", "S5"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := z.run(t, "ann", blank, c.opts, nil)
			want := z.run(t, "ann", shown, c.opts, nil)
			if names := detailCells(got, "name"); len(names)+len(c.details) > 0 && !reflect.DeepEqual(names, c.details) {
				t.Fatalf("detail rows %v, want %v", names, c.details)
			}
			for _, r := range got.Rows {
				if _, has := r.Cells["pct"]; has && r.Kind == KindDetail {
					t.Fatalf("detail row %s shows the blank percent: %v", r.ID, r.Cells)
				}
			}
			if !reflect.DeepEqual(got.Rows, withoutDetailPct(want.Rows)) {
				t.Fatalf("rows:\n got  %+v\n want %+v", got.Rows, withoutDetailPct(want.Rows))
			}
			if !reflect.DeepEqual(got.Columns, want.Columns) {
				t.Fatalf("columns:\n got  %+v\n want %+v", got.Columns, want.Columns)
			}
			if !reflect.DeepEqual(got.Notices, want.Notices) {
				t.Fatalf("notices %+v, want %+v", got.Notices, want.Notices)
			}
			if g := groupPercents(t, z, got); !reflect.DeepEqual(g, stallPercents) {
				t.Fatalf("group percents %q, want %q", g, stallPercents)
			}
			if total := got.Rows[len(got.Rows)-1]; total.Kind != KindTotal || total.Cells["amount"] != int64(31) {
				t.Fatalf("total row %+v", total)
			}
		})
	}
	t.Run("the percent column", func(t *testing.T) {
		res := z.run(t, "ann", blank, Options{Expand: [][]any{food}, Collapse: [][]any{}}, nil)
		col := res.Columns[3]
		if col.ID != "pct" || col.Type != Percent || col.Sortable || col.Currency != "" || col.CurrencyCell != "" || len(col.Options) != 0 {
			t.Fatalf("pct column = %+v, want a percent, not sortable, no currency, no options", col)
		}
		if name := res.Columns[1]; name.ID != "name" || !name.Sortable {
			t.Fatalf("name column = %+v, want sortable", name)
		}
	})
	t.Run("in pages", func(t *testing.T) {
		opts := Options{Expand: [][]any{food, gifts}, Collapse: [][]any{}, Limit: 1}
		got := groupedPages(t, z, "ann", blank, opts)
		want := groupedPages(t, z, "ann", shown, opts)
		if len(got) != len(want) {
			t.Fatalf("%d pages, want %d", len(got), len(want))
		}
		if g, w := mergedRows(t, got), withoutDetailPct(mergedRows(t, want)); !reflect.DeepEqual(g, w) {
			t.Fatalf("pages:\n got  %+v\n want %+v", g, w)
		}
		if names := detailCells(&Result{Rows: mergedRows(t, got)}, "name"); !reflect.DeepEqual(names, []any{"S1", "S2"}) {
			t.Fatalf("detail rows of the pages %v, want S1, S2", names)
		}
	})
}

// withoutDetailCells is rows with no cell of the columns ids (their value and
// their "<id>@…" cells) on its detail rows.
func withoutDetailCells(rows []Row, ids ...string) []Row {
	out := make([]Row, len(rows))
	for i, r := range rows {
		if r.Kind == KindDetail {
			r.Cells = maps.Clone(r.Cells)
			for k := range r.Cells {
				for _, id := range ids {
					if k == id || strings.HasPrefix(k, id+"@") {
						delete(r.Cells, k)
					}
				}
			}
		}
		out[i] = r
	}
	return out
}

// A column blank on detail rows keeps the column's metadata of the same
// report with the column shown: a money formula keeps its currency, a formula
// over a select field keeps its options, and every other cell stays. The one
// difference is GroupOnly: the blank column is group-only exactly when its
// type is not a figure (Number, Money or Percent), so the select formula gets
// GroupOnly and the money formula does not; the shown run has it on no column.
func TestABlankColumnKeepsItsCurrencyAndOptions(t *testing.T) {
	z := newZoo(t)
	withStalls(t, z, false)
	for _, c := range []struct {
		name, user, spec, col string
		has                   func(ResultColumn) bool
		groupOnly             bool
	}{
		{"a money formula", "ann", `{"version":4,"source":"zoo.feedings","columns":[
			{"id":"animal","path":[{"link":"animal"}]},
			{"id":"note","path":[{"field":"note"}]},
			{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
			{"id":"twice","formula":{"op":"mul","args":[{"col":"cost"},{"value":2}]}BLANK}],
			"groups":[{"id":"g","col":"animal"}],"sort":[{"col":"note"}]}`, "twice",
			func(col ResultColumn) bool {
				return col.Type == Money && (col.Currency != "" || col.CurrencyCell != "")
			}, false},
		{"a formula with options", "ann", `{"version":4,"source":"zoo.sales","columns":[
			{"id":"stall","path":[{"link":"stall"}]},
			{"id":"name","path":[{"field":"name"}]},
			{"id":"amount","path":[{"field":"amount"}],"aggregate":"sum"},
			{"id":"st","formula":{"path":[{"field":"state"}]}BLANK}],
			"groups":[{"id":"g","col":"stall"}],"sort":[{"col":"name"}]}`, "st",
			func(col ResultColumn) bool { return len(col.Options) > 0 }, true},
	} {
		blank := strings.Replace(c.spec, "BLANK", blankOnDetail, 1)
		shown := strings.Replace(c.spec, "BLANK", "", 1)
		for _, o := range []struct {
			name string
			opts Options
		}{{"closed", Options{}}, {"every group opened", Options{ExpandAll: true}}} {
			t.Run(c.name+", "+o.name, func(t *testing.T) {
				got := z.run(t, c.user, blank, o.opts, nil)
				want := z.run(t, c.user, shown, o.opts, nil)
				i := slices.IndexFunc(want.Columns, func(col ResultColumn) bool { return col.ID == c.col })
				if i < 0 || !c.has(want.Columns[i]) {
					t.Fatalf("the shown spec's %s column is not the case under test: %+v", c.col, want.Columns)
				}
				for _, col := range want.Columns {
					if col.GroupOnly {
						t.Fatalf("the shown run's %s column is group-only", col.ID)
					}
				}
				if g := slices.IndexFunc(got.Columns, func(col ResultColumn) bool { return col.ID == c.col }); g < 0 || got.Columns[g].GroupOnly != c.groupOnly {
					t.Fatalf("the blank %s column: GroupOnly is not %v: %+v", c.col, c.groupOnly, got.Columns)
				}
				wantCols := slices.Clone(want.Columns)
				wantCols[i].GroupOnly = c.groupOnly
				if !reflect.DeepEqual(got.Columns, wantCols) {
					t.Fatalf("columns:\n got  %+v\n want %+v", got.Columns, wantCols)
				}
				if w := withoutDetailCells(want.Rows, c.col); !reflect.DeepEqual(got.Rows, w) {
					t.Fatalf("rows:\n got  %+v\n want %+v", got.Rows, w)
				}
				if !reflect.DeepEqual(got.Notices, want.Notices) {
					t.Fatalf("notices %+v, want %+v", got.Notices, want.Notices)
				}
			})
		}
	}
}

// rowFees is bob's sum of the fees (USD and GBP) of every animal but the
// lions, read on the report's row (the where reads its id), so it has a value
// on detail rows only, and that value mixes currencies.
const rowFees = `{"op":"lookup","source":"zoo.animals","pick":"sum","value":{"path":[{"field":"fee"}]},
	"where":{"op":"and","args":[{"op":"ne","args":[{"path":[{"field":"species"}]},{"value":"lion"}]},
		{"op":"ne","args":[{"path":[{"field":"id"}]},{"outer":[{"field":"id"}]}]}]}}`

// rowMixedWhere is a count of the animals whose where reads bob's mixed sum
// of fees and the report's row: its where is unknown on detail rows only.
const rowMixedWhere = `{"op":"lookup","source":"zoo.animals","pick":"count","value":{"path":[{"field":"id"}]},
	"where":{"op":"and","args":[{"op":"gt","args":[` + mixedFees + `,{"value":0}]},
		{"op":"ne","args":[{"path":[{"link":"site"}]},{"outer":[{"field":"id"}]}]}]}}`

// A column blank on detail rows gives the notices of the same report with the
// column shown: a detail value that mixes currencies still names the column,
// and a lookup's where that is unknown on detail rows still names the where,
// closed, with one group opened and with every group opened.
func TestABlankColumnKeepsTheNoticesOfItsDetailRows(t *testing.T) {
	z := newZoo(t)
	for _, c := range []struct {
		name, formula string
		named         func(Notice) bool
	}{
		{"a mixed sum lookup", rowFees, func(n Notice) bool { return slices.Contains(n.Columns, "x") }},
		{"a lookup whose where reads a mixed sum", rowMixedWhere, func(n Notice) bool {
			return slices.Contains(n.Conditions, "columns.2.formula.where")
		}},
	} {
		spec := func(member string) string {
			return `{"version":4,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]},
				{"id":"region","path":[{"field":"region"}]},
				{"id":"x","formula":` + c.formula + member + `}],
				"groups":[{"id":"r","col":"region"}],"sort":[{"col":"name"}]}`
		}
		opened := z.run(t, "bob", spec(""), Options{ExpandAll: true}, nil)
		if !slices.ContainsFunc(opened.Notices, func(n Notice) bool { return n.Code == "mixedCurrency" && c.named(n) }) {
			t.Fatalf("%s: the shown spec opened names nothing, so the case tests nothing: %+v", c.name, opened.Notices)
		}
		for _, o := range []struct {
			name string
			opts Options
		}{
			{"closed", Options{}},
			{"one group opened", Options{Expand: [][]any{{"north"}}, Collapse: [][]any{}}},
			{"every group opened", Options{ExpandAll: true}},
		} {
			t.Run(c.name+", "+o.name, func(t *testing.T) {
				got := z.run(t, "bob", spec(blankOnDetail), o.opts, nil)
				want := z.run(t, "bob", spec(""), o.opts, nil)
				if !reflect.DeepEqual(got.Notices, want.Notices) {
					t.Fatalf("notices:\n got  %+v\n want %+v", got.Notices, want.Notices)
				}
			})
		}
	}
}

// A percent shown on detail rows reads rows outside the open leaves: its
// detail cell is the sale's share of every shown sale, and the detail
// statement keeps every row (no leaf cut).
func TestAShownPercentColumnStillTurnsTheCutOff(t *testing.T) {
	z := newZoo(t)
	withStalls(t, z, false)
	seen := observeReads(t)
	res := z.run(t, "ann", salesPct(""), Options{Expand: [][]any{{z.id("gifts")}}, Collapse: [][]any{}}, nil)
	observeRead = nil
	// S2's 2 of the 31 of every shown sale.
	if got, want := detailCells(res, "pct"), []any{6.5}; !reflect.DeepEqual(got, want) {
		t.Fatalf("percent of total %#v, want %#v", got, want)
	}
	detail := openedIn(t, *seen)
	if strings.Contains(detail.sql, leafCutTerm) {
		t.Errorf("the detail statement cuts w to the open leaf, which the shown percent reads past:\n%s", detail.sql)
	}
}
