package reports

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nathanstitt/omnidoc/pkg/xlsx"
)

// feedingsSheet groups the reader's feedings by species with subtotals,
// over a period, with a second param the header names.
const feedingsSheet = `{"version":4,"source":"zoo.feedings","params":[
	{"id":"period","type":"dateRange","label":"Period"},
	{"id":"rows","type":"choice","label":"Rows","default":"all","options":[{"value":"all","label":"All"},{"value":"some","label":"Some"}]}],
	"columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"note","path":[{"field":"note"}]},
	{"id":"fed","path":[{"field":"fed"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"filter":{"op":"between","args":[{"path":[{"field":"fed"}]},{"param":"period"}]},
	"groups":[{"id":"bySpecies","col":"species","subtotals":true}],
	"sort":[{"col":"note"}]}`

// sheetOf reads a written Excel file back: it must hold exactly one sheet.
func sheetOf(t *testing.T, file []byte) xlsx.Sheet {
	t.Helper()
	wb, err := xlsx.OpenBytes(context.Background(), file)
	if err != nil {
		t.Fatalf("read the Excel file: %v", err)
	}
	if len(wb.Sheets) != 1 {
		t.Fatalf("%d sheets, want 1", len(wb.Sheets))
	}
	return wb.Sheets[0]
}

// sheetCell is the cell at a 1-based row and column; a cell past the used
// range is the zero (empty) cell.
func sheetCell(sheet xlsx.Sheet, row, col int) xlsx.Cell {
	if row < 1 || row > len(sheet.Cells) || col < 1 || col > len(sheet.Cells[row-1]) {
		return xlsx.Cell{Style: &xlsx.Style{}}
	}
	return sheet.Cells[row-1][col-1]
}

// sheetString is the string value of a cell, failing when it is not a
// string cell.
func sheetString(t *testing.T, sheet xlsx.Sheet, row, col int) string {
	t.Helper()
	c := sheetCell(sheet, row, col)
	if c.Value.Kind != xlsx.KindString {
		t.Fatalf("cell %s is kind %d (%q), want a string", xlsx.CellRef(row, col), c.Value.Kind, c.Text)
	}
	return c.Value.S
}

// rowIsBlank says no cell of the 1-based row holds a value.
func rowIsBlank(sheet xlsx.Sheet, row int) bool {
	if row > len(sheet.Cells) {
		return true
	}
	for _, c := range sheet.Cells[row-1] {
		if c.Value.Kind != xlsx.KindEmpty {
			return false
		}
	}
	return true
}

// headingColumn is the 1-based column whose heading in row 6 is heading.
func headingColumn(t *testing.T, sheet xlsx.Sheet, heading string) int {
	t.Helper()
	if len(sheet.Cells) < 6 {
		t.Fatalf("the sheet has %d rows, no heading row 6", len(sheet.Cells))
	}
	for i, c := range sheet.Cells[5] {
		if c.Value.Kind == xlsx.KindString && c.Value.S == heading {
			return i + 1
		}
	}
	t.Fatalf("row 6 has no heading %q", heading)
	return 0
}

