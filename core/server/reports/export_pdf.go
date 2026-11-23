package reports

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"math"
	"strconv"
	"strings"

	"tinycld.org/core/branding"
	"tinycld.org/core/docfonts"
	"tinycld.org/core/pdfrender"
)

//go:embed export_pdf.html
var exportPDFSource string

var exportPDFTemplate = template.Must(template.New("export_pdf").Parse(exportPDFSource))

// exportFontsHermetic makes a PDF export ignore the machine's fonts. Tests set
// it; production leaves it false.
var exportFontsHermetic bool

// pdfPage is what export_pdf.html prints. Every value that reaches the
// page's style is validated first: the orientation is a fixed word, the type
// size an int from pdfLayout, the brand color a #rrggbb color, the made-at
// text a quoted CSS string with every unusual character escaped, the label
// column's width an int of pt from the layout.
type pdfPage struct {
	Orientation  template.CSS
	Pt           template.CSS
	LabelWidth   template.CSS
	Brand        template.CSS
	MadeCSS      template.CSS
	Logo         template.URL
	Heading      string
	Title        string
	Lines        []string
	Notices      []string
	Empty        string
	LabelHeading string
	Sets         []pdfSet
}

// pdfSet is one table: the label column and one set of columns.
type pdfSet struct {
	Headings []pdfCell
	Rows     []pdfTableRow
}

// pdfTableRow is one row of a table: its class (weight and rule), the label's
// indent, the label and the set's cells.
type pdfTableRow struct {
	Class  string
	Indent template.CSS
	Label  string
	Cells  []pdfCell
}

// pdfCell is a heading or a cell: its text, whether it is a figure (right
// aligned), and its class.
type pdfCell struct {
	Text  string
	Num   bool
	Class string
}

// writePDF writes the report as a Letter PDF from export_pdf.html.
func writePDF(ctx context.Context, d *exportData) ([]byte, error) {
	page, err := exportPDFHTML(d)
	if err != nil {
		return nil, err
	}
	fonts := pdfrender.Fonts{Dir: docfonts.Dir(), Hermetic: exportFontsHermetic}
	return pdfrender.HTMLToPDF(ctx, page, fonts, pdfrender.Diagnostics(ctx, log))
}

// exportPDFHTML is the page a PDF export prints: a centered header on the
// first page (the logo, the heading, the title, the period in words, the
// zone line, "Amounts in thousands" and the params line), the notices one
// per line, then the rows the view shows in one table per column set, each
// with the label column. An empty report prints the view's empty text and no
// table. Every text comes from the export's formatter, as the view shows it.
func exportPDFHTML(d *exportData) ([]byte, error) {
	f := d.Format
	figures := d.Columns
	if len(figures) > 0 {
		figures = figures[1:]
	}
	labelHeading := ""
	if len(d.Columns) > 0 {
		labelHeading = f.Heading(d.Columns[0])
	}
	headings := make([]pdfCell, len(figures))
	widths := make([]float64, len(figures))
	for i, col := range figures {
		headings[i] = pdfCell{Text: f.Heading(col), Num: isFigureColumn(col)}
		widths[i] = pdfTextWidth(headings[i].Text)
	}
	rows := make([]pdfTableRow, len(d.Rows))
	labels := make([]pdfRowText, len(d.Rows))
	for n, r := range d.Rows {
		depth := max(r.Row.Depth, 0)
		rows[n] = pdfTableRow{
			Class:  pdfRowClass(r.Row.Kind),
			Indent: template.CSS("padding-left: " + strconv.Itoa(pdfIndentPt*depth) + "pt"),
			Label:  f.RowLabel(r.Row),
			Cells:  make([]pdfCell, len(figures)),
		}
		labels[n] = pdfRowText{Label: rows[n].Label, Depth: depth}
		for i, col := range figures {
			rows[n].Cells[i] = pdfCellOf(f, col, r)
			widths[i] = max(widths[i], pdfTextWidth(rows[n].Cells[i].Text))
		}
	}
	labelWidth := pdfLabelWidth(labelHeading, labels)
	plan := pdfLayout(widths, labelWidth)

	page := pdfPage{
		Orientation:  "portrait",
		Pt:           template.CSS(strconv.Itoa(plan.Pt)),
		LabelWidth:   template.CSS("width: " + strconv.Itoa(int(math.Round(labelWidth(plan.Pt)))) + "pt"),
		Brand:        template.CSS(brandColor(d.Org.Color)),
		MadeCSS:      template.CSS(cssString(f.MadeText(d.Made))),
		Logo:         logoURL(d.Org),
		Heading:      d.headingText(),
		Title:        d.Title,
		Lines:        d.pdfLines(f),
		Notices:      noticeLines(d.Notices),
		LabelHeading: labelHeading,
	}
	if plan.Landscape {
		page.Orientation = "landscape"
	}
	if len(d.Rows) == 0 {
		page.Empty = d.emptyText()
	} else {
		page.Sets = pdfSets(plan.Sets, headings, rows)
	}
	var out bytes.Buffer
	if err := exportPDFTemplate.Execute(&out, page); err != nil {
		return nil, fmt.Errorf("export pdf: %w", err)
	}
	return out.Bytes(), nil
}

