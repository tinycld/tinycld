package reports

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// pagedBySpeciesHaving is pagedBySpecies with a having on its group, so the
// detail statement reads its rows under the shown groups (ws).
var pagedBySpeciesHaving = strings.Replace(pagedBySpecies, `"opening":{"label":"Start"}}`,
	`"opening":{"label":"Start"},"having":{"op":"gt","args":[{"op":"count","args":[{"path":[{"field":"id"}]}]},{"value":0}]}}`, 1)

// speciesShares is pagedBySpecies with each feeding's share of its species.
var speciesShares = strings.Replace(pagedBySpecies, `"running","formula":{"op":"runningSum","args":[{"col":"cost"}]}}`,
	`"running","formula":{"op":"runningSum","args":[{"col":"cost"}]}},
	{"id":"ofParent","formula":{"op":"percentOfParent","args":[{"col":"cost"}],"scale":1}}`, 1)

// reportShares is speciesShares with each feeding's share of the whole report.
var reportShares = strings.Replace(speciesShares, `"ofParent","formula":{"op":"percentOfParent","args":[{"col":"cost"}],"scale":1}}`,
	`"ofParent","formula":{"op":"percentOfParent","args":[{"col":"cost"}],"scale":1}},
	{"id":"ofTotal","formula":{"op":"percentOfTotal","args":[{"col":"cost"}],"scale":1}}`, 1)

// reachedRows is how many rows the one page statement of seen computed its
// detail select over (the rows of wl), or -1 when it computed it over the
// rows of every open group (no wl). The page statement joins gord; the read
// of the shown rows and a probe of the conditions join it too.
func reachedRows(t *testing.T, z *zoo, seen []readStatement) int {
	t.Helper()
	found, n := 0, -1
	for _, st := range seen {
		if !strings.Contains(st.sql, "JOIN gord ON") || strings.Contains(st.sql, "WHERE d.__id IN (") || strings.Contains(st.sql, "SELECT EXISTS (") {
			continue
		}
		found++
		at := strings.Index(st.sql, "\nwl AS MATERIALIZED (")
		if at < 0 {
			continue
		}
		end := at + 1 + strings.Index(st.sql[at+1:], "\nSELECT ")
		if err := z.app.DB().NewQuery(st.sql[:end+1] + "SELECT count(*) FROM wl").Bind(st.params).Row(&n); err != nil {
			t.Fatal(err)
		}
	}
	if found != 1 {
		t.Fatalf("%d page statements that join the open groups, want 1", found)
	}
	return n
}

// A page shows its rows in the groups' order, so it ends in the first open
// group at which the groups up to it hold a page of rows. The detail
// statement computes its select (the sort keys, the running values) over the
// rows of the groups up to that one only: lion's 502 rows of 504, with and
// without a having on the group.
func TestAPageComputesTheRowsOfTheGroupsItCanReach(t *testing.T) {
	for name, spec := range map[string]string{"no having": pagedBySpecies, "a having": pagedBySpeciesHaving} {
		t.Run(name, func(t *testing.T) {
			z := newZoo(t)
			bulkFeedings(t, z, 500)
			seen := observeReads(t)
			res := z.run(t, "ann", spec, Options{Limit: 10}, nil)
			if got := reachedRows(t, z, *seen); got != 502 {
				t.Fatalf("the select was computed over %d rows, want lion's 502", got)
			}
			// The page: lion, its opening and its first 8 feedings.
			details := 0
			for _, r := range res.Rows {
				if r.Kind == KindDetail {
					details++
				}
			}
			if details != 8 {
				t.Fatalf("%d detail rows on the page, want 8", details)
			}
			// A page that every group fits on reads every group.
			*seen = nil
			z.run(t, "ann", spec, Options{Limit: 1000}, nil)
			if got := reachedRows(t, z, *seen); got != 504 {
				t.Fatalf("a page of 1000 computed %d rows, want 504", got)
			}
			legacySQL.everyLeafRows = true
			t.Cleanup(func() { legacySQL.everyLeafRows = false })
			*seen = nil
			z.run(t, "ann", spec, Options{Limit: 10}, nil)
			if got := reachedRows(t, z, *seen); got != -1 {
				t.Fatalf("with the switch on, the statement still has wl (%d rows)", got)
			}
		})
	}
}

// requireLeafPagesSame walks every page of spec, each page with the rows of
// every open group and with the rows of the groups it can reach, and requires
// the same page (rows, cursor) both ways. It returns the number of pages.
func requireLeafPagesSame(t *testing.T, z *zoo, user, spec string, opts Options, params map[string]string) (pages int) {
	t.Helper()
	for range 200 {
		pages++
		requireSameAsLegacy(t, z, user, spec, opts, params, &legacySQL.everyLeafRows)
		res := z.run(t, user, spec, opts, params)
		if res.Page == nil || res.Page.Next == "" {
			return pages
		}
		opts.Cursor = res.Page.Next
	}
	t.Fatal("the pages do not end")
	return pages
}

