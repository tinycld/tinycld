package reports

import (
	"fmt"
	"net/http"
	"slices"
	"testing"
)

// The export's cap is lowered with exportLimits (keepExportLimits puts it
// back), so a few hundred bulk feedings pass it.

// speciesExport is feedingsBySpecies saved for ann with n bulk feedings of
// leo: every group open, the report is lion's group row, its 2+n feedings
// and its subtotal, zebra's group row, its 2 feedings and its subtotal, and
// the Total, 9+n rows.
func speciesExport(t *testing.T, n int) (*savedZoo, string) {
	t.Helper()
	s := newSavedZoo(t)
	rep := s.saved(t, "bySpecies", "ann", "Feedings by Species", feedingsBySpecies, false)
	bulkFeedings(t, s.zoo, n)
	keepExportLimits(t)
	return s, rep.Id
}

// detailStatements counts the grouped detail statements among seen.
func detailStatements(seen []readStatement) int {
	n := 0
	for _, st := range seen {
		if isGroupedDetailStatement(st.sql) {
			n++
		}
	}
	return n
}

// A CSV export past the cap is refused from the group statement's counts:
// its group rows, subtotals and Total plus the feedings its groups hold are
// past the cap, so the detail statement, the largest of the run, never runs.
// The refusal is the one the export gives after reading every row.
func TestAnExportPastTheCapRefusesBeforeTheDetailStatement(t *testing.T) {
	s, rep := speciesExport(t, 200)
	exportLimits.sheetRows = 208
	seen := observeReads(t)
	res := s.exportAs(t, "ann", csvExportBody(rep))
	observeRead = nil
	requireStatus(t, res, http.StatusUnprocessableEntity, fmt.Sprintf(`"message":%q`, exportSheetRows))
	if n := detailStatements(*seen); n != 0 {
		t.Fatalf("the refused export ran %d detail statements, want 0: the group statement's counts are past the cap", n)
	}
	if n := len(exportAuditRows(t, s.app)); n != 0 {
		t.Fatalf("a refused export wrote %d audit rows", n)
	}
}

// At the cap the report exports whole, every row in order; one row more is
// refused with the same sentence and no audit row.
func TestAnExportAtTheCapStillExports(t *testing.T) {
	s, rep := speciesExport(t, 200)
	exportLimits.sheetRows = 209
	whole := s.exportAs(t, "ann", csvExportBody(rep))
	requireStatus(t, whole, http.StatusOK)
	records := csvRecords(t, whole.Body.Bytes())
	notes := make([]string, 0, len(records))
	for _, r := range records[1:] {
		notes = append(notes, r[1])
	}
	want := []string{"", "f1", "f2"}
	for i := 1; i <= 200; i++ {
		want = append(want, fmt.Sprintf("n%05d", i))
	}
	want = append(want, "", "", "f3", "f4", "", "")
	if !slices.Equal(notes, want) {
		t.Fatalf("%d rows at the cap, want %d\nfirst %q\nlast %q", len(notes), len(want), notes[:min(len(notes), 5)], notes[max(0, len(notes)-6):])
	}
	if labels := firstFields(records); labels[0] != "lion" || labels[203] != "Total lion" || labels[204] != "zebra" || labels[208] != "Total" {
		t.Fatalf("labels %q … %q", labels[:2], labels[200:])
	}

	exportLimits.sheetRows = 208
	requireStatus(t, s.exportAs(t, "ann", csvExportBody(rep)), http.StatusUnprocessableEntity, fmt.Sprintf(`"message":%q`, exportSheetRows))
	if n := len(exportAuditRows(t, s.app)); n != 1 {
		t.Fatalf("%d audit rows, want 1: the whole export writes one, the refused one none", n)
	}
}

// The rows a group statement returns are not all rows of the file, and not
// every group's base rows are detail rows of it. Each report below is
// exactly at its cap, so a count from the group statement that counted
// such a row would refuse it:
//   - an inlined group (inline) returns a level row that draws no group row
//     and no subtotal;
//   - a skip level's blank key (blank "skip") returns a level row that draws
//     no group row and no subtotal;
//   - a group above the innermost level holds the base rows its child groups
//     hold too (__n at each level);
//   - a PDF keeps the reader's folds: a closed group's base rows are no rows
//     of the file.
//
// A having and a hideEmpty hide their groups in the group statement (s<L>),
// so a hidden group returns no level row and no __n; a filter cuts w, which
// both the group statement's __n and the detail statement read, so neither
// over-counts.
func TestAnExportAtTheCapIsNeverRefusedFromTheGroupCounts(t *testing.T) {
	t.Run("an inlined group", func(t *testing.T) {
		s, rep := inlineSavedZoo(t)
		keepExportLimits(t)
		exportLimits.sheetRows = len(inlineExportLabels)
		res := s.exportAs(t, "ann", csvExportBody(rep.Id))
		requireStatus(t, res, http.StatusOK)
		if got := firstFields(csvRecords(t, res.Body.Bytes())); !slices.Equal(got, inlineExportLabels) {
			t.Fatalf("labels %q\nwant %q", got, inlineExportLabels)
		}
	})
	t.Run("a skip level's blank key", func(t *testing.T) {
		s := newSavedZoo(t)
		s.rec(t, "k5", "zoo_keepers", map[string]any{"user": s.id("ann"), "animal": s.id("kit")})
		spec := fmt.Sprintf(zooRagged, "")
		rep := s.saved(t, "ragged", "ann", "Feedings Ragged", spec, false)
		open := s.run(t, "ann", spec, Options{ExpandAll: true}, nil)
		keepExportLimits(t)
		exportLimits.sheetRows = len(open.Rows)
		res := s.exportAs(t, "ann", csvExportBody(rep.Id))
		requireStatus(t, res, http.StatusOK)
		if got, want := len(csvRecords(t, res.Body.Bytes()))-1, len(open.Rows); got != want {
			t.Fatalf("%d rows in the file, want %d", got, want)
		}
	})
	// A PDF reads the report as it was left (closed), so it guards the early
	// stop's writer.ExpandAll condition rather than the __n count.
	t.Run("a closed PDF", func(t *testing.T) {
		s := newSavedZoo(t)
		hermeticExportFonts(t)
		rep := s.saved(t, "byNote", "ann", "Feedings by Note", byNoteClosed, false)
		bulkFeedings(t, s.zoo, 46)
		keepExportLimits(t)
		// 50 closed note groups and the Total; their 50 feedings are not rows.
		exportLimits.pdfRows = 51
		body := fmt.Sprintf(`{"reportId":%q,"params":{},"options":{"today":"2026-11-16","timeZone":"UTC"},"format":"pdf","hour12":false}`, rep.Id)
		requireFile(t, s.exportAs(t, "ann", body), "application/pdf", ".pdf")
		exportLimits.pdfRows = 50
		requireStatus(t, s.exportAs(t, "ann", body), http.StatusUnprocessableEntity, fmt.Sprintf(`"message":%q`, exportPDFRows))
	})
}
