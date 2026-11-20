package reports

import (
	"strings"
	"testing"
)

// knownLinks writes each part of a lookup as a CASE over its two forms, tests
// only the values whose bound the where holds, and leaves a lookup with no
// link bound as it is.
func TestKnownLinksWritesEachPartInTwoForms(t *testing.T) {
	form := func(w string) expr {
		return expr{
			sql: "(CASE WHEN M(" + w + ") THEN NULL ELSE S(" + w + ") END)", unguarded: "S(" + w + ")", mixed: "M(" + w + ")",
			typ: Money, cur: currency{sql: "C(" + w + ")", id: "lk"}, scan: "SCAN " + w,
		}
	}
	eqs := []linkEq{{rng: "R1", eq: "E1", v: "a.rc1"}, {rng: "gone", eq: "X", v: "a.rc9"}, {rng: "R2", eq: "E2", v: "a.rc2"}}
	got, err := knownLinks(form("R1 AND R2 AND d"), eqs, "R1 AND R2 AND d", func(where string) (expr, error) {
		return form(where), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	either := func(part string) string {
		return "(CASE WHEN a.rc1 IS NOT NULL AND a.rc2 IS NOT NULL THEN " + part + "(E1 AND E2 AND d) ELSE " + part + "(R1 AND R2 AND d) END)"
	}
	for _, c := range []struct{ name, got, want string }{
		{"the value before the check", got.unguarded, either("S")},
		{"the check", got.mixed, either("M")},
		{"the currency", got.cur.sql, either("C")},
		{"the shown form", got.shown, ""},
		{"the one-read form", got.scan, "SCAN R1 AND R2 AND d"},
	} {
		if c.got != c.want {
			t.Errorf("%s: %q, want %q", c.name, c.got, c.want)
		}
	}
	if !strings.HasPrefix(got.sql, "(CASE WHEN a.rc1 IS NOT NULL AND a.rc2 IS NOT NULL THEN (CASE WHEN M(E1 AND E2 AND d) THEN NULL") {
		t.Errorf("the value: %q", got.sql)
	}
	if got.typ != Money || got.cur.id != "lk" {
		t.Errorf("the type and the currency's name changed: %v, %q", got.typ, got.cur.id)
	}
	plain := expr{sql: "S", typ: Number}
	same, err := knownLinks(plain, nil, "d", func(string) (expr, error) {
		t.Fatal("a lookup with no link bound was built twice")
		return expr{}, nil
	})
	if err != nil || same.sql != "S" {
		t.Fatalf("a lookup with no link bound: %q, %v", same.sql, err)
	}
}

// knownLinkSearches counts, over the statements seen, the plan steps that
// read the ledger lines of one account before a day (the equality form with
// its day bound) and the least such count of a statement.
func knownLinkSearches(t *testing.T, z *zoo, seen []readStatement) (total, least int) {
	t.Helper()
	db := readPool(t, z)
	least = -1
	for _, st := range seen {
		raw, args := rawStatement(t, st.sql, st.params)
		n := 0
		for _, step := range queryPlan(t, db, raw, args) {
			if strings.Contains(step, "zoo_ledger_lines USING INDEX idx_zoo_ledger_lines_account_date (account=? AND date<?)") {
				n++
			}
		}
		total += n
		if least < 0 || n < least {
			least = n
		}
	}
	return total, least
}

// The beginning balance of a section is a sum lookup of one account's lines
// before the period. Each statement of the run reads it for a known account
// by the account and the day, so by the lines before the period only; with
// the switch on, no statement does (the range on the account leaves the day
// out of the search).
func TestALookupOnAKnownLinkSearchesTheLinkAndTheDay(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	withLedgerScale(t, z, 40)
	params := map[string]string{"period": ledgerPeriod, "accounts": ledgerAccounts(0, 2, 4)}
	opts := Options{Today: "2025-07-15", ExpandAll: true}
	seen := observeReads(t)
	if _, err := runLedger(z, params, opts); err != nil {
		t.Fatal(err)
	}
	if len(*seen) == 0 {
		t.Fatal("no statement was read")
	}
	// The value, the two-currency check and the currency: three reads.
	if _, least := knownLinkSearches(t, z, *seen); least < 3 {
		t.Fatalf("a statement reads the lines by account and day %d times, want 3 or more", least)
	}
	legacySQL.rangeLinkBounds = true
	t.Cleanup(func() { legacySQL.rangeLinkBounds = false })
	*seen = nil
	if _, err := runLedger(z, params, opts); err != nil {
		t.Fatal(err)
	}
	if total, _ := knownLinkSearches(t, z, *seen); total != 0 {
		t.Fatalf("with the switch on, %d reads by account and day, want none", total)
	}
	for _, st := range *seen {
		if strings.Contains(st.sql, ".account]] = ") {
			t.Fatalf("with the switch on, a statement bounds the account by an equality:\n%s", st.sql)
		}
	}
}

// Every run of the ledger fixture returns the same pages with the two forms
// and with the range alone: on a plain and a mixed book, with lines whose
// account is blank (their section's record is blank, as the Total row's is,
// so they take the range form), and for a reader who may list no account.
func TestKnownLinksKeepTheResult(t *testing.T) {
	t.Cleanup(func() { legacySQL.rangeLinkBounds = false })
	for _, mixed := range []bool{false, true} {
		z := newZoo(t)
		withBalanceZoo(t, z)
		withLedgerLines(t, z, 45, mixed)
		for i, day := range []string{"2025-01-10", "2025-04-10", "2025-04-11"} {
			z.recWithID(t, ledgerID("blnk", i), "zoo_ledger_lines", map[string]any{
				"entry": ledgerID("entr", i), "account": "", "date": day, "debit": 700 + i, "currency": "USD", "position": 8, "posted": true,
			})
		}
		for _, r := range ledgerRuns() {
			legacySQL.rangeLinkBounds = true
			before := ledgerCanonical(t, z, ledgerPages(t, z, r))
			legacySQL.rangeLinkBounds = false
			if after := ledgerCanonical(t, z, ledgerPages(t, z, r)); after != before {
				t.Fatalf("%s (mixed %v): the pages changed\nbefore: %s\nafter:  %s", r.name, mixed, before, after)
			}
		}
		requireSameAsLegacy(t, z, "bob", balanceSpec(), Options{Today: "2025-07-15", ExpandAll: true},
			map[string]string{"period": ledgerPeriod, "accounts": ledgerAccounts(0, 2)}, &legacySQL.rangeLinkBounds)
	}
}
