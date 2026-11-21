package reports

import (
	"encoding/json"
	"strings"
	"testing"
)

// A detail statement whose section CTEs read w twice or more computes it
// once; one whose only reads are its page's keeps it inline.
func TestMaterializeSharedMarksADetailStatementsRowsItsCTEsReadTwice(t *testing.T) {
	const inline, once = "w AS (SELECT 1)", "w AS MATERIALIZED (SELECT 1)"
	section := []string{inline, "f1 AS (SELECT k0 FROM w GROUP BY k0)", "a1 AS (SELECT k0, count(*) FROM w GROUP BY k0)", "ws AS (SELECT w.* FROM w)"}
	cases := []struct {
		name, body, want string
		ctes             []string
	}{
		{"a section's CTEs", "SELECT * FROM ws LIMIT 5", once, section},
		// The Total statement: its level CTEs read w, its body reads the
		// Total row and names no __level.
		{"the Total row's levels", "SELECT * FROM o0", once, []string{inline, "a0 AS (SELECT 0 AS __level FROM w)", "a1 AS (SELECT k0 FROM w GROUP BY k0)", "o0 AS (SELECT * FROM a0)"}},
		{"the sorted rows only", "SELECT * FROM ws JOIN ws w2 LIMIT 5", inline, []string{inline, "ws AS (SELECT w.* FROM w)"}},
		{"one CTE and the page", "SELECT * FROM w JOIN f1 LIMIT 5", inline, []string{inline, "f1 AS (SELECT k0 FROM w GROUP BY k0)"}},
		{"the page and a probe", "SELECT * FROM w WHERE EXISTS (SELECT 1 FROM w WHERE x) LIMIT 5", inline, []string{inline}},
	}
	for _, c := range cases {
		if got := materializeShared(c.ctes, c.body); got[0] != c.want {
			t.Errorf("%s: %q, want %q", c.name, got[0], c.want)
		}
	}
	// The switch writes the SQL of before: the cross-check compares with it.
	legacySQL.inlineDetailRows = true
	t.Cleanup(func() { legacySQL.inlineDetailRows = false })
	if got := materializeShared(section, "SELECT * FROM ws LIMIT 5"); got[0] != inline {
		t.Errorf("with the switch on: %q, want %q", got[0], inline)
	}
	if got := materializeShared(section, "SELECT __level FROM a1"); got[0] != once {
		t.Errorf("a group statement with the switch on: %q, want %q", got[0], once)
	}
}

// periodReads counts, in each detail statement of seen that has the report's
// rows, the plan steps that read the ledger lines by account and day range:
// the reads of the period's own rows. It returns the smallest and the largest
// count, and fails when no detail statement has the rows. The Total statement
// is left out on purpose (its level CTEs name __level): the unit case "the
// Total row's levels" covers it.
func periodReads(t *testing.T, z *zoo, seen []readStatement) (least, most int) {
	t.Helper()
	db := readPool(t, z)
	least = -1
	for _, st := range seen {
		if strings.Contains(st.sql, "__level") || !rowsWhere.MatchString(st.sql) {
			continue
		}
		raw, args := rawStatement(t, st.sql, st.params)
		n := 0
		for _, step := range queryPlan(t, db, raw, args) {
			if strings.Contains(step, "zoo_ledger_lines USING INDEX idx_zoo_ledger_lines_account_date (account=? AND date>? AND date<?)") {
				n++
			}
		}
		if least < 0 || n < least {
			least = n
		}
		most = max(most, n)
	}
	if least < 0 {
		t.Fatal("no detail statement has the report's rows")
	}
	return least, most
}

// The detail statement of a report with sections reads the period's rows in
// each section CTE: the lines table is read once for them, not once per CTE.
func TestADetailStatementWithSectionsReadsThePeriodsRowsOnce(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	withLedgerScale(t, z, 40)
	params := map[string]string{"period": ledgerPeriod, "accounts": ledgerAccounts(0, 2, 4)}
	opts := Options{Today: "2025-07-15", ExpandAll: true}
	seen := observeReads(t)
	if _, err := runLedger(z, params, opts); err != nil {
		t.Fatal(err)
	}
	if least, most := periodReads(t, z, *seen); least != 1 || most != 1 {
		t.Fatalf("a detail statement reads the period's rows %d to %d times, want once", least, most)
	}
	legacySQL.inlineDetailRows = true
	t.Cleanup(func() { legacySQL.inlineDetailRows = false })
	*seen = nil
	if _, err := runLedger(z, params, opts); err != nil {
		t.Fatal(err)
	}
	if least, _ := periodReads(t, z, *seen); least < 2 {
		t.Fatalf("with the switch on, a detail statement reads the period's rows %d times, want 2 or more", least)
	}
}

