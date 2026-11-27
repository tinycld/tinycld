package reports

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nathanstitt/omnidoc/pkg/omnidoc"

	"tinycld.org/core/branding"
)

// hermeticExportFonts makes the PDF writer use the bundled fonts only, so a
// test proves the same on every host.
func hermeticExportFonts(t *testing.T) {
	t.Helper()
	saved := exportFontsHermetic
	exportFontsHermetic = true
	t.Cleanup(func() { exportFontsHermetic = saved })
}

// tinyPNG is a 2x2 PNG logo.
func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 200, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// The labels of pdfExport's rows; none is part of another.
const (
	pdfGroup    = "Grain"
	pdfDetail   = "Hay"
	pdfDeep     = "Clover"
	pdfComputed = "Margin"
	pdfSubtotal = "Sum of feed"
	pdfTotal    = "Grand total"
)

// pdfExport is a narrow report written by hand: an org with a logo and a
// brand color, the source's heading left to the org's name, a period and two
// other params, two notices, and a row of each kind. The reader is in
// Chicago and shows negatives in red.
func pdfExport(t *testing.T) *exportData {
	t.Helper()
	display := exportDisplay{Negative: "minus", NegativeRed: true, Hidden: []string{}}
	rows := []Row{
		{ID: "g", Kind: KindGroup, Depth: 0, Label: pdfGroup, Expanded: true, Cells: map[string]any{}},
		{ID: "hay", Kind: KindDetail, Depth: 1, Label: pdfDetail, Section: "g", Cells: map[string]any{"amount": int64(-500), "share": 12.5}},
		{ID: "clover", Kind: KindDetail, Depth: 2, Label: pdfDeep, Section: "g", Cells: map[string]any{"amount": int64(100), "share": 2.5}},
		{ID: "sub", Kind: KindSubtotal, Depth: 0, Label: pdfSubtotal, Section: "g", Cells: map[string]any{"amount": int64(-400)}},
		{ID: "margin", Kind: KindComputed, Depth: 0, Label: pdfComputed, Cells: map[string]any{"amount": int64(200)}},
		{ID: "total", Kind: KindTotal, Depth: 0, Label: pdfTotal, Cells: map[string]any{"amount": int64(-400)}},
	}
	out := make([]exportRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, exportRow{Row: r, Cells: r.Cells})
	}
	return &exportData{
		Title:    "Feeding Costs",
		ReportID: "pdfreport00000",
		Heading:  "",
		Columns: []ResultColumn{
			{ID: "name", Label: "Feed", Type: Text},
			{ID: "amount", Label: "Amount", Type: Money, Currency: "USD"},
			{ID: "share", Label: "Share", Type: Percent},
		},
		Rows: out,
		Params: []ResolvedParam{
			{ID: "period", Type: ParamDateRange, From: "2026-01-01", To: "2026-03-31"},
			{ID: "rows", Type: ParamChoice, Value: "nonzero"},
			{ID: "accounts", Type: ParamChoice, Value: "all"},
		},
		Defs: []ParamDef{
			{ID: "period", Type: ParamDateRange, Label: "Period"},
			{ID: "rows", Type: ParamChoice, Label: "Rows", Options: []Option{{Value: "nonzero", Label: "Non-zero"}}},
			{ID: "accounts", Type: ParamChoice, Label: "Accounts", Options: []Option{{Value: "all", Label: "All income accounts"}}},
		},
		Notices:     []Notice{{Code: "first", Message: "The first notice."}, {Code: "second", Message: "The second notice."}},
		ParamLabels: map[string]map[string]string{},
		Reader:      exportReader{Zone: "America/Chicago", Hour12: false, Today: "2026-11-16"},
		Display:     display,
		Format:      newExportFormatter(display, "America/Chicago", false),
		Made:        time.Date(2026, 11, 14, 22, 7, 0, 0, time.UTC),
		Org:         branding.Org{Name: "Zoo Trust", Color: "#123abc", Logo: tinyPNG(t), LogoType: "image/png"},
	}
}

