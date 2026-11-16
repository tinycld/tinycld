package reports

import (
	"bytes"
	"context"
	"encoding/csv"
	"strings"
)

// csvBOM is the UTF-8 byte order mark a spreadsheet reads the encoding from.
const csvBOM = "\xef\xbb\xbf"

// writeCSV writes the report as a CSV file: UTF-8 with a byte order mark,
// comma separated, CRLF lines, quoted by RFC 4180, a lone carriage return in
// a field kept as a CRLF line break. The first line holds the
// headings: the label column's, then each shown column's, a mixed-currency
// money column followed by "<heading> currency". Then one line per row, in
// report order: the row's label, then each cell's raw value (RawCell), a text
// cell guarded against formulas (csvGuard), and the row's currency code after
// a mixed-currency money cell. No header lines, notices or page text.
func writeCSV(_ context.Context, d *exportData) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(csvBOM)
	w := csv.NewWriter(&buf)
	w.UseCRLF = true
	f := d.Format
	mixed := make([]bool, len(d.Columns))
	heading := make([]string, 0, len(d.Columns)+1)
	for i, col := range d.Columns {
		mixed[i] = i > 0 && mixedCurrencyColumn(col, d.Rows)
		heading = append(heading, f.Heading(col))
		if mixed[i] {
			heading = append(heading, f.Heading(col)+" currency")
		}
	}
	if err := w.Write(csvLineBreaks(heading)); err != nil {
		return nil, err
	}
	for _, r := range d.Rows {
		line := make([]string, 0, len(heading))
		line = append(line, csvGuard(f.RowLabel(r.Row)))
		for i, col := range d.Columns[1:] {
			line = append(line, csvCell(f, col, r))
			if mixed[i+1] {
				line = append(line, csvCurrency(col, r))
			}
		}
		if err := w.Write(csvLineBreaks(line)); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// csvCell is one cell of a row: its raw value, a text guarded; empty for a
// cell the row does not have.
func csvCell(f *exportFormatter, col ResultColumn, r exportRow) string {
	value, ok := r.Cells[col.ID]
	if !ok {
		return ""
	}
	raw := f.RawCell(col, r.Row, value)
	if raw.Kind == rawText {
		return csvGuard(raw.Text)
	}
	return raw.Text
}

// csvCurrency is the currency code of a mixed-currency money cell; empty when
// the row shows no amount in the column.
func csvCurrency(col ResultColumn, r exportRow) string {
	if value, ok := r.Cells[col.ID]; !ok || value == nil {
		return ""
	}
	return cellCurrency(col, r.Row)
}

// mixedCurrencyColumn says a money column's rows carry different
// currencies: the run marked it, or its currency cell holds more than one
// code over the rows.
func mixedCurrencyColumn(col ResultColumn, rows []exportRow) bool {
	if col.Type != Money && col.TotalType != Money {
		return false
	}
	if col.MixedCurrency {
		return true
	}
	if col.Currency != "" || col.CurrencyCell == "" {
		return false
	}
	seen := ""
	for _, r := range rows {
		code, _ := r.Row.Cells[col.CurrencyCell].(string)
		if code == "" {
			continue
		}
		if seen != "" && code != seen {
			return true
		}
		seen = code
	}
	return false
}

// csvLineBreaks turns each lone carriage return (one with no line feed after
// it) into CRLF. With UseCRLF, encoding/csv drops a lone \r from a field and
// writes \n as CRLF, so without this a text "a\rb" would reach the file as
// "ab".
func csvLineBreaks(fields []string) []string {
	for i, field := range fields {
		if !strings.Contains(field, "\r") {
			continue
		}
		var b strings.Builder
		for j := 0; j < len(field); j++ {
			b.WriteByte(field[j])
			if field[j] == '\r' && (j+1 == len(field) || field[j+1] != '\n') {
				b.WriteByte('\n')
			}
		}
		fields[i] = b.String()
	}
	return fields
}

// csvGuard puts an apostrophe before a text a spreadsheet would run as a
// formula: one that starts with =, +, -, @, a tab or a carriage return. Any
// other text is unchanged (D15: the CSV's rule only).
func csvGuard(text string) string {
	if text == "" {
		return text
	}
	switch text[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + text
	}
	return text
}
