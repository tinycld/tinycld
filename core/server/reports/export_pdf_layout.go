package reports

import (
	"strings"
	"unicode/utf8"
)

// The PDF's page widths inside 36 pt margins on Letter paper, its type sizes,
// and the label column's bounds.
const (
	pdfPortraitWidth  = 540
	pdfLandscapeWidth = 720
	pdfLargestPt      = 9
	pdfSmallestPt     = 7
	// pdfColumnPadding is a column's padding at any type size.
	pdfColumnPadding = 8
	// pdfIndentPt is a row label's indent per depth level.
	pdfIndentPt   = 12
	pdfLabelMinPt = 120
	pdfLabelMaxPt = 240
)

// pdfPlan is how a PDF lays its table out: the page's orientation, the type
// size, and the column sets (indexes into the columns after the label
// column), each printed as its own table beside the label column.
type pdfPlan struct {
	Landscape bool
	Pt        int
	Sets      [][]int
}

// pdfTextWidth is a text's width at 1 pt: 0.5 pt per digit, sign and
// separator, 0.55 pt per other character.
func pdfTextWidth(text string) float64 {
	narrow := 0
	for _, r := range text {
		if (r >= '0' && r <= '9') || strings.ContainsRune("+-−.,:/%()$ ", r) {
			narrow++
		}
	}
	return float64(narrow)*0.5 + float64(utf8.RuneCountInString(text)-narrow)*0.55
}

// tableWidth is the width of the label column and every column at size pt.
func tableWidth(widths []float64, labelWidth float64, pt int) float64 {
	total := labelWidth
	for _, w := range widths {
		total += w*float64(pt) + pdfColumnPadding
	}
	return total
}

// pdfLayout picks the page and the type size for columns of the given text
// widths at 1 pt beside a label column whose width label gives at each type
// size: portrait at 9 pt; else landscape at 9, 8 or 7 pt; else landscape at
// 7 pt with the columns in sets, filled in column order, each set as wide as
// fits beside the label column at 7 pt. A column wider than that space alone
// gets a set of its own.
func pdfLayout(widths []float64, label func(pt int) float64) pdfPlan {
	all := make([]int, len(widths))
	for i := range all {
		all[i] = i
	}
	if tableWidth(widths, label(pdfLargestPt), pdfLargestPt) <= pdfPortraitWidth {
		return pdfPlan{Landscape: false, Pt: pdfLargestPt, Sets: [][]int{all}}
	}
	for pt := pdfLargestPt; pt >= pdfSmallestPt; pt-- {
		if tableWidth(widths, label(pt), pt) <= pdfLandscapeWidth {
			return pdfPlan{Landscape: true, Pt: pt, Sets: [][]int{all}}
		}
	}
	room := pdfLandscapeWidth - label(pdfSmallestPt)
	sets := [][]int{}
	set := []int{}
	used := 0.0
	for i, w := range widths {
		width := w*pdfSmallestPt + pdfColumnPadding
		if len(set) > 0 && used+width > room {
			sets = append(sets, set)
			set, used = []int{}, 0
		}
		set = append(set, i)
		used += width
	}
	if len(set) > 0 {
		sets = append(sets, set)
	}
	return pdfPlan{Landscape: true, Pt: pdfSmallestPt, Sets: sets}
}

// pdfLabelWidth gives the label column's width in pt at a type size: its
// widest text (a row's label with its indent, or the heading) at that size,
// with its padding, within 120 to 240 pt (a longer label wraps).
func pdfLabelWidth(heading string, rows []pdfRowText) func(pt int) float64 {
	return func(pt int) float64 {
		size := float64(pt)
		widest := pdfTextWidth(heading) * size
		for _, r := range rows {
			widest = max(widest, pdfTextWidth(r.Label)*size+float64(pdfIndentPt*r.Depth))
		}
		return min(pdfLabelMaxPt, max(pdfLabelMinPt, widest+pdfColumnPadding))
	}
}

// pdfRowText is a row's label and depth, as the label column's width reads
// them.
type pdfRowText struct {
	Label string
	Depth int
}