// The grouped zoo report as an Excel file: one sheet; the header block, an
// empty row and the headings; the heading row frozen; every group open
// although the reader folded one; money as a number in units with a
// two-place format whatever the number format options; a date as a date.
func TestXLSXHoldsTheWholeGroupedReport(t *testing.T) {
	s := newSavedZoo(t)
	s.app.Settings().Meta.AppName = "Zoo Trust"
	s.rec(t, "refund", "zoo_feedings", map[string]any{"animal": s.id("leo"), "cost": -123450, "currency": "USD", "fed": "2026-03-15 00:00:00.000Z", "note": "refund"})
	rep := s.saved(t, "sheet", "ann", "Feedings by Species", feedingsSheet, false)
	file := buildExportAs(t, s, "ann", rep.Id, "xlsx",
		`{"period":{"from":"2026-01-01","to":"2026-03-31"},"rows":"all"}`,
		`{"collapse":[["lion","USD"]],"display":{"divideBy1000":true,"withoutCents":true,"negative":"parens","negativeRed":true},"today":"2026-11-16","timeZone":"UTC"}`)
	if file.Mime != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" || file.Format != "xlsx" || !strings.HasSuffix(file.Name, ".xlsx") {
		t.Fatalf("file = {Mime:%q Format:%q Name:%q}", file.Mime, file.Format, file.Name)
	}
	sheet := sheetOf(t, file.Bytes)

	for row, want := range map[int]string{
		1: "Zoo Trust",
		2: "Feedings by Species",
		3: "January 1 – March 31, 2026",
		4: "Rows: All",
	} {
		if got := sheetString(t, sheet, row, 1); got != want {
			t.Fatalf("row %d = %q, want %q", row, got, want)
		}
	}
	if !rowIsBlank(sheet, 5) {
		t.Fatalf("row 5 is not empty: %+v", sheet.Cells[4])
	}
	for i, want := range []string{"Species", "Note", "Fed", "Cost"} {
		if got := sheetString(t, sheet, 6, i+1); got != want {
			t.Fatalf("heading %d = %q, want %q", i+1, got, want)
		}
	}

	edit, err := xlsx.Edit(context.Background(), file.Bytes)
	if err != nil {
		t.Fatalf("edit the Excel file: %v", err)
	}
	se, err := edit.Sheet(edit.SheetNames()[0])
	if err != nil {
		t.Fatal(err)
	}
	if rows, cols := se.Frozen(); rows != 6 || cols != 0 {
		t.Fatalf("Frozen() = %d rows, %d columns; want 6, 0", rows, cols)
	}

	noteCol := headingColumn(t, sheet, "Note")
	var notes []string
	for row := 7; row <= len(sheet.Cells); row++ {
		if c := sheetCell(sheet, row, noteCol); c.Value.Kind == xlsx.KindString {
			notes = append(notes, c.Value.S)
		}
	}
	if want := []string{"f1", "f2", "refund", "f3", "f4"}; !slices.Equal(notes, want) {
		t.Fatalf("detail notes = %q, want %q (every group open, the folded lion too)", notes, want)
	}

	costCol, fedCol := headingColumn(t, sheet, "Cost"), headingColumn(t, sheet, "Fed")
	refund, f1 := 0, 0
	for row := 7; row <= len(sheet.Cells); row++ {
		switch sheetCell(sheet, row, noteCol).Value.S {
		case "refund":
			refund = row
		case "f1":
			f1 = row
		}
	}
	money := sheetCell(sheet, refund, costCol)
	if money.Value.Kind != xlsx.KindNumber || money.Value.F != -1234.5 {
		t.Fatalf("the refund's cost is kind %d value %v (%q), want the number -1234.5", money.Value.Kind, money.Value.F, money.Text)
	}
	if money.Style.NumFmt != "#,##0.00" {
		t.Fatalf("the refund's cost has format %q, want #,##0.00", money.Style.NumFmt)
	}
	day := sheetCell(sheet, f1, fedCol)
	if day.Value.Kind != xlsx.KindDate || !day.Value.T.Equal(time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("f1's fed is kind %d at %v (%q), want the date 2026-01-05", day.Value.Kind, day.Value.T, day.Text)
	}
}

// excelForbidden are the characters Excel refuses in a sheet name.
const excelForbidden = `/\?*[]:`

