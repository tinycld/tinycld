package reports

import (
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
