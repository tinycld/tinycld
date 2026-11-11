package reports

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	validation "github.com/pocketbase/ozzo-validation/v4"
)

// Column fit (spec v4, column-fit-design.md §3): the engine checks the
// settings; the client measures and lays the columns out.

const (
	maxLineClamp = 4
	maxFlexGrow  = 10
)

const neverCutMessage = "a figure is never cut: it takes only flexGrow and minWidth auto or max-content"

var fitLengthPattern = regexp.MustCompile(`^(\d{1,4}(?:\.\d+)?)(ch|px|%)$`)

// fitUnitRange is each unit's least and greatest value.
var fitUnitRange = map[string][2]float64{"ch": {1, 120}, "px": {1, 4000}, "%": {1, 100}}

// lengthRule is what one setting takes: its keywords and a length in its units.
type lengthRule struct {
	name     string
	keywords []string
	units    []string
}

var (
	allUnits     = []string{"ch", "px", "%"}
	widthRule    = lengthRule{name: "width", units: allUnits}
	minWidthRule = lengthRule{name: "minWidth", keywords: []string{"auto", "min-content", "max-content"}, units: allUnits}
	maxWidthRule = lengthRule{name: "maxWidth", keywords: []string{"none"}, units: allUnits}
	pxRule       = lengthRule{name: "minWidth", units: []string{"px"}}
)

