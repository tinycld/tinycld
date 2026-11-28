package reports

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nathanstitt/omnidoc/pkg/xlsx"
)

// noTotalSpecies is feedingsBySpecies with noTotal: the report ends in no
// Total row (a sum over rows of different things means nothing).
const noTotalSpecies = `{"version":4,"source":"zoo.feedings","noTotal":true,"columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"bySpecies","col":"species","subtotals":true}],
	"sort":[{"col":"note"}]}`

// A grouped report with noTotal keeps its group, detail and subtotal rows and
// pushes no Total row.
func TestNoTotalGroupedReportHasNoTotalRow(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", noTotalSpecies, Options{}, nil)
	requireRows(t, res, "kind", "group", "group")
	requireRows(t, res, "label", "lion", "zebra")
	requireRows(t, res, "cost", int64(3500), int64(1000))

	open := z.run(t, "ann", noTotalSpecies, Options{Expand: [][]any{res.Rows[0].GroupKey}}, nil)
	requireRows(t, open, "kind", "group", "detail", "detail", "subtotal", "group")
	requireRows(t, open, "label", "lion", "lion", "lion", "Total lion", "zebra")
}

// A run of a noTotal report over no rows gives zero rows; the same report
// without the option ends in its Total row over no rows.
func TestNoTotalEmptyRunHasNoRows(t *testing.T) {
	z := newZoo(t)
	filter := `"filter":{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"none"}]},`
	res := z.run(t, "ann", strings.Replace(noTotalSpecies, `"columns"`, filter+`"columns"`, 1), Options{}, nil)
	if len(res.Rows) != 0 {
		t.Fatalf("rows = %+v, want none", res.Rows)
	}
	with := strings.Replace(noTotalSpecies, `"noTotal":true,`, filter, 1)
	requireRows(t, z.run(t, "ann", with, Options{}, nil), "kind", "total")
}

// Load keeps noTotal, and a spec without it saves no noTotal key, so the
// JSON (and the pinned sha) of a spec that does not set it does not change.
func TestLoadRoundTripsNoTotal(t *testing.T) {
	withZooRegistry(t)
	saved, err := json.Marshal(loadSpec(t, noTotalSpecies))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(saved), `"noTotal":true`) {
		t.Fatalf("the saved spec lacks noTotal: %s", saved)
	}
	plain, err := json.Marshal(loadSpec(t, feedingsBySpecies))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain), "noTotal") {
		t.Fatalf("a spec without noTotal saves one: %s", plain)
	}
}

// The exports of a noTotal report print no Total row; an empty one prints the
// empty text, as a report with no rows does.
func TestExportsOfANoTotalReport(t *testing.T) {
	s := newSavedZoo(t)
	hermeticExportFonts(t)
	rep := s.saved(t, "species", "ann", "Feedings by Species", noTotalSpecies, true)

	sheet := sheetOf(t, buildExportAs(t, s, "ann", rep.Id, "xlsx", `{}`, `{"today":"2026-11-16"}`).Bytes)
	for row := 1; row <= len(sheet.Cells); row++ {
		if c := sheetCell(sheet, row, 1); c.Value.Kind == xlsx.KindString && c.Value.S == "Total" {
			t.Fatalf("row %d of the Excel file is a Total row", row)
		}
	}
	text, _ := pdfTextOf(t, buildExportAs(t, s, "ann", rep.Id, "pdf", `{}`, `{"today":"2026-11-16"}`).Bytes)
	if !pdfWord("zebra").MatchString(text) {
		t.Fatalf("the PDF lacks the zebra group:\n%s", text)
	}
	// A subtotal row ("Total lion") is the one place the word may stand.
	subtotals := strings.Count(text, "Total lion") + strings.Count(text, "Total zebra")
	if strings.Count(text, "Total") != subtotals {
		t.Fatalf("the PDF prints a Total row:\n%s", text)
	}

	// ot keeps no animal, so the report has no row for ot.
	empty := buildExportAs(t, s, "ot", rep.Id, "xlsx", `{}`, `{"today":"2026-11-16"}`)
	if got := sheetString(t, sheetOf(t, empty.Bytes), 7, 1); got != "No rows" {
		t.Fatalf("row 7 = %q, want the empty text", got)
	}
	text, _ = pdfTextOf(t, buildExportAs(t, s, "ot", rep.Id, "pdf", `{}`, `{"today":"2026-11-16"}`).Bytes)
	if !strings.Contains(text, "No rows") {
		t.Fatalf("the empty PDF lacks the empty text:\n%s", text)
	}
}
