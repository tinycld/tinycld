package reports

import (
	"encoding/json"
	"math"
	"os"
	"strconv"
	"testing"
)

// formatCasesPath is the case list core/reports/lib/__tests__/format-cases.test.ts
// runs against lib/format.ts. The same file here keeps export_format.go and the
// client from drifting: a cell, a label, a heading, the period, the zone line,
// a param's value and the empty text read the same in a file as on screen.
const formatCasesPath = "../../reports/lib/fixtures/format-cases.json"

type cellCase struct {
	Name      string
	Column    ResultColumn
	Row       Row
	Value     any
	NonFinite string
	Display   exportDisplay
	TimeZone  string
	Hour12    bool
	Text      string
	IsRed     bool
	IsError   bool
}

type periodCase struct {
	Name     string
	Def      *ParamDef
	Resolved *ResolvedParam
	// Text is null where the client's function answers null; the Go text is "".
	Text *string
}

// emptyCase is a periodCase with the spec's own empty text; nil for none.
type emptyCase struct {
	periodCase
	EmptyText *string
}

type formatCases struct {
	Cells  []cellCase
	Labels []struct {
		Name    string
		Row     Row
		Display exportDisplay
		Text    string
	}
	Headings []struct {
		Name    string
		Column  ResultColumn
		Display exportDisplay
		Text    string
	}
	Periods []periodCase
	Zones   []struct {
		Name     string
		Columns  []ResultColumn
		TimeZone string
		Text     *string
	}
	Params []struct {
		Name     string
		Def      ParamDef
		Resolved ResolvedParam
		Labels   map[string]string
		Text     string
	}
	Empty     []emptyCase
	FileNames []struct {
		Name   string
		Title  string
		Defs   []ParamDef
		Params []ResolvedParam
		Today  string
		Ext    string
		Text   string
	}
}

func loadFormatCases(t *testing.T) formatCases {
	t.Helper()
	raw, err := os.ReadFile(formatCasesPath)
	if err != nil {
		t.Fatalf("read %s: %v", formatCasesPath, err)
	}
	var cases formatCases
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("decode %s: %v", formatCasesPath, err)
	}
	if len(cases.Cells) == 0 || len(cases.FileNames) == 0 {
		t.Fatalf("%s has no cells or no file names", formatCasesPath)
	}
	return cases
}

func orEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// engineForm is a value as a run holds it: JSON decodes every number as a
// float64, the engine gives a whole figure (cents, a count, a key) as int64.
// Each case runs in both forms.
func engineForm(v any) any {
	switch x := v.(type) {
	case float64:
		if x == math.Trunc(x) && math.Abs(x) <= 1<<53 {
			return int64(x)
		}
		return x
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, item := range x {
			out[k] = engineForm(item)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = engineForm(item)
		}
		return out
	}
	return v
}

func engineRow(r Row) Row {
	if r.Cells != nil {
		r.Cells = engineForm(r.Cells).(map[string]any)
	}
	r.LabelValue = engineForm(r.LabelValue)
	return r
}

func engineColumn(c ResultColumn) ResultColumn {
	if c.Pivot != nil {
		p := *c.Pivot
		p.Key, p.LabelValue = engineForm(p.Key), engineForm(p.LabelValue)
		c.Pivot = &p
	}
	if c.Heading != nil {
		h := *c.Heading
		h.Value = engineForm(h.Value)
		c.Heading = &h
	}
	return c
}

func engineResolved(r ResolvedParam) ResolvedParam {
	r.Value = engineForm(r.Value)
	return r
}

func (c cellCase) value(t *testing.T) any {
	if c.NonFinite == "" {
		return c.Value
	}
	f, err := strconv.ParseFloat(c.NonFinite, 64)
	if err != nil {
		t.Fatalf("%s: nonFinite %q: %v", c.Name, c.NonFinite, err)
	}
	return f
}

