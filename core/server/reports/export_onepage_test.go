package reports

import (
	"bytes"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

// isGroupPageStatement is the level-row statement of a paged grouped run: its
// body reads the first level's rows beside their path (pagedGroupStatement).
func isGroupPageStatement(sql string) bool {
	return strings.Contains(sql, "FROM o1 o JOIN pp1 pp ON")
}

// isGroupedDetailStatement is the detail statement of a grouped run: it joins
// the open groups' order (leafOrder).
func isGroupedDetailStatement(sql string) bool {
	return strings.Contains(sql, "JOIN gord ON")
}

// byNoteRows is the first field of every line of byNoteClosed's CSV after the
// heading, over ann's four feedings and n bulk ones: a group row, its detail
// row and its subtotal for each note in order, then the Total.
func byNoteRows(n int) []string {
	notes := []string{"f1", "f2", "f3", "f4"}
	for i := 1; i <= n; i++ {
		notes = append(notes, fmt.Sprintf("n%05d", i))
	}
	out := make([]string, 0, 3*len(notes)+1)
	for _, note := range notes {
		out = append(out, note, note, "Total "+note)
	}
	return append(out, "Total")
}

// Each page of a grouped run computes every group level over every row, so an
// export in pages of maxPage rows costs pages x rows. The export reads its
// rows in one page: one statement of each kind, however many pages of maxPage
// rows the report holds (here 1,350 rows, 14 such pages).
func TestExportReadsAGroupedReportInOnePage(t *testing.T) {
	s := newSavedZoo(t)
	rep := s.saved(t, "byNote", "ann", "Feedings by Note", byNoteClosed, false)
	bulkFeedings(t, s.zoo, 446)
	withLimits(t, func(l *limits) { l.maxPage = 100 })

	seen := observeReads(t)
	file := s.buildCSV(t, "ann", rep.Id, `{"today":"2026-11-16"}`)
	pages, totals, details := 0, 0, 0
	for _, st := range *seen {
		switch {
		case isGroupPageStatement(st.sql):
			pages++
		case isTotalStatement(st.sql):
			totals++
		case isGroupedDetailStatement(st.sql):
			details++
		}
	}
	if pages != 1 || totals != 1 || details != 1 {
		t.Fatalf("%d group, %d Total and %d detail statements, want 1 of each", pages, totals, details)
	}
	if got, want := firstFields(csvRecords(t, file.Bytes)), byNoteRows(446); !slices.Equal(got, want) {
		t.Fatalf("%d rows, want %d: the file is not the report in order\nfirst %q\nlast %q", len(got), len(want), got[:min(len(got), 7)], got[max(0, len(got)-4):])
	}
	if file.Rows != 1351 {
		t.Fatalf("file.Rows = %d, want 1351", file.Rows)
	}
}

// The one page ends at the format's limit: a report of exactly the limit is
// written whole, and one row more is refused, never cut.
func TestExportOfAGroupedReportStopsAtTheFormatsLimit(t *testing.T) {
	s := newSavedZoo(t)
	rep := s.saved(t, "byNote", "ann", "Feedings by Note", byNoteClosed, false)
	bulkFeedings(t, s.zoo, 446)
	keepExportLimits(t)

	exportLimits.sheetRows = 1351
	whole := s.exportAs(t, "ann", csvExportBody(rep.Id))
	requireStatus(t, whole, http.StatusOK)
	if got, want := firstFields(csvRecords(t, whole.Body.Bytes())), byNoteRows(446); !slices.Equal(got, want) {
		t.Fatalf("%d rows at the limit, want %d", len(got), len(want))
	}

	exportLimits.sheetRows = 1350
	requireStatus(t, s.exportAs(t, "ann", csvExportBody(rep.Id)), http.StatusUnprocessableEntity,
		`"message":"This report has more than 50,000 rows. Narrow the period."`)
	if n := len(exportAuditRows(t, s.app)); n != 1 {
		t.Fatalf("%d audit rows, want 1: the whole export writes one, the refused one none", n)
	}
}

// A PDF keeps the reader's folds (byNoteClosed opens closed, so a PDF of it is
// only the group rows): the reader opens every group (expandAll), the report
// is 1,351 rows, and that is the PDF's limit. At the limit it is written
// whole, one row under it is refused with the PDF's sentence and no audit row.
func TestExportOfAGroupedPDFStopsAtTheFormatsLimit(t *testing.T) {
	s := newSavedZoo(t)
	hermeticExportFonts(t)
	rep := s.saved(t, "byNote", "ann", "Feedings by Note", byNoteClosed, false)
	bulkFeedings(t, s.zoo, 446)
	keepExportLimits(t)
	body := fmt.Sprintf(`{"reportId":%q,"params":{},"options":{"today":"2026-11-16","timeZone":"UTC","expandAll":true},"format":"pdf","hour12":false}`, rep.Id)

	exportLimits.pdfRows = 1351
	whole := s.exportAs(t, "ann", body)
	requireFile(t, whole, "application/pdf", ".pdf")
	if !bytes.HasPrefix(whole.Body.Bytes(), []byte("%PDF-")) {
		t.Fatalf("the file does not start %%PDF-: %q", whole.Body.Bytes()[:min(whole.Body.Len(), 20)])
	}
	if n := len(exportAuditRows(t, s.app)); n != 1 {
		t.Fatalf("%d audit rows after the whole export, want 1", n)
	}

	exportLimits.pdfRows = 1350
	requireStatus(t, s.exportAs(t, "ann", body), http.StatusUnprocessableEntity,
		`"message":"This report has more than 5,000 rows. Narrow the period or download CSV."`)
	if n := len(exportAuditRows(t, s.app)); n != 1 {
		t.Fatalf("%d audit rows, want 1: the whole export writes one, the refused one none", n)
	}
}

// A flat report reads in one page too: its rows and its Total, two
// statements, where pages of maxPage rows took two for each page.
func TestExportReadsAFlatReportInOnePage(t *testing.T) {
	s := newSavedZoo(t)
	rep := s.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)
	bulkFeedings(t, s.zoo, 446)
	withLimits(t, func(l *limits) { l.maxPage = 100 })

	seen := observeReads(t)
	file := s.buildCSV(t, "ann", rep.Id, `{"today":"2026-11-16"}`)
	if n := len(*seen); n != 2 {
		t.Fatalf("%d statements, want 2 (the Total and the rows)", n)
	}
	rows := firstFields(csvRecords(t, file.Bytes))
	if len(rows) != 451 || rows[0] != "f1" || rows[449] != "n00446" || rows[450] != "Total" {
		t.Fatalf("%d rows, first %q, want 451 from f1 to n00446 and the Total", len(rows), rows[0])
	}
}

