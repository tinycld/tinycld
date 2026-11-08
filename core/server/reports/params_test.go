package reports

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func day(s string) time.Time {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return d
}

func TestRangePresetsMatchTheShell(t *testing.T) {
	july := periodContext{today: day("2026-08-15"), fiscalStart: 7}
	january := periodContext{today: day("2026-02-10"), fiscalStart: 1}
	// February with a July fiscal start: today is before the fiscal start month, so the
	// fiscal year began the previous calendar year.
	february := periodContext{today: day("2026-02-10"), fiscalStart: 7}
	// January with a March fiscal start: the fiscal year began in March 2025.
	marchStart := periodContext{today: day("2026-01-15"), fiscalStart: 3}
	cases := []struct {
		preset   string
		pc       periodContext
		from, to string
	}{
		{"today", july, "2026-08-15", "2026-08-15"},
		{"thisMonth", july, "2026-08-01", "2026-08-31"},
		{"thisMonthToDate", july, "2026-08-01", "2026-08-15"},
		{"thisQuarter", july, "2026-07-01", "2026-09-30"},
		{"thisQuarterToDate", july, "2026-07-01", "2026-08-15"},
		{"thisFiscalYear", july, "2026-07-01", "2027-06-30"},
		{"thisFiscalYearToDate", july, "2026-07-01", "2026-08-15"},
		{"lastMonth", july, "2026-07-01", "2026-07-31"},
		{"lastQuarter", july, "2026-04-01", "2026-06-30"},
		{"lastFiscalYear", july, "2025-07-01", "2026-06-30"},
		{"last12Months", july, "2025-08-01", "2026-07-31"},
		{"thisQuarter", january, "2026-01-01", "2026-03-31"},
		{"lastMonth", january, "2026-01-01", "2026-01-31"},
		{"lastFiscalYear", january, "2025-01-01", "2025-12-31"},
		{"lastQuarter", january, "2025-10-01", "2025-12-31"},
		{"thisMonth", february, "2026-02-01", "2026-02-28"},
		{"thisFiscalYear", february, "2025-07-01", "2026-06-30"},
		{"thisFiscalYearToDate", february, "2025-07-01", "2026-02-10"},
		{"thisQuarter", february, "2026-01-01", "2026-03-31"},
		{"lastQuarter", february, "2025-10-01", "2025-12-31"},
		{"lastFiscalYear", february, "2024-07-01", "2025-06-30"},
		{"lastMonth", february, "2026-01-01", "2026-01-31"},
		{"last12Months", february, "2025-02-01", "2026-01-31"},
		{"thisFiscalYear", marchStart, "2025-03-01", "2026-02-28"},
		{"thisQuarter", marchStart, "2025-12-01", "2026-02-28"},
		{"lastQuarter", marchStart, "2025-09-01", "2025-11-30"},
		{"lastFiscalYear", marchStart, "2024-03-01", "2025-02-28"},
		{"lastMonth", marchStart, "2025-12-01", "2025-12-31"},
		{"last12Months", marchStart, "2025-01-01", "2025-12-31"},
		{"allDates", july, "", ""},
	}
	for _, c := range cases {
		from, to := rangeDays(c.preset, c.pc)
		if from != c.from || to != c.to {
			t.Errorf("%s (fiscal start %d): %s..%s, want %s..%s", c.preset, c.pc.fiscalStart, from, to, c.from, c.to)
		}
	}
}

func TestAsOfPresetsMatchTheShell(t *testing.T) {
	july := periodContext{today: day("2026-08-15"), fiscalStart: 7}
	february := periodContext{today: day("2026-02-10"), fiscalStart: 7}
	marchStart := periodContext{today: day("2026-01-15"), fiscalStart: 3}
	cases := []struct {
		preset string
		pc     periodContext
		want   string
	}{
		{"today", july, "2026-08-15"},
		{"endOfLastMonth", july, "2026-07-31"},
		{"endOfLastQuarter", july, "2026-06-30"},
		{"endOfLastFiscalYear", july, "2026-06-30"},
		{"endOfLastMonth", february, "2026-01-31"},
		{"endOfLastQuarter", february, "2025-12-31"},
		{"endOfLastFiscalYear", february, "2025-06-30"},
		{"endOfLastMonth", marchStart, "2025-12-31"},
		{"endOfLastQuarter", marchStart, "2025-11-30"},
		{"endOfLastFiscalYear", marchStart, "2025-02-28"},
	}
	for _, c := range cases {
		if got := asOfDay(c.preset, c.pc); got != c.want {
			t.Errorf("%s (today %s, fiscal start %d) = %s, want %s", c.preset, c.pc.today.Format(time.DateOnly), c.pc.fiscalStart, got, c.want)
		}
	}
}

