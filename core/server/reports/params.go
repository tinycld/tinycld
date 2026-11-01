package reports

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"time"
)

// Param types. date and asOf hold one day (asOf offers as-of presets),
// dateRange a from and a to day, choice one option (or several with
// Multiple), text a string, number a number.
const (
	ParamDate      = "date"
	ParamAsOf      = "asOf"
	ParamDateRange = "dateRange"
	ParamChoice    = "choice"
	ParamText      = "text"
	ParamNumber    = "number"
)

const (
	maxParams    = 20
	maxParamText = 500
)

var idRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)

// rangePresets and asOfPresets are the shell's period presets.
var (
	rangePresets = []string{
		"today", "thisMonth", "thisMonthToDate", "thisQuarter", "thisQuarterToDate",
		"thisFiscalYear", "thisFiscalYearToDate", "lastMonth", "lastQuarter",
		"lastFiscalYear", "last12Months",
	}
	asOfPresets = []string{"today", "endOfLastMonth", "endOfLastQuarter", "endOfLastFiscalYear"}
)

// paramValue is a param resolved for one run.
type paramValue struct {
	typ      string
	multiple bool
	unset    bool
	value    any      // a day, a text, a number or one choice
	list     []string // a multiple choice
	from     string   // a date range
	to       string
}

// periodContext is what a preset is resolved against.
type periodContext struct {
	today       time.Time
	fiscalStart int
}

// anyDay resolves a preset where its day does not matter: checking a
// default, or compiling a spec with no run.
var anyDay = periodContext{today: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), fiscalStart: 1}

func checkParamDefs(defs []ParamDef) *SpecError {
	if len(defs) > maxParams {
		return invalid("params", fmt.Sprintf("a report has at most %d params", maxParams))
	}
	seen := map[string]bool{}
	for i, d := range defs {
		at := fmt.Sprintf("params.%d", i)
		if !idRe.MatchString(d.ID) || seen[d.ID] {
			return invalid(at+".id", "a param needs a unique id of letters, digits and _")
		}
		seen[d.ID] = true
		switch d.Type {
		case ParamDate, ParamAsOf, ParamDateRange, ParamText, ParamNumber:
			if len(d.Options) > 0 || d.Multiple {
				return invalid(at, "only a choice param has options")
			}
		case ParamChoice:
			if len(d.Options) == 0 {
				return invalid(at+".options", "a choice param lists its options")
			}
		default:
			return invalid(at+".type", fmt.Sprintf("unknown param type %q", d.Type))
		}
		if d.Default != nil {
			// A preset default must be a real preset; its day does not matter.
			if _, err := parseParam(d, d.Default, anyDay, at+".default"); err != nil {
				return err
			}
		}
	}
	return nil
}

// resolveParams gives every param its value for this run: the posted value,
// else its default, else unset. A posted id the spec does not declare is an
// error.
func resolveParams(defs []ParamDef, given map[string]json.RawMessage, pc periodContext) (map[string]paramValue, *SpecError) {
	out := map[string]paramValue{}
	for id := range given {
		if !slices.ContainsFunc(defs, func(d ParamDef) bool { return d.ID == id }) {
			return nil, invalid("params."+id, fmt.Sprintf("the report has no param %q", id))
		}
	}
	for _, d := range defs {
		// A posted null clears the param; only an absent one takes its default.
		raw, at := given[d.ID], "params."+d.ID
		if raw == nil {
			raw, at = d.Default, "params."+d.ID+".default"
		}
		v, err := parseParam(d, raw, pc, at)
		if err != nil {
			return nil, err
		}
		out[d.ID] = v
	}
	return out, nil
}

// ResolvedParam is one param as a run resolved it: the posted value, else its
// default, else unset. A preset is given as its days, so the shell shows the
// period the server ran without repeating the fiscal-year rules.
type ResolvedParam struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Unset bool   `json:"unset,omitempty"`
	// omitempty leaves out only a nil interface: a resolved 0 or "" is kept.
	Value any      `json:"value,omitempty"`
	List  []string `json:"list,omitempty"`
	From  string   `json:"from,omitempty"`
	To    string   `json:"to,omitempty"`
}

