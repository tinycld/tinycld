package reports

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// stallHaving holds for every visible stall with a sale (each sale's amount
// is above 0), so the detail statement builds its shown-level CTEs, as a
// balance report's does, and no group is hidden by it.
const stallHaving = `"having":{"op":"gt","args":[{"op":"sum","args":[{"path":[{"field":"amount"}]}]},{"value":0}]}`

// salesGrouped is a report on zoo.sales with the detail columns name and
// amount (and more), sorted by name, under groups.
func salesGrouped(groups, more string) string {
	return `{"version":4,"source":"zoo.sales","columns":[
		{"id":"stall","path":[{"link":"stall"}]},
		{"id":"state","path":[{"field":"state"}]},
		{"id":"name","path":[{"field":"name"}]},
		{"id":"amount","path":[{"field":"amount"}],"aggregate":"sum"}` + more + `],
		"groups":[` + groups + `],"sort":[{"col":"name"}]}`
}

// byStall groups the sales by the link stall, under the having.
var byStall = salesGrouped(`{"id":"g","col":"stall",`+stallHaving+`}`, "")

// openedIn finds the detail statement of a run (the one that reads the open
// leaves from gord) among the statements seen, failing when none is seen.
func openedIn(t *testing.T, seen []readStatement) readStatement {
	t.Helper()
	for _, st := range seen {
		if strings.Contains(st.sql, "gord") && strings.Contains(st.sql, "zoo_sales") {
			return st
		}
	}
	t.Fatalf("no statement of the run reads the open leaves from gord (%d reads)", len(seen))
	return readStatement{}
}

// The detail statement of a run with one opened stall reads the sales of
// that stall through the stall index, with the stall's key bound: it never
// reads every sale to cut them to the open group at the end.
func TestAnOpenedGroupIsReadByItsLinkIndex(t *testing.T) {
	z := newZoo(t)
	withStalls(t, z, true)
	const opened = "stall0000000042"
	want := planSearch{"zoo_sales", "idx_zoo_sales_stall", "stall=?"}
	for _, stats := range []string{"no statistics", "after ANALYZE"} {
		if stats == "after ANALYZE" {
			analyze(t, z)
		}
		t.Run(stats, func(t *testing.T) {
			seen := observeReads(t)
			res := z.run(t, "ann", byStall, Options{Expand: [][]any{{opened}}, Collapse: [][]any{}}, nil)
			observeRead = nil
			if got := len(detailCells(res, "name")); got != 50 {
				t.Fatalf("the opened stall shows %d sales, want 50", got)
			}
			detail := openedIn(t, *seen)
			steps := stmtPlan(t, z, detail)
			for _, step := range steps {
				if strings.HasPrefix(step, "SCAN zoo_sales") {
					t.Errorf("the detail statement scans the sales (%s):\n%s\n%s", step, strings.Join(steps, "\n"), detail.sql)
				}
			}
			if !want.in(steps) {
				t.Errorf("the detail statement has no step SEARCH zoo_sales USING … idx_zoo_sales_stall (stall=?):\n%s\n%s", strings.Join(steps, "\n"), detail.sql)
			}
			if strings.Contains(detail.sql, opened) {
				t.Errorf("the detail statement holds the opened stall's key %s as SQL text:\n%s", opened, detail.sql)
			}
		})
	}
}

// leafRows lists the group rows of res as "<key>=<amount>" and its detail
// rows as "<key of their group>/<name>", in order. A key part that is a
// stall's id reads as the stall's name, and a blank part as "-".
func leafRows(t *testing.T, z *zoo, res *Result) (groups, details []string) {
	t.Helper()
	return pageRows(t, z, res, map[string][]any{})
}