func raws(m map[string]string) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	for k, v := range m {
		out[k] = json.RawMessage(v)
	}
	return out
}

var zooParams = []ParamDef{
	{ID: "period", Type: ParamDateRange, Default: json.RawMessage(`{"preset":"thisMonth"}`)},
	{ID: "asOf", Type: ParamAsOf},
	{ID: "kind", Type: ParamChoice, Options: []Option{{Value: "hay", Label: "Hay"}, {Value: "fish", Label: "Fish"}}},
	{ID: "kinds", Type: ParamChoice, Multiple: true, Options: []Option{{Value: "hay", Label: "Hay"}, {Value: "fish", Label: "Fish"}}},
	{ID: "note", Type: ParamText},
	{ID: "min", Type: ParamNumber},
}

func TestResolveParamsTakesPostedThenDefaultThenUnset(t *testing.T) {
	pc := periodContext{today: day("2026-08-15"), fiscalStart: 1}
	got, err := resolveParams(zooParams, raws(map[string]string{
		"asOf": `{"preset":"endOfLastMonth"}`, "kinds": `["fish","hay"]`, "min": `2.5`, "note": `"x"`,
	}), pc)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]paramValue{
		"period": {typ: ParamDateRange, from: "2026-08-01", to: "2026-08-31"},
		"asOf":   {typ: ParamAsOf, value: "2026-07-31"},
		"kind":   {typ: ParamChoice, unset: true},
		"kinds":  {typ: ParamChoice, multiple: true, list: []string{"fish", "hay"}},
		"note":   {typ: ParamText, value: "x"},
		"min":    {typ: ParamNumber, value: 2.5},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
}

func TestResolveParamsAcceptsValidValues(t *testing.T) {
	pc := periodContext{today: day("2026-08-15"), fiscalStart: 1}
	choice := []Option{{Value: "hay", Label: "Hay"}, {Value: "fish", Label: "Fish"}}
	cases := map[string]struct {
		def   ParamDef
		given string // "" means the param is not posted
		want  paramValue
	}{
		"explicit range":               {ParamDef{ID: "p", Type: ParamDateRange}, `{"from":"2026-03-01","to":"2026-03-31"}`, paramValue{typ: ParamDateRange, from: "2026-03-01", to: "2026-03-31"}},
		"date":                         {ParamDef{ID: "p", Type: ParamDate}, `"2026-05-05"`, paramValue{typ: ParamDate, value: "2026-05-05"}},
		"today preset on date":         {ParamDef{ID: "p", Type: ParamDate}, `{"preset":"today"}`, paramValue{typ: ParamDate, value: "2026-08-15"}},
		"default on text":              {ParamDef{ID: "p", Type: ParamText, Default: json.RawMessage(`"hello"`)}, "", paramValue{typ: ParamText, value: "hello"}},
		"posted null clears default":   {ParamDef{ID: "p", Type: ParamChoice, Options: choice, Default: json.RawMessage(`"hay"`)}, `null`, paramValue{typ: ParamChoice, unset: true}},
		"empty multiple list is unset": {ParamDef{ID: "p", Type: ParamChoice, Multiple: true, Options: choice}, `[]`, paramValue{typ: ParamChoice, multiple: true, unset: true}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			given := map[string]string{}
			if c.given != "" {
				given["p"] = c.given
			}
			got, err := resolveParams([]ParamDef{c.def}, raws(given), pc)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got["p"], c.want) {
				t.Fatalf("got  %+v\nwant %+v", got["p"], c.want)
			}
		})
	}
}

