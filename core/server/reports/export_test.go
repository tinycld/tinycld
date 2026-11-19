package reports

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nathanstitt/omnidoc/pkg/xlsx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/hook"

	"tinycld.org/core/oauth"
)

const exportURL = "/api/reports/export"

// feedingsFlat is every feeding the reader may list, by note: a detail row
// per feeding and a total.
const feedingsFlat = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"sort":[{"col":"note"}]}`

// csvExportBody asks for a saved report as a CSV file on the reader's day
// 2026-11-16.
func csvExportBody(reportID string) string {
	return fmt.Sprintf(`{"reportId":%q,"params":{},"options":{"today":"2026-11-16","timeZone":"UTC"},"format":"csv","hour12":false}`, reportID)
}

// exportAs sends one export request through the real router as user ("" =
// signed out).
func (s *savedZoo) exportAs(t *testing.T, user, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, exportURL, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if user != "" {
		req.Header.Set("Authorization", s.token(t, user))
	}
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	return rec
}

func requireStatus(t *testing.T, rec *httptest.ResponseRecorder, status int, parts ...string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status %d, want %d\n%s", rec.Code, status, rec.Body.String())
	}
	for _, p := range parts {
		if !strings.Contains(rec.Body.String(), p) {
			t.Fatalf("body lacks %s\n%s", p, rec.Body.String())
		}
	}
}

// csvRecords reads a CSV file after its byte order mark.
func csvRecords(t *testing.T, file []byte) [][]string {
	t.Helper()
	body, ok := bytes.CutPrefix(file, []byte("\xef\xbb\xbf"))
	if !ok {
		t.Fatalf("the file does not start with a byte order mark: %q", file)
	}
	records, err := csv.NewReader(bytes.NewReader(body)).ReadAll()
	if err != nil {
		t.Fatalf("read CSV: %v\n%q", err, file)
	}
	return records
}

// firstFields is the first field of every record after the heading line.
func firstFields(records [][]string) []string {
	out := make([]string, 0, len(records))
	for _, r := range records[1:] {
		out = append(out, r[0])
	}
	return out
}

// exportAuditRows lists the report.export audit rows.
func exportAuditRows(t *testing.T, app core.App) []*core.Record {
	t.Helper()
	rows, err := app.FindRecordsByFilter("audit_logs", "action = 'report.export'", "", 0, 0)
	if err != nil {
		t.Fatalf("find audit rows: %v", err)
	}
	return rows
}

// keepExportLimits puts the export limits back after a test that shrinks
// them.
func keepExportLimits(t *testing.T) {
	t.Helper()
	saved := exportLimits
	t.Cleanup(func() { exportLimits = saved })
}

// A reader gets the report as their own run returns it: the file, its
// semantic name and headers that keep it out of every cache. A second reader
// with a narrower rule gets fewer lines of the same report, never more.
func TestExportAnswersTheReadersCSV(t *testing.T) {
	s := newSavedZoo(t)
	rep := s.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, true)

	ann := s.exportAs(t, "ann", csvExportBody(rep.Id))
	requireStatus(t, ann, http.StatusOK)
	for header, want := range map[string]string{
		"Content-Type":                  "text/csv; charset=utf-8",
		"Content-Disposition":           `attachment; filename="feeding-costs-2026-11-16.csv"`,
		"Cache-Control":                 "no-store",
		"Access-Control-Expose-Headers": "Content-Disposition",
	} {
		if got := ann.Header().Get(header); got != want {
			t.Fatalf("%s = %q, want %q", header, got, want)
		}
	}
	if got, want := firstFields(csvRecords(t, ann.Body.Bytes())), []string{"f1", "f2", "f3", "f4", "Total"}; !slices.Equal(got, want) {
		t.Fatalf("ann's rows = %q, want %q", got, want)
	}

	bob := s.exportAs(t, "bob", csvExportBody(rep.Id))
	requireStatus(t, bob, http.StatusOK)
	if got, want := firstFields(csvRecords(t, bob.Body.Bytes())), []string{"f3", "f4", "f5", "Total"}; !slices.Equal(got, want) {
		t.Fatalf("bob's rows = %q, want %q", got, want)
	}
}

func TestExportRefusesWhoCannotExport(t *testing.T) {
	s := newSavedZoo(t)
	rep := s.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)
	gone := `"message":"You no longer have access to this report."`

	requireStatus(t, s.exportAs(t, "", csvExportBody(rep.Id)), http.StatusUnauthorized)
	requireStatus(t, s.exportAs(t, "ot", csvExportBody(rep.Id)), http.StatusForbidden, gone)
	requireStatus(t, s.exportAs(t, "ann", csvExportBody("nosuchreport00")), http.StatusForbidden, gone)

	ed := s.recs["ed"]
	ed.Set("disabled", true)
	if err := s.app.Save(ed); err != nil {
		t.Fatal(err)
	}
	// ed holds an editor's share of R; a disabled account still gets nothing.
	requireStatus(t, s.exportAs(t, "ed", csvExportBody(s.id("R"))), http.StatusForbidden, `"message":"Your account is disabled."`)

	if n := len(exportAuditRows(t, s.app)); n != 0 {
		t.Fatalf("refused exports wrote %d audit rows", n)
	}
}

// A draft cannot be exported, and the body is read strictly.
func TestExportReadsItsBodyStrictly(t *testing.T) {
	s := newSavedZoo(t)
	rep := s.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)
	for name, body := range map[string]string{
		"a spec":           fmt.Sprintf(`{"reportId":%q,"spec":%s,"format":"csv"}`, rep.Id, feedingsFlat),
		"a draft alone":    fmt.Sprintf(`{"spec":%s,"format":"csv"}`, feedingsFlat),
		"an unknown field": fmt.Sprintf(`{"reportId":%q,"format":"csv","sql":"DELETE FROM zoo_feedings"}`, rep.Id),
		"no reportId":      `{"params":{},"format":"csv"}`,
	} {
		t.Run(name, func(t *testing.T) {
			requireStatus(t, s.exportAs(t, "ann", body), http.StatusBadRequest)
		})
	}
	requireStatus(t, s.exportAs(t, "ann", fmt.Sprintf(`{"reportId":%q,"format":"docx"}`, rep.Id)),
		http.StatusBadRequest, `"message":"The format is not pdf, xlsx or csv."`)
	if n, _ := s.app.CountRecords("zoo_feedings"); n != 6 {
		t.Fatalf("the route wrote: %d feedings", n)
	}
}

// An OAuth access token, even one holding every scope, meets the real grant
// middleware: the export route is session-only, as the run route is.
func TestExportRefusesAnOAuthTokenThroughTheMiddleware(t *testing.T) {
	z := newZoo(t)
	oauth.Register(z.app)
	Register(z.app)
	rep := z.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)
	client, err := oauth.FindClientByClientID(z.app, "tinycld-cli")
	if err != nil {
		t.Fatal(err)
	}
	grant, err := oauth.NewGrant(z.app, z.id("ann"), client.Id, oauth.AllScopes(), "active")
	if err != nil {
		t.Fatal(err)
	}
	token, err := oauth.MintAccessToken(z.app, z.recs["ann"], grant, oauth.AccessTokenTTL)
	if err != nil {
		t.Fatal(err)
	}
	for _, sc := range []tests.ApiScenario{
		{Name: "an OAuth token", Headers: map[string]string{"Authorization": "Bearer " + token},
			ExpectedStatus: http.StatusForbidden, ExpectedContent: []string{`not available to API tokens`}},
		// The same reader's session token gets the file.
		{Name: "a session token", Headers: map[string]string{"Authorization": z.token(t, "ann")},
			ExpectedStatus: http.StatusOK, ExpectedContent: []string{"f1"}},
	} {
		sc.Method, sc.URL = http.MethodPost, exportURL
		sc.Body = strings.NewReader(csvExportBody(rep.Id))
		sc.TestAppFactory = func(testing.TB) *tests.TestApp { return z.app }
		sc.DisableTestAppCleanup = true
		sc.Test(t)
	}
}

// A file never holds less than the report: past the format's limit the
// export is refused, and nothing is logged as downloaded.
func TestExportRefusesMoreRowsThanTheFormatHolds(t *testing.T) {
	s := newSavedZoo(t)
	rep := s.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)
	s.feeding(t, "f7", s.recs["leo"], 400, "2026-04-01", 10)
	// ann's report is now five feedings and a total: six rows.
	keepExportLimits(t)
	exportLimits.sheetRows = 5
	requireStatus(t, s.exportAs(t, "ann", csvExportBody(rep.Id)), http.StatusUnprocessableEntity,
		`"message":"This report has more than 50,000 rows. Narrow the period."`)
	if n := len(exportAuditRows(t, s.app)); n != 0 {
		t.Fatalf("a refused export wrote %d audit rows", n)
	}
}

// feedingsPivotByNote is a group row per feeding of the reader's with its
// cost by month: the engine stops such a run at runLimits.rowCap rows.
const feedingsPivotByNote = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"note"}],
	"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"cost"}}`