// pageRows is leafRows of one page of a paged run, whose detail rows can be
// under a group row of an earlier page: byID holds the group rows of the
// pages read so far.
func pageRows(t *testing.T, z *zoo, res *Result, byID map[string][]any) (groups, details []string) {
	t.Helper()
	names := map[any]string{}
	for _, r := range z.recs {
		if n := r.Collection().Name; n == "zoo_stalls" || n == "zoo_booths" {
			names[r.Id] = r.GetString("name")
		}
	}
	keyOf := func(key []any) string {
		parts := make([]string, len(key))
		for i, v := range key {
			switch {
			case v == nil:
				parts[i] = "-"
			case names[v] != "":
				parts[i] = names[v]
			default:
				parts[i] = fmt.Sprint(v)
			}
		}
		return strings.Join(parts, ",")
	}
	for _, r := range res.Rows {
		switch r.Kind {
		case KindGroup:
			byID[r.ID] = r.GroupKey
			groups = append(groups, fmt.Sprintf("%s=%v", keyOf(r.GroupKey), r.Cells["amount"]))
		case KindDetail:
			key, ok := byID[r.Section]
			if !ok {
				t.Fatalf("detail row %s is under no group row of the result (section %q)", r.ID, r.Section)
			}
			details = append(details, fmt.Sprintf("%s/%v", keyOf(key), r.Cells["name"]))
		}
	}
	return groups, details
}