func TestResolveParamsRefusesBadValues(t *testing.T) {
	pc := periodContext{today: day("2026-08-15"), fiscalStart: 1}
	cases := map[string]struct {
		given    map[string]string
		at, want string
	}{
		"undeclared":     {map[string]string{"color": `"red"`}, "params.color", `no param "color"`},
		"not a day":      {map[string]string{"asOf": `"2026-02-30"`}, "params.asOf", "YYYY-MM-DD"},
		"before 1900":    {map[string]string{"asOf": `"1899-12-31"`}, "params.asOf", "YYYY-MM-DD"},
		"range preset":   {map[string]string{"asOf": `{"preset":"thisMonth"}`}, "params.asOf", "a date or a preset"},
		"backward range": {map[string]string{"period": `{"from":"2026-02-01","to":"2026-01-01"}`}, "params.period", "on or before"},
		"range keys":     {map[string]string{"period": `{"from":"2026-02-01"}`}, "params.period", "a date range is"},
		"choice":         {map[string]string{"kind": `"meat"`}, "params.kind", "one of the param's options"},
		"multiple":       {map[string]string{"kinds": `"hay"`}, "params.kinds", "a list of the param's options"},
		"long text":      {map[string]string{"note": `"` + strings.Repeat("x", 501) + `"`}, "params.note", "at most 500"},
		"number":         {map[string]string{"min": `"2"`}, "params.min", "a number"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := resolveParams(zooParams, raws(c.given), pc)
			requireSpecError(t, err, 400, c.at, c.want)
		})
	}
}

func TestCheckParamDefsRefusesBadDeclarations(t *testing.T) {
	cases := map[string]struct {
		defs     []ParamDef
		at, want string
	}{
		"duplicate id":   {[]ParamDef{{ID: "a", Type: ParamText}, {ID: "a", Type: ParamText}}, "params.1.id", "unique id"},
		"unknown type":   {[]ParamDef{{ID: "a", Type: "color"}}, "params.0.type", `unknown param type "color"`},
		"choice options": {[]ParamDef{{ID: "a", Type: ParamChoice}}, "params.0.options", "lists its options"},
		"text options":   {[]ParamDef{{ID: "a", Type: ParamText, Options: []Option{{Value: "x", Label: "X"}}}}, "params.0", "only a choice param"},
		"bad default":    {[]ParamDef{{ID: "a", Type: ParamDateRange, Default: json.RawMessage(`{"preset":"someday"}`)}}, "params.0.default", "the preset is one of"},
		"group label without group": {
			[]ParamDef{groupedTeams("All {group} teams", nil)}, "params.0.optionsFrom.groupLabel", "needs optionsFrom.group",
		},
		"group label without placeholder": {
			[]ParamDef{groupedTeams("Every team", Path{{Field: "region"}})}, "params.0.optionsFrom.groupLabel", "{group}",
		},
		"long group label": {
			[]ParamDef{groupedTeams("{group}"+strings.Repeat("x", maxLabel), Path{{Field: "region"}})}, "params.0.optionsFrom.groupLabel", "at most 200",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			requireSpecError(t, checkParamDefs(c.defs), 400, c.at, c.want)
		})
	}
	if err := checkParamDefs(zooParams); err != nil {
		t.Fatalf("zoo params: %v", err)
	}
	if err := checkParamDefs([]ParamDef{groupedTeams("All {group} teams", Path{{Field: "region"}})}); err != nil {
		t.Fatalf("a group label with its group: %v", err)
	}
}

// groupedTeams is a multiple choice of records whose group pick reads label.
func groupedTeams(label string, group Path) ParamDef {
	return ParamDef{ID: "teams", Type: ParamChoice, Multiple: true, OptionsFrom: &OptionsFrom{
		Source: "zoo.teams", Value: Path{{Field: "id"}}, Label: Path{{Field: "name"}}, Group: group, GroupLabel: label,
	}}
}

// boundSpec reads the period's first and last day, compares each feeding's
// day with the first, and sums ann's grams fed before it.
const boundSpec = `{"version":4,"source":"zoo.feedings","params":[{"id":"period","type":"dateRange"}],"columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"from","formula":{"param":"period","bound":"from"}},
	{"id":"to","formula":{"param":"period","bound":"to"}},
	{"id":"before","formula":{"op":"lt","args":[{"path":[{"field":"fed"}]},{"param":"period","bound":"from"}]}},
	{"id":"gramsBefore","formula":{"op":"coalesce","args":[{"op":"lookup","source":"zoo.feedings","pick":"sum",
		"where":{"op":"lt","args":[{"path":[{"field":"fed"}]},{"param":"period","bound":"from"}]},
		"value":{"path":[{"field":"grams"}]}},{"value":0}]}}],
	"sort":[{"col":"note"}]}`