func TestExportFormatterCellsMatchTheClient(t *testing.T) {
	for _, c := range loadFormatCases(t).Cells {
		t.Run(c.Name, func(t *testing.T) {
			f := newExportFormatter(c.Display, c.TimeZone, c.Hour12)
			for form, in := range map[string]struct {
				col   ResultColumn
				row   Row
				value any
			}{
				"json":   {c.Column, c.Row, c.value(t)},
				"engine": {engineColumn(c.Column), engineRow(c.Row), engineForm(c.value(t))},
			} {
				got := f.CellText(in.col, in.row, in.value)
				if got.Text != c.Text || got.IsRed != c.IsRed || got.IsError != c.IsError {
					t.Fatalf("%s form: CellText = {%q red:%v error:%v}, want {%q red:%v error:%v}",
						form, got.Text, got.IsRed, got.IsError, c.Text, c.IsRed, c.IsError)
				}
			}
		})
	}
}

func TestExportFormatterRowLabelsMatchTheClient(t *testing.T) {
	for _, c := range loadFormatCases(t).Labels {
		t.Run(c.Name, func(t *testing.T) {
			f := newExportFormatter(c.Display, "UTC", false)
			for form, row := range map[string]Row{"json": c.Row, "engine": engineRow(c.Row)} {
				if got := f.RowLabel(row); got != c.Text {
					t.Fatalf("%s form: RowLabel = %q, want %q", form, got, c.Text)
				}
			}
		})
	}
}

func TestExportFormatterHeadingsMatchTheClient(t *testing.T) {
	for _, c := range loadFormatCases(t).Headings {
		t.Run(c.Name, func(t *testing.T) {
			f := newExportFormatter(c.Display, "UTC", false)
			for form, col := range map[string]ResultColumn{"json": c.Column, "engine": engineColumn(c.Column)} {
				if got := f.Heading(col); got != c.Text {
					t.Fatalf("%s form: Heading = %q, want %q", form, got, c.Text)
				}
			}
		})
	}
}

func TestExportFormatterPeriodWordsMatchTheClient(t *testing.T) {
	for _, c := range loadFormatCases(t).Periods {
		t.Run(c.Name, func(t *testing.T) {
			f := newExportFormatter(exportDisplay{}, "UTC", false)
			if got := f.PeriodWords(c.Def, c.Resolved); got != orEmpty(c.Text) {
				t.Fatalf("PeriodWords = %q, want %q", got, orEmpty(c.Text))
			}
		})
	}
}

func TestExportFormatterZoneLinesMatchTheClient(t *testing.T) {
	for _, c := range loadFormatCases(t).Zones {
		t.Run(c.Name, func(t *testing.T) {
			f := newExportFormatter(exportDisplay{}, c.TimeZone, false)
			if got := f.ZoneLine(c.Columns); got != orEmpty(c.Text) {
				t.Fatalf("ZoneLine = %q, want %q", got, orEmpty(c.Text))
			}
		})
	}
}

// The empty cases run through exportData.emptyText, the path the PDF and the
// XLSX print, so the spec's own text (the client's emptyMessage third
// parameter) and the engine's words read the same in a file as on screen.
func TestExportEmptyTextMatchesTheClient(t *testing.T) {
	for _, c := range loadFormatCases(t).Empty {
		t.Run(c.Name, func(t *testing.T) {
			d := &exportData{
				EmptyText: orEmpty(c.EmptyText),
				Format:    newExportFormatter(exportDisplay{}, "UTC", false),
			}
			if c.Def != nil {
				d.Defs = []ParamDef{*c.Def}
			}
			if c.Resolved != nil {
				d.Params = []ResolvedParam{*c.Resolved}
			}
			if got := d.emptyText(); got != orEmpty(c.Text) {
				t.Fatalf("emptyText = %q, want %q", got, orEmpty(c.Text))
			}
		})
	}
}

// A params case holds one param and the client's value text alone; the
// params line names it with its label.
func TestExportFormatterParamsLineMatchesTheClient(t *testing.T) {
	for _, c := range loadFormatCases(t).Params {
		t.Run(c.Name, func(t *testing.T) {
			f := newExportFormatter(exportDisplay{}, "UTC", false)
			want := c.Def.Label + ": " + c.Text
			labels := map[string]map[string]string{c.Def.ID: c.Labels}
			for form, resolved := range map[string]ResolvedParam{"json": c.Resolved, "engine": engineResolved(c.Resolved)} {
				if got := f.ParamsLine([]ParamDef{c.Def}, []ResolvedParam{resolved}, labels); got != want {
					t.Fatalf("%s form: ParamsLine = %q, want %q", form, got, want)
				}
			}
		})
	}
}

