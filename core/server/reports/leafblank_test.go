package reports

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// The zoo's huts and chores: a hut is read by its warden only, a chore by
// any signed-in user, so a reader reads chores whose hut they cannot read.
//
//	ann is the warden of ha and hb, bob of hc
//	chores: c1-c3 in ha, c4-c6 in hb, c7-c9 in hc, c10 in no hut
//
// ann reads c7-c9 but not their hut; bob reads c1-c6 but not theirs.
func addZooChores(t *testing.T, z *zoo) {
	t.Helper()
	users, err := z.app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	huts := core.NewBaseCollection("zoo_huts")
	huts.ListRule = rule(`warden = @request.auth.id`)
	huts.ViewRule = huts.ListRule
	huts.Fields.Add(
		&core.TextField{Name: "name"},
		&core.RelationField{Name: "warden", CollectionId: users.Id, MaxSelect: 1},
	)
	z.save(t, huts)
	chores := core.NewBaseCollection("zoo_chores")
	chores.ListRule = rule(`@request.auth.id != ""`)
	chores.ViewRule = chores.ListRule
	chores.Fields.Add(
		&core.TextField{Name: "name"},
		&core.NumberField{Name: "hours"},
		&core.RelationField{Name: "hut", CollectionId: huts.Id, MaxSelect: 1},
	)
	z.save(t, chores)

	for key, warden := range map[string]string{"ha": "ann", "hb": "ann", "hc": "bob"} {
		z.rec(t, key, "zoo_huts", map[string]any{"name": key, "warden": z.id(warden)})
	}
	for i, hut := range []string{"ha", "ha", "ha", "hb", "hb", "hb", "hc", "hc", "hc", ""} {
		data := map[string]any{"name": choreName(i), "hours": i + 1}
		if hut != "" {
			data["hut"] = z.id(hut)
		}
		z.rec(t, choreName(i), "zoo_chores", data)
	}

	RegisterSource(Source{
		Key: "zoo.huts", Collection: "zoo_huts", Label: "Huts",
		Fields: []Field{{Key: "name", Label: "Name", Type: Text, Aggregates: MinMax}},
	})
	RegisterSource(Source{
		Key: "zoo.chores", Collection: "zoo_chores", Label: "Chores",
		Fields: []Field{
			{Key: "name", Label: "Name", Type: Text, Aggregates: MinMax},
			{Key: "hours", Label: "Hours", Type: Number, Aggregates: SumAll},
		},
		Links: []Link{{Key: "hut", Label: "Hut", Target: "zoo.huts", Cardinality: ToOne, Field: "hut"}},
	})
	if err := CheckSources(z.app); err != nil {
		t.Fatalf("CheckSources: %v", err)
	}
}

// choreName is the key and name of the chore at i: c1 to c10, sorted by
// their place (c01 sorts before c10).
func choreName(i int) string {
	return "c" + string(rune('0'+(i+1)/10)) + string(rune('0'+(i+1)%10))
}

// choresByHut groups the chores by their hut, each group open, with a running
// value, so a page's detail statement is computed over the groups it reaches.
const choresByHut = `{"version":4,"source":"zoo.chores","columns":[
	{"id":"hut","path":[{"link":"hut"}]},
	{"id":"name","path":[{"field":"name"}]},
	{"id":"hours","path":[{"field":"hours"}],"aggregate":"sum"},
	{"id":"running","formula":{"op":"runningSum","args":[{"col":"hours"}]}}],
	"groups":[{"id":"g","col":"hut","open":true,"subtotals":true}],
	"sort":[{"col":"name"}]}`

// A chore whose hut the reader cannot read reads a blank hut, so on a paged
// run it falls under the blank group with the chore of no hut, on every page,
// whether the page ends inside a group or not: the join to the open groups
// matches a blank key with IS. The pages are the same with the rows of every
// open group (everyLeafRows) and with the planner's join (plannedLeafJoin).
func TestARowWhoseLinkTheReaderCannotReadFallsUnderTheBlankGroupOnACutPage(t *testing.T) {
	z := newZoo(t)
	addZooChores(t, z)
	mux := z.mux(t)
	blank := "g:" + keyString([]any{nil})
	readers := map[string][]string{
		"ann": {"c07", "c08", "c09", "c10"},
		"bob": {"c01", "c02", "c03", "c04", "c05", "c06", "c10"},
	}
	// The case is reached: each reader lists every chore but not every hut.
	allChores := z.ids("c01", "c02", "c03", "c04", "c05", "c06", "c07", "c08", "c09", "c10")
	for user, huts := range map[string][]string{"ann": z.ids("ha", "hb"), "bob": z.ids("hc")} {
		if got := z.listIDs(t, mux, "zoo_huts", user, ""); !reflect.DeepEqual(got, huts) {
			t.Fatalf("%s lists huts %v, want %v", user, got, huts)
		}
		if got := z.listIDs(t, mux, "zoo_chores", user, ""); !reflect.DeepEqual(got, allChores) {
			t.Fatalf("%s lists chores %v, want %v", user, got, allChores)
		}
	}
	seen := observeReads(t)
	for user, want := range readers {
		whole := z.run(t, user, choresByHut, Options{ExpandAll: true, Limit: 1000}, nil)
		midGroup := false
		for _, limit := range []int{1, 2, 3, 5} {
			opts := Options{ExpandAll: true, Limit: limit}
			pages := groupedPages(t, z, user, choresByHut, opts)
			if len(pages) < 2 {
				t.Fatalf("%s, pages of %d: one page", user, limit)
			}
			midGroup = midGroup || opensMidGroup(pages)
			var under []string
			for i, p := range pages {
				for _, r := range p.Rows {
					if r.Kind != KindDetail {
						continue
					}
					if r.Section == blank {
						under = append(under, r.Cells["name"].(string))
						continue
					}
					if hut := r.Cells["hut"]; hut == nil {
						t.Fatalf("%s, pages of %d, page %d: %s has a blank hut but is under %q", user, limit, i+1, r.Cells["name"], r.Section)
					}
				}
			}
			if !reflect.DeepEqual(under, want) {
				t.Fatalf("%s, pages of %d: the blank group holds %v, want %v", user, limit, under, want)
			}
			if got := mergedRows(t, pages); !reflect.DeepEqual(got, whole.Rows) {
				a, _ := json.Marshal(got)
				b, _ := json.Marshal(whole.Rows)
				t.Fatalf("%s, pages of %d:\n%s\nwhole:\n%s", user, limit, a, b)
			}
			requireLeafPagesSame(t, z, user, choresByHut, opts, nil)
			for range 200 {
				requireSameAsLegacy(t, z, user, choresByHut, opts, nil, &legacySQL.plannedLeafJoin)
				res := z.run(t, user, choresByHut, opts, nil)
				if res.Page == nil || res.Page.Next == "" {
					break
				}
				opts.Cursor = res.Page.Next
			}
		}
		if !midGroup {
			t.Fatalf("%s: no page starts inside a group", user)
		}
	}
	cut := 0
	for _, st := range *seen {
		if strings.Contains(st.sql, "\nwl AS MATERIALIZED (") {
			cut++
		}
	}
	if cut == 0 {
		t.Fatal("no statement was computed over the groups a page can reach: the test cut no page")
	}
}
