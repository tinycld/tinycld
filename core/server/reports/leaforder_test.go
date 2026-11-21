package reports

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// The detail statement of a grouped run joins each detail row to its open
// group (gord, a json_each over one bound list). Measured on darwin/arm64,
// 14 CPUs: with gord read inline the statement's time grows with rows x open
// groups (4,000 open groups 6.7 s, 8,000 about 27 s); with gord MATERIALIZED
// it grows with the rows (16,000 open groups 0.16 s). The limit of 2 s is for
// each statement of the run: the group statement (0.13 s at 16,660 groups)
// and the Total (0.08 s) are under it too. It is 13 times under the one
// reading and over 10 times above the other, so this fails only on the
// quadratic read. A failure is a result: never raise the limit to pass.
func TestAGroupedRunOfManyOpenGroupsReadsItsDetailsOnce(t *testing.T) {
	z := newZoo(t)
	bulkFeedings(t, z, 8000)
	withLimits(t, func(l *limits) {
		l.timeout = 2 * time.Second
		l.maxPage = 30000
	})
	res, err := z.try(t, "ann", byNoteClosed, Options{ExpandAll: true, Limit: 30000}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	details := 0
	for _, r := range res.Rows {
		if r.Kind == KindDetail {
			details++
		}
	}
	if details != 8004 {
		t.Fatalf("%d detail rows, want 8004", details)
	}
}

// Both grouped paths write the join: a run that pages its groups, and a
// layout, which does not. Each computes the open groups once.
func TestTheDetailStatementComputesItsOpenGroupsOnce(t *testing.T) {
	for name, spec := range map[string]string{"paged groups": byNoteClosed, "a layout": fmt.Sprintf(zooStatement, "")} {
		t.Run(name, func(t *testing.T) {
			z := newZoo(t)
			seen := observeReads(t)
			z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
			found := 0
			for _, st := range *seen {
				if !strings.Contains(st.sql, "JOIN gord ON") {
					continue
				}
				found++
				if !strings.Contains(st.sql, "gord AS MATERIALIZED (") {
					t.Fatalf("the open groups are read inline:\n%s", st.sql)
				}
			}
			if found != 1 {
				t.Fatalf("%d grouped detail statements, want 1", found)
			}
		})
	}
}

// mealsByAnimal is the union's meals of a period, one open section an animal:
// a day bound alone, no link bound.
const mealsByAnimal = `{"version":4,"source":"zoo.meals","params":[{"id":"days","type":"dateRange","label":"Days","default":{"preset":"allDates"}}],
	"columns":[
	{"id":"animal","path":[{"link":"animal"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"filter":{"op":"between","args":[{"path":[{"field":"fed"}]},{"param":"days"}]},
	"groups":[{"id":"g","col":"animal","subtotals":true}],"sort":[{"col":"animal"}]}`

// mealsByNote groups the union's meals by their note: a note may be blank, and
// one note may hold meals of two currencies (two groups).
const mealsByNote = `{"version":4,"source":"zoo.meals","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"note","subtotals":true}],"sort":[{"col":"note"}]}`

// addOddTreats gives ann a treat with a blank note and a treat in a second
// currency whose note another treat has.
func addOddTreats(t *testing.T, z *zoo) {
	t.Helper()
	z.rec(t, "t5", "zoo_treats", map[string]any{"pet": z.id("kit"), "price": 5, "currency": "USD", "given": "2026-01-09 00:00:00.000Z", "label": ""})
	z.rec(t, "t6", "zoo_treats", map[string]any{"pet": z.id("kit"), "price": 7, "currency": "EUR", "given": "2026-01-10 00:00:00.000Z", "label": "t1"})
}

// gordPlans is the plan lines of the grouped detail statement of a run that
// name the open groups (gord) or the detail rows' base (t1).
func gordPlans(t *testing.T, z *zoo, seen []readStatement) []string {
	t.Helper()
	var lines []string
	found := 0
	for _, st := range seen {
		if !strings.Contains(st.sql, "JOIN gord ON") {
			continue
		}
		found++
		for _, line := range explainPlan(t, z, st) {
			if strings.Contains(line, "gord") || strings.HasSuffix(line, " t1") || strings.Contains(line, " t1 ") {
				lines = append(lines, line)
			}
		}
	}
	if found != 1 {
		t.Fatalf("%d grouped detail statements, want 1", found)
	}
	return lines
}

// The detail statement reads its rows once and finds each row's open group by
// its keys. Left to SQLite's planner, a base that is a union (a subquery) was
// read once for each open group, through an index SQLite built on the rows'
// currency: rows x open groups. The engine fixes the order with CROSS JOIN.
func TestAGroupedDetailStatementReadsItsRowsOnce(t *testing.T) {
	z := newUnionZoo(t)
	seen := observeReads(t)
	z.run(t, "ann", mealsByAnimal, Options{ExpandAll: true}, mealDays)
	plan := strings.Join(gordPlans(t, z, *seen), "\n")
	if strings.Contains(plan, "SCAN gord") || strings.Contains(plan, "SEARCH t1") {
		t.Fatalf("the rows are read once for each open group:\n%s", plan)
	}
	if !strings.Contains(plan, "SCAN t1") || !strings.Contains(plan, "SEARCH gord USING AUTOMATIC") {
		t.Fatalf("the rows do not find their group by its keys:\n%s", plan)
	}
	legacySQL.plannedLeafJoin = true
	t.Cleanup(func() { legacySQL.plannedLeafJoin = false })
	*seen = nil
	z.run(t, "ann", mealsByAnimal, Options{ExpandAll: true}, mealDays)
	for _, st := range *seen {
		if strings.Contains(st.sql, "CROSS JOIN gord") || strings.Contains(st.sql, "+d.k0 IS gord.g0") {
			t.Fatalf("with the switch on, the join is not the planner's:\n%s", st.sql)
		}
	}
}

// Every reader gets the same rows whichever side drives the join: a union and
// a collection, paged groups and a layout, a period and all dates, a group
// with a blank key and one note in two currencies.
func TestTheOpenGroupJoinOrderKeepsTheResult(t *testing.T) {
	z := newUnionZoo(t)
	addOddTreats(t, z)
	open := Options{ExpandAll: true}
	for _, user := range []string{"ann", "bob", "dan", "eve"} {
		for _, params := range []map[string]string{mealDays, nil} {
			requireSameAsLegacy(t, z, user, mealsByAnimal, open, params, &legacySQL.plannedLeafJoin)
		}
		requireSameAsLegacy(t, z, user, mealsByNote, open, nil, &legacySQL.plannedLeafJoin)
		requireSameAsLegacy(t, z, user, byNoteClosed, open, nil, &legacySQL.plannedLeafJoin)
		requireSameAsLegacy(t, z, user, fmt.Sprintf(zooStatement, ""), open, nil, &legacySQL.plannedLeafJoin)
	}
	// ann reads f1-f4 and t1, t3, t5 (blank note), t6 (EUR): the blank note is
	// a group of its own, and note t1 is two groups, one a currency.
	res := z.run(t, "ann", mealsByNote, open, nil)
	groups, details := 0, 0
	for _, r := range res.Rows {
		switch r.Kind {
		case KindGroup:
			groups++
		case KindDetail:
			details++
		}
	}
	if groups != 8 || details != 8 {
		t.Fatalf("%d groups and %d detail rows, want 8 and 8", groups, details)
	}
}

// A base that is a collection finds each row's group through an index on the
// open groups too. A key that is a stored text column compares with the
// column's affinity, and SQLite builds no index on gord for such a term: the
// rows were then read against every open group. The unary + of leafFrom takes
// the affinity off.
func TestAGroupedDetailStatementOfACollectionFindsItsGroupByAnIndex(t *testing.T) {
	z := newZoo(t)
	seen := observeReads(t)
	z.run(t, "ann", byNoteClosed, Options{ExpandAll: true}, nil)
	plan := strings.Join(gordPlans(t, z, *seen), "\n")
	if strings.Contains(plan, "SCAN gord") || !strings.Contains(plan, "SEARCH gord USING AUTOMATIC") {
		t.Fatalf("the rows do not find their group by its keys:\n%s", plan)
	}
	for _, key := range []string{"g0=?", "g1=?"} {
		if !strings.Contains(plan, key) {
			t.Fatalf("the index on the open groups has no %s:\n%s", key, plan)
		}
	}
}

// Group keys of each stored kind: a number, a month of a date, a bool read
// through a link.
const (
	byGrams = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"grams","path":[{"field":"grams"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"grams","subtotals":true}],"sort":[{"col":"grams"}]}`
	byFedMonth = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"fed","path":[{"field":"fed"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"fed","bucket":"month","subtotals":true}]}`
	byActive = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"active","path":[{"link":"animal"},{"field":"active"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"active","subtotals":true}]}`
)

// The join compares a row's key with its group's with no column affinity. A
// key of every stored kind still finds its group: each run shows the detail
// rows the planner's join showed, and every feeding the reader may read.
func TestTheOpenGroupJoinMatchesAKeyOfEveryKind(t *testing.T) {
	z := newZoo(t)
	z.feeding(t, "half", z.recs["leo"], 3, "2026-03-05", 0)
	half := z.recs["half"]
	half.Set("grams", 2.5)
	if err := z.app.Save(half); err != nil {
		t.Fatal(err)
	}
	open := Options{ExpandAll: true}
	flat := 0
	for _, r := range z.run(t, "ann", `{"version":4,"source":"zoo.feedings","columns":[{"id":"cost","path":[{"field":"cost"}]}]}`, Options{}, nil).Rows {
		if r.Kind == KindDetail {
			flat++
		}
	}
	if flat == 0 {
		t.Fatal("ann reads no feeding")
	}
	for name, spec := range map[string]string{"a number": byGrams, "a month": byFedMonth, "a bool through a link": byActive} {
		t.Run(name, func(t *testing.T) {
			requireSameAsLegacy(t, z, "ann", spec, open, nil, &legacySQL.plannedLeafJoin)
			details := 0
			for _, r := range z.run(t, "ann", spec, open, nil).Rows {
				if r.Kind == KindDetail {
					details++
				}
			}
			if details != flat {
				t.Fatalf("%d detail rows under the open groups, want %d", details, flat)
			}
		})
	}
}

// A union with 8,000 open groups stays under the limit of
// TestAGroupedRunOfManyOpenGroupsReadsItsDetailsOnce: SQLite's plan of this
// join changes with the number of rows, so the small plan tests above do not
// prove the large one. A failure is a result: never raise the limit to pass.
func TestAGroupedRunOfAUnionWithManyOpenGroupsReadsItsRowsOnce(t *testing.T) {
	z := newUnionZoo(t)
	bulkFeedings(t, z, 8000)
	withLimits(t, func(l *limits) {
		l.timeout = 2 * time.Second
		l.maxPage = 30000
	})
	res, err := z.try(t, "ann", mealsByNote, Options{ExpandAll: true, Limit: 30000}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	details := 0
	for _, r := range res.Rows {
		if r.Kind == KindDetail {
			details++
		}
	}
	if details != 8006 {
		t.Fatalf("%d detail rows, want 8006", details)
	}
}

// A run cut into pages gives the same pages whichever side drives the join.
func TestTheOpenGroupJoinOrderKeepsEveryPage(t *testing.T) {
	z := newUnionZoo(t)
	addOddTreats(t, z)
	for _, spec := range []string{mealsByNote, mealsByAnimal} {
		opts := Options{ExpandAll: true, Limit: 3}
		after, err := json.Marshal(mergedRows(t, groupedPages(t, z, "ann", spec, opts)))
		if err != nil {
			t.Fatal(err)
		}
		legacySQL.plannedLeafJoin = true
		t.Cleanup(func() { legacySQL.plannedLeafJoin = false })
		before, err := json.Marshal(mergedRows(t, groupedPages(t, z, "ann", spec, opts)))
		legacySQL.plannedLeafJoin = false
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) {
			t.Fatalf("the pages changed\nbefore: %s\nafter:  %s", before, after)
		}
	}
}
