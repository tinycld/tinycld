package reports

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// feedingsBySpeciesCSV groups the reader's feedings by species with
// subtotals; grams is a column the reader hides.
const feedingsBySpeciesCSV = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"note","path":[{"field":"note"}]},
	{"id":"fed","path":[{"field":"fed"}]},
	{"id":"grams","path":[{"field":"grams"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"bySpecies","col":"species","subtotals":true}],
	"sort":[{"col":"note"}]}`

// feedingsNoteTwice shows each feeding's note as its label and again as a
// text cell.
const feedingsNoteTwice = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"again","label":"Again","path":[{"field":"note"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"sort":[{"col":"note"}]}`

// buildCSV reads a saved report as user through buildExport, with options as
// the run endpoint takes them.
func (s *savedZoo) buildCSV(t *testing.T, user, reportID, options string) *exportFile {
	t.Helper()
	var body exportBody
	raw := fmt.Sprintf(`{"reportId":%q,"params":{},"options":%s,"format":"csv","hour12":false}`, reportID, options)
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	re := &core.RequestEvent{App: s.app, Auth: s.recs[user]}
	re.Request = httptest.NewRequest(http.MethodPost, exportURL, nil)
	re.Response = httptest.NewRecorder()
	file, err := buildExport(re, body)
	if err != nil {
		t.Fatalf("buildExport: %v", err)
	}
	return file
}

func requireCRLF(t *testing.T, file []byte) {
	t.Helper()
	if !bytes.HasSuffix(file, []byte("\r\n")) {
		t.Fatalf("the file does not end in CRLF: %q", file)
	}
	for i, b := range file {
		if b == '\n' && (i == 0 || file[i-1] != '\r') {
			t.Fatalf("a line ends in a bare LF at byte %d: %q", i, file)
		}
	}
}

// The whole grouped report, every group open although the reader folded
// one: a byte order mark, CRLF lines, the label column's heading and the
// shown columns, then group, detail, subtotal and total rows in report order.
// A group row its subtotal follows has no figures (D7); a detail row's label
// is its first column, which no cell repeats.
func TestCSVHoldsTheWholeGroupedReport(t *testing.T) {
	s := newSavedZoo(t)
	rep := s.saved(t, "species", "ann", "Feedings by Species", feedingsBySpeciesCSV, false)
	file := s.buildCSV(t, "ann", rep.Id, `{"collapse":[["lion","USD"]],"display":{"hidden":["grams"]},"today":"2026-11-16"}`)

	want := "\xef\xbb\xbf" +
		"Species,Note,Fed,Cost\r\n" +
		"lion,,,\r\n" +
		"lion,f1,2026-01-05,10.00\r\n" +
		"lion,f2,2026-02-10,25.00\r\n" +
		"Total lion,,,35.00\r\n" +
		"zebra,,,\r\n" +
		"zebra,f3,2026-01-20,7.00\r\n" +
		"zebra,f4,2026-03-03,3.00\r\n" +
		"Total zebra,,,10.00\r\n" +
		"Total,,,45.00\r\n"
	if got := string(file.Bytes); got != want {
		t.Fatalf("file =\n%q\nwant\n%q", got, want)
	}
	requireCRLF(t, file.Bytes)
	if file.Mime != "text/csv; charset=utf-8" || file.Rows != 9 || file.Title != "Feedings by Species" || file.ReportID != rep.Id || file.Format != "csv" {
		t.Fatalf("file = {Mime:%q Rows:%d Title:%q ReportID:%q Format:%q}", file.Mime, file.Rows, file.Title, file.ReportID, file.Format)
	}
	if file.Name != "feedings-by-species-2026-11-16.csv" {
		t.Fatalf("name = %q", file.Name)
	}
}

// The reader's heading sort orders the detail rows inside each group.
func TestCSVFollowsTheReadersSort(t *testing.T) {
	s := newSavedZoo(t)
	rep := s.saved(t, "species", "ann", "Feedings by Species", feedingsBySpeciesCSV, false)
	file := s.buildCSV(t, "ann", rep.Id, `{"sort":[{"col":"note","dir":"desc"}]}`)
	var notes []string
	for _, r := range csvRecords(t, file.Bytes)[1:] {
		if r[1] != "" {
			notes = append(notes, r[1])
		}
	}
	if want := []string{"f2", "f1", "f4", "f3"}; !slices.Equal(notes, want) {
		t.Fatalf("detail notes = %q, want %q", notes, want)
	}
}

// RFC 4180: a field with a comma, a quote or a line break is quoted, its
// quotes doubled; the line break inside it is CRLF too.
func TestCSVQuotesByRFC4180(t *testing.T) {
	s := newSavedZoo(t)
	s.rec(t, "quoted", "zoo_feedings", map[string]any{"animal": s.id("leo"), "cost": 100, "currency": "USD", "fed": "2026-04-01 00:00:00.000Z",
		"note": "Fed \"Leo\", twice\nat noon"})
	rep := s.saved(t, "twice", "ann", "Notes", feedingsNoteTwice, false)
	file := s.buildCSV(t, "ann", rep.Id, `{}`)
	field := `"Fed ""Leo"", twice` + "\r\n" + `at noon"`
	if want := field + "," + field + ",1.00\r\n"; !strings.Contains(string(file.Bytes), want) {
		t.Fatalf("file lacks the quoted line %q:\n%q", want, file.Bytes)
	}
	requireCRLF(t, file.Bytes)
}