func TestABoundReadsARangesFirstAndLastDay(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", boundSpec, Options{}, map[string]string{"period": `{"from":"2026-02-01","to":"2026-02-28"}`})
	requireRows(t, res, "from", "2026-02-01", "2026-02-01", "2026-02-01", "2026-02-01", "2026-02-01")
	requireRows(t, res, "to", "2026-02-28", "2026-02-28", "2026-02-28", "2026-02-28", "2026-02-28")
	// f1 01-05, f2 02-10, f3 01-20, f4 03-03; the total has no one day.
	requireOrderedRows(t, boundSpec, res, "before", true, false, true, false, nil)
	// f1's 500 and f3's 300 grams were fed before February.
	requireRows(t, res, "gramsBefore", int64(800), int64(800), int64(800), int64(800), int64(800))
	for _, c := range res.Columns {
		if (c.ID == "from" || c.ID == "to") && c.Type != Date {
			t.Fatalf("%s column = %+v", c.ID, c)
		}
	}
}

// An unset range and All dates have no first day: a bound is blank, and a
// comparison with it matches no row (R2), so nothing was fed before it.
func TestABoundOfAnOpenRangeHasNoValue(t *testing.T) {
	z := newZoo(t)
	for name, period := range map[string]string{"unset": `null`, "all dates": `{"preset":"allDates"}`} {
		t.Run(name, func(t *testing.T) {
			res := z.run(t, "ann", boundSpec, Options{}, map[string]string{"period": period})
			requireRows(t, res, "from", nil, nil, nil, nil, nil)
			requireRows(t, res, "to", nil, nil, nil, nil, nil)
			requireRows(t, res, "before", nil, nil, nil, nil, nil)
			requireRows(t, res, "gramsBefore", int64(0), int64(0), int64(0), int64(0), int64(0))
		})
	}
}

// All dates keeps every row, one with no day too, in a filter, a formula
// and a lookup, and resolves to a set range with no bounds.
func TestAllDatesMatchesEveryRowAndResolvesWithNoBounds(t *testing.T) {
	z := newZoo(t)
	z.rec(t, "f7", "zoo_feedings", map[string]any{"animal": z.id("leo"), "cost": 5, "currency": "USD", "grams": 1, "note": "f7"})
	inPeriod := `{"op":"between","args":[{"path":[{"field":"fed"}]},{"param":"period"}]}`
	spec := `{"version":4,"source":"zoo.feedings","params":[{"id":"period","type":"dateRange","default":{"preset":"allDates"}}],
		"columns":[{"id":"note","path":[{"field":"note"}]},
		{"id":"inPeriod","formula":` + inPeriod + `},
		{"id":"counted","formula":{"op":"lookup","source":"zoo.feedings","pick":"count","where":` + inPeriod + `,"value":{"path":[{"field":"id"}]}}}],
		"filter":` + inPeriod + `,"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireOrderedRows(t, spec, res, "note", "f1", "f2", "f3", "f4", "f7", nil)
	requireRows(t, res, "inPeriod", true, true, true, true, true, nil)
	requireRows(t, res, "counted", int64(5), int64(5), int64(5), int64(5), int64(5), int64(5))
	if want := []ResolvedParam{{ID: "period", Type: ParamDateRange}}; !reflect.DeepEqual(res.Params, want) {
		t.Fatalf("params = %+v", res.Params)
	}
	// A set range drops the row with no day.
	res = z.run(t, "ann", spec, Options{}, map[string]string{"period": `{"from":"2026-01-01","to":"2026-01-31"}`})
	requireOrderedRows(t, spec, res, "note", "f1", "f3", nil)
	requireRows(t, res, "counted", int64(2), int64(2), int64(2))
}

func TestABoundIsRefusedOffADateRange(t *testing.T) {
	app := withZooRegistry(t)
	spec := func(formula string) string {
		return `{"version":4,"source":"zoo.feedings","params":[{"id":"period","type":"dateRange"},{"id":"day","type":"date"}],
			"columns":[{"id":"x","formula":` + formula + `}]}`
	}
	cases := map[string]struct{ formula, want string }{
		"a date param":       {`{"param":"day","bound":"from"}`, "bound reads a date range's start or end"},
		"a bound mid":        {`{"param":"period","bound":"mid"}`, "bound is from or to"},
		"a bound on a value": {`{"value":"2026-01-01","bound":"from"}`, "bound goes with a param"},
		"a bound on a lookup": {`{"op":"lookup","source":"zoo.animals","bound":"from","value":{"path":[{"field":"born"}]}}`,
			"one of op, col, path"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			err := Validate(app, loadSpec(t, spec(c.formula)))
			requireSpecError(t, err, 400, "columns.0.formula", c.want)
		})
	}
}
