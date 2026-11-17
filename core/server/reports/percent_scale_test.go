package reports

import (
	"context"
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nathanstitt/omnidoc/pkg/xlsx"
)

// clientShares groups the client zoo's lines (inline_test.go withClientZoo:
// Total 555, Acme 100) by client, with three percent columns: a share of the
// Total at 2 places, a share of the parent at 0 places, and a ratio over a
// div that is a percent by its format only, which carries no column scale.
var clientShares = `{"version":4,"source":"zoo.client_lines",
	"columns":[
		{"id":"name","label":"Client","path":[{"link":"client"}]},
		{"id":"balance","label":"Balance","formula":{"op":"sum","args":[` + netOf + `]}},
		{"id":"ofTotal","label":"Of total","formula":{"op":"percentOfTotal","args":[{"col":"balance"}],"scale":2}},
		{"id":"ofParent","label":"Of parent","formula":{"op":"percentOfParent","args":[{"col":"balance"}],"scale":0}},
		{"id":"ratio","label":"Ratio","format":"percent","formula":{"op":"div","args":[{"col":"balance"},{"value":5.55}],"scale":2}},
		{"id":"clientKey","label":"Client key","hidden":true,"path":[{"link":"client"}]}],
	"groups":[{"id":"byClient","col":"clientKey","label":` + nameOr("byClient") + `}],
	"sort":[{"formula":{"op":"lower","args":[{"record":"byClient","path":[{"field":"name"}]}]}}]}`

// shareBesidePivot is a month pivot of cost with a share of the Total beside
// it, at 2 places.
const shareBesidePivot = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
	{"id":"share","formula":{"op":"percentOfTotal","args":[{"col":"cost"}],"scale":2}}],
	"groups":[{"id":"g","col":"species"}],
	"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"cost"}}`

// scaleText is a result column's scale as a word: "nil" when it has none.
func scaleText(c ResultColumn) string {
	if c.Scale == nil {
		return "nil"
	}
	return strconv.Itoa(*c.Scale)
}