// D5: the export reads a pivot whole up to the format's limit, past the
// engine's own row cap; a run the cap still stops is refused, never cut.
func TestExportReadsAPivotPastTheEngineCap(t *testing.T) {
	s := newSavedZoo(t)
	rep := s.saved(t, "pivot", "ann", "Feedings by Month", feedingsPivotByNote, false)
	withLimits(t, func(l *limits) { l.rowCap = 3 })
	keepExportLimits(t)
	exportLimits.sheetRows = 10
	whole := s.exportAs(t, "ann", csvExportBody(rep.Id))
	requireStatus(t, whole, http.StatusOK)
	if got, want := firstFields(csvRecords(t, whole.Body.Bytes())), []string{"f1", "f2", "f3", "f4", "Total"}; !slices.Equal(got, want) {
		t.Fatalf("rows = %q, want every feeding and the total", got)
	}

	exportLimits.sheetRows = 3
	requireStatus(t, s.exportAs(t, "ann", csvExportBody(rep.Id)), http.StatusUnprocessableEntity,
		`"message":"This report has more than 50,000 rows. Narrow the period."`)
}

// The file holds every row of a report longer than a page of the viewer
// (maxPage): a build that stops at a viewer's page fails on the count.
func TestExportReadsEveryPage(t *testing.T) {
	s := newSavedZoo(t)
	rep := s.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)
	withLimits(t, func(l *limits) { l.maxPage = 100 })
	// ann lists f1 to f4; 446 more of leo's make 450 detail rows, five pages.
	col, err := s.app.FindCollectionByNameOrId("zoo_feedings")
	if err != nil {
		t.Fatal(err)
	}
	err = s.app.RunInTransaction(func(tx core.App) error {
		for i := range 446 {
			r := core.NewRecord(col)
			r.Load(map[string]any{"animal": s.id("leo"), "cost": 100, "currency": "USD", "fed": "2026-05-01 00:00:00.000Z", "note": fmt.Sprintf("p%03d", i)})
			if err := tx.Save(r); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := s.exportAs(t, "ann", csvExportBody(rep.Id))
	requireStatus(t, rec, http.StatusOK)
	rows := firstFields(csvRecords(t, rec.Body.Bytes()))
	details := rows[:len(rows)-1]
	if len(details) != 450 || rows[len(rows)-1] != "Total" {
		t.Fatalf("%d detail rows then %q, want 450 then Total", len(details), rows[len(rows)-1])
	}
	if details[0] != "f1" || details[len(details)-1] != "p445" {
		t.Fatalf("first %q, last %q, want f1 and p445", details[0], details[len(details)-1])
	}
	seen := map[string]bool{}
	for _, note := range details {
		if seen[note] {
			t.Fatalf("row %q is in the file twice", note)
		}
		seen[note] = true
	}
}

func TestExportStopsAtTheTimeLimit(t *testing.T) {
	s := newSavedZoo(t)
	rep := s.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)
	keepExportLimits(t)
	exportLimits.timeout = time.Nanosecond
	requireStatus(t, s.exportAs(t, "ann", csvExportBody(rep.Id)), http.StatusRequestTimeout,
		`"message":"The report took too long to export."`)
}

// One download is one audit row, naming the report, the format and the rows
// the file holds, with the reader as the actor; a refusal writes none.
func TestExportWritesOneAuditRowPerDownload(t *testing.T) {
	s := newSavedZoo(t)
	rep := s.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)
	requireStatus(t, s.exportAs(t, "ann", csvExportBody(rep.Id)), http.StatusOK)
	requireStatus(t, s.exportAs(t, "ot", csvExportBody(rep.Id)), http.StatusForbidden)

	rows := exportAuditRows(t, s.app)
	if len(rows) != 1 {
		t.Fatalf("%d audit rows, want 1", len(rows))
	}
	row := rows[0]
	for field, want := range map[string]string{
		"resource_type":  "reports",
		"resource_id":    rep.Id,
		"resource_label": "Feeding Costs",
		"actor":          s.id("ann"),
	} {
		if got := row.GetString(field); got != want {
			t.Fatalf("%s = %q, want %q", field, got, want)
		}
	}
	var meta struct {
		Format string  `json:"format"`
		Rows   float64 `json:"rows"`
	}
	if err := json.Unmarshal([]byte(row.GetString("metadata")), &meta); err != nil {
		t.Fatalf("metadata %s: %v", row.GetString("metadata"), err)
	}
	// Four feedings and the total.
	if meta.Format != "csv" || meta.Rows != 5 {
		t.Fatalf("metadata = %+v, want format csv and 5 rows", meta)
	}
}