// pdfHTML is the page exportPDFHTML makes of d.
func pdfHTML(t *testing.T, d *exportData) string {
	t.Helper()
	page, err := exportPDFHTML(d)
	if err != nil {
		t.Fatalf("exportPDFHTML: %v", err)
	}
	return string(page)
}

// withDisplay sets d's display options and its formatter together.
func withDisplay(d *exportData, display exportDisplay) {
	d.Display = display
	d.Format = newExportFormatter(display, d.Reader.Zone, d.Reader.Hour12)
}

var (
	trChunk    = regexp.MustCompile(`(?s)<tr\b.*?</tr>`)
	classValue = regexp.MustCompile(`class="([^"]*)"`)
)

// rowMarkup is the one <tr> of the table that holds label.
func rowMarkup(t *testing.T, page, label string) string {
	t.Helper()
	for _, tr := range trChunk.FindAllString(page, -1) {
		if strings.Contains(tr, label) {
			return tr
		}
	}
	t.Fatalf("no <tr> holds %q in\n%s", label, page)
	return ""
}

// classesOf is every class name used in markup.
func classesOf(markup string) map[string]bool {
	out := map[string]bool{}
	for _, m := range classValue.FindAllStringSubmatch(markup, -1) {
		for _, name := range strings.Fields(m[1]) {
			out[name] = true
		}
	}
	return out
}

// requireInOrder fails unless each part is in page, each after the one
// before it.
func requireInOrder(t *testing.T, page string, parts ...string) {
	t.Helper()
	at := 0
	for _, p := range parts {
		i := strings.Index(page[at:], p)
		if i < 0 {
			t.Fatalf("%q is not after byte %d of\n%s", p, at, page)
		}
		at += i + len(p)
	}
}

// pdfTextOf reads a written PDF back: its text and its page count.
func pdfTextOf(t *testing.T, pdf []byte) (string, int) {
	t.Helper()
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatalf("the file is not a PDF: %q", pdf[:min(len(pdf), 20)])
	}
	doc, err := omnidoc.OpenBytes(pdf)
	if err != nil {
		t.Fatalf("open the PDF: %v", err)
	}
	var buf bytes.Buffer
	if err := doc.WriteText(context.Background(), &buf, omnidoc.MarkdownOptions{}); err != nil {
		t.Fatalf("read the PDF's text: %v", err)
	}
	return buf.String(), doc.PageCount()
}

func writePDFOf(t *testing.T, d *exportData) []byte {
	t.Helper()
	hermeticExportFonts(t)
	pdf, err := writePDF(context.Background(), d)
	if err != nil {
		t.Fatalf("writePDF: %v", err)
	}
	return pdf
}

// The logo is an inline image when the org has one, and absent when not.
func TestPDFHTMLShowsTheLogoWhenSet(t *testing.T) {
	d := pdfExport(t)
	if page := pdfHTML(t, d); !regexp.MustCompile(`<img\b[^>]*src="data:image/png;base64,`).MatchString(page) {
		t.Fatalf("no <img> with a data: URI of the logo in\n%s", page)
	}
	d.Org.Logo, d.Org.LogoType = nil, ""
	if page := pdfHTML(t, d); strings.Contains(page, "<img") {
		t.Fatalf("an org with no logo has an <img>:\n%s", page)
	}
}

// The header holds, in order, the heading, the title and the period in
// words; the heading is the source's, else the organization's name.
func TestPDFHTMLHeaderOrder(t *testing.T) {
	d := pdfExport(t)
	requireInOrder(t, pdfHTML(t, d), "Zoo Trust", "Feeding Costs", "January 1 – March 31, 2026")
	d.Heading = "Animal Ledger"
	requireInOrder(t, pdfHTML(t, d), "Animal Ledger", "Feeding Costs", "January 1 – March 31, 2026")
}

