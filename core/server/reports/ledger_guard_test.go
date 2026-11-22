package reports

import (
	"context"
	"database/sql"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/changesignal"
)

// recordSubquery is a group record's field read as a subquery keyed by a
// level row (a.k<i>) or by a base row (w.k<i>).
var recordSubquery = regexp.MustCompile(`\(SELECT (?:NULLIF\()?\[\[t\d+\.\w+\]\](?:, '')?\)?\nFROM r\d+ t\d+\nWHERE \[\[t\d+\.id\]\] IS ([aw])\.k\d+\)`)

// The ledger's beginning balance (two sum lookups) is read in the line
// balance, the rollup, the having and the opening cell, and its group record
// by the label, the sort keys and the having. Each statement computes each
// lookup once per group row and joins the record once per level, so the SQL
// does not repeat them: checked on the SQL, not on a clock.
func TestABalanceShapedReportComputesItsBeginningBalanceOncePerStatement(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	withLedgerScale(t, z, 20)
	seen := observeReads(t)
	if _, err := runLedger(z, map[string]string{"period": ledgerPeriod}, Options{Today: "2025-07-15"}); err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, st := range *seen {
		kind := "detail"
		if isTotalStatement(st.sql) {
			kind = "total"
		} else if strings.Contains(st.sql, "__level") {
			kind = "group"
		}
		kinds = append(kinds, kind)
		// Two lookups, on two levels (the total and the accounts) or in two
		// selects (the shown accounts and the lines). The lookup that matches
		// the account is written in two forms (knownLinks), and a row runs one
		// of them: 3 sums a level.
		if n := strings.Count(st.sql, "(SELECT sum("); n > 6 {
			t.Errorf("the %s statement holds %d sum lookups, want at most 6 (2 lookups, one of them in two forms, once per level)", kind, n)
		}
		byLevel, byLine := 0, 0
		for _, m := range recordSubquery.FindAllStringSubmatch(st.sql, -1) {
			if m[1] == "a" {
				byLevel++
			} else {
				byLine++
			}
		}
		if byLevel != 0 {
			t.Errorf("the %s statement reads a group record by %d subqueries on a level row, want a join", kind, byLevel)
		}
		// The 3 reads of the account inside the beginning-balance lookup's
		// where, in each of its 3 parts (the sum, its mixed check, its
		// currency), all computed once per account. With the two forms of
		// that lookup (knownLinks) each part also reads the account 2 times in
		// its equality form and once in the test that picks the form: 18 in
		// the text, of which a row runs 9 (a known account) or 12 (a blank one).
		if byLine > 18 {
			t.Errorf("the %s statement reads a group record by %d subqueries on a line, want at most 18", kind, byLine)
		}
	}
	got := append([]string(nil), kinds...)
	sort.Strings(got)
	// The group statement reads the Total too.
	if !slices.Equal(got, []string{"detail", "group"}) {
		t.Fatalf("statements %v, want the group statement (with the Total) and the detail statement", got)
	}
}

// J1a: a cursor page's level statement, which reads the Total too, keeps the
// #452 shape (each lookup once per group row, the group record joined once
// per level).
func TestACursorPageComputesItsBeginningBalanceOncePerStatement(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	withLedgerScale(t, z, 20)
	first, err := runLedger(z, map[string]string{"period": ledgerPeriod}, Options{Today: "2025-07-15", Limit: 7})
	if err != nil || first.Page == nil || first.Page.Next == "" {
		t.Fatalf("first page: %+v, %v", first.Page, err)
	}
	seen := observeReads(t)
	if _, err := runLedger(z, map[string]string{"period": ledgerPeriod}, Options{Today: "2025-07-15", Limit: 7, Cursor: first.Page.Next}); err != nil {
		t.Fatal(err)
	}
	db := readPool(t, z)
	kinds := map[string]int{}
	for _, st := range *seen {
		kind := "detail"
		switch {
		case isTotalStatement(st.sql):
			kind = "total"
		case strings.Contains(st.sql, "__p1"):
			kind = "page"
		}
		kinds[kind]++
		if kind == "page" {
			requireLevelRowsOnce(t, db, st)
		}
		if n := strings.Count(st.sql, "(SELECT sum("); n > 6 {
			t.Errorf("the %s statement holds %d sum lookups, want at most 6", kind, n)
		}
		for _, m := range recordSubquery.FindAllStringSubmatch(st.sql, -1) {
			if m[1] == "a" {
				t.Errorf("the %s statement reads a group record by a subquery on a level row", kind)
			}
		}
	}
	if kinds["page"] != 1 || kinds["total"] != 0 {
		t.Fatalf("statements %v, want one page statement (with the Total) and no Total statement of its own", kinds)
	}
}

// levelRowsStep is a level's rows o<L> computed in a query plan.
var levelRowsStep = regexp.MustCompile(`^\s*(MATERIALIZE|CO-ROUTINE) (o\d+)$`)

// requireLevelRowsOnce fails unless the plan of a page statement computes
// each o<L> of a group level once, materialized: its leg, the cursor's
// sibling counts and the Total's levels read it, and SQLite computes an
// inlined o<L> again for each reader, its lookups with it. o0, the Total
// rows, is read by the Total's leg alone, so it may be a co-routine, but it
// too is computed once. The SQL text is the same either way, so this reads
// the plan.
func requireLevelRowsOnce(t *testing.T, db *sql.DB, st readStatement) {
	t.Helper()
	raw, args := rawStatement(t, st.sql, st.params)
	plan := queryPlan(t, db, raw, args)
	computed := map[string][]string{}
	for _, step := range plan {
		if m := levelRowsStep.FindStringSubmatch(step); m != nil {
			computed[m[2]] = append(computed[m[2]], m[1])
		}
	}
	if len(computed) == 0 {
		t.Fatalf("the page statement's plan computes no o<L>:\n%s", strings.Join(plan, "\n"))
	}
	for name, how := range computed {
		if name == "o0" {
			if len(how) != 1 {
				t.Errorf("the page statement's plan computes o0 as %v, want once", how)
			}
			continue
		}
		if !slices.Equal(how, []string{"MATERIALIZE"}) {
			t.Errorf("the page statement's plan computes %s as %v, want MATERIALIZE once", name, how)
		}
	}
}

// A run finds the topics of each statement without compiling the statement a
// second time: only the rule selects it reads are explained.
func TestARunExplainsNoWholeStatementForItsTopics(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	var explained []string
	saved := topicsRead
	topicsRead = func(ctx context.Context, app core.App, query string, params dbx.Params) ([]string, error) {
		explained = append(explained, query)
		return saved(ctx, app, query, params)
	}
	t.Cleanup(func() { topicsRead = saved })
	seen := observeReads(t)
	res, err := runLedger(z, map[string]string{"period": ledgerPeriod}, Options{Today: "2025-07-15"})
	if err != nil {
		t.Fatal(err)
	}
	if len(explained) == 0 {
		t.Fatal("no topics were read")
	}
	for _, q := range explained {
		for _, st := range *seen {
			if strings.Contains(q, "__level") || q == st.sql {
				t.Fatalf("the topics read explained a whole statement:\n%.300s", q)
			}
		}
	}
	want := []string{"zoo_ledger_access", "zoo_ledger_accounts", "zoo_ledger_entries", "zoo_ledger_lines"}
	for i, w := range want {
		want[i] = changesignal.Topic(w)
	}
	slices.Sort(want)
	if !slices.Equal(res.Topics, want) {
		t.Fatalf("topics = %v, want %v", res.Topics, want)
	}
}