// The cut of the detail read to the open leaves keeps every result: each
// case gives the detail rows and the group totals today's engine gives.
func TestAnOpenedGroupKeepsItsRows(t *testing.T) {
	z := newZoo(t)
	withStalls(t, z, false)
	food, gifts := []any{z.recs["food"].Id}, []any{z.recs["gifts"].Id}
	// The blank group holds S4 (no stall) and S5 (its stall "Staff" is
	// hidden from ann); S3's stall "Blank" is a group of its own.
	blank := []any{nil}
	stallTotals := []string{"Blank=4", "Food=1", "Gifts=2", "-=24"}
	opened := func(keys ...[]any) Options { return Options{Expand: keys, Collapse: [][]any{}} }
	// runningSum runs inside its group; percentOfTotal is a window over
	// every detail row of the shown groups, so it reads rows outside the
	// open leaves.
	windows := `,{"id":"running","formula":{"op":"runningSum","args":[{"col":"amount"}]}},
		{"id":"ofTotal","formula":{"op":"percentOfTotal","args":[{"col":"amount"}],"scale":1}}`
	byState := salesGrouped(`{"id":"g","col":"state",`+stallHaving+`}`, "")
	byStallThenState := salesGrouped(`{"id":"g","col":"stall",`+stallHaving+`},{"id":"h","col":"state"}`, "")

	cases := []struct {
		name    string
		raw     string
		opts    Options
		groups  []string
		details []string
	}{
		{"one stall opened", byStall, opened(food), stallTotals, []string{"Food/S1"}},
		{"the blank group opened", byStall, opened(blank), stallTotals, []string{"-/S4", "-/S5"}},
		{"a stall and the blank group opened", byStall, opened(food, blank), stallTotals, []string{"Food/S1", "-/S4", "-/S5"}},
		{"every group opened", byStall, Options{ExpandAll: true}, stallTotals,
			[]string{"Blank/S3", "Food/S1", "Gifts/S2", "-/S4", "-/S5"}},
		{"two group levels, one leaf opened", byStallThenState, opened(food, []any{food[0], "open"}),
			[]string{"Blank=4", "Food=1", "Food,open=1", "Gifts=2", "-=24"}, []string{"Food,open/S1"}},
		{"a group key that is no link", byState, opened([]any{"open"}),
			[]string{"open=9", "paid=18", "-=4"}, []string{"open/S1", "open/S4"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			groups, details := leafRows(t, z, z.run(t, "ann", c.raw, c.opts, nil))
			if !reflect.DeepEqual(details, c.details) {
				t.Errorf("detail rows %q\nwant %q", details, c.details)
			}
			if !reflect.DeepEqual(groups, c.groups) {
				t.Errorf("group rows %q\nwant %q", groups, c.groups)
			}
		})
	}

	t.Run("a paged run", func(t *testing.T) {
		pages := groupedPages(t, z, "ann", byStall, Options{Expand: [][]any{food, gifts}, Collapse: [][]any{}, Limit: 1})
		byID := map[string][]any{}
		var all [][]string
		for _, p := range pages {
			groups, details := pageRows(t, z, p, byID)
			all = append(all, append(groups, details...))
		}
		// A page of one row: each group row and each detail row has its own,
		// and a detail page reads its group row from an earlier page.
		want := [][]string{{"Blank=4"}, {"Food=1"}, {"Food/S1"}, {"Gifts=2"}, {"Gifts/S2"}, {"-=24"}}
		if !reflect.DeepEqual(all, want) {
			t.Fatalf("pages %q\nwant %q", all, want)
		}
	})

	t.Run("a window value across groups", func(t *testing.T) {
		res := z.run(t, "ann", salesGrouped(`{"id":"g","col":"stall",`+stallHaving+`}`, windows), opened(gifts), nil)
		groups, details := leafRows(t, z, res)
		if want := []string{"Gifts/S2"}; !reflect.DeepEqual(details, want) {
			t.Fatalf("detail rows %q\nwant %q", details, want)
		}
		if !reflect.DeepEqual(groups, stallTotals) {
			t.Fatalf("group rows %q\nwant %q", groups, stallTotals)
		}
		if got, want := detailCells(res, "running"), []any{int64(2)}; !reflect.DeepEqual(got, want) {
			t.Fatalf("running %#v, want %#v", got, want)
		}
		// S2's 2 of the 31 of every shown sale (S1 to S5), not of Gifts' 2.
		if got, want := detailCells(res, "ofTotal"), []any{6.5}; !reflect.DeepEqual(got, want) {
			t.Fatalf("percent of total %#v, want %#v", got, want)
		}
	})

	// Each run below has its own zoo, made after the cases above: a zoo's
	// cleanup clears the source registry the cases above read.
	t.Run("two link levels", func(t *testing.T) {
		z := newZoo(t)
		withStalls(t, z, false)
		withBooths(t, z)
		food := z.recs["food"].Id
		east := z.recs["east"].Id
		booth := `,{"id":"booth","path":[{"link":"booth"}]}`
		// Food's 97 is above 10, but its leaf East holds only S1's 1: the
		// first level's having reads the rows of Food outside the open leaf.
		over10 := `"having":{"op":"gt","args":[{"op":"sum","args":[{"path":[{"field":"amount"}]}]},{"value":10}]}`
		withHaving := salesGrouped(`{"id":"g","col":"stall",`+over10+`},{"id":"h","col":"booth"}`, booth)
		noHaving := salesGrouped(`{"id":"g","col":"stall"},{"id":"h","col":"booth"}`, booth)
		cases := []struct {
			name    string
			raw     string
			opts    Options
			groups  []string
			details []string
		}{
			{"a first-level having", withHaving, opened([]any{food}, []any{food, east}),
				[]string{"Food=97", "Food,East=1", "Food,West=32", "Food,-=64", "-=24"},
				[]string{"Food,East/S1"}},
			{"no having, a link cut at each level", noHaving,
				opened([]any{food}, []any{food, east}, []any{food, nil}, []any{nil}, []any{nil, nil}),
				[]string{"Blank=4", "Food=97", "Food,East=1", "Food,West=32", "Food,-=64", "Gifts=2", "-=24", "-,-=24"},
				[]string{"Food,East/S1", "Food,-/S7", "-,-/S4", "-,-/S5"}},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				groups, details := leafRows(t, z, z.run(t, "ann", c.raw, c.opts, nil))
				if !reflect.DeepEqual(details, c.details) {
					t.Errorf("detail rows %q\nwant %q", details, c.details)
				}
				if !reflect.DeepEqual(groups, c.groups) {
					t.Errorf("group rows %q\nwant %q", groups, c.groups)
				}
			})
		}
	})

	// A held group that is not on the page: page 1 ends inside Mo (e0, f5);
	// Mo then weighs most, so page 2 reads on from the cursor with Mo held
	// but not on it; f5 is then deleted. The pages after give the rest of the
	// report once each, and never e0 again (review A, BLOCK 1).
	t.Run("a held group that is not on the page", func(t *testing.T) {
		z := newZoo(t)
		z.feeding(t, "m2", z.recs["mo"], 40, "2026-02-20", 5)
		z.feeding(t, "e0", z.recs["mo"], 3, "2026-02-21", 1)
		for i, w := range []int{400, 500, 600, 700} {
			key := fmt.Sprintf("x%d", i)
			a := z.rec(t, key, "zoo_animals", map[string]any{"name": key, "species": "cat", "site": z.id("north"), "owner": z.id("bob"), "weight": w})
			z.rec(t, "kx"+key, "zoo_keepers", map[string]any{"user": z.id("bob"), "animal": a.Id})
			z.feeding(t, "fx"+key, a, 10, "2026-01-09", 1)
		}
		notes := func(res *Result) []any {
			out := []any{}
			for _, r := range res.Rows {
				if r.Kind == KindDetail {
					out = append(out, r.Cells["note"])
				}
			}
			return out
		}
		first := z.run(t, "bob", byAnimalWeight, Options{Limit: 3}, nil)
		if got, want := notes(first), []any{"e0", "f5"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("page 1 %q, want %q", got, want)
		}
		mo := z.recs["mo"]
		mo.Set("weight", 999)
		if err := z.app.Save(mo); err != nil {
			t.Fatal(err)
		}
		second := z.run(t, "bob", byAnimalWeight, Options{Limit: 3, Cursor: first.Page.Next}, nil)
		if err := z.app.Delete(z.recs["f5"]); err != nil {
			t.Fatal(err)
		}
		all := [][]any{notes(first), notes(second)}
		for _, p := range groupedPages(t, z, "bob", byAnimalWeight, Options{Limit: 3, Cursor: second.Page.Next}) {
			all = append(all, notes(p))
		}
		want := [][]any{{"e0", "f5"}, {"f3", "f4"}, {"fxx0"}, {"fxx1"}, {"fxx2"}, {"fxx3"}, {"m2"}, {}}
		if !reflect.DeepEqual(all, want) {
			t.Fatalf("detail notes of each page %q\nwant %q", all, want)
		}
	})
}