// The one page is read under the statement limit, where each page had a
// limit of its own: a statement past it ends the export with the export's
// sentence, and no file and no audit row.
func TestExportPastTheStatementLimitIsATimeout(t *testing.T) {
	s := newSavedZoo(t)
	rep := s.saved(t, "byNote", "ann", "Feedings by Note", byNoteClosed, false)
	withLimits(t, func(l *limits) { l.timeout = time.Nanosecond })
	requireStatus(t, s.exportAs(t, "ann", csvExportBody(rep.Id)), http.StatusRequestTimeout,
		`"message":"The report took too long to export."`)
	if n := len(exportAuditRows(t, s.app)); n != 0 {
		t.Fatalf("a refused export wrote %d audit rows", n)
	}
}

// The page cap is the export's alone: a run with none keeps maxPage, a run
// with no limit keeps pageSize, and a cap never raises a smaller limit.
func TestPageLimit(t *testing.T) {
	for _, c := range []struct {
		name               string
		limit, asked, want int
	}{
		{"no limit", 0, 0, runLimits.pageSize},
		{"no limit, a cap", 0, 50001, runLimits.pageSize},
		{"a limit under maxPage", 50, 0, 50},
		{"a limit over maxPage", 50001, 0, runLimits.maxPage},
		{"a limit at the cap", 50001, 50001, 50001},
		{"a limit over the cap", 60000, 50001, 50001},
		{"a limit under the cap", 300, 50001, 300},
	} {
		rn := &runner{opts: Options{Limit: c.limit}, askedPageCap: c.asked}
		if got := rn.pageLimit(); got != c.want {
			t.Fatalf("%s: pageLimit = %d, want %d", c.name, got, c.want)
		}
	}
}

// The run endpoint has no page cap: a body that asks for more than maxPage
// rows gets a page of maxPage rows, and a body that names a page cap is
// refused as any unknown field is.
func TestTheRunEndpointKeepsMaxPage(t *testing.T) {
	s := newSavedZoo(t)
	bulkFeedings(t, s.zoo, 446)
	withLimits(t, func(l *limits) { l.maxPage = 100 })
	run := func(rest string) (int, string) {
		return s.call(t, "ann", http.MethodPost, runURL, `{"spec":`+feedingsFlat+`,"params":{},`+rest+`}`)
	}
	status, body := run(`"options":{"limit":50001}`)
	if status != http.StatusOK || !strings.Contains(body, `"size":100`) {
		t.Fatalf("status %d, want 200 and a page of 100 rows:\n%.300s", status, body)
	}
	for _, rest := range []string{`"options":{"limit":50001,"pageCap":50001}`, `"options":{"limit":50001},"pageCap":50001`} {
		if status, body := run(rest); status != http.StatusBadRequest {
			t.Fatalf("%s: status %d, want 400\n%.300s", rest, status, body)
		}
	}
}