// buildExportAs reads a saved report as user through buildExport in format,
// with params and options as the run endpoint takes them.
func buildExportAs(t *testing.T, s *savedZoo, user, reportID, format, params, options string) *exportFile {
	t.Helper()
	var body exportBody
	raw := fmt.Sprintf(`{"reportId":%q,"params":%s,"options":%s,"format":%q,"hour12":false}`, reportID, params, options, format)
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	re := &core.RequestEvent{App: s.app, Auth: s.recs[user]}
	re.Request = httptest.NewRequest(http.MethodPost, exportURL, nil)
	re.Response = httptest.NewRecorder()
	file, err := buildExport(re, body)
	if err != nil {
		t.Fatalf("buildExport as %s: %v", format, err)
	}
	return file
}

// exportBodyIn asks for a saved report in format on the reader's day
// 2026-11-16.
func exportBodyIn(reportID, format string) string {
	return fmt.Sprintf(`{"reportId":%q,"params":{},"options":{"today":"2026-11-16","timeZone":"UTC"},"format":%q,"hour12":false}`, reportID, format)
}

var attachmentName = regexp.MustCompile(`filename="([^"]+)"`)

// requireFile fails unless the answer is a file of type mime whose name ends
// in ext.
func requireFile(t *testing.T, rec *httptest.ResponseRecorder, mime, ext string) {
	t.Helper()
	requireStatus(t, rec, http.StatusOK)
	if got := rec.Header().Get("Content-Type"); got != mime {
		t.Fatalf("Content-Type = %q, want %q", got, mime)
	}
	m := attachmentName.FindStringSubmatch(rec.Header().Get("Content-Disposition"))
	if m == nil || !strings.HasSuffix(m[1], ext) {
		t.Fatalf("Content-Disposition = %q, want a file name ending %s", rec.Header().Get("Content-Disposition"), ext)
	}
}

