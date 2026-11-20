package reports

import (
	"slices"
	"strings"
	"testing"
)

// The statement by account type with a third section, Equity. Its one account,
// 3900, has a stored month of June (credit 700) and no line of July, so no
// base row gives the report an Equity row: the account names its own section.

const ownParentsBefore = "Balance before July"

func ownParentsSpec(having string) string {
	return strings.Replace(rowsRollupLayoutSpec(having), `{"row":"owed"`, `{"section":"equity","title":"Equity","key":"equity"},{"row":"owed"`, 1)
}

func ownParentsZoo(t *testing.T, currency string) *zoo {
	t.Helper()
	z := rowsRollupZoo(t)
	z.rec(t, "reJune", "zoo_ledger_months", map[string]any{
		"account": z.id("re"), "first_day": "2026-06-01", "debit_total": 0, "credit_total": 700, "currency": currency,
	})
	return z
}

// ownParentsNoLine moves the period to August, where no line passes.
func ownParentsNoLine(spec string) string {
	return strings.Replace(spec, `"2026-07-01"`, `"2026-08-01"`, 1)
}

// ownParentsGroups drops the detail and opening rows.
func ownParentsGroups(res *Result) []rowsRollupLine {
	var out []rowsRollupLine
	for _, line := range rowsRollupLines(res) {
		if line.Kind != KindDetail && line.Kind != KindOpening {
			out = append(out, line)
		}
	}
	return out
}

// The lines of the Assets and Liabilities sections, as in
// TestARowsRollupTotalsUnderLayoutSections.
func ownParentsUpper() []rowsRollupLine {
	return []rowsRollupLine{
		{KindGroup, "Assets", int64(300), int64(6500), int64(-6200), "USD"},
		{KindGroup, "1000", nil, int64(6500), int64(-6500), "USD"},
		{KindOpening, ownParentsBefore, int64(500), nil, nil, "USD"},
		{KindDetail, "1000", nil, int64(10000), int64(-10000), "USD"},
		{KindDetail, "1000", int64(3000), nil, int64(3000), "USD"},
		{KindGroup, "1300", int64(300), nil, int64(300), "USD"},
		{KindOpening, ownParentsBefore, int64(300), nil, nil, "USD"},
		{KindSubtotal, "Total Assets", int64(300), int64(6500), int64(-6200), "USD"},
		{KindGroup, "Liabilities", int64(0), int64(4500), int64(-4500), "USD"},
		{KindGroup, "2000", nil, int64(4500), int64(-4500), "USD"},
		{KindOpening, ownParentsBefore, nil, nil, nil, "USD"},
		{KindDetail, "2000", nil, int64(4500), int64(-4500), "USD"},
		{KindSubtotal, "Total Liabilities", int64(0), int64(4500), int64(-4500), "USD"},
	}
}

func ownParentsEquity() []rowsRollupLine {
	return []rowsRollupLine{
		{KindGroup, "Equity", int64(0), int64(700), int64(-700), "USD"},
		{KindGroup, "3900", nil, int64(700), int64(-700), "USD"},
		{KindOpening, ownParentsBefore, nil, int64(700), nil, "USD"},
		{KindSubtotal, "Total Equity", int64(0), int64(700), int64(-700), "USD"},
	}
}

// 3900 shows under Equity, a section no line of July reaches, and the section
// and its subtotal add it. The computed row reads Assets and Liabilities only.
func TestAFillRecordShowsUnderItsOwnParentWithNoBaseRow(t *testing.T) {
	z := ownParentsZoo(t, "USD")
	res := z.run(t, "ann", ownParentsSpec(rowsRollupHaving), Options{ExpandAll: true}, nil)
	want := append(ownParentsUpper(), ownParentsEquity()...)
	want = append(want, rowsRollupLine{KindComputed, "Assets and liabilities", int64(300), int64(11000), int64(-10700), "USD"})
	requireRowsRollupLines(t, rowsRollupLines(res), want)
}

// 3900's month is in EUR and the report's rows are in USD: the Equity section
// would show a EUR figure under the USD code, so the account row, the section
// and its subtotal are blank with no code, and the run names the columns.
func TestAFillRecordOfAnotherCurrencyBlanksItsOwnParent(t *testing.T) {
	z := ownParentsZoo(t, "EUR")
	res := z.run(t, "ann", ownParentsSpec(""), Options{ExpandAll: true}, nil)
	got := ownParentsGroups(res)
	i := slices.IndexFunc(got, func(l rowsRollupLine) bool { return l.Label == "Equity" })
	if i < 0 {
		t.Fatalf("no Equity row in %#v", got)
	}
	requireRowsRollupLines(t, got[i:], []rowsRollupLine{
		{KindGroup, "Equity", nil, nil, nil, nil},
		{KindGroup, "3900", nil, nil, nil, nil},
		{KindSubtotal, "Total Equity", nil, nil, nil, nil},
		{KindComputed, "Assets and liabilities", nil, nil, nil, nil},
	})
	for _, col := range []string{"debit", "credit", "gap"} {
		if !slices.ContainsFunc(res.Notices, func(n Notice) bool {
			return n.Code == "mixedCurrency" && slices.Contains(n.Columns, col)
		}) {
			t.Fatalf("notices = %+v, want mixedCurrency naming %s", res.Notices, col)
		}
	}
}

