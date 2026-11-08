package reports

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Column fit (spec v4, column-fit-design.md §3): the engine checks the
// settings; the client measures and lays the columns out.

const (
	maxLineClamp = 4
	maxFlexGrow  = 10
)

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