// Every page of every reader is the same whether the select is computed over
// every open group's rows or over the rows of the groups the page can reach:
// running values and a percent of the parent, a having, a union, a blank key
// and one key in two currencies, pages that end inside a group (the next page
// holds that group: its rows count as none, and it is read) and pages that
// every group fits on.
func TestThePagesAreTheSameOverTheGroupsAPageCanReach(t *testing.T) {
	z := newUnionZoo(t)
	addOddTreats(t, z)
	withF7(t, z)
	bulkFeedings(t, z, 12)
	seen := observeReads(t)
	for _, limit := range []int{1, 2, 3, 5, 1000} {
		open := Options{ExpandAll: true, Limit: limit}
		for _, spec := range []string{pagedBySpecies, pagedBySpeciesHaving, speciesShares, byNoteClosed} {
			requireLeafPagesSame(t, z, "ann", spec, open, nil)
		}
		for _, user := range []string{"ann", "bob", "dan", "eve"} {
			requireLeafPagesSame(t, z, user, mealsByAnimal, open, mealDays)
			requireLeafPagesSame(t, z, user, mealsByNote, open, nil)
		}
	}
	cut, held := 0, 0
	for _, st := range *seen {
		if strings.Contains(st.sql, "\nwl AS MATERIALIZED (") {
			cut++
			if strings.Contains(st.sql, "gord.__g NOT IN (") {
				held++
			}
		}
	}
	if cut == 0 || held == 0 {
		t.Fatalf("%d statements over the groups a page can reach, %d of them with a held group: the test compared nothing", cut, held)
	}
}

// Pages of a few rows join into exactly the one-page run: a running value
// goes on across pages and a percent of the parent divides by the whole
// group, though each page computed only the groups it could reach.
func TestPagesOverTheGroupsTheyReachJoinIntoTheWholeRun(t *testing.T) {
	z := newZoo(t)
	withF7(t, z)
	bulkFeedings(t, z, 12)
	for _, spec := range []string{pagedBySpecies, speciesShares, reportShares} {
		whole := z.run(t, "ann", spec, Options{Limit: 1000}, nil)
		for _, limit := range []int{1, 3, 4} {
			if got := mergedRows(t, groupedPages(t, z, "ann", spec, Options{Limit: limit})); !reflect.DeepEqual(got, whole.Rows) {
				a, _ := json.Marshal(got)
				b, _ := json.Marshal(whole.Rows)
				t.Fatalf("pages of %d:\n%s\nwhole:\n%s", limit, a, b)
			}
		}
	}
}

// A percent of the whole report reads the rows of every group, so its
// statement keeps every open group's rows.
func TestAPercentOfTheWholeReportKeepsEveryGroupsRows(t *testing.T) {
	z := newZoo(t)
	bulkFeedings(t, z, 500)
	seen := observeReads(t)
	z.run(t, "ann", reportShares, Options{Limit: 10}, nil)
	if got := reachedRows(t, z, *seen); got != -1 {
		t.Fatalf("the select of a percent of the whole report was computed over %d rows, want every group's", got)
	}
}