// The brand color reaches the page's style; one that is not a valid color
// falls back to the default.
func TestPDFHTMLUsesAValidBrandColor(t *testing.T) {
	d := pdfExport(t)
	if style := styleOf(t, pdfHTML(t, d)); !strings.Contains(style, "#123abc") {
		t.Fatalf("the brand color #123abc is not in the page's style:\n%s", style)
	}
	d.Org.Color = "red;}body{display:none"
	page := pdfHTML(t, d)
	style := strings.ToLower(styleOf(t, page))
	if strings.Contains(page, "display:none") || !strings.Contains(style, strings.ToLower(branding.DefaultColor)) {
		t.Fatalf("an invalid color %q is in the page or the default %s is not in its style:\n%s", d.Org.Color, branding.DefaultColor, page)
	}
}

var styleBlock = regexp.MustCompile(`(?s)<style\b[^>]*>(.*?)</style>`)

// styleOf is the text of the page's <style> block.
func styleOf(t *testing.T, page string) string {
	t.Helper()
	m := styleBlock.FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("the page has no <style> block:\n%s", page)
	}
	return m[1]
}

// "Times in <zone>" only with a date-time column; "Amounts in thousands"
// only with that option.
func TestPDFHTMLZoneAndThousandsLines(t *testing.T) {
	d := pdfExport(t)
	page := pdfHTML(t, d)
	if strings.Contains(page, "Times in") || strings.Contains(page, "Amounts in thousands") {
		t.Fatalf("a report with no date-time column and no thousands option has their lines:\n%s", page)
	}
	d.Columns = append(d.Columns, ResultColumn{ID: "at", Label: "At", Type: DateTime})
	withDisplay(d, exportDisplay{Negative: "minus", NegativeRed: true, DivideBy1000: true, Hidden: []string{}})
	page = pdfHTML(t, d)
	if !strings.Contains(page, "Times in Central Time") || !strings.Contains(page, "Amounts in thousands") {
		t.Fatalf("the zone line or the thousands line is missing:\n%s", page)
	}
}

// The params line names every param but the period with its value, and
// each notice is its own line under the header.
func TestPDFHTMLParamsLineAndNotices(t *testing.T) {
	page := pdfHTML(t, pdfExport(t))
	line := "Rows: Non-zero · Accounts: All income accounts"
	if !strings.Contains(page, line) {
		t.Fatalf("the params line %q is not in the page:\n%s", line, page)
	}
	if strings.Contains(page, "Period:") {
		t.Fatalf("the params line names the period:\n%s", page)
	}
	for _, n := range []string{"The first notice.", "The second notice."} {
		if !regexp.MustCompile(`<p\b[^>]*>\s*` + regexp.QuoteMeta(n) + `\s*</p>`).MatchString(page) {
			t.Fatalf("the notice %q is not a line of its own:\n%s", n, page)
		}
	}
	requireInOrder(t, page, line, "The first notice.", "The second notice.", "<table")
}

// Rows keep their weight and rules, and a depth-2 label is indented 24 pt.
func TestPDFHTMLRowWeightsAndRules(t *testing.T) {
	page := pdfHTML(t, pdfExport(t))
	for label, class := range map[string]string{pdfGroup: "semibold", pdfComputed: "semibold", pdfSubtotal: "rule", pdfTotal: "rule-heavy"} {
		if !classesOf(rowMarkup(t, page, label))[class] {
			t.Fatalf("the row %q has no class %s:\n%s", label, class, rowMarkup(t, page, label))
		}
	}
	detail := classesOf(rowMarkup(t, page, pdfDetail))
	if detail["semibold"] || detail["rule"] || detail["rule-heavy"] {
		t.Fatalf("a detail row has a weight or a rule: %v", detail)
	}
	if deep := rowMarkup(t, page, pdfDeep); !regexp.MustCompile(`padding-left:\s*24pt`).MatchString(deep) {
		t.Fatalf("the depth-2 row is not indented 24pt:\n%s", deep)
	}
}