// A closed section reads the same as an open one: it adds its fill records
// (1300 in Assets, 3900 in Equity) as it adds its base rows, and shows no row
// under it.
func TestAClosedSectionAddsItsFillRecords(t *testing.T) {
	z := ownParentsZoo(t, "USD")
	spec := strings.Replace(ownParentsSpec(rowsRollupHaving), `"open":true,`, ``, 1)
	res := z.run(t, "ann", spec, Options{}, nil)
	requireRowsRollupLines(t, rowsRollupLines(res), []rowsRollupLine{
		{KindGroup, "Assets", int64(300), int64(6500), int64(-6200), "USD"},
		{KindSubtotal, "Total Assets", int64(300), int64(6500), int64(-6200), "USD"},
		{KindGroup, "Liabilities", int64(0), int64(4500), int64(-4500), "USD"},
		{KindSubtotal, "Total Liabilities", int64(0), int64(4500), int64(-4500), "USD"},
		{KindGroup, "Equity", int64(0), int64(700), int64(-700), "USD"},
		{KindSubtotal, "Total Equity", int64(0), int64(700), int64(-700), "USD"},
		{KindComputed, "Assets and liabilities", int64(300), int64(11000), int64(-10700), "USD"},
	})
}

// With no line in the period every account row is a fill row. A source with a
// currency gives the records their parents in it: each section adds its
// accounts (2000 has net 0 and is hidden). A source with no currency gives
// them no parent: a section would add months of USD, EUR and GBP as one figure
// with no code and no notice, so the sections stay empty, as before.
func TestFillRecordsNameTheirParentsWhenNoBaseRowPasses(t *testing.T) {
	t.Run("the source has a currency", func(t *testing.T) {
		z := ownParentsZoo(t, "USD")
		withLedgerCurrency(t, "USD")
		res := z.run(t, "ann", ownParentsNoLine(ownParentsSpec(rowsRollupHaving)), Options{ExpandAll: true}, nil)
		want := []rowsRollupLine{
			{KindGroup, "Assets", int64(800), int64(0), int64(800), "USD"},
			{KindGroup, "1000", int64(500), nil, int64(500), "USD"},
			{KindOpening, ownParentsBefore, int64(500), nil, nil, "USD"},
			{KindGroup, "1300", int64(300), nil, int64(300), "USD"},
			{KindOpening, ownParentsBefore, int64(300), nil, nil, "USD"},
			{KindSubtotal, "Total Assets", int64(800), int64(0), int64(800), "USD"},
			{KindGroup, "Liabilities", int64(0), int64(0), int64(0), "USD"},
			{KindSubtotal, "Total Liabilities", int64(0), int64(0), int64(0), "USD"},
		}
		want = append(want, ownParentsEquity()...)
		want = append(want, rowsRollupLine{KindComputed, "Assets and liabilities", int64(800), int64(0), int64(800), "USD"})
		requireRowsRollupLines(t, rowsRollupLines(res), want)
	})
	t.Run("the source has no currency", func(t *testing.T) {
		z := ownParentsZoo(t, "USD")
		res := z.run(t, "ann", ownParentsNoLine(ownParentsSpec("")), Options{ExpandAll: true}, nil)
		requireRowsRollupLines(t, rowsRollupLines(res), []rowsRollupLine{
			{KindGroup, "Assets", int64(0), int64(0), int64(0), nil},
			{KindSubtotal, "Total Assets", int64(0), int64(0), int64(0), nil},
			{KindGroup, "Liabilities", int64(0), int64(0), int64(0), nil},
			{KindSubtotal, "Total Liabilities", int64(0), int64(0), int64(0), nil},
			{KindGroup, "Equity", int64(0), int64(0), int64(0), nil},
			{KindSubtotal, "Total Equity", int64(0), int64(0), int64(0), nil},
			{KindComputed, "Assets and liabilities", int64(0), int64(0), int64(0), nil},
		})
		if len(res.Notices) != 0 {
			t.Fatalf("notices = %+v, want none", res.Notices)
		}
	})
}

// Paged, the rows of the pages are the rows of the whole run in order, less
// the computed row each page repeats: the Equity section and 3900 are rows of
// the last page, not rows a page loses.
func TestFillRecordsUnderTheirOwnParentsPage(t *testing.T) {
	z := ownParentsZoo(t, "USD")
	owed := rowsRollupLine{KindComputed, "Assets and liabilities", int64(300), int64(11000), int64(-10700), "USD"}
	opts := Options{Limit: 1, ExpandAll: true}
	var merged []rowsRollupLine
	pages := 0
	for {
		res := z.run(t, "ann", ownParentsSpec(rowsRollupHaving), opts, nil)
		lines := rowsRollupLines(res)
		if len(lines) == 0 || lines[len(lines)-1] != owed {
			t.Fatalf("page %d ends in %#v, want the computed row", pages+1, lines)
		}
		merged = append(merged, lines[:len(lines)-1]...)
		pages++
		if res.Page == nil || res.Page.Next == "" {
			break
		}
		if pages == 20 {
			t.Fatal("the pages do not end")
		}
		opts.Cursor = res.Page.Next
	}
	requireRowsRollupLines(t, merged, append(ownParentsUpper(), ownParentsEquity()...))
	if pages < 2 {
		t.Fatalf("pages = %d, want more than one", pages)
	}
}