// A condition that may be unknown on a row is probed over every open group
// of the page (probeConditions), so such a statement keeps every open group's
// rows.
func TestAProbedConditionKeepsEveryGroupsRows(t *testing.T) {
	z := newZoo(t)
	withEuroFeeding(t, z)
	spec := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal","open":true}],
		"sort":[{"formula":` + allCosts + `},{"col":"note"}]}`
	seen := observeReads(t)
	z.run(t, "ann", spec, Options{Limit: 2}, nil)
	if got := reachedRows(t, z, *seen); got != -1 {
		t.Fatalf("the select of a probed condition was computed over %d rows, want every group's", got)
	}
	probes := 0
	for _, st := range *seen {
		if strings.Contains(st.sql, "SELECT EXISTS (") && strings.Contains(st.sql, "__smx0 = 1") && !strings.Contains(st.sql, "__level") {
			probes++
		}
	}
	if probes != 1 {
		t.Fatalf("%d probes of the sort key, want 1: the spec no longer has a probed condition", probes)
	}
}

// A group that moved between pages (its record's sort value changed) is read
// wherever it sorts now: the rows the next page reads by id (readShownNow)
// are among the rows the statement keeps.
func TestAMovedGroupIsReadOverTheGroupsAPageCanReach(t *testing.T) {
	z := newZoo(t)
	z.feeding(t, "m2", z.recs["mo"], 40, "2026-02-20", 5)
	first := z.run(t, "bob", byAnimalWeight, Options{Limit: 2}, nil)
	if first.Page == nil || first.Page.Next == "" {
		t.Fatalf("the first page has no next cursor: %+v", first.Page)
	}
	mo := z.recs["mo"]
	mo.Set("weight", 999)
	if err := z.app.Save(mo); err != nil {
		t.Fatal(err)
	}
	seen := observeReads(t)
	if n := requireLeafPagesSame(t, z, "bob", byAnimalWeight, Options{Limit: 2, Cursor: first.Page.Next}, nil); n < 2 {
		t.Fatalf("%d pages after the move, want 2 or more", n)
	}
	shownRead := 0
	for _, st := range *seen {
		if strings.Contains(st.sql, "WHERE d.__id IN (") && strings.Contains(st.sql, "\nwl AS MATERIALIZED (") {
			shownRead++
		}
	}
	if shownRead == 0 {
		t.Fatal("no read of the shown rows ran over the groups a page can reach: the test compared nothing")
	}
}

// A group the cursor holds that moved after the next page's open groups is
// not a leaf of that page: the page leaves its rows out, but its shown row is
// still read where it sorts now (readShownNow), so the next cursor holds its
// current sort values, as with every open group's rows.
func TestAMovedGroupOffThePageIsReadOverTheGroupsAPageCanReach(t *testing.T) {
	z := newZoo(t)
	z.feeding(t, "m2", z.recs["mo"], 40, "2026-02-20", 5)
	for i, name := range []string{"ava", "bea", "cy", "dot"} {
		a := z.rec(t, name, "zoo_animals", map[string]any{"name": name, "species": "goat", "owner": z.id("bob"), "weight": 20 + 10*i})
		z.rec(t, "k"+name, "zoo_keepers", map[string]any{"user": z.id("bob"), "animal": a.Id})
		z.feeding(t, "f"+name, a, 10+i, "2026-01-1"+string(rune('1'+i)), 1)
	}
	first := z.run(t, "bob", byAnimalWeight, Options{Limit: 2}, nil)
	moGroup := "g:" + keyString([]any{z.id("mo"), "USD"})
	requireRows(t, first, "id", moGroup, "d:"+z.id("f5"), `total:["USD"]`)
	if c := decodedCursor(t, first.Page.Next); c.Mode != cursorIn || "g:"+keyString(c.Group) != moGroup {
		t.Fatalf("cursor %+v, want mode in, in Mo", c)
	}
	mo := z.recs["mo"]
	mo.Set("weight", 999)
	if err := z.app.Save(mo); err != nil {
		t.Fatal(err)
	}
	// Page 2's statement is cut, so it has two or more leaves (detailPass);
	// with no held place in it, Mo, which the cursor holds, is none of them.
	seen := observeReads(t)
	second := z.run(t, "bob", byAnimalWeight, Options{Limit: 2, Cursor: first.Page.Next}, nil)
	if slices.ContainsFunc(second.Rows, func(r Row) bool { return r.ID == moGroup }) {
		t.Fatalf("Mo is on page 2: %+v", second.Rows)
	}
	cut, kept, shownRead := 0, 0, 0
	for _, st := range *seen {
		if !strings.Contains(st.sql, "\nwl AS MATERIALIZED (") {
			continue
		}
		cut++
		if strings.Contains(st.sql, "gord.__g NOT IN (") {
			t.Fatal("Mo is a held leaf of page 2: the test does not reach a held group off the page")
		}
		if strings.Contains(st.sql, "gord.__g IS NULL AND w.__id IN (") {
			kept++
		}
		if strings.Contains(st.sql, "WHERE d.__id IN (") {
			shownRead++
		}
	}
	if cut == 0 || kept != cut || shownRead == 0 {
		t.Fatalf("%d cut statements, %d of them keep Mo's shown row, %d reads of the shown rows over them: the test compared nothing", cut, kept, shownRead)
	}
	// The LEFT JOIN form of wl must still find each row's group by its keys:
	// a scan of gord would read every open group for each row.
	planned := 0
	for _, st := range *seen {
		if !strings.Contains(st.sql, "LEFT JOIN gord ON") {
			continue
		}
		planned++
		for _, line := range explainPlan(t, z, st) {
			if strings.Contains(line, "SCAN gord") {
				t.Fatalf("the LEFT JOIN form of wl scans the open groups: %q", line)
			}
		}
	}
	if planned == 0 {
		t.Fatal("no statement holds LEFT JOIN gord ON: the test checked no plan")
	}
	if second.Page == nil || second.Page.Next == "" {
		t.Fatal("page 2 has no next cursor")
	}
	holds := decodedCursor(t, second.Page.Next).Held
	if !slices.ContainsFunc(holds, func(h held) bool { return "g:"+keyString(h.Group) == moGroup }) {
		t.Fatalf("page 2's cursor does not hold Mo: %+v", holds)
	}
	if n := requireLeafPagesSame(t, z, "bob", byAnimalWeight, Options{Limit: 2, Cursor: first.Page.Next}, nil); n < 2 {
		t.Fatalf("%d pages after the move, want 2 or more", n)
	}
}