// resolvedParams lists defs in order with the values a run resolved for them.
func resolvedParams(defs []ParamDef, values map[string]paramValue) []ResolvedParam {
	out := make([]ResolvedParam, 0, len(defs))
	for _, d := range defs {
		v := values[d.ID]
		rp := ResolvedParam{ID: d.ID, Type: d.Type, Unset: v.unset}
		if !v.unset {
			rp.Value, rp.List, rp.From, rp.To = v.value, v.list, v.from, v.to
		}
		out = append(out, rp)
	}
	return out
}

func parseParam(d ParamDef, raw json.RawMessage, pc periodContext, at string) (paramValue, *SpecError) {
	out := paramValue{typ: d.Type, multiple: d.Multiple}
	if raw == nil || string(bytes.TrimSpace(raw)) == "null" {
		out.unset = true
		return out, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return out, invalid(at, "the value is not valid JSON")
	}
	switch d.Type {
	case ParamDate, ParamAsOf:
		day, err := dayValue(v, d.Type, pc, at)
		if err != nil {
			return out, err
		}
		out.value = day
	case ParamDateRange:
		from, to, err := rangeValue(v, pc, at)
		if err != nil {
			return out, err
		}
		out.from, out.to = from, to
	case ParamText:
		s, ok := v.(string)
		if !ok || len(s) > maxParamText {
			return out, invalid(at, fmt.Sprintf("the value is text of at most %d characters", maxParamText))
		}
		out.value = s
	case ParamNumber:
		n, ok := v.(json.Number)
		if !ok {
			return out, invalid(at, "the value is a number")
		}
		out.value = numberValue(n)
	case ParamChoice:
		return choiceValue(d, v, at)
	}
	return out, nil
}

func choiceValue(d ParamDef, v any, at string) (paramValue, *SpecError) {
	out := paramValue{typ: d.Type, multiple: d.Multiple}
	valid := func(s string) bool {
		return slices.ContainsFunc(d.Options, func(o Option) bool { return o.Value == s })
	}
	if !d.Multiple {
		s, ok := v.(string)
		if !ok || !valid(s) {
			return out, invalid(at, "the value is one of the param's options")
		}
		out.value = s
		return out, nil
	}
	items, ok := v.([]any)
	if !ok {
		return out, invalid(at, "the value is a list of the param's options")
	}
	for _, item := range items {
		s, ok := item.(string)
		if !ok || !valid(s) {
			return out, invalid(at, "the value is a list of the param's options")
		}
		out.list = append(out.list, s)
	}
	// An empty list is a cleared chip: it filters nothing, as an unset param does.
	out.unset = len(out.list) == 0
	return out, nil
}

func dayValue(v any, typ string, pc periodContext, at string) (string, *SpecError) {
	if s, ok := v.(string); ok {
		if !isDay(s) {
			return "", invalid(at, "a date is YYYY-MM-DD")
		}
		return s, nil
	}
	preset, ok := presetOf(v)
	presets := []string{"today"}
	if typ == ParamAsOf {
		presets = asOfPresets
	}
	if !ok || !slices.Contains(presets, preset) {
		return "", invalid(at, fmt.Sprintf("the value is a date or a preset of %v", presets))
	}
	return asOfDay(preset, pc), nil
}

func rangeValue(v any, pc periodContext, at string) (string, string, *SpecError) {
	if preset, ok := presetOf(v); ok {
		if !slices.Contains(rangePresets, preset) {
			return "", "", invalid(at, fmt.Sprintf("the preset is one of %v", rangePresets))
		}
		from, to := rangeDays(preset, pc)
		return from, to, nil
	}
	m, ok := v.(map[string]any)
	from, okFrom := m["from"].(string)
	to, okTo := m["to"].(string)
	if !ok || len(m) != 2 || !okFrom || !okTo || !isDay(from) || !isDay(to) {
		return "", "", invalid(at, `a date range is {"from": "YYYY-MM-DD", "to": "YYYY-MM-DD"} or {"preset": …}`)
	}
	if from > to {
		return "", "", invalid(at, "from must be on or before to")
	}
	return from, to, nil
}

