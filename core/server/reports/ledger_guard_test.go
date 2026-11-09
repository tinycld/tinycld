package reports

import (
	"context"
	"regexp"
	"slices"
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
	kinds := 0
	for _, st := range *seen {
		kind := "detail"
		if strings.Contains(st.sql, "__level") {
			kind = "group"
		}
		kinds++
		// Two lookups, on two levels (the total and the accounts) or in two
		// selects (the shown accounts and the lines).
		if n := strings.Count(st.sql, "(SELECT sum("); n > 4 {
			t.Errorf("the %s statement holds %d sum lookups, want at most 4 (2 lookups, once per level)", kind, n)
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
		// currency), all computed once per account.
		if byLine > 9 {
			t.Errorf("the %s statement reads a group record by %d subqueries on a line, want at most 9", kind, byLine)
		}
	}
	if kinds != 2 {
		t.Fatalf("%d statements, want the group and the detail statement", kinds)
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