// A skip level between the section and the account: an asset account is
// "Current", any other has a blank class. 3900's own class is blank, so it
// sits directly under Equity, as 2000 sits directly under Liabilities.
func TestAFillRecordSkipsItsOwnBlankParentLevel(t *testing.T) {
	z := ownParentsZoo(t, "USD")
	spec := ownParentsSpec(rowsRollupHaving)
	spec = strings.Replace(spec, `{"id":"day"`, `{"id":"cls","formula":{"op":"if","args":[{"op":"eq","args":[{"col":"kind"},{"value":"asset"}]},{"value":"Current"},{"value":""}]}},{"id":"day"`, 1)
	spec = strings.Replace(spec, `{"id":"g","col":"account"`, `{"id":"c","col":"cls","blank":"skip","open":true},{"id":"g","col":"account"`, 1)
	res := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
	requireRowsRollupLines(t, ownParentsGroups(res), []rowsRollupLine{
		{KindGroup, "Assets", int64(300), int64(6500), int64(-6200), "USD"},
		{KindGroup, "Current", int64(300), int64(6500), int64(-6200), "USD"},
		{KindGroup, "1000", nil, int64(6500), int64(-6500), "USD"},
		{KindGroup, "1300", int64(300), nil, int64(300), "USD"},
		{KindSubtotal, "Total Assets", int64(300), int64(6500), int64(-6200), "USD"},
		{KindGroup, "Liabilities", int64(0), int64(4500), int64(-4500), "USD"},
		{KindGroup, "2000", nil, int64(4500), int64(-4500), "USD"},
		{KindSubtotal, "Total Liabilities", int64(0), int64(4500), int64(-4500), "USD"},
		{KindGroup, "Equity", int64(0), int64(700), int64(-700), "USD"},
		{KindGroup, "3900", nil, int64(700), int64(-700), "USD"},
		{KindSubtotal, "Total Equity", int64(0), int64(700), int64(-700), "USD"},
		{KindComputed, "Assets and liabilities", int64(300), int64(11000), int64(-10700), "USD"},
	})
}

// A having on the site group, above the filled animals. Each animal names its
// own site: East (Eli, no feeding) and "(none)" (Kit, no site) have no base
// row. Every group is open, so the feedings show, each labeled with its site;
// the statement of the feedings, which compiles only the levels down to the
// having, still reads the fill records, with a "rows" column (the site's
// having reads the animals' sums) and without one. A having that keeps every
// site shows East and "(none)"; one that keeps the sites with a cost hides
// them and their animals.
func TestAFillRecordNamesItsParentUnderAHavingAbove(t *testing.T) {
	spec := func(total, having string) string {
		return `{"version":4,"source":"zoo.feedings","columns":[
			{"id":"site","path":[{"link":"animal"},{"link":"site"}]},
			{"id":"animal","path":[{"link":"animal"}]},
			{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"` + total + `}],
			"groups":[
				{"id":"s","col":"site","having":{"op":"` + having + `","args":[{"col":"cost"},{"value":0}]}},
				{"id":"a","col":"animal","fill":{}}],
			"sort":[{"formula":{"record":"s","path":[{"field":"name"}]}},{"formula":{"record":"a","path":[{"field":"name"}]}},{"col":"cost"}]}`
	}
	every := []any{"East", "Eli", "North", "Leo", "North", "North", "Lia", "South", "Zed", "South", "South", "(none)", "Kit", "Total"}
	everyCost := []any{int64(0), int64(0), int64(3500), int64(3500), int64(1000), int64(2500), int64(0),
		int64(1000), int64(1000), int64(300), int64(700), int64(0), int64(0), int64(4500)}
	withCost := []any{"North", "Leo", "North", "North", "Lia", "South", "Zed", "South", "South", "Total"}
	withCostCost := []any{int64(3500), int64(3500), int64(1000), int64(2500), int64(0), int64(1000), int64(1000), int64(300), int64(700), int64(4500)}
	for _, tc := range []struct {
		name, total, having string
		labels, cost        []any
	}{
		{"no rows column, every site", "", "gte", every, everyCost},
		{"no rows column, the sites with a cost", "", "gt", withCost, withCostCost},
		{"a rows column, every site", `,"total":"rows"`, "gte", every, everyCost},
		{"a rows column, the sites with a cost", `,"total":"rows"`, "gt", withCost, withCostCost},
	} {
		t.Run(tc.name, func(t *testing.T) {
			z := newZoo(t)
			withEmptyAnimals(t, z)
			res, err := z.try(t, "ann", spec(tc.total, tc.having), Options{ExpandAll: true}, nil)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			requireRows(t, res, "label", tc.labels...)
			requireRows(t, res, "cost", tc.cost...)
		})
	}
}