// A negative amount is red only with "negatives in red"; a subtotal shows its
// currency symbol and a detail row does not.
func TestPDFHTMLCellsReadAsOnScreen(t *testing.T) {
	d := pdfExport(t)
	page := pdfHTML(t, d)
	if !classesOf(rowMarkup(t, page, pdfDetail))["red"] {
		t.Fatalf("the negative amount is not red:\n%s", rowMarkup(t, page, pdfDetail))
	}
	if !strings.Contains(rowMarkup(t, page, pdfSubtotal), "-$4.00") {
		t.Fatalf("the subtotal has no currency symbol:\n%s", rowMarkup(t, page, pdfSubtotal))
	}
	if hay := rowMarkup(t, page, pdfDetail); strings.Contains(hay, "$") || !strings.Contains(hay, "-5.00") {
		t.Fatalf("the detail amount is not -5.00 without a symbol:\n%s", hay)
	}
	if !strings.Contains(rowMarkup(t, page, pdfDetail), "12.5%") {
		t.Fatalf("the percent is not 12.5%%:\n%s", rowMarkup(t, page, pdfDetail))
	}
	withDisplay(d, exportDisplay{Negative: "minus", Hidden: []string{}})
	if classesOf(rowMarkup(t, pdfHTML(t, d), pdfDetail))["red"] {
		t.Fatal("the negative amount is red without the option")
	}
}

// Figures are plain text, and text from a report is escaped.
func TestPDFHTMLHasNoLinksAndEscapesText(t *testing.T) {
	d := pdfExport(t)
	d.Title = "<script>alert(1)</script>"
	d.Rows[1].Row.Label = "<script>bad()</script>"
	d.Columns = append(d.Columns, ResultColumn{ID: "memo", Label: "Memo", Type: Text})
	d.Rows[1].Cells["memo"] = "<script>cell()</script>"
	page := pdfHTML(t, d)
	if regexp.MustCompile(`(?i)<a[\s>]`).MatchString(page) || strings.Contains(strings.ToLower(page), "href") {
		t.Fatalf("the page has a link:\n%s", page)
	}
	for _, escaped := range []string{"&lt;script&gt;alert(1)&lt;/script&gt;", "&lt;script&gt;bad()&lt;/script&gt;", "&lt;script&gt;cell()&lt;/script&gt;"} {
		if strings.Contains(page, "<script") || !strings.Contains(page, escaped) {
			t.Fatalf("a title, a label or a cell text with <script> is not escaped as %q:\n%s", escaped, page)
		}
	}
}

// wideExport is pdfExport with 30 wide text columns after the label column,
// two rows of them.
func wideExport(t *testing.T) *exportData {
	t.Helper()
	d := pdfExport(t)
	d.Columns = []ResultColumn{{ID: "name", Label: "Ledger line", Type: Text}}
	cells := map[string]any{}
	for i := 1; i <= 30; i++ {
		id := fmt.Sprintf("c%02d", i)
		d.Columns = append(d.Columns, ResultColumn{ID: id, Label: fmt.Sprintf("Wide column heading %02d", i), Type: Text})
		cells[id] = fmt.Sprintf("a long cell value %02d", i)
	}
	d.Rows = []exportRow{
		{Row: Row{ID: "r1", Kind: KindDetail, Label: "First line", Cells: cells}, Cells: cells},
		{Row: Row{ID: "r2", Kind: KindDetail, Label: "Second line", Cells: cells}, Cells: cells},
	}
	return d
}

var pageSize = regexp.MustCompile(`size:\s*letter\s+(portrait|landscape)`)

// The page is Letter, portrait for a narrow report and landscape for a wide
// one.
func TestPDFHTMLPageOrientation(t *testing.T) {
	for name, c := range map[string]struct {
		d    *exportData
		want string
	}{
		"narrow": {pdfExport(t), "portrait"},
		"wide":   {wideExport(t), "landscape"},
	} {
		page := pdfHTML(t, c.d)
		m := pageSize.FindStringSubmatch(page)
		if m == nil || m[1] != c.want {
			t.Fatalf("%s: @page size %v, want letter %s:\n%s", name, m, c.want, page)
		}
	}
}