// withBooths gives the sales of withStalls a second one-hop link, booth, with
// its own index: S1 is at the booth East, and two more sales at Food, S6 at
// West and S7 at no booth, make Food 97. No other sale has a booth.
func withBooths(t *testing.T, z *zoo) {
	t.Helper()
	booths := core.NewBaseCollection("zoo_booths")
	booths.ListRule = rule(`@request.auth.id != ""`)
	booths.ViewRule = booths.ListRule
	booths.Fields.Add(&core.TextField{Name: "name"})
	z.save(t, booths)
	sales, err := z.app.FindCollectionByNameOrId("zoo_sales")
	if err != nil {
		t.Fatal(err)
	}
	sales.Fields.Add(&core.RelationField{Name: "booth", CollectionId: booths.Id, MaxSelect: 1})
	sales.AddIndex("idx_zoo_sales_booth", false, "booth", "")
	z.save(t, sales)

	east := z.rec(t, "east", "zoo_booths", map[string]any{"name": "East"})
	west := z.rec(t, "west", "zoo_booths", map[string]any{"name": "West"})
	s1, err := z.app.FindRecordById("zoo_sales", z.recs["S1"].Id)
	if err != nil {
		t.Fatal(err)
	}
	s1.Set("booth", east.Id)
	if err := z.app.Save(s1); err != nil {
		t.Fatal(err)
	}
	food := z.recs["food"].Id
	z.rec(t, "S6", "zoo_sales", map[string]any{"name": "S6", "stall": food, "booth": west.Id, "state": "paid", "amount": 32})
	z.rec(t, "S7", "zoo_sales", map[string]any{"name": "S7", "stall": food, "state": "open", "amount": 64})

	RegisterSource(Source{
		Key: "zoo.booths", Collection: "zoo_booths", Label: "Booths",
		Fields: []Field{{Key: "name", Label: "Name", Type: Text, Aggregates: MinMax}},
	})
	src := *lookupRegistered("zoo.sales")
	src.Links = append(slices.Clone(src.Links), Link{Key: "booth", Label: "Booth", Target: "zoo.booths", Cardinality: ToOne, Field: "booth"})
	RegisterSource(src)
}
