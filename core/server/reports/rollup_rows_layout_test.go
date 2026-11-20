package reports

import (
	"slices"
	"strings"
	"testing"
)

// A statement by account type: the balance list's columns under layout
// sections, one per type, and a computed row that adds two sections. Each
// account fills in under its own type, so an account with a stored month and
// no line of July has a row.
const (
	rowsRollupSections = `,"layout":[
		{"section":"assets","title":"Assets","key":"asset"},
		{"section":"liabilities","title":"Liabilities","key":"liability"},
		{"row":"owed","label":"Assets and liabilities","formula":{"op":"add","args":[{"subtotal":"assets"},{"subtotal":"liabilities"}]}}]}`
	rowsRollupUnderSections = `[{"id":"k","col":"kind","open":true,"subtotals":true},
		{"id":"g","col":"account","fill":{"where":{"op":"eq","args":[{"path":[{"field":"type"}]},{"col":"kind"}]}},
		"label":{"record":"g","path":[{"field":"number"}]}HAVING,
		"opening":{"label":"Balance before July","cells":{"debit":BEFORE_D,"credit":BEFORE_C}}}]`
)

func rowsRollupLayoutSpec(having string) string {
	spec := rowsRollupSpec(strings.ReplaceAll(rowsRollupUnderSections, "HAVING", having))
	return strings.TrimSuffix(spec, "}") + rowsRollupSections
}

// Under a layout the account row shows the rollup, and its section, the
// section's subtotal and a computed row that reads the sections add the
// account rows shown: idle (a month, no line) counts in Assets. bank and petty
// are hidden by the having, as in the report with no layout.
func TestARowsRollupTotalsUnderLayoutSections(t *testing.T) {
	z := rowsRollupZoo(t)
	res := z.run(t, "ann", rowsRollupLayoutSpec(rowsRollupHaving), Options{ExpandAll: true}, nil)
	const before = "Balance before July"
	requireRowsRollupLines(t, rowsRollupLines(res), []rowsRollupLine{
		{KindGroup, "Assets", int64(300), int64(6500), int64(-6200), "USD"},
		{KindGroup, "1000", nil, int64(6500), int64(-6500), "USD"},
		{KindOpening, before, int64(500), nil, nil, "USD"},
		{KindDetail, "1000", nil, int64(10000), int64(-10000), "USD"},
		{KindDetail, "1000", int64(3000), nil, int64(3000), "USD"},
		{KindGroup, "1300", int64(300), nil, int64(300), "USD"},
		{KindOpening, before, int64(300), nil, nil, "USD"},
		{KindSubtotal, "Total Assets", int64(300), int64(6500), int64(-6200), "USD"},
		{KindGroup, "Liabilities", int64(0), int64(4500), int64(-4500), "USD"},
		{KindGroup, "2000", nil, int64(4500), int64(-4500), "USD"},
		{KindOpening, before, nil, nil, nil, "USD"},
		{KindDetail, "2000", nil, int64(4500), int64(-4500), "USD"},
		{KindSubtotal, "Total Liabilities", int64(0), int64(4500), int64(-4500), "USD"},
		{KindComputed, "Assets and liabilities", int64(300), int64(11000), int64(-10700), "USD"},
	})
}

// With no having bank (a month in EUR) and petty (GBP) show as blank rows
// under Assets. The section and its subtotal would add three currencies as
// one figure, so they are blank with no code, the computed row that reads the
// section is blank too, and the run names the columns. Liabilities, all in one
// currency, keeps its figures.
func TestARowsRollupSectionOverTwoCurrenciesIsBlank(t *testing.T) {
	z := rowsRollupZoo(t)
	res := z.run(t, "ann", rowsRollupLayoutSpec(""), Options{ExpandAll: true}, nil)
	var got []rowsRollupLine
	for _, line := range rowsRollupLines(res) {
		if line.Kind != KindDetail && line.Kind != KindOpening {
			got = append(got, line)
		}
	}
	requireRowsRollupLines(t, got, []rowsRollupLine{
		{KindGroup, "Assets", nil, nil, nil, nil},
		{KindGroup, "1000", nil, int64(6500), int64(-6500), "USD"},
		{KindGroup, "1100", nil, nil, nil, nil},
		{KindGroup, "1200", nil, nil, nil, nil},
		{KindGroup, "1300", int64(300), nil, int64(300), "USD"},
		{KindSubtotal, "Total Assets", nil, nil, nil, nil},
		{KindGroup, "Liabilities", int64(0), int64(4500), int64(-4500), "USD"},
		{KindGroup, "2000", nil, int64(4500), int64(-4500), "USD"},
		{KindSubtotal, "Total Liabilities", int64(0), int64(4500), int64(-4500), "USD"},
		{KindComputed, "Assets and liabilities", nil, nil, nil, nil},
	})
	for _, col := range []string{"debit", "credit"} {
		if !slices.ContainsFunc(res.Notices, func(n Notice) bool {
			return n.Code == "mixedCurrency" && slices.Contains(n.Columns, col)
		}) {
			t.Fatalf("notices = %+v, want mixedCurrency naming %s", res.Notices, col)
		}
	}
}