// The params line names every param but the period, in spec order, joined
// with " · "; the period is in the header's period line, not here.
func TestExportFormatterParamsLineJoinsEveryParamButThePeriod(t *testing.T) {
	defs := []ParamDef{
		{ID: "region", Type: ParamChoice, Label: "Region", Options: []Option{{Value: "north", Label: "North"}, {Value: "south", Label: "South"}}},
		{ID: "period", Type: "dateRange", Label: "Period"},
		{ID: "grams", Type: "number", Label: "Grams"},
	}
	resolved := []ResolvedParam{
		{ID: "region", Type: ParamChoice, Value: "north"},
		{ID: "period", Type: "dateRange", From: "2026-01-01", To: "2026-03-31"},
		{ID: "grams", Type: "number", Value: int64(42)},
	}
	f := newExportFormatter(exportDisplay{}, "UTC", false)
	if got, want := f.ParamsLine(defs, resolved, nil), "Region: North · Grams: 42"; got != want {
		t.Fatalf("ParamsLine = %q, want %q", got, want)
	}
}

// The CSV's values are raw: no symbol, no grouping, no number format option,
// a day as YYYY-MM-DD, a date-time on the reader's 24-hour wall clock. RawCell
// never adds the CSV's apostrophe (the CSV writer does, D15).
func TestExportFormatterRawCells(t *testing.T) {
	every := exportDisplay{DivideBy1000: true, WithoutCents: true, Negative: "parens", NegativeRed: true}
	f := newExportFormatter(every, "America/Chicago", true)
	usd := ResultColumn{ID: "amt", Label: "Amount", Type: Money, Currency: "USD"}
	detail := Row{ID: "d:1", Kind: KindDetail, Cells: map[string]any{}}
	total := Row{ID: "total", Kind: KindTotal, Cells: map[string]any{}}
	for _, tc := range []struct {
		name  string
		col   ResultColumn
		row   Row
		value any
		text  string
		kind  string
	}{
		{"money with every number format option on", usd, detail, int64(-123450), "-1234.50", "money"},
		{"money on a total row has no symbol", usd, total, int64(-123450), "-1234.50", "money"},
		{"money decoded from JSON", usd, detail, float64(123450), "1234.50", "money"},
		{"a large amount has no grouping", usd, detail, int64(123456789012), "1234567890.12", "money"},
		{"a percent is a number", ResultColumn{ID: "p", Label: "Share", Type: Percent}, detail, 12.5, "12.5", "percent"},
		{"a number has no grouping", ResultColumn{ID: "n", Label: "Grams", Type: Number}, detail, int64(1234567), "1234567", "number"},
		{"a date", ResultColumn{ID: "d", Label: "Fed", Type: Date}, detail, "2026-11-15", "2026-11-15", "date"},
		{"a date stored as an instant", ResultColumn{ID: "d", Label: "Fed", Type: Date}, detail, "2026-11-15 00:00:00.000Z", "2026-11-15", "date"},
		{"a date-time in the reader's zone", ResultColumn{ID: "at", Label: "At", Type: DateTime}, detail, "2026-11-14 22:07:00.000Z", "2026-11-14 16:07", "datetime"},
		{"a bool true", ResultColumn{ID: "b", Label: "Active", Type: Bool}, detail, true, "TRUE", "bool"},
		{"a bool false", ResultColumn{ID: "b", Label: "Active", Type: Bool}, detail, false, "FALSE", "bool"},
		{"a month bucket in words", ResultColumn{ID: "m", Label: "Month", Type: Month}, detail, "2026-01", "Jan 2026", "text"},
		{"a formula text comes back unchanged", ResultColumn{ID: "t", Label: "Note", Type: Text}, detail, "=SUM(A1)", "=SUM(A1)", "text"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := f.RawCell(tc.col, tc.row, tc.value)
			if got.Text != tc.text || got.Kind != tc.kind {
				t.Fatalf("RawCell = {%q %q}, want {%q %q}", got.Text, got.Kind, tc.text, tc.kind)
			}
		})
	}
}
