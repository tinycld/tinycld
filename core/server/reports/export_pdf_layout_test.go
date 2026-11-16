package reports

import (
	"slices"
	"testing"
)

// layoutColumnPadding is the padding of a column at any type size.
const layoutColumnPadding = 8

// setWidth is a column set's width at size pt: the label column, then each
// column's text width at 1 pt scaled to pt, with its padding.
func setWidth(widths []float64, set []int, labelWidth float64, pt int) float64 {
	total := labelWidth
	for _, i := range set {
		total += widths[i]*float64(pt) + layoutColumnPadding
	}
	return total
}

// fixedLabel is a label column of the same width at every type size.
func fixedLabel(width float64) func(pt int) float64 {
	return func(int) float64 { return width }
}

// textLabel is a label column whose widest text is textWidth at 1 pt: that
// text at pt, with its padding, within 120 to 240 pt.
func textLabel(textWidth float64) func(pt int) float64 {
	return func(pt int) float64 {
		return min(240, max(120, textWidth*float64(pt)+layoutColumnPadding))
	}
}

// requireSetsCoverInOrder fails unless every column is in exactly one set,
// in column order, and no set is empty.
func requireSetsCoverInOrder(t *testing.T, sets [][]int, columns int) {
	t.Helper()
	var all []int
	for _, set := range sets {
		if len(set) == 0 {
			t.Fatalf("an empty column set in %v", sets)
		}
		all = append(all, set...)
	}
	want := make([]int, columns)
	for i := range want {
		want[i] = i
	}
	if !slices.Equal(all, want) {
		t.Fatalf("sets %v do not hold every column once, in order", sets)
	}
}

// pdfLayout picks the page and type size: portrait at 9 pt, else landscape at
// 9, 8 or 7 pt, else landscape at 7 pt with column sets. Widths are text
// widths at 1 pt; the label column is 120 pt.
func TestPDFLayoutByTable(t *testing.T) {
	label := fixedLabel(120)
	for name, c := range map[string]struct {
		widths    []float64
		landscape bool
		pt        int
	}{
		// 120 + 2 × (90 + 8) = 316 pt at 9 pt: fits 540.
		"fits portrait at 9": {[]float64{10, 10}, false, 9},
		// 120 + 2 × (225 + 8) = 586 pt at 9 pt: over 540, within 720.
		"fits landscape at 9": {[]float64{25, 25}, true, 9},
		// 766 pt at 9, 696 pt at 8.
		"fits landscape at 8 only": {[]float64{35, 35}, true, 8},
		// 776 pt at 8, 696 pt at 7.
		"fits landscape at 7 only": {[]float64{40, 40}, true, 7},
	} {
		t.Run(name, func(t *testing.T) {
			plan := pdfLayout(c.widths, label)
			if plan.Landscape != c.landscape || plan.Pt != c.pt {
				t.Fatalf("pdfLayout = landscape %v at %d pt, want landscape %v at %d pt", plan.Landscape, plan.Pt, c.landscape, c.pt)
			}
			if len(plan.Sets) != 1 {
				t.Fatalf("sets = %v, want one set", plan.Sets)
			}
			requireSetsCoverInOrder(t, plan.Sets, len(c.widths))
		})
	}
}

// The label column is measured at each type size tried, so a table whose
// label column is narrower at 7 pt fits one landscape table at 7 pt. A label
// of 50 letters is 27.5 pt wide at 1 pt: 240 pt (the most) at 9 pt, 228 pt at
// 8 pt, 200.5 pt at 7 pt. Eight columns of "($1,234,567.89)" are 7.5 pt wide
// at 1 pt each: 544 pt at 8 pt, 484 pt at 7 pt. At 7 pt the table is
// 684.5 pt, within 720; with the 9 pt label it would be 724 pt.
func TestPDFLayoutMeasuresTheLabelColumnAtEachSize(t *testing.T) {
	widths := slices.Repeat([]float64{7.5}, 8)
	plan := pdfLayout(widths, textLabel(27.5))
	if !plan.Landscape || plan.Pt != 7 {
		t.Fatalf("pdfLayout = landscape %v at %d pt, want landscape at 7 pt", plan.Landscape, plan.Pt)
	}
	if len(plan.Sets) != 1 {
		t.Fatalf("sets = %v, want one set: the table is 684.5 pt at 7 pt with the label measured at 7 pt", plan.Sets)
	}
	requireSetsCoverInOrder(t, plan.Sets, len(widths))
}

// Columns that do not fit at 7 pt continue in sets, each within the
// landscape width beside the label column measured at 7 pt, and each as full
// as fits.
func TestPDFLayoutSplitsColumnsIntoSets(t *testing.T) {
	// The label column is 240 pt at 9 and 8 pt, 120 pt at 7 pt.
	label := func(pt int) float64 {
		if pt == 7 {
			return 120
		}
		return 240
	}
	widths := make([]float64, 10)
	for i := range widths {
		widths[i] = 20 // 148 pt each at 7 pt: 1 480 pt in all
	}
	plan := pdfLayout(widths, label)
	if !plan.Landscape || plan.Pt != 7 {
		t.Fatalf("pdfLayout = landscape %v at %d pt, want landscape at 7 pt", plan.Landscape, plan.Pt)
	}
	if len(plan.Sets) < 2 {
		t.Fatalf("sets = %v, want more than one", plan.Sets)
	}
	requireSetsCoverInOrder(t, plan.Sets, len(widths))
	for n, set := range plan.Sets {
		if w := setWidth(widths, set, label(7), 7); w > 720 {
			t.Fatalf("set %v is %v pt wide with the label column, over 720", set, w)
		}
		if n == len(plan.Sets)-1 {
			continue
		}
		// Four columns (592 pt) fit beside the 120 pt label column.
		grown := append(slices.Clone(set), plan.Sets[n+1][0])
		if w := setWidth(widths, grown, label(7), 7); w <= 720 {
			t.Fatalf("set %v is not full: with the next column it is %v pt, within 720 (sets %v)", set, w, plan.Sets)
		}
	}
}

// A column wider than the page alone still gets a set of its own.
func TestPDFLayoutGivesAnOverwideColumnItsOwnSet(t *testing.T) {
	widths := []float64{10, 200, 10}
	plan := pdfLayout(widths, fixedLabel(120))
	if !plan.Landscape || plan.Pt != 7 {
		t.Fatalf("pdfLayout = landscape %v at %d pt, want landscape at 7 pt", plan.Landscape, plan.Pt)
	}
	requireSetsCoverInOrder(t, plan.Sets, len(widths))
	if !slices.ContainsFunc(plan.Sets, func(set []int) bool { return slices.Equal(set, []int{1}) }) {
		t.Fatalf("sets = %v, want the 1 400 pt column alone in a set", plan.Sets)
	}
}