// labelExport is pdfExport with one detail row whose label is the given
// number of letters, and eight text columns of "($1,234,567.89)".
func labelExport(t *testing.T, letters int) *exportData {
	t.Helper()
	d := pdfExport(t)
	d.Columns = []ResultColumn{{ID: "name", Label: "Feed", Type: Text}}
	cells := map[string]any{}
	for i := 1; i <= 8; i++ {
		id := fmt.Sprintf("c%d", i)
		d.Columns = append(d.Columns, ResultColumn{ID: id, Label: fmt.Sprintf("C%d", i), Type: Text})
		cells[id] = "($1,234,567.89)"
	}
	d.Rows = []exportRow{{Row: Row{ID: "r1", Kind: KindDetail, Label: strings.Repeat("x", letters), Cells: cells}, Cells: cells}}
	return d
}

var (
	tableChunk  = regexp.MustCompile(`(?s)<table\b.*?</table>`)
	firstColumn = regexp.MustCompile(`<(?:col|th)\b[^>]*>`)
	widthPt     = regexp.MustCompile(`width:\s*([0-9.]+)pt`)
	firstTD     = regexp.MustCompile(`<td\b[^>]*>`)
	styleRule   = regexp.MustCompile(`([^{};]+)\{([^{}]*)\}`)
)

// labelColumnWidths is the width in pt that each table gives its label
// column, on its first <col> or <th>.
func labelColumnWidths(t *testing.T, page string) []float64 {
	t.Helper()
	tables := tableChunk.FindAllString(page, -1)
	if len(tables) == 0 {
		t.Fatalf("the page has no table:\n%s", page)
	}
	out := make([]float64, 0, len(tables))
	for _, table := range tables {
		first := firstColumn.FindString(table)
		m := widthPt.FindStringSubmatch(first)
		if m == nil {
			t.Fatalf("the label column %q has no width in pt:\n%s", first, table)
		}
		var w float64
		if _, err := fmt.Sscanf(m[1], "%g", &w); err != nil {
			t.Fatalf("the label column's width %q: %v", m[1], err)
		}
		out = append(out, w)
	}
	return out
}

// The label column has the width the layout measured at the chosen type
// size, at most 240 pt, and a longer label wraps: only figure cells are
// nowrap.
func TestPDFHTMLGivesTheLabelColumnItsWidth(t *testing.T) {
	// 50 letters are 27.5 pt at 1 pt: the page is landscape at 7 pt, where the
	// label column is 27.5 × 7 + 8 = 200.5 pt.
	page := pdfHTML(t, labelExport(t, 50))
	widths := labelColumnWidths(t, page)
	if len(widths) != 1 || widths[0] < 199.5 || widths[0] > 201.5 {
		t.Fatalf("label column widths = %v, want one table with a label column of about 200.5 pt", widths)
	}
	// 80 letters are wider than 240 pt at 7 pt: the label column is 240 pt
	// in every table and the label wraps.
	page = pdfHTML(t, labelExport(t, 80))
	for _, w := range labelColumnWidths(t, page) {
		if w != 240 {
			t.Fatalf("a label column is %v pt wide, want 240 pt:\n%s", w, page)
		}
	}
	row := rowMarkup(t, page, strings.Repeat("x", 80))
	if cell := firstTD.FindString(row); strings.Contains(cell, "nowrap") || classesOf(cell)["num"] {
		t.Fatalf("the label cell %q does not wrap", cell)
	}
	for _, m := range styleRule.FindAllStringSubmatch(styleOf(t, page), -1) {
		if !strings.Contains(m[2], "nowrap") {
			continue
		}
		for _, selector := range strings.Split(m[1], ",") {
			if !strings.Contains(selector, ".num") {
				t.Fatalf("the style rule %q keeps more than figure cells on one line", strings.TrimSpace(m[0]))
			}
		}
	}
}