// typedExport is a report of every cell type, written by hand: the reader is
// in Chicago, and the number format options are all on.
func typedExport() *exportData {
	display := exportDisplay{DivideBy1000: true, WithoutCents: true, Negative: "parens", NegativeRed: true, Hidden: []string{}}
	defs := []ParamDef{
		{ID: "period", Type: ParamDateRange, Label: "Period"},
		{ID: "rows", Type: ParamChoice, Label: "Rows", Options: []Option{{Value: "all", Label: "All"}}},
	}
	long := strings.Repeat("Long memo ", 7)
	rows := []Row{
		{ID: "g", Kind: KindGroup, Depth: 0, Label: "Income", Expanded: true, Cells: map[string]any{}},
		{ID: "fees", Kind: KindDetail, Depth: 1, Label: "Fees", Cells: map[string]any{
			"amount": int64(-123450), "cur": "USD", "share": 12.5, "day": "2026-03-04",
			"at": "2026-03-04 22:07:00.000Z", "ok": true, "month": "2026-03", "memo": "=SUM(A1)",
		}},
		{ID: "deep", Kind: KindDetail, Depth: 2, Label: "Deep", Cells: map[string]any{"amount": int64(1000), "cur": "EUR", "memo": long}},
		{ID: "refund", Kind: KindDetail, Depth: 0, Label: "-Refund", Cells: map[string]any{"amount": int64(-100), "cur": "USD"}},
		{ID: "sub", Kind: KindSubtotal, Depth: 0, Label: "Total Income", Section: "g", Cells: map[string]any{"amount": int64(5000), "cur": "USD"}},
		{ID: "net", Kind: KindComputed, Depth: 0, Label: "Net margin per animal over the year", Cells: map[string]any{"amount": int64(100), "cur": "USD"}},
		{ID: "total", Kind: KindTotal, Depth: 0, Label: "Total", Cells: map[string]any{"amount": int64(5000), "cur": "USD"}},
	}
	out := make([]exportRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, exportRow{Row: r, Cells: r.Cells})
	}
	return &exportData{
		Title:    "Feeding Costs",
		ReportID: "typedreport000",
		Heading:  "Zoo Holdings",
		Columns: []ResultColumn{
			{ID: "name", Label: "Name", Type: Text},
			{ID: "amount", Label: "Amount", Type: Money, CurrencyCell: "cur"},
			{ID: "share", Label: "Share", Type: Percent},
			{ID: "day", Label: "Day", Type: Date},
			{ID: "at", Label: "At", Type: DateTime},
			{ID: "ok", Label: "Ok", Type: Bool},
			{ID: "month", Label: "Month", Type: Month},
			{ID: "memo", Label: "Memo", Type: Text},
		},
		Rows: out,
		Params: []ResolvedParam{
			{ID: "period", Type: ParamDateRange, From: "2026-01-01", To: "2026-03-31"},
			{ID: "rows", Type: ParamChoice, Value: "all"},
		},
		Defs:        defs,
		Notices:     []Notice{},
		ParamLabels: map[string]map[string]string{},
		Reader:      exportReader{Zone: "America/Chicago", Hour12: false, Today: "2026-11-16"},
		Display:     display,
		Format:      newExportFormatter(display, "America/Chicago", false),
		Made:        time.Date(2026, 11, 14, 22, 7, 0, 0, time.UTC),
	}
}

func writeTypedXLSX(t *testing.T, d *exportData) xlsx.Sheet {
	t.Helper()
	file, err := writeXLSX(context.Background(), d)
	if err != nil {
		t.Fatalf("writeXLSX: %v", err)
	}
	return sheetOf(t, file)
}

// One sheet, named after the title cut to 31 characters with every
// character Excel refuses removed.
func TestXLSXNamesItsSheetAfterTheTitle(t *testing.T) {
	d := typedExport()
	d.Title = "a/b\\c?d*e[f]g:" + strings.Repeat("x", 26)
	if n := len([]rune(d.Title)); n != 40 {
		t.Fatalf("the title has %d characters, want 40", n)
	}
	sheet := writeTypedXLSX(t, d)
	if got, want := sheet.Name, "abcdefg"+strings.Repeat("x", 24); got != want {
		t.Fatalf("sheet name = %q, want %q", got, want)
	}
	if n := len([]rune(sheet.Name)); n > 31 || strings.ContainsAny(sheet.Name, excelForbidden) {
		t.Fatalf("sheet name %q has %d characters or one Excel refuses", sheet.Name, n)
	}
}

// Excel reserves the sheet name "History" in any case, so a report of that
// title names its sheet "Report".
func TestXLSXDoesNotNameItsSheetHistory(t *testing.T) {
	for _, title := range []string{"History", "HISTORY", "history", "History:"} {
		d := typedExport()
		d.Title = title
		if got := writeTypedXLSX(t, d).Name; got != "Report" {
			t.Fatalf("the title %q gives the sheet name %q, want %q", title, got, "Report")
		}
	}
}

