package reports

import (
	"encoding/base64"
	"encoding/json"
	"slices"
	"testing"
)

// A group cursor holds the order values its page ended at (R1). When the
// cursor's group, or a group around it, sorts somewhere else by the next
// page (another reader renamed or re-weighed it), the next pages still go on
// from the cursor: they skip no row the reader has not seen, repeat no detail
// row the reader has seen, keep document order, and chain page.next to the
// report's end (review A, BLOCK 1).

// decodedCursor reads a group cursor as the engine wrote it.
func decodedCursor(t *testing.T, text string) groupCursor {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(text)
	if err != nil {
		t.Fatal(err)
	}
	var c groupCursor
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

// requireGoesOnAfterAMove fails unless rest, the pages read from first's
// cursor after a group moved, hold every counted row of the current one-page
// run that first did not show, each once and in the current run's order;
// repeat no detail row of first; end in the whole report's Total; and give a
// next cursor on every page but the last.
func requireGoesOnAfterAMove(t *testing.T, first *Result, rest []*Result, whole *Result) {
	t.Helper()
	shown := map[string]bool{}
	for _, r := range first.Rows {
		if !isEnd(r) {
			shown[r.ID] = true
		}
	}
	var got []string
	for i, p := range rest {
		if last := i == len(rest)-1; (p.Page == nil || p.Page.Next == "") != last {
			t.Fatalf("page %d of %d: page %+v", i+2, len(rest)+1, p.Page)
		}
		if lastTotal(t, p).Cells["cost"] != lastTotal(t, whole).Cells["cost"] {
			t.Fatalf("page %d total = %v, want %v", i+2, lastTotal(t, p).Cells["cost"], lastTotal(t, whole).Cells["cost"])
		}
		for _, r := range p.Rows {
			if isEnd(r) {
				continue
			}
			if r.Kind == KindDetail && shown[r.ID] {
				t.Fatalf("page %d repeats detail %s, shown before the cursor", i+2, r.ID)
			}
			if slices.Contains(got, r.ID) {
				t.Fatalf("page %d repeats %s", i+2, r.ID)
			}
			got = append(got, r.ID)
		}
	}
	var order []string
	for _, r := range whole.Rows {
		if !isEnd(r) {
			order = append(order, r.ID)
		}
	}
	for _, id := range order {
		if !shown[id] && !slices.Contains(got, id) {
			t.Fatalf("the pages after the cursor skip %s\ngot   %v\nwhole %v", id, got, order)
		}
	}
	// got is in the current run's order: each id comes later there than the one
	// before it.
	at := -1
	for _, id := range got {
		i := slices.Index(order, id)
		if i < 0 {
			t.Fatalf("%s is not a row of the current run %v", id, order)
		}
		if i <= at {
			t.Fatalf("rows out of document order\ngot   %v\nwhole %v", got, order)
		}
		at = i
	}
}

// byAnimalWeight groups bob's feedings by animal, open, ordered by the
// animal's weight, its details by note.
const byAnimalWeight = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"animal","path":[{"link":"animal"}]},
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"animal","open":true,"subtotals":true}],
	"sort":[{"formula":{"record":"g","path":[{"field":"weight"}]}},{"col":"note"}]}`

// Mode in, depth 1: page 1 ends after Mo's first feeding; Mo then weighs more
// than Zed, so Mo sorts after Zed. The next pages give Zed whole and the rest
// of Mo (m2 and its subtotal), and never f5 again.
func TestACursorInAGroupThatMovedLaterSkipsAndRepeatsNothing(t *testing.T) {
	z := newZoo(t)
	z.feeding(t, "m2", z.recs["mo"], 40, "2026-02-20", 5)
	first := z.run(t, "bob", byAnimalWeight, Options{Limit: 2}, nil)
	requireRows(t, first, "id", "g:"+keyString([]any{z.id("mo"), "USD"}), "d:"+z.id("f5"), `total:["USD"]`)
	if c := decodedCursor(t, first.Page.Next); c.Mode != cursorIn || len(c.Path) != 1 {
		t.Fatalf("cursor %+v, want mode in at depth 1", c)
	}
	mo := z.recs["mo"]
	mo.Set("weight", 999)
	if err := z.app.Save(mo); err != nil {
		t.Fatal(err)
	}
	whole := z.run(t, "bob", byAnimalWeight, Options{Limit: 1000}, nil)
	requireRows(t, whole, "note", nil, "f3", "f4", nil, nil, "f5", "m2", nil, nil)
	rest := groupedPages(t, z, "bob", byAnimalWeight, Options{Limit: 2, Cursor: first.Page.Next})
	requireGoesOnAfterAMove(t, first, rest, whole)
}

// bySiteThenAnimal groups ann's feedings by site, open, then by animal,
// closed: one row per animal. Sites sort by name, animals by their label.
const bySiteThenAnimal = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"site","path":[{"link":"animal"},{"link":"site"}]},
	{"id":"animal","path":[{"link":"animal"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"s","col":"site","open":true,"subtotals":true},{"id":"g","col":"animal","subtotals":true}],
	"sort":[{"formula":{"record":"s","path":[{"field":"name"}]}}]}`

// Mode after, depth 2: page 1 ends after Ari's closed row inside North; North
// is then renamed so it sorts after South. The next pages give South whole
// and the rest of North (Leo and North's subtotal).
func TestACursorAfterAGroupWhoseParentMovedLaterSkipsNothing(t *testing.T) {
	z := newZoo(t)
	ari := z.rec(t, "ari", "zoo_animals", map[string]any{"name": "Ari", "species": "lion", "site": z.id("north"), "owner": z.id("ann"), "weight": 50})
	z.rec(t, "k5", "zoo_keepers", map[string]any{"user": z.id("ann"), "animal": ari.Id})
	z.feeding(t, "f9", ari, 10, "2026-01-09", 1)
	first := z.run(t, "ann", bySiteThenAnimal, Options{Limit: 2}, nil)
	requireRows(t, first, "label", "North", "Ari", "Total")
	if c := decodedCursor(t, first.Page.Next); c.Mode != cursorAfter || len(c.Path) != 2 {
		t.Fatalf("cursor %+v, want mode after at depth 2", c)
	}
	north := z.recs["north"]
	north.Set("name", "Zorth")
	if err := z.app.Save(north); err != nil {
		t.Fatal(err)
	}
	whole := z.run(t, "ann", bySiteThenAnimal, Options{Limit: 1000}, nil)
	requireRows(t, whole, "label", "South", "Zed", "Total South", "Zorth", "Ari", "Leo", "Total Zorth", "Total")
	rest := groupedPages(t, z, "ann", bySiteThenAnimal, Options{Limit: 2, Cursor: first.Page.Next})
	requireGoesOnAfterAMove(t, first, rest, whole)
}
