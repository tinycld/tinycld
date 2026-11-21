package reports

import (
	"fmt"
	"regexp"
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

// readsOf runs a spec and returns the text of the statements it sent.
func readsOf(t *testing.T, z *zoo, spec string, params map[string]string) string {
	t.Helper()
	seen := observeReads(t)
	z.run(t, "ann", spec, Options{}, params)
	var all []string
	for _, st := range *seen {
		all = append(all, st.sql)
	}
	if len(all) == 0 {
		t.Fatal("no statement was read")
	}
	return strings.Join(all, "\n")
}

// linesByAccount is a lookup of the ledger lines of one account, matched by
// the looked-for value v.
func linesByAccount(v string) string {
	return `{"op":"lookup","source":"zoo.ledger_lines","pick":"count","value":{"path":[{"field":"id"}]},
		"where":{"op":"eq","args":[{"path":[{"link":"account"}]},` + v + `]}}`
}

// A looked-for value that is a column whose formula holds a lookup is computed
// by the formula in every place the column is read, so the lookup that matches
// it keeps the range alone: a second form would read the formula again in the
// test that picks the form. The result is the one the range gives.
func TestAFormulaColumnAsTheLookedForValueKeepsTheRange(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	spec := `{"version":4,"source":"zoo.ledger_accounts","columns":[
		{"id":"number","path":[{"field":"number"}]},
		{"id":"own","formula":{"op":"lookup","source":"zoo.ledger_accounts","pick":"max","value":{"path":[{"field":"id"}]},
			"where":{"op":"eq","args":[{"path":[{"field":"number"}]},{"outer":[{"field":"number"}]}]}}},
		{"id":"lines","formula":` + linesByAccount(`{"col":"own"}`) + `}],
		"sort":[{"col":"number"}]}`
	t.Cleanup(func() { legacySQL.rangeLinkBounds = false })
	legacySQL.rangeLinkBounds = false
	text := readsOf(t, z, spec, nil)
	if !strings.Contains(text, ".account]] >= ") {
		t.Fatalf("the range form was not written:\n%s", text)
	}
	if strings.Contains(text, ".account]] = ") {
		t.Fatalf("a formula column got the equality form:\n%s", text)
	}
	legacySQL.rangeLinkBounds = true
	if rangeOnly := readsOf(t, z, spec, nil); strings.Count(text, "max(") != strings.Count(rangeOnly, "max(") {
		t.Fatalf("the column's lookup is computed %d times, with the range alone %d", strings.Count(text, "max("), strings.Count(rangeOnly, "max("))
	}
	requireSameAsLegacy(t, z, "ann", spec, Options{}, nil, &legacySQL.rangeLinkBounds)
	res := z.run(t, "ann", spec, Options{}, nil)
	if got := rowsOf(res, "lines")[0]; got != int64(3) {
		t.Fatalf("the first account (cash) holds %v lines, want 3", got)
	}
}

// A looked-for value that is a param or a constant is known before the lookup
// runs, so the lookup gets the equality form: the statement has it with the
// switch off and not with it on, and the result is the same.
func TestAParamOrAConstantAsTheLookedForValueGetsTheEquality(t *testing.T) {
	for _, c := range []struct{ name, value string }{
		{"a param", `{"param":"acct"}`},
		{"a constant", `{"value":"%s"}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			z := newZoo(t)
			withBalanceZoo(t, z)
			value := c.value
			if strings.Contains(value, "%s") {
				value = fmt.Sprintf(value, z.id("cash"))
			}
			spec := `{"version":4,"source":"zoo.ledger_accounts","params":[{"id":"acct","type":"text"}],"columns":[
				{"id":"number","path":[{"field":"number"}]},
				{"id":"lines","formula":` + linesByAccount(value) + `}],
				"sort":[{"col":"number"}]}`
			params := map[string]string{"acct": fmt.Sprintf("%q", z.id("cash"))}
			t.Cleanup(func() { legacySQL.rangeLinkBounds = false })
			legacySQL.rangeLinkBounds = false
			text := readsOf(t, z, spec, params)
			if !strings.Contains(text, ".account]] = ") || !strings.Contains(text, ".account]] >= ") {
				t.Fatalf("the statement lacks the equality or the range form:\n%s", text)
			}
			legacySQL.rangeLinkBounds = true
			if text := readsOf(t, z, spec, params); strings.Contains(text, ".account]] = ") {
				t.Fatalf("with the switch on, the statement has the equality form:\n%s", text)
			}
			requireSameAsLegacy(t, z, "ann", spec, Options{}, params, &legacySQL.rangeLinkBounds)
			res := z.run(t, "ann", spec, Options{}, params)
			if got := rowsOf(res, "lines")[0]; got != int64(3) {
				t.Fatalf("the first account (cash) holds %v lines, want 3", got)
			}
		})
	}
}

// bothKnown matches a test that names two values.
var bothKnown = regexp.MustCompile(`CASE WHEN [^\n]*? IS NOT NULL AND [^\n]*? IS NOT NULL THEN`)

// A where with two link bounds writes both as equalities in the one form, and
// the form is chosen by a test that names both values.
func TestAWhereWithTwoLinkBoundsTakesBothEqualities(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	spec := `{"version":4,"source":"zoo.ledger_lines","columns":[
		{"id":"date","path":[{"field":"date"}]},
		{"id":"same","formula":{"op":"lookup","source":"zoo.ledger_lines","pick":"count","value":{"path":[{"field":"id"}]},
			"where":{"op":"and","args":[
				{"op":"eq","args":[{"path":[{"link":"account"}]},{"outer":[{"link":"account"}]}]},
				{"op":"eq","args":[{"path":[{"link":"entry"}]},{"outer":[{"link":"entry"}]}]}]}}}],
		"sort":[{"col":"date"}]}`
	t.Cleanup(func() { legacySQL.rangeLinkBounds = false })
	legacySQL.rangeLinkBounds = false
	text := readsOf(t, z, spec, nil)
	for _, link := range []string{"account", "entry"} {
		if !strings.Contains(text, "."+link+"]] = ") || !strings.Contains(text, "."+link+"]] >= ") {
			t.Fatalf("the statement lacks the equality or the range form of %s:\n%s", link, text)
		}
	}
	if !bothKnown.MatchString(text) {
		t.Fatalf("no test names both values:\n%s", text)
	}
	legacySQL.rangeLinkBounds = true
	if text := readsOf(t, z, spec, nil); strings.Contains(text, ".account]] = ") || strings.Contains(text, ".entry]] = ") {
		t.Fatalf("with the switch on, the statement has an equality form:\n%s", text)
	}
	requireSameAsLegacy(t, z, "ann", spec, Options{}, nil, &legacySQL.rangeLinkBounds)
	res := z.run(t, "ann", spec, Options{}, nil)
	for i, got := range rowsOf(res, "same") {
		if res.Rows[i].Kind == "detail" && got != int64(1) {
			t.Fatalf("row %d holds %v lines of its account and entry, want 1", i, got)
		}
	}
}