// Each cell is typed so the spreadsheet can calculate, and no text is ever a
// formula.
func TestXLSXTypesEveryCell(t *testing.T) {
	sheet := writeTypedXLSX(t, typedExport())
	for row, want := range map[int]string{1: "Zoo Holdings", 2: "Feeding Costs", 3: "January 1 – March 31, 2026", 4: "Rows: All"} {
		if got := sheetString(t, sheet, row, 1); got != want {
			t.Fatalf("row %d = %q, want %q", row, got, want)
		}
	}
	const fees = 8
	cell := func(heading string) xlsx.Cell { return sheetCell(sheet, fees, headingColumn(t, sheet, heading)) }

	if c := cell("Amount"); c.Value.Kind != xlsx.KindNumber || c.Value.F != -1234.5 || c.Style.NumFmt != "#,##0.00" {
		t.Fatalf("amount: kind %d value %v format %q, want the number -1234.5 with #,##0.00", c.Value.Kind, c.Value.F, c.Style.NumFmt)
	}
	if c := cell("Share"); c.Value.Kind != xlsx.KindNumber || c.Value.F != 0.125 || c.Style.NumFmt != "0.0%" {
		t.Fatalf("share: kind %d value %v format %q, want the number 0.125 with 0.0%%", c.Value.Kind, c.Value.F, c.Style.NumFmt)
	}
	if c := cell("Day"); c.Value.Kind != xlsx.KindDate || !c.Value.T.Equal(time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("day: kind %d at %v, want the date 2026-03-04", c.Value.Kind, c.Value.T)
	}
	// 22:07 UTC on March 4 is 16:07 on a Chicago wall clock.
	if c := cell("At"); c.Value.Kind != xlsx.KindDate || !c.Value.T.Round(time.Second).Equal(time.Date(2026, 3, 4, 16, 7, 0, 0, time.UTC)) {
		t.Fatalf("at: kind %d at %v, want the date-time 2026-03-04 16:07 (reader's wall clock)", c.Value.Kind, c.Value.T)
	}
	if c := cell("Ok"); c.Value.Kind != xlsx.KindBool || !c.Value.B {
		t.Fatalf("ok: kind %d (%q), want the bool true", c.Value.Kind, c.Text)
	}
	if c := cell("Month"); c.Value.Kind != xlsx.KindString || c.Value.S != "Mar 2026" {
		t.Fatalf("month: kind %d %q, want the string Mar 2026", c.Value.Kind, c.Value.S)
	}
	if c := cell("Memo"); c.Value.Kind != xlsx.KindString || c.Value.S != "=SUM(A1)" || c.Formula != "" {
		t.Fatalf("memo: kind %d %q formula %q, want the string =SUM(A1) with no formula and no apostrophe", c.Value.Kind, c.Value.S, c.Formula)
	}
	if got := sheetString(t, sheet, 10, 1); got != "-Refund" {
		t.Fatalf("the refund's label = %q, want -Refund", got)
	}
	if got := sheetString(t, sheet, 9, 1); got != "    Deep" {
		t.Fatalf("the depth-2 label = %q, want four spaces then Deep", got)
	}
}

// Group, subtotal, computed and total rows are bold and detail rows are
// not; a subtotal row has a thin rule above it, a total a medium one.
func TestXLSXStylesRowsByKind(t *testing.T) {
	sheet := writeTypedXLSX(t, typedExport())
	width := len(sheet.Cells[5])
	for row, bold := range map[int]bool{7: true, 8: false, 9: false, 10: false, 11: true, 12: true, 13: true} {
		for col := 1; col <= width; col++ {
			c := sheetCell(sheet, row, col)
			if c.Value.Kind == xlsx.KindEmpty {
				continue
			}
			if c.Style.Font.Bold != bold {
				t.Fatalf("cell %s (%q) bold = %v, want %v", xlsx.CellRef(row, col), c.Text, c.Style.Font.Bold, bold)
			}
		}
	}
	for row, edge := range map[int]string{11: "thin", 13: "medium", 8: ""} {
		for col := 1; col <= width; col++ {
			if got := sheetCell(sheet, row, col).Style.Border.Top.Style; got != edge {
				t.Fatalf("cell %s top border = %q, want %q", xlsx.CellRef(row, col), got, edge)
			}
		}
	}
}

