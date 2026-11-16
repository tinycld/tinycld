package reports

import (
	"context"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nathanstitt/omnidoc/pkg/xlsx"
)

// The Excel file's layout: the header block in rows 1 to 4, an empty row,
// the column headings in row 6 (frozen), the report's rows from row 7.
const (
	xlsxHeadingRow  = 6
	xlsxFirstRow    = 7
	xlsxMaxWidth    = 60
	xlsxMinWidth    = 8
	xlsxSheetRunes  = 31
	xlsxMoneyFormat = "#,##0.00"
	xlsxPctFormat   = "0.0%"
	// xlsxIndent is the text before a row's label per depth level: omnidoc's
	// AlignmentPatch has no indent.
	xlsxIndent = "  "
)

// xlsxForbidden are the characters Excel refuses in a sheet name.
const xlsxForbidden = `/\?*[]:`

// xlsxColumn is one column of the sheet: a report column, or the
// "<heading> currency" column after a mixed-currency money column.
type xlsxColumn struct {
	Col      ResultColumn
	Currency bool
	// longest is the rune count of the column's longest text, its heading
	// included.
	longest int
}

// writeXLSX writes the report as an Excel file of one sheet named after the
// title: the header block (heading, title, period in words, params line), an
// empty row, the column headings (frozen), then one row per report row with
// every group open. Cells are typed (RawCell's kind): money and numbers are
// numbers with a two-place format, money in units; a percent is a fraction
// with a percent format; a date and a date-time (the reader's wall clock) are
// dates; a bool is a bool; any other value is a string, never a formula.
// Group, subtotal, computed and total rows are bold; a subtotal row has a
// thin rule above it and a total row a medium one.
func writeXLSX(ctx context.Context, d *exportData) ([]byte, error) {
	f := xlsx.New()
	se, err := f.Sheet(f.SheetNames()[0])
	if err != nil {
		return nil, err
	}
	if err := se.SetName(xlsxSheetName(d.Title)); err != nil {
		return nil, err
	}
	if err := writeXLSXHeader(se, d); err != nil {
		return nil, err
	}
	columns := xlsxColumns(d)
	for i := range columns {
		text := d.Format.Heading(columns[i].Col)
		if columns[i].Currency {
			text += " currency"
		}
		columns[i].longest = utf8.RuneCountInString(text)
		if err := se.SetString(xlsxHeadingRow, i+1, text); err != nil {
			return nil, err
		}
		if err := se.PatchCellStyle(xlsxHeadingRow, i+1, xlsx.StylePatch{Font: &xlsx.FontPatch{Bold: ptr(true)}}); err != nil {
			return nil, err
		}
	}
	se.SetFrozen(xlsxHeadingRow, 0)

	if len(d.Rows) == 0 {
		if err := se.SetString(xlsxFirstRow, 1, d.emptyText()); err != nil {
			return nil, err
		}
	}
	for i, r := range d.Rows {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := writeXLSXRow(se, d.Format, columns, xlsxFirstRow+i, r); err != nil {
			return nil, err
		}
	}
	for i, c := range columns {
		se.SetColWidth(i+1, float64(min(xlsxMaxWidth, max(xlsxMinWidth, c.longest+2))))
	}
	return f.Save(ctx)
}

// xlsxSheetName is the title with every character Excel refuses in a sheet
// name removed (and an apostrophe at either end, which Excel also refuses),
// cut to 31 characters; "Report" when nothing is left or the name is
// "History" in any case, which Excel keeps for itself.
func xlsxSheetName(title string) string {
	name := strings.Map(func(r rune) rune {
		if strings.ContainsRune(xlsxForbidden, r) || r < ' ' {
			return -1
		}
		return r
	}, title)
	name = strings.Trim(strings.TrimSpace(name), "'")
	if utf8.RuneCountInString(name) > xlsxSheetRunes {
		name = string([]rune(name)[:xlsxSheetRunes])
	}
	name = strings.Trim(strings.TrimSpace(name), "'")
	if name == "" || strings.EqualFold(name, "History") {
		return "Report"
	}
	return name
}

// writeXLSXHeader writes rows 1 to 4: the heading and the title (bold), the
// period in words and the params line. A line with no text leaves its row
// empty.
func writeXLSXHeader(se *xlsx.SheetEdit, d *exportData) error {
	lines := []string{d.headingText(), d.Title, d.periodWords(), d.paramsLine()}
	for i, text := range lines {
		if text == "" {
			continue
		}
		if err := se.SetString(i+1, 1, text); err != nil {
			return err
		}
		if i < 2 {
			if err := se.PatchCellStyle(i+1, 1, xlsx.StylePatch{Font: &xlsx.FontPatch{Bold: ptr(true)}}); err != nil {
				return err
			}
		}
	}
	return nil
}