// fitLength reads a length: its value and unit; false when s is not one.
func fitLength(s string) (float64, string, bool) {
	m := fitLengthPattern.FindStringSubmatch(s)
	if m == nil {
		return 0, "", false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	return v, m[2], err == nil
}

// orList is "a, b or c".
func orList(items []string) string {
	if len(items) == 1 {
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " or " + items[len(items)-1]
}

// fitLengthProblem is why rule refuses s, or "" when it takes it.
func fitLengthProblem(s string, rule lengthRule) string {
	if slices.Contains(rule.keywords, s) {
		return ""
	}
	v, unit, ok := fitLength(s)
	if !ok || !slices.Contains(rule.units, unit) {
		takes := append(slices.Clone(rule.keywords), "a length in "+orList(rule.units))
		return rule.name + " is " + orList(takes)
	}
	if r := fitUnitRange[unit]; v < r[0] || v > r[1] {
		return fmt.Sprintf("a length in %s is %g to %g", unit, r[0], r[1])
	}
	return ""
}

// columnFits checks display.oneLine and display.columns. Whether a setting
// could cut a figure needs the columns' types, so fitRefusals checks that
// after the dry compile.
func (b *binder) columnFits(p *plan) {
	d := p.spec.Display
	if d.OneLine != nil {
		if msg := fitLengthProblem(d.OneLine.MinWidth, pxRule); msg != "" {
			b.bad("display.oneLine.minWidth", msg)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(d.Columns)) {
		at := "display.columns." + id
		if b.column(p, id, at) != nil {
			b.columnFit(d.Columns[id], at)
		}
	}
}

// figureTypes are the cells shown whole (column-measure.ts FIGURE_TYPES), and
// typedLabelTypes the labels shown whole (column-measure.ts TYPED_LABELS).
var (
	figureTypes     = []FieldType{Number, Money, Percent, Date, DateTime, Bool}
	typedLabelTypes = []FieldType{Date, DateTime, Number, Money}
)

// resultType is a column's type and, when its group cells differ, their type:
// what ResultColumn.Type and TotalType say.
func (c *compiler) resultType(cp *colPlan) (typ, total FieldType) {
	typ = c.detailTypes[cp.col.ID]
	if typ == "" {
		typ = c.levelTypes[cp.col.ID]
	}
	if cp.path != nil {
		typ = cp.path.field.Type
		if cp.tm >= 0 {
			typ = aggregateType(cp.col.Aggregate, cp.path.field.Type, cp.scale)
		}
	}
	if t := c.levelTypes[cp.col.ID]; t != "" && t != typ && cp.col.Aggregate != "" {
		total = t
	}
	return typ, total
}

// neverCut is true for a column whose cells the client shows whole: a figure,
// or (first, the label column) a label typed as a date or an amount.
func (c *compiler) neverCut(cp *colPlan, first bool) bool {
	typ, total := c.resultType(cp)
	if slices.Contains(figureTypes, typ) || slices.Contains(figureTypes, total) {
		return true
	}
	if !first {
		return false
	}
	if t, ok := c.labelTypes[cp.col.ID]; ok && slices.Contains(typedLabelTypes, t) {
		return true
	}
	return c.detailLabel != nil && slices.Contains(typedLabelTypes, c.detailLabel.typ)
}

// fitRefusals refuses a fit that could cut a figure (decision 2): on such a
// column only flexGrow and a whole-value minWidth are taken.
func fitRefusals(c *compiler) error {
	d := c.p.spec.Display
	if d == nil || len(d.Columns) == 0 {
		return nil
	}
	problems := map[string]validation.Error{}
	// The label column is the spec's first column. c.p.cols keeps the spec's
	// order, and pivot columns (after every column, or under order "spec"
	// before their cell column, which is never first) never come first, so
	// the label is the same in both orders.
	label := c.p.spec.Columns[0].ID
	for _, cp := range c.p.cols {
		f, ok := d.Columns[cp.col.ID]
		if !ok || !c.neverCut(cp, cp.col.ID == label) {
			continue
		}
		at := "display.columns." + cp.col.ID + "."
		for _, s := range []struct {
			name  string
			isSet bool
		}{
			{"width", f.Width != ""},
			{"minWidth", f.MinWidth != "" && f.MinWidth != "auto" && f.MinWidth != "max-content"},
			{"maxWidth", f.MaxWidth != ""},
			{"whiteSpace", f.WhiteSpace != ""},
			{"lineClamp", f.LineClamp != 0},
			{"textOverflow", f.TextOverflow != ""},
		} {
			if s.isSet {
				problems[at+s.name] = validation.NewError("invalid", neverCutMessage)
			}
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return &SpecError{Status: 400, Problems: problems}
}

func (b *binder) columnFit(f ColumnFit, at string) {
	for _, s := range []struct {
		value string
		rule  lengthRule
	}{{f.Width, widthRule}, {f.MinWidth, minWidthRule}, {f.MaxWidth, maxWidthRule}} {
		if s.value == "" {
			continue
		}
		if msg := fitLengthProblem(s.value, s.rule); msg != "" {
			b.bad(at+"."+s.rule.name, msg)
		}
	}
	// Only lengths of one unit compare here; across units the client's CSS
	// rule (the min wins) decides at draw time.
	if lo, loUnit, ok := fitLength(f.MinWidth); ok {
		if hi, hiUnit, ok := fitLength(f.MaxWidth); ok && loUnit == hiUnit && lo > hi {
			b.bad(at+".minWidth", "minWidth is at most maxWidth")
		}
	}
	if f.WhiteSpace != "" && f.WhiteSpace != "nowrap" && f.WhiteSpace != "normal" {
		b.bad(at+".whiteSpace", "whiteSpace is nowrap or normal")
	}
	switch {
	case f.LineClamp == 0:
	case f.LineClamp < 1 || f.LineClamp > maxLineClamp:
		b.bad(at+".lineClamp", fmt.Sprintf("lineClamp is 1 to %d", maxLineClamp))
	case f.WhiteSpace != "normal":
		b.bad(at+".lineClamp", "lineClamp goes with whiteSpace normal")
	}
	if f.TextOverflow != "" && f.TextOverflow != "ellipsis" {
		b.bad(at+".textOverflow", "textOverflow is ellipsis")
	}
	if f.FlexGrow != nil && (*f.FlexGrow < 0 || *f.FlexGrow > maxFlexGrow) {
		b.bad(at+".flexGrow", fmt.Sprintf("flexGrow is 0 to %d", maxFlexGrow))
	}
}