// A mixed-currency money column is followed by its currency column, which
// names each row's code.
func TestXLSXNamesTheCurrencyOfAMixedColumn(t *testing.T) {
	sheet := writeTypedXLSX(t, typedExport())
	amount, currency := headingColumn(t, sheet, "Amount"), headingColumn(t, sheet, "Amount currency")
	if currency != amount+1 {
		t.Fatalf("Amount currency is column %d, want %d (right after Amount)", currency, amount+1)
	}
	if got := sheetString(t, sheet, 8, currency); got != "USD" {
		t.Fatalf("Fees' currency = %q, want USD", got)
	}
	if got := sheetString(t, sheet, 9, currency); got != "EUR" {
		t.Fatalf("Deep's currency = %q, want EUR", got)
	}
}

// Column widths fit the headings and values, up to 60.
func TestXLSXFitsColumnWidths(t *testing.T) {
	sheet := writeTypedXLSX(t, typedExport())
	width := func(heading string) float64 {
		w, ok := sheet.ColWidths[headingColumn(t, sheet, heading)-1]
		if !ok {
			t.Fatalf("column %s has no width", heading)
		}
		return w
	}
	if got := width("Memo"); got != 60 {
		t.Fatalf("the long-text column is %v wide, want 60", got)
	}
	ok, name := width("Ok"), width("Name")
	if ok >= name || name >= 60 {
		t.Fatalf("Ok is %v wide and Name %v: want a short column narrower than a longer one, both under 60", ok, name)
	}
}

// D8: an empty report has the header block, the heading row, and the empty
// text in the first cell under it.
func TestXLSXOfAnEmptyReport(t *testing.T) {
	s := newSavedZoo(t)
	s.app.Settings().Meta.AppName = "Zoo Trust"
	rep := s.saved(t, "notes", "ann", "Feeding Notes", `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},{"id":"fed","path":[{"field":"fed"}]}]}`, true)
	file := buildExportAs(t, s, "ot", rep.Id, "xlsx", `{}`, `{"today":"2026-11-16"}`)
	sheet := sheetOf(t, file.Bytes)
	if got := sheetString(t, sheet, 1, 1); got != "Zoo Trust" {
		t.Fatalf("row 1 = %q, want the organization's name", got)
	}
	if got := sheetString(t, sheet, 2, 1); got != "Feeding Notes" {
		t.Fatalf("row 2 = %q, want the title", got)
	}
	if a, b := sheetString(t, sheet, 6, 1), sheetString(t, sheet, 6, 2); a != "Note" || b != "Fed" {
		t.Fatalf("row 6 = %q, %q; want Note, Fed", a, b)
	}
	if got := sheetString(t, sheet, 7, 1); got != "No rows" {
		t.Fatalf("row 7 = %q, want the empty text", got)
	}
	if len(sheet.Cells) > 7 {
		t.Fatalf("the sheet has %d rows, want 7", len(sheet.Cells))
	}
}

// An empty report with its own empty text prints that text in place of
// "No rows".
func TestXLSXOfAnEmptyReportPrintsItsOwnEmptyText(t *testing.T) {
	s := newSavedZoo(t)
	rep := s.saved(t, "notes", "ann", "Feeding Notes", `{"version":4,"source":"zoo.feedings","emptyText":"No feedings to show","columns":[
		{"id":"note","path":[{"field":"note"}]},{"id":"fed","path":[{"field":"fed"}]}]}`, true)
	file := buildExportAs(t, s, "ot", rep.Id, "xlsx", `{}`, `{"today":"2026-11-16"}`)
	sheet := sheetOf(t, file.Bytes)
	if a, b := sheetString(t, sheet, 6, 1), sheetString(t, sheet, 6, 2); a != "Note" || b != "Fed" {
		t.Fatalf("row 6 = %q, %q; want Note, Fed", a, b)
	}
	if got := sheetString(t, sheet, 7, 1); got != "No feedings to show" {
		t.Fatalf("row 7 = %q, want the report's own empty text", got)
	}
}