func resultColumn(t *testing.T, res *Result, id string) ResultColumn {
	t.Helper()
	for _, c := range res.Columns {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("the result has no column %q", id)
	return ResultColumn{}
}

func TestPercentColumnCarriesItsScale(t *testing.T) {
	z := newZoo(t)
	withClientZoo(t, z)
	res := z.run(t, "ann", clientShares, Options{}, nil)
	for id, want := range map[string]string{"ofTotal": "2", "ofParent": "0", "ratio": "nil", "balance": "nil", "name": "nil"} {
		if got := scaleText(resultColumn(t, res, id)); got != want {
			t.Fatalf("column %s scale = %s, want %s", id, got, want)
		}
	}

	// In JSON a scale of 0 is written; a column with no scale has no key.
	for id, want := range map[string]string{"ofTotal": `"scale":2`, "ofParent": `"scale":0`} {
		raw, err := json.Marshal(resultColumn(t, res, id))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), want) {
			t.Fatalf("column %s JSON = %s, want it to hold %s", id, raw, want)
		}
	}
	raw, err := json.Marshal(resultColumn(t, res, "ratio"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"scale"`) {
		t.Fatalf("the ratio column's JSON = %s, want no scale key", raw)
	}

	// A percent of a section carries its formula's scale too.
	section := z.run(t, "ann", percentStatement(shareOfLions, bySpecies, sortByNote, gapRow), Options{}, nil)
	if got := scaleText(resultColumn(t, section, "share")); got != "1" {
		t.Fatalf("the percent of a section's scale = %s, want 1", got)
	}

	// On a pivot report the percent beside the pivot keeps its scale; the
	// columns the pivot makes of a money cell carry none.
	pivot := z.run(t, "ann", shareBesidePivot, Options{}, nil)
	if got := scaleText(resultColumn(t, pivot, "share")); got != "2" {
		t.Fatalf("the share beside the pivot has scale %s, want 2", got)
	}
	pivots := 0
	for _, c := range pivot.Columns {
		if c.Pivot == nil {
			continue
		}
		pivots++
		if got := scaleText(c); got != "nil" {
			t.Fatalf("pivot column %s has scale %s, want none", c.ID, got)
		}
	}
	if pivots == 0 {
		t.Fatal("the pivot report has no pivot columns")
	}
}

// clientSharesExport is clientShares as an export writes it.
func clientSharesExport(t *testing.T) *exportData {
	t.Helper()
	z := newZoo(t)
	withClientZoo(t, z)
	res := z.run(t, "ann", clientShares, Options{}, nil)
	display := exportDisplay{Negative: "minus", Hidden: []string{}}
	return &exportData{
		Title:       "Client shares",
		ReportID:    "sharesreport00",
		Columns:     shownExportColumns(res.Columns, nil),
		Rows:        exportRows(res.Rows, loadSpec(t, clientShares)),
		Params:      res.Params,
		Notices:     []Notice{},
		ParamLabels: map[string]map[string]string{},
		Reader:      exportReader{Zone: "UTC", Hour12: false, Today: "2026-11-16"},
		Display:     display,
		Format:      newExportFormatter(display, "UTC", false),
		Made:        time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC),
	}
}

var tdText = regexp.MustCompile(`(?s)<td\b[^>]*>(.*?)</td>`)

// clientSharesCSV is the CSV of clientShares as core main 87ed80f9 wrote it,
// before result columns carried a scale: a CSV holds the plain numbers.
const clientSharesCSV = "\xef\xbb\xbfClient,Balance,Of total,Of parent,Ratio\r\n" +
	"Acme,100,18.02,18,18.02\r\n" +
	"Harbor,200,36.04,36,36.04\r\n" +
	"Harbor Bar,30,5.41,5,5.41\r\n" +
	"Harbor Kitchen,60,10.81,11,10.81\r\n" +
	"Mill,0,0,0,0\r\n" +
	"Zeta,150,27.03,27,27.03\r\n" +
	"No client,15,2.7,3,2.7\r\n" +
	"Total,555,100,100,100\r\n"

func TestPercentExportsPrintTheScale(t *testing.T) {
	hermeticExportFonts(t)
	d := clientSharesExport(t)

	// PDF: Acme's cells read at each column's places; the ratio, with no
	// column scale, at 1 place.
	var cells []string
	for _, m := range tdText.FindAllStringSubmatch(rowMarkup(t, pdfHTML(t, d), "Acme"), -1) {
		cells = append(cells, strings.TrimSpace(m[1]))
	}
	if want := []string{"Acme", "100", "18.02%", "18%", "18.0%"}; strings.Join(cells, "|") != strings.Join(want, "|") {
		t.Fatalf("Acme's PDF cells = %q, want %q", cells, want)
	}

	// Excel: the number format follows the places; the values stay fractions.
	file, err := writeXLSX(context.Background(), d)
	if err != nil {
		t.Fatalf("writeXLSX: %v", err)
	}
	sheet := sheetOf(t, file)
	acme, headings := 0, map[string]int{}
	for r, row := range sheet.Cells {
		for c, cell := range row {
			if cell.Value.Kind != xlsx.KindString {
				continue
			}
			switch cell.Value.S {
			case "Of total", "Of parent", "Ratio":
				headings[cell.Value.S] = c + 1
			case "Acme":
				if c == 0 {
					acme = r + 1
				}
			}
		}
	}
	if acme == 0 || len(headings) != 3 {
		t.Fatalf("the sheet has Acme at row %d and headings %v", acme, headings)
	}
	for heading, want := range map[string]struct {
		format string
		value  float64
	}{
		"Of total":  {"0.00%", 0.1802},
		"Of parent": {"0%", 0.18},
		"Ratio":     {"0.0%", 0.1802},
	} {
		c := sheetCell(sheet, acme, headings[heading])
		if c.Value.Kind != xlsx.KindNumber || math.Abs(c.Value.F-want.value) > 1e-9 || c.Style.NumFmt != want.format {
			t.Fatalf("%s: kind %d value %v format %q, want the number %v with %s", heading, c.Value.Kind, c.Value.F, c.Style.NumFmt, want.value, want.format)
		}
	}

	// CSV: the plain numbers, byte for byte as before the change.
	csv, err := writeCSV(context.Background(), d)
	if err != nil {
		t.Fatalf("writeCSV: %v", err)
	}
	if string(csv) != clientSharesCSV {
		t.Fatalf("CSV =\n%q\nwant\n%q", csv, clientSharesCSV)
	}
}

func TestFormatReportPercentPlaces(t *testing.T) {
	for _, tc := range []struct {
		value  float64
		places int
		want   string
	}{
		{12.345, 2, "12.35%"},
		{-0.005, 2, "-0.01%"},
		{100, 2, "100.00%"},
		{12.5, 0, "13%"},
		{-12.5, 0, "-13%"},
		{12.34, 1, "12.3%"},
		{math.NaN(), 2, ""},
		{math.Inf(1), 2, ""},
		{math.Inf(-1), 0, ""},
	} {
		if got := formatReportPercent(tc.value, tc.places); got != tc.want {
			t.Fatalf("formatReportPercent(%v, %d) = %q, want %q", tc.value, tc.places, got, tc.want)
		}
	}
}