// A long report runs over pages: each page repeats the headings and ends with
// "Page n of y" and the made-at day and time in the reader's zone.
func TestPDFRepeatsHeadingsAndPageTextOnEveryPage(t *testing.T) {
	d := pdfExport(t)
	d.Columns = []ResultColumn{{ID: "name", Label: "Ledger line", Type: Text}, {ID: "amount", Label: "Charged amount", Type: Money, Currency: "USD"}}
	d.Notices = []Notice{}
	d.Rows = nil
	for i := 1; i <= 200; i++ {
		cells := map[string]any{"amount": int64(100 * i)}
		d.Rows = append(d.Rows, exportRow{Row: Row{ID: fmt.Sprintf("r%03d", i), Kind: KindDetail, Label: fmt.Sprintf("Line %03d", i), Cells: cells}, Cells: cells})
	}
	text, pages := pdfTextOf(t, writePDFOf(t, d))
	if pages < 2 {
		t.Fatalf("200 rows fill %d page, want more than one", pages)
	}
	for n := 1; n <= pages; n++ {
		if want := fmt.Sprintf("Page %d of %d", n, pages); strings.Count(text, want) != 1 {
			t.Fatalf("the text does not hold %q once:\n%s", want, text)
		}
	}
	// 22:07 UTC on November 14 is 16:07 in Chicago.
	// omnidoc gives no text per page, so each is counted: once per page.
	for _, want := range []string{"Ledger line", "Charged amount", "Nov 14, 2026, 16:07"} {
		if n := strings.Count(text, want); n != pages {
			t.Fatalf("%q is in the text %d times over %d pages:\n%s", want, n, pages, text)
		}
	}
}

// Columns that do not fit continue on further pages, each of which repeats
// the label column.
func TestPDFContinuesWideColumnsOnPagesWithTheLabelColumn(t *testing.T) {
	text, pages := pdfTextOf(t, writePDFOf(t, wideExport(t)))
	if pages < 2 {
		t.Fatalf("30 wide columns fit %d page, want more than one", pages)
	}
	// omnidoc gives no text per page: the label column's heading is counted,
	// once per page.
	if n := strings.Count(text, "Ledger line"); n != pages {
		t.Fatalf("the label column's heading is on %d of %d pages:\n%s", n, pages, text)
	}
	for _, want := range []string{"Wide column heading 01", "Wide column heading 30", "a long cell value 30"} {
		if !strings.Contains(text, want) {
			t.Fatalf("the text lacks %q:\n%s", want, text)
		}
	}
}

// An empty report prints the header and the view's empty text, no table.
func TestPDFOfAnEmptyReport(t *testing.T) {
	d := pdfExport(t)
	d.Rows = []exportRow{}
	text, _ := pdfTextOf(t, writePDFOf(t, d))
	for _, want := range []string{"Zoo Trust", "Feeding Costs", "No rows for January 1 – March 31, 2026"} {
		if !strings.Contains(text, want) {
			t.Fatalf("the text lacks %q:\n%s", want, text)
		}
	}
	for _, heading := range []string{"Amount", "Share"} {
		if strings.Contains(text, heading) {
			t.Fatalf("an empty report has the table heading %q:\n%s", heading, text)
		}
	}
}

// emptyFeedingsSpec is a report of feedings in a period, with extra spec keys
// (for example its own empty text).
func emptyFeedingsSpec(extra string) string {
	return `{"version":4,"source":"zoo.feedings",` + extra + `
		"params":[{"id":"period","type":"dateRange","default":{"preset":"thisFiscalYear"}}],
		"columns":[{"id":"note","path":[{"field":"note"}]},{"id":"cost","path":[{"field":"cost"}]}],
		"filter":{"op":"between","args":[{"path":[{"field":"fed"}]},{"param":"period"}]}}`
}