// pdfSets cuts the headings and each row's cells into the plan's column
// sets.
func pdfSets(sets [][]int, headings []pdfCell, rows []pdfTableRow) []pdfSet {
	out := make([]pdfSet, 0, len(sets))
	for _, set := range sets {
		s := pdfSet{Headings: make([]pdfCell, 0, len(set)), Rows: make([]pdfTableRow, len(rows))}
		for _, i := range set {
			s.Headings = append(s.Headings, headings[i])
		}
		for n, r := range rows {
			cut := r
			cut.Cells = make([]pdfCell, 0, len(set))
			for _, i := range set {
				cut.Cells = append(cut.Cells, r.Cells[i])
			}
			s.Rows[n] = cut
		}
		out = append(out, s)
	}
	return out
}

// pdfCellOf is one cell as the view shows it: its text, right aligned when
// it is a figure, red when the view draws it red. A cell the row does not
// have is empty.
func pdfCellOf(f *exportFormatter, col ResultColumn, r exportRow) pdfCell {
	cell := pdfCell{Num: isFigureColumn(col)}
	value, ok := r.Cells[col.ID]
	if !ok {
		cell.Class = figureClass(cell.Num, false)
		return cell
	}
	text := f.CellText(col, r.Row, value)
	cell.Text = text.Text
	cell.Class = figureClass(cell.Num, text.IsRed)
	return cell
}

// figureClass is a cell's class: num for a figure, red for a red figure.
func figureClass(num, red bool) string {
	classes := []string{}
	if num {
		classes = append(classes, "num")
	}
	if red {
		classes = append(classes, "red")
	}
	return strings.Join(classes, " ")
}

// isFigureColumn says a column holds figures: money, numbers or percents.
func isFigureColumn(col ResultColumn) bool {
	switch col.Type {
	case Money, Number, Percent:
		return true
	}
	return col.TotalType == Money
}

// pdfRowClass is a row's weight and rule: group and computed rows semibold,
// a rule above a subtotal, a heavier rule above a total.
func pdfRowClass(kind string) string {
	switch kind {
	case KindGroup, KindComputed:
		return "semibold"
	case KindSubtotal:
		return "rule"
	case KindTotal:
		return "rule-heavy"
	}
	return ""
}

// brandColor is the org's brand color when it is a #rrggbb color, else the
// default.
func brandColor(color string) string {
	if branding.IsValidColor(color) {
		return strings.ToLower(color)
	}
	return branding.DefaultColor
}

// logoURL is the org's logo as a data: URI; "" when it has none or the logo
// is not an image a document embeds.
func logoURL(org branding.Org) template.URL {
	if len(org.Logo) == 0 || !branding.IsLogoType(org.LogoType) {
		return ""
	}
	return template.URL("data:" + org.LogoType + ";base64," + base64.StdEncoding.EncodeToString(org.Logo))
}

// thousandsLine is "Amounts in thousands" when that option is on.
func thousandsLine(display exportDisplay) string {
	if display.DivideBy1000 {
		return "Amounts in thousands"
	}
	return ""
}

// noticeLines are the notices' sentences, one line per notice.
func noticeLines(notices []Notice) []string {
	out := make([]string, 0, len(notices))
	for _, n := range notices {
		if text := strings.Join(sentencesOf(n), " "); text != "" {
			out = append(out, text)
		}
	}
	return out
}

// pdfLines are the header's lines under the title: the period, the spec's
// header lines, the zone line, the thousands line and the params line, each
// that has text.
func (d *exportData) pdfLines(f *exportFormatter) []string {
	lines := append([]string{d.periodWords()}, d.SpecLines...)
	return nonEmpty(append(lines, f.ZoneLine(d.Columns), thousandsLine(d.Display), d.paramsLine())...)
}

// nonEmpty is the lines that have text.
func nonEmpty(lines ...string) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// cssString quotes text as a CSS string: letters, digits, spaces and
// , . : as they are, any other character as a CSS hex escape.
func cssString(text string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range text {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == ' ', r == ',', r == '.', r == ':':
			b.WriteRune(r)
		default:
			fmt.Fprintf(&b, "\\%06x", r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