// A text cell and a row label that a spreadsheet would run as a formula get
// a leading apostrophe; a negative amount is a number and gets none.
func TestCSVGuardsFormulaText(t *testing.T) {
	s := newSavedZoo(t)
	starts := map[string]string{"=": "=SUM(A1)", "+": "+1", "-": "-2", "@": "@keeper", "tab": "\tTab"}
	for key, note := range starts {
		s.rec(t, "guard"+key, "zoo_feedings", map[string]any{"animal": s.id("leo"), "cost": 100, "currency": "USD", "fed": "2026-04-01 00:00:00.000Z", "note": note})
	}
	s.rec(t, "guardcr", "zoo_feedings", map[string]any{"animal": s.id("leo"), "cost": 100, "currency": "USD", "fed": "2026-04-01 00:00:00.000Z", "note": "\rCR"})
	s.rec(t, "midcr", "zoo_feedings", map[string]any{"animal": s.id("leo"), "cost": 100, "currency": "USD", "fed": "2026-04-01 00:00:00.000Z", "note": "a\rb"})
	s.rec(t, "refund", "zoo_feedings", map[string]any{"animal": s.id("leo"), "cost": -123450, "currency": "USD", "fed": "2026-04-01 00:00:00.000Z", "note": "refund"})
	rep := s.saved(t, "twice", "ann", "Notes", feedingsNoteTwice, false)
	file := s.buildCSV(t, "ann", rep.Id, `{}`)
	records := csvRecords(t, file.Bytes)

	byLabel := map[string][]string{}
	for _, r := range records[1:] {
		byLabel[r[0]] = r
	}
	for key, note := range starts {
		guarded := "'" + note
		r, ok := byLabel[guarded]
		if !ok || r[1] != guarded {
			t.Fatalf("%s: no row with label and cell %q in %q", key, guarded, records)
		}
	}
	// A lone carriage return is a line break the run returned: the file keeps
	// it as the CRLF it writes for any line break in a quoted field, and
	// csv.Reader reads that back as "\n".
	if !strings.Contains(string(file.Bytes), "\"a\r\nb\",\"a\r\nb\",") {
		t.Fatalf("the line break of the note \"a\\rb\" is lost: %q", file.Bytes)
	}
	if r, ok := byLabel["a\nb"]; !ok || r[1] != "a\nb" {
		t.Fatalf("no row with label and cell %q (its line break kept) in %q", "a\nb", records)
	}
	if !strings.Contains(string(file.Bytes), "\"'\r\nCR\",\"'\r\nCR\",") {
		t.Fatalf("a text that starts with a carriage return is not guarded with its line break kept: %q", file.Bytes)
	}
	if r, ok := byLabel["'\nCR"]; !ok || r[1] != "'\nCR" {
		t.Fatalf("no row with label and cell %q (apostrophe, line break, text) in %q", "'\nCR", records)
	}
	if r := byLabel["refund"]; r == nil || r[2] != "-1234.50" {
		t.Fatalf("refund row = %q, want the amount -1234.50 with no apostrophe", r)
	}
}

func TestCSVGuardByTable(t *testing.T) {
	for in, want := range map[string]string{
		"=SUM(A1)": "'=SUM(A1)",
		"+1":       "'+1",
		"-2":       "'-2",
		"@keeper":  "'@keeper",
		"\tTab":    "'\tTab",
		"\rCR":     "'\rCR",
		"Leo":      "Leo",
		"":         "",
		"a=b":      "a=b",
		" =x":      " =x",
		"'quoted":  "'quoted",
	} {
		if got := csvGuard(in); got != want {
			t.Fatalf("csvGuard(%q) = %q, want %q", in, got, want)
		}
	}
}

// A money column whose rows carry different currencies is followed by its
// currency column; the run's notice about it is not in the file.
func TestCSVNamesTheCurrencyOfAMixedColumn(t *testing.T) {
	s := newSavedZoo(t)
	s.rec(t, "eur", "zoo_feedings", map[string]any{"animal": s.id("leo"), "cost": 1234, "currency": "EUR", "fed": "2026-04-01 00:00:00.000Z", "note": "eur"})
	rep := s.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)
	file := s.buildCSV(t, "ann", rep.Id, `{}`)
	want := "\xef\xbb\xbf" +
		"Note,Cost,Cost currency\r\n" +
		"eur,12.34,EUR\r\n" +
		"f1,10.00,USD\r\n" +
		"f2,25.00,USD\r\n" +
		"f3,7.00,USD\r\n" +
		"f4,3.00,USD\r\n" +
		"Total,12.34,EUR\r\n" +
		"Total,45.00,USD\r\n"
	if got := string(file.Bytes); got != want {
		t.Fatalf("file =\n%q\nwant\n%q", got, want)
	}
	if strings.Contains(string(file.Bytes), "different currencies") {
		t.Fatal("the run's notice is in the file")
	}
}

// D8: a report with no rows is its heading line alone.
func TestCSVOfAnEmptyReportIsItsHeadingLine(t *testing.T) {
	s := newSavedZoo(t)
	// ot keeps no animal, so the published report has no feeding for ot.
	rep := s.saved(t, "notes", "ann", "Feeding Notes", `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},{"id":"fed","path":[{"field":"fed"}]}]}`, true)
	file := s.buildCSV(t, "ot", rep.Id, `{}`)
	if got, want := string(file.Bytes), "\xef\xbb\xbfNote,Fed\r\n"; got != want {
		t.Fatalf("file = %q, want %q", got, want)
	}
}