// An empty report with its own empty text prints that text, also when it has
// a period; without one it prints the engine's "No rows".
func TestPDFOfAnEmptyReportPrintsItsOwnEmptyText(t *testing.T) {
	s := newSavedZoo(t)
	hermeticExportFonts(t)
	// ot keeps no animal, so the report has no row for ot.
	own := s.saved(t, "own", "ann", "Feeding Notes", emptyFeedingsSpec(`"emptyText":"No feedings to show",`), true)
	text, _ := pdfTextOf(t, buildExportAs(t, s, "ot", own.Id, "pdf", `{}`, `{"today":"2026-11-16"}`).Bytes)
	if !strings.Contains(text, "No feedings to show") {
		t.Fatalf("the PDF lacks the report's own empty text:\n%s", text)
	}
	if strings.Contains(text, "No rows") {
		t.Fatalf("the PDF prints the engine's empty text over the report's own:\n%s", text)
	}
	plain := s.saved(t, "plain", "ann", "Feeding Notes", emptyFeedingsSpec(``), true)
	text, _ = pdfTextOf(t, buildExportAs(t, s, "ot", plain.Id, "pdf", `{}`, `{"today":"2026-11-16"}`).Bytes)
	if !strings.Contains(text, "No rows") {
		t.Fatalf("a report without its own empty text does not print No rows:\n%s", text)
	}
}

// pdfWord matches a note as a whole word of the PDF text.
func pdfWord(note string) *regexp.Regexp {
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(note) + `\b`)
}

// The PDF holds the rows the view shows: a folded group's detail rows are
// not in it.
func TestPDFKeepsTheReadersFolds(t *testing.T) {
	s := newSavedZoo(t)
	hermeticExportFonts(t)
	rep := s.saved(t, "species", "ann", "Feedings by Species", feedingsBySpeciesCSV, false)
	file := buildExportAs(t, s, "ann", rep.Id, "pdf", `{}`, `{"expandAll":true,"collapse":[["lion","USD"]],"today":"2026-11-16"}`)
	text, _ := pdfTextOf(t, file.Bytes)
	for _, note := range []string{"f1", "f2"} {
		if pdfWord(note).MatchString(text) {
			t.Fatalf("the folded lion group's row %s is in the PDF:\n%s", note, text)
		}
	}
	for _, note := range []string{"f3", "f4"} {
		if !pdfWord(note).MatchString(text) {
			t.Fatalf("the open zebra group's row %s is not in the PDF:\n%s", note, text)
		}
	}
}

// The reader's sort orders the detail rows.
func TestPDFFollowsTheReadersSort(t *testing.T) {
	s := newSavedZoo(t)
	hermeticExportFonts(t)
	rep := s.saved(t, "species", "ann", "Feedings by Species", feedingsBySpeciesCSV, false)
	file := buildExportAs(t, s, "ann", rep.Id, "pdf", `{}`, `{"expandAll":true,"sort":[{"col":"note","dir":"desc"}],"today":"2026-11-16"}`)
	text, _ := pdfTextOf(t, file.Bytes)
	at := func(note string) int {
		loc := pdfWord(note).FindStringIndex(text)
		if loc == nil {
			t.Fatalf("%s is not in the PDF:\n%s", note, text)
		}
		return loc[0]
	}
	if at("f2") > at("f1") || at("f4") > at("f3") {
		t.Fatalf("the detail rows are not in descending note order:\n%s", text)
	}
}

// A hidden column's heading is not in the PDF.
func TestPDFLeavesOutHiddenColumns(t *testing.T) {
	s := newSavedZoo(t)
	hermeticExportFonts(t)
	rep := s.saved(t, "species", "ann", "Feedings by Species", feedingsBySpeciesCSV, false)
	shown, _ := pdfTextOf(t, buildExportAs(t, s, "ann", rep.Id, "pdf", `{}`, `{"expandAll":true,"today":"2026-11-16"}`).Bytes)
	if !strings.Contains(shown, "Grams") {
		t.Fatalf("the Grams heading is not in the PDF of the report with every column shown:\n%s", shown)
	}
	hidden, _ := pdfTextOf(t, buildExportAs(t, s, "ann", rep.Id, "pdf", `{}`, `{"expandAll":true,"display":{"hidden":["grams"]},"today":"2026-11-16"}`).Bytes)
	if strings.Contains(hidden, "Grams") {
		t.Fatalf("the hidden Grams column is in the PDF:\n%s", hidden)
	}
}