func TestExportAnswersAnExcelFile(t *testing.T) {
	s := newSavedZoo(t)
	rep := s.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)
	rec := s.exportAs(t, "ann", exportBodyIn(rep.Id, "xlsx"))
	requireFile(t, rec, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xlsx")
	if _, err := xlsx.OpenBytes(context.Background(), rec.Body.Bytes()); err != nil {
		t.Fatalf("the file is not an Excel file: %v", err)
	}
}

func TestExportAnswersAPDFFile(t *testing.T) {
	s := newSavedZoo(t)
	hermeticExportFonts(t)
	rep := s.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)
	rec := s.exportAs(t, "ann", exportBodyIn(rep.Id, "pdf"))
	requireFile(t, rec, "application/pdf", ".pdf")
	if !bytes.HasPrefix(rec.Body.Bytes(), []byte("%PDF-")) {
		t.Fatalf("the file does not start %%PDF-: %q", rec.Body.Bytes()[:min(rec.Body.Len(), 20)])
	}
}

// A PDF holds fewer rows than a CSV: past its limit a PDF is refused with its
// own sentence while the same report is a CSV file; Excel has the CSV's limit.
func TestExportRefusesEachFormatPastItsLimit(t *testing.T) {
	s := newSavedZoo(t)
	hermeticExportFonts(t)
	rep := s.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)
	s.feeding(t, "f7", s.recs["leo"], 400, "2026-04-01", 10)
	// ann's report is now five feedings and a total: six rows.
	keepExportLimits(t)
	exportLimits.pdfRows = 5
	requireStatus(t, s.exportAs(t, "ann", exportBodyIn(rep.Id, "pdf")), http.StatusUnprocessableEntity,
		`"message":"This report has more than 5,000 rows. Narrow the period or download CSV."`)
	requireStatus(t, s.exportAs(t, "ann", exportBodyIn(rep.Id, "csv")), http.StatusOK)

	exportLimits.pdfRows = 5000
	exportLimits.sheetRows = 5
	requireStatus(t, s.exportAs(t, "ann", exportBodyIn(rep.Id, "xlsx")), http.StatusUnprocessableEntity,
		`"message":"This report has more than 50,000 rows. Narrow the period."`)
}