// Every run of the ledger fixture returns the same pages, the first and each
// one after it, with w computed once in the detail statement and with w
// inline.
func TestDetailRowsComputedOnceKeepTheResult(t *testing.T) {
	t.Cleanup(func() { legacySQL.inlineDetailRows = false })
	for _, mixed := range []bool{false, true} {
		z := newZoo(t)
		withBalanceZoo(t, z)
		withLedgerLines(t, z, 45, mixed)
		for _, r := range ledgerRuns() {
			legacySQL.inlineDetailRows = true
			before := ledgerCanonical(t, z, ledgerPages(t, z, r))
			legacySQL.inlineDetailRows = false
			if after := ledgerCanonical(t, z, ledgerPages(t, z, r)); after != before {
				t.Fatalf("%s (mixed %v): the pages changed\nbefore: %s\nafter:  %s", r.name, mixed, before, after)
			}
		}
		requireSameAsLegacy(t, z, "bob", balanceSpec(), Options{Today: "2025-07-15", ExpandAll: true},
			map[string]string{"period": ledgerPeriod, "accounts": ledgerAccounts(0, 2)}, &legacySQL.inlineDetailRows)
	}
}

// sameWithAndWithoutRowsOnce runs pages twice, with w inline in the detail
// statements and with w computed once, and requires the same pages. It also
// requires that a statement isTarget picks ran in both, with w inline in the
// first and MATERIALIZED in the second: a fixture that never reaches the
// statement, or reaches it with w inline in both, compares nothing.
func sameWithAndWithoutRowsOnce(t *testing.T, name string, isTarget func(sql string) bool, pages func() []*Result) {
	t.Helper()
	t.Cleanup(func() { legacySQL.inlineDetailRows = false })
	seen := observeReads(t)
	var canonical [2]string
	for i, inline := range []bool{true, false} {
		legacySQL.inlineDetailRows = inline
		*seen = nil
		out, err := json.Marshal(pages())
		if err != nil {
			t.Fatal(err)
		}
		canonical[i] = string(out)
		ran, once := 0, 0
		for _, st := range *seen {
			if !isTarget(st.sql) {
				continue
			}
			ran++
			if strings.Contains(st.sql, "w AS MATERIALIZED (") {
				once++
			}
		}
		if ran == 0 {
			t.Fatalf("%s: no statement ran (inline %v)", name, inline)
		}
		if want := map[bool]int{true: 0, false: ran}[inline]; once != want {
			t.Fatalf("%s: %d of %d statements have w MATERIALIZED (inline %v), want %d", name, once, ran, inline, want)
		}
	}
	if canonical[0] != canonical[1] {
		t.Fatalf("%s: the pages changed\ninline: %s\nonce:   %s", name, canonical[0], canonical[1])
	}
}

// A paged grouped run whose group has a having and moved between pages reads
// the shown rows now (readShownNow): the detail select of the page's groups,
// filtered to the cursor's row ids. Its w is computed once, as the page's.
func TestTheShownRowsReadOfAMovedGroupKeepsThePages(t *testing.T) {
	z := newZoo(t)
	z.feeding(t, "m2", z.recs["mo"], 40, "2026-02-20", 5)
	spec := strings.Replace(byAnimalWeight, `"open":true,"subtotals":true}`,
		`"open":true,"subtotals":true,"having":{"op":"gt","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":0}]}}`, 1)
	first := z.run(t, "bob", spec, Options{Limit: 2}, nil)
	if first.Page == nil || first.Page.Next == "" {
		t.Fatalf("the first page has no next cursor: %+v", first.Page)
	}
	mo := z.recs["mo"]
	mo.Set("weight", 999)
	if err := z.app.Save(mo); err != nil {
		t.Fatal(err)
	}
	sameWithAndWithoutRowsOnce(t, "the shown rows read",
		func(sql string) bool { return strings.Contains(sql, "WHERE d.__id IN (") },
		func() []*Result { return groupedPages(t, z, "bob", spec, Options{Limit: 2, Cursor: first.Page.Next}) })
}

// A view sort over a sum lookup that met two currencies, in a report whose
// group has a having, is probed over the rows the detail pass reads
// (detailSortMixed): an EXISTS statement over the detail select, with w
// computed once as the page's is.
func TestTheDetailProbeOfAMixedSortKeepsThePages(t *testing.T) {
	z := newZoo(t)
	withEuroFeeding(t, z)
	spec := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal","open":true,"having":{"op":"gt","args":[{"op":"count","args":[{"path":[{"field":"id"}]}]},{"value":0}]}}],
		"sort":[{"formula":` + allCosts + `},{"col":"note"}]}`
	sameWithAndWithoutRowsOnce(t, "the detail probe",
		func(sql string) bool {
			return strings.Contains(sql, "SELECT EXISTS (") && strings.Contains(sql, "__smx0 = 1") && !strings.Contains(sql, "__level")
		},
		func() []*Result { return []*Result{z.run(t, "ann", spec, Options{ExpandAll: true}, nil)} })
}