// xlsxColumns are the sheet's columns: the shown columns, each
// mixed-currency money column followed by its currency column.
func xlsxColumns(d *exportData) []xlsxColumn {
	out := make([]xlsxColumn, 0, len(d.Columns)+1)
	for i, col := range d.Columns {
		out = append(out, xlsxColumn{Col: col})
		if i > 0 && mixedCurrencyColumn(col, d.Rows) {
			out = append(out, xlsxColumn{Col: col, Currency: true})
		}
	}
	return out
}

// writeXLSXRow writes one report row at the 1-based row n: its label, each
// cell typed, then the row's weight and rule on every cell.
func writeXLSXRow(se *xlsx.SheetEdit, f *exportFormatter, columns []xlsxColumn, n int, r exportRow) error {
	label := strings.Repeat(xlsxIndent, max(r.Row.Depth, 0)) + f.RowLabel(r.Row)
	if label != "" {
		if err := se.SetString(n, 1, label); err != nil {
			return err
		}
	}
	columns[0].longest = max(columns[0].longest, utf8.RuneCountInString(label))
	formats := make([]string, len(columns))
	for i := 1; i < len(columns); i++ {
		c := &columns[i]
		text, format, err := writeXLSXCell(se, f, *c, n, i+1, r)
		if err != nil {
			return err
		}
		c.longest = max(c.longest, utf8.RuneCountInString(text))
		formats[i] = format
	}
	bold := isStrongRow(r.Row.Kind)
	border := xlsxRuleOf(r.Row.Kind)
	if bold {
		if err := se.PatchRowStyle(n, xlsx.StylePatch{Font: &xlsx.FontPatch{Bold: ptr(true)}}); err != nil {
			return err
		}
	}
	for i := range columns {
		patch := xlsx.StylePatch{}
		if bold {
			patch.Font = &xlsx.FontPatch{Bold: ptr(true)}
		}
		if border != "" {
			patch.Border = &xlsx.BorderPatch{Top: &xlsx.EdgePatch{Style: ptr(border)}}
		}
		if formats[i] != "" {
			patch.NumFmt = ptr(formats[i])
		}
		if err := se.PatchCellStyle(n, i+1, patch); err != nil {
			return err
		}
	}
	return nil
}

// isStrongRow says a row is set in a heavier weight: a group, subtotal,
// computed or total row; detail and opening rows are not.
func isStrongRow(kind string) bool {
	return kind == KindGroup || kind == KindSubtotal || kind == KindComputed || kind == KindTotal
}

// xlsxRuleOf is the rule above a row: thin over a subtotal, medium over a
// total, none over any other row.
func xlsxRuleOf(kind string) string {
	switch kind {
	case KindSubtotal:
		return "thin"
	case KindTotal:
		return "medium"
	}
	return ""
}

// writeXLSXCell writes one cell of a row at (n, col) by its raw kind, and
// returns its text (for the column's width) and the number format it needs.
// A cell the row does not have stays empty.
func writeXLSXCell(se *xlsx.SheetEdit, f *exportFormatter, c xlsxColumn, n, col int, r exportRow) (string, string, error) {
	value, ok := r.Cells[c.Col.ID]
	if !ok || value == nil {
		return "", "", nil
	}
	if c.Currency {
		code := cellCurrency(c.Col, r.Row)
		if code == "" {
			return "", "", nil
		}
		return code, "", se.SetString(n, col, code)
	}
	raw := f.RawCell(c.Col, r.Row, value)
	switch raw.Kind {
	case rawMoney, rawNumber, rawPercent:
		number, err := strconv.ParseFloat(raw.Text, 64)
		if err != nil {
			break
		}
		format := xlsxMoneyFormat
		if raw.Kind == rawPercent {
			number, format = number/100, xlsxPctFormat
		}
		return raw.Text, format, se.SetNumber(n, col, number)
	case rawDate:
		day, err := time.Parse(time.DateOnly, raw.Text)
		if err != nil {
			break
		}
		return raw.Text, "", se.SetDate(n, col, day)
	case rawDateTime:
		at, err := time.Parse("2006-01-02 15:04", raw.Text)
		if err != nil {
			break
		}
		return raw.Text, "", se.SetDate(n, col, at)
	case rawBool:
		return raw.Text, "", se.SetBool(n, col, raw.Text == "TRUE")
	}
	if raw.Text == "" {
		return "", "", nil
	}
	return raw.Text, "", se.SetString(n, col, raw.Text)
}

// ptr is a pointer to v, for omnidoc's style patches.
func ptr[T any](v T) *T { return &v }