func presetOf(v any) (string, bool) {
	m, ok := v.(map[string]any)
	if !ok || len(m) != 1 {
		return "", false
	}
	p, ok := m["preset"].(string)
	return p, ok
}

// isDay is the shell's day rule: YYYY-MM-DD, a real day, year 1900 or later.
func isDay(s string) bool {
	day, err := time.Parse(time.DateOnly, s)
	return err == nil && day.Year() >= 1900
}

// Months are counted from year 0 (index = year*12 + month-1), as the shell's
// period presets count them.
func monthIndex(day time.Time) int { return day.Year()*12 + int(day.Month()) - 1 }

func firstDay(index int) string {
	return time.Date(index/12, time.Month(index%12+1), 1, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
}

func lastDay(index int) string {
	return time.Date(index/12, time.Month(index%12+1)+1, 0, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
}

// marks are the month indexes of today, its fiscal year's first month and
// its fiscal quarter's first month.
func marks(pc periodContext) (today, fyStart, quarterStart int) {
	today = monthIndex(pc.today)
	start := pc.fiscalStart
	if start < 1 || start > 12 {
		start = 1
	}
	year := pc.today.Year()
	if int(pc.today.Month()) < start {
		year--
	}
	fyStart = year*12 + start - 1
	quarterStart = fyStart + (today-fyStart)/3*3
	return today, fyStart, quarterStart
}

func rangeDays(preset string, pc periodContext) (string, string) {
	today, fy, q := marks(pc)
	day := pc.today.Format(time.DateOnly)
	switch preset {
	case "today":
		return day, day
	case "thisMonth":
		return firstDay(today), lastDay(today)
	case "thisMonthToDate":
		return firstDay(today), day
	case "thisQuarter":
		return firstDay(q), lastDay(q + 2)
	case "thisQuarterToDate":
		return firstDay(q), day
	case "thisFiscalYear":
		return firstDay(fy), lastDay(fy + 11)
	case "thisFiscalYearToDate":
		return firstDay(fy), day
	case "lastMonth":
		return firstDay(today - 1), lastDay(today - 1)
	case "lastQuarter":
		return firstDay(q - 3), lastDay(q - 1)
	case "lastFiscalYear":
		return firstDay(fy - 12), lastDay(fy - 1)
	}
	return firstDay(today - 12), lastDay(today - 1) // last12Months
}

func asOfDay(preset string, pc periodContext) string {
	today, fy, q := marks(pc)
	switch preset {
	case "endOfLastMonth":
		return lastDay(today - 1)
	case "endOfLastQuarter":
		return lastDay(q - 1)
	case "endOfLastFiscalYear":
		return lastDay(fy - 1)
	}
	return pc.today.Format(time.DateOnly)
}

// paramExpr is a resolved param as a formula leaf.
func paramExpr(c fctx, v paramValue) expr {
	switch v.typ {
	case ParamDateRange:
		if v.unset {
			return expr{rng: true, lo: "NULL", hi: "NULL", unset: true}
		}
		return expr{rng: true, lo: c.bind(v.from), hi: c.bind(v.to)}
	case ParamChoice:
		if v.multiple {
			parts := make([]any, len(v.list))
			for i, s := range v.list {
				parts[i] = s
			}
			return expr{sql: bindList(c, parts), typ: Text, list: true, unset: v.unset}
		}
	}
	typ := map[string]FieldType{ParamDate: Date, ParamAsOf: Date, ParamText: Text, ParamNumber: Number, ParamChoice: Text}[v.typ]
	if v.unset {
		return expr{sql: "NULL", typ: typ, unset: true}
	}
	return expr{sql: c.bind(v.value), typ: typ}
}

func bindList(c fctx, values []any) string {
	if len(values) == 0 {
		return "()"
	}
	out := "("
	for i, v := range values {
		if i > 0 {
			out += ", "
		}
		out += c.bind(v)
	}
	return out + ")"
}