// Each format's download writes its own audit row with its format.
func TestExportAuditsEveryFormat(t *testing.T) {
	s := newSavedZoo(t)
	hermeticExportFonts(t)
	rep := s.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)
	for _, format := range []string{"csv", "xlsx", "pdf"} {
		requireStatus(t, s.exportAs(t, "ann", exportBodyIn(rep.Id, format)), http.StatusOK)
	}
	var formats []string
	for _, row := range exportAuditRows(t, s.app) {
		var meta struct {
			Format string `json:"format"`
		}
		if err := json.Unmarshal([]byte(row.GetString("metadata")), &meta); err != nil {
			t.Fatalf("metadata %s: %v", row.GetString("metadata"), err)
		}
		formats = append(formats, meta.Format)
	}
	slices.Sort(formats)
	if want := []string{"csv", "pdf", "xlsx"}; !slices.Equal(formats, want) {
		t.Fatalf("audit formats = %q, want %q", formats, want)
	}
}

// A deadline reached while the file is written is the export's time-out,
// not a failure to write.
func TestExportPastItsDeadlineInTheWriterIsATimeout(t *testing.T) {
	s := newSavedZoo(t)
	rep := s.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)
	keepExportLimits(t)
	exportLimits.timeout = time.Second
	saved := exportWriters["csv"]
	t.Cleanup(func() { exportWriters["csv"] = saved })
	slow := saved
	slow.Write = func(ctx context.Context, _ *exportData) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	exportWriters["csv"] = slow
	requireStatus(t, s.exportAs(t, "ann", csvExportBody(rep.Id)), http.StatusRequestTimeout,
		`"message":"The report took too long to export."`)
}

// feedingsAtSite names a site param from a source; the run never reads the
// sites.
const feedingsAtSite = `{"version":4,"source":"zoo.feedings","params":[
	{"id":"site","type":"choice","label":"Site","optionsFrom":{"source":"zoo.sites","value":[{"field":"id"}],"label":[{"field":"name"}]}}],
	"columns":[{"id":"note","path":[{"field":"note"}]},{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"sort":[{"col":"note"}]}`

// A CSV has no params line, so its export reads no param labels: with the
// labels' source unreadable the CSV is still written.
func TestCSVExportReadsNoParamLabels(t *testing.T) {
	s := newSavedZoo(t)
	rep := s.saved(t, "site", "ann", "Feedings at a Site", feedingsAtSite, false)
	body := fmt.Sprintf(`{"reportId":%q,"params":{"site":%q},"options":{"today":"2026-11-16"},"format":"csv","hour12":false}`, rep.Id, s.id("north"))
	requireStatus(t, s.exportAs(t, "ann", body), http.StatusOK)

	if _, err := s.app.DB().NewQuery("ALTER TABLE zoo_sites RENAME TO gone_zoo_sites").Execute(); err != nil {
		t.Fatal(err)
	}
	rec := s.exportAs(t, "ann", body)
	requireStatus(t, rec, http.StatusOK)
	if got, want := firstFields(csvRecords(t, rec.Body.Bytes())), []string{"f1", "f2", "f3", "f4", "Total"}; !slices.Equal(got, want) {
		t.Fatalf("rows = %q, want %q", got, want)
	}
}

// A download that cannot be logged is not sent: the audit row is the record
// that the file left the server.
func TestExportWithoutItsAuditRowSendsNoFile(t *testing.T) {
	s := newSavedZoo(t)
	rep := s.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)
	s.app.OnRecordCreate("audit_logs").Bind(&hook.Handler[*core.RecordEvent]{
		Priority: -1,
		Func: func(e *core.RecordEvent) error {
			if _, err := e.App.DB().NewQuery("ALTER TABLE audit_logs RENAME TO gone_audit_logs").Execute(); err != nil {
				return err
			}
			return e.Next()
		},
	})
	rec := s.exportAs(t, "ann", csvExportBody(rep.Id))
	requireStatus(t, rec, http.StatusInternalServerError, `"message":"Could not create the file."`)
	if body := rec.Body.String(); strings.Contains(body, "\xef\xbb\xbf") || strings.Contains(body, "f1") {
		t.Fatalf("the refusal carries file bytes: %q", body)
	}
}
