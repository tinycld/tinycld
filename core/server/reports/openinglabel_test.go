package reports

import (
	"fmt"
	"testing"
)

// gramsWithOpening groups ann's feedings by animal, each group opening onto
// the given opening row. asOf is a day param and note a text param: the two
// kinds of value an opening row's label names.
func gramsWithOpening(opening string) string {
	return `{"version":4,"source":"zoo.feedings",
		"params":[{"id":"asOf","type":"asOf","label":"As of"},{"id":"note","type":"text","label":"Note"}],
		"columns":[
			{"id":"animal","path":[{"link":"animal"}]},
			{"id":"note","path":[{"field":"note"}]},
			{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal","open":true,"subtotals":true,"opening":` + opening + `}],
		"sort":[{"col":"note"}]}`
}

var julyEnd = map[string]string{"asOf": `"2026-07-31"`}

// openingRows are the run's opening rows, failing the test when it has none.
func openingRows(t *testing.T, res *Result) []Row {
	t.Helper()
	var out []Row
	for _, r := range res.Rows {
		if r.Kind == KindOpening {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		t.Fatalf("no opening row in %v", rowsOf(res, "kind"))
	}
	return out
}

func TestAnOpeningLabelNamesTheMonthOfADayParam(t *testing.T) {
	z := newZoo(t)
	spec := gramsWithOpening(`{"label":"Balance before {value}","labelValue":{"param":"asOf"},"labelBucket":"month"}`)
	res := z.run(t, "ann", spec, Options{}, julyEnd)
	requireRows(t, res, "kind", "group", "opening", "detail", "detail", "subtotal", "group", "opening", "detail", "detail", "subtotal", "total")
	f := newExportFormatter(exportDisplay{}, "UTC", false)
	for _, r := range openingRows(t, res) {
		if r.Label != "Balance before 2026-07" || r.LabelTemplate != "Balance before {value}" || r.LabelType != Month || r.LabelValue != "2026-07" || r.LabelCurrency != "" {
			t.Fatalf("opening row = %+v", r)
		}
		if got := f.RowLabel(r); got != "Balance before Jul 2026" {
			t.Fatalf("exported label = %q", got)
		}
	}
}

// Each bucket gives its key and its label type, as a bucketed group does; with
// no bucket the value is the day itself.
func TestAnOpeningLabelValueByBucket(t *testing.T) {
	z := newZoo(t)
	cases := []struct {
		bucket string
		typ    FieldType
		value  string
		words  string
	}{
		{"", Date, "2026-07-31", "Jul 31, 2026"},
		{"day", Day, "2026-07-31", "Jul 31, 2026"},
		{"month", Month, "2026-07", "Jul 2026"},
		{"quarter", Quarter, "2026-Q3", "Q3 2026"},
		{"year", Year, "2026", "2026"},
		{"fiscalYear", FiscalYear, "2026", "FY 2026"},
	}
	f := newExportFormatter(exportDisplay{}, "UTC", false)
	for _, c := range cases {
		t.Run("bucket "+c.bucket, func(t *testing.T) {
			bucket := ""
			if c.bucket != "" {
				bucket = `,"labelBucket":"` + c.bucket + `"`
			}
			res := z.run(t, "ann", gramsWithOpening(`{"label":"Before {value}:","labelValue":{"param":"asOf"}`+bucket+`}`), Options{}, julyEnd)
			r := openingRows(t, res)[0]
			if r.LabelType != c.typ || fmt.Sprint(r.LabelValue) != c.value || r.Label != "Before "+c.value+":" {
				t.Fatalf("opening row = %+v", r)
			}
			if got := f.RowLabel(r); got != "Before "+c.words+":" {
				t.Fatalf("exported label = %q", got)
			}
		})
	}
}

func TestAnOpeningLabelNamesATextOrAComputedValue(t *testing.T) {
	z := newZoo(t)
	f := newExportFormatter(exportDisplay{}, "UTC", false)
	text := z.run(t, "ann", gramsWithOpening(`{"label":"Before {value}","labelValue":{"param":"note"}}`), Options{}, map[string]string{"note": `"the $1 $& audit"`})
	r := openingRows(t, text)[0]
	if r.Label != "Before the $1 $& audit" || r.LabelType != Text || r.LabelValue != "the $1 $& audit" || f.RowLabel(r) != "Before the $1 $& audit" {
		t.Fatalf("text opening row = %+v, exported %q", r, f.RowLabel(r))
	}
	// A number is a value of the run too.
	r = openingRows(t, z.run(t, "ann", gramsWithOpening(`{"label":"Before {value}","labelValue":{"value":12}}`), Options{}, nil))[0]
	if r.Label != "Before 12" || r.LabelType != Number || fmt.Sprint(r.LabelValue) != "12" || f.RowLabel(r) != "Before 12" {
		t.Fatalf("number opening row = %+v, exported %q", r, f.RowLabel(r))
	}
	// The first day of a period: the bound of a date range is a day too.
	period := `{"version":4,"source":"zoo.feedings","params":[{"id":"period","type":"dateRange"}],
		"columns":[{"id":"animal","path":[{"link":"animal"}]},{"id":"note","path":[{"field":"note"}]}],
		"filter":{"op":"between","args":[{"path":[{"field":"due"}]},{"param":"period"}]},
		"groups":[{"id":"g","col":"animal","open":true,"opening":{"label":"Balance before {value}","labelValue":{"param":"period","bound":"from"},"labelBucket":"month"}}]}`
	r = openingRows(t, z.run(t, "ann", period, Options{}, fromFebruary))[0]
	if r.LabelType != Month || r.LabelValue != "2026-02" || f.RowLabel(r) != "Balance before Feb 2026" {
		t.Fatalf("period opening row = %+v, exported %q", r, f.RowLabel(r))
	}
	// A year before the day asked: the value is computed, then cut to its month.
	prior := `{"label":"Before {value}","labelValue":{"op":"addMonths","args":[{"param":"asOf"},{"value":-12}]},"labelBucket":"month"}`
	r = openingRows(t, z.run(t, "ann", gramsWithOpening(prior), Options{}, julyEnd))[0]
	if r.LabelValue != "2025-07" || f.RowLabel(r) != "Before Jul 2025" {
		t.Fatalf("computed opening row = %+v, exported %q", r, f.RowLabel(r))
	}
}

// A value that is blank on this run leaves the token out, with the space it
// stood in, and the row has no typed label: a view shows Label as it is.
func TestAnOpeningLabelWithNoValueLeavesTheTokenOut(t *testing.T) {
	z := newZoo(t)
	f := newExportFormatter(exportDisplay{}, "UTC", false)
	spec := gramsWithOpening(`{"label":"Balance before {value} (opening)","labelValue":{"param":"note"}}`)
	for name, params := range map[string]map[string]string{
		"unset":  nil,
		"empty":  {"note": `""`},
		"spaces": {"note": `"  "`},
	} {
		t.Run(name, func(t *testing.T) {
			for _, r := range openingRows(t, z.run(t, "ann", spec, Options{}, params)) {
				if r.Label != "Balance before (opening)" || r.LabelTemplate != "" || r.LabelType != "" || r.LabelValue != nil {
					t.Fatalf("opening row = %+v", r)
				}
				if got := f.RowLabel(r); got != "Balance before (opening)" {
					t.Fatalf("exported label = %q", got)
				}
			}
		})
	}
}

// Only the token and the spaces around it go; the rest of the label keeps its
// spacing, and a token at an end leaves no space there.
func TestAnOpeningLabelWithNoValueKeepsTheRestOfTheLabelAsItIs(t *testing.T) {
	z := newZoo(t)
	cases := map[string]struct{ label, want string }{
		"double space away from the token": {"Balance  before {value} (opening)", "Balance  before (opening)"},
		"token at the start":               {"{value} opening balance", "opening balance"},
		"token at the end":                 {"Balance before  {value}", "Balance before"},
		"spaces around the token":          {"Balance   {value}   (opening)", "Balance (opening)"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			spec := gramsWithOpening(`{"label":"` + c.label + `","labelValue":{"param":"note"}}`)
			for _, r := range openingRows(t, z.run(t, "ann", spec, Options{}, nil)) {
				if r.Label != c.want {
					t.Fatalf("label = %q, want %q", r.Label, c.want)
				}
			}
		})
	}
}

// An opening row with no label value is as before: a plain label, no template.
func TestRegressionGuardAnOpeningLabelWithNoLabelValueIsPlainText(t *testing.T) {
	z := newZoo(t)
	for _, r := range openingRows(t, z.run(t, "ann", gramsWithOpening(`{"label":"Before the period"}`), Options{}, julyEnd)) {
		if r.Label != "Before the period" || r.LabelTemplate != "" || r.LabelType != "" || r.LabelValue != nil {
			t.Fatalf("opening row = %+v", r)
		}
	}
}

// A layout section's opening row is made on the same path, and so is one on
// an inner level: the value is found by the group's level, not its depth.
func TestAnOpeningLabelValueOnALayoutSectionAndAnInnerLevel(t *testing.T) {
	z := newZoo(t)
	opening := `{"label":"Start of {value}","labelValue":{"param":"asOf"},"labelBucket":"year"}`
	layout := `{"version":4,"source":"zoo.feedings","params":[{"id":"asOf","type":"asOf","label":"As of"}],"columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species","opening":` + opening + `}],
		"sort":[{"col":"note"}],
		"layout":[{"section":"cats","title":"Lions","key":"lion"},{"section":"stripes","title":"Zebras","key":"zebra"}]}`
	inner := `{"version":4,"source":"zoo.feedings","params":[{"id":"asOf","type":"asOf","label":"As of"}],"columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"groups":[{"id":"s","col":"species","open":true},{"id":"g","col":"animal","open":true,"opening":` + opening + `}],
		"sort":[{"col":"note"}]}`
	for name, spec := range map[string]string{"layout": layout, "inner": inner} {
		t.Run(name, func(t *testing.T) {
			for _, r := range openingRows(t, z.run(t, "ann", spec, Options{ExpandAll: true}, julyEnd)) {
				if r.Label != "Start of 2026" || r.LabelType != Year || r.LabelValue != "2026" {
					t.Fatalf("opening row = %+v", r)
				}
			}
		})
	}
}

// A page that starts after the first carries the label value too: the value
// is read on every run, not only on the first page.
func TestAnOpeningLabelValueIsOnEveryPage(t *testing.T) {
	z := newZoo(t)
	spec := gramsWithOpening(`{"label":"Balance before {value}","labelValue":{"param":"asOf"},"labelBucket":"month"}`)
	whole := openingRows(t, z.run(t, "ann", spec, Options{}, julyEnd))
	var paged []Row
	opts := Options{Limit: 3}
	for range 20 {
		res := z.run(t, "ann", spec, opts, julyEnd)
		for _, r := range res.Rows {
			if r.Kind == KindOpening {
				paged = append(paged, r)
			}
		}
		if res.Page == nil || res.Page.Next == "" {
			break
		}
		opts.Cursor = res.Page.Next
	}
	if len(paged) != len(whole) {
		t.Fatalf("%d opening rows over the pages, %d in the whole run", len(paged), len(whole))
	}
	for _, r := range paged {
		if r.Label != "Balance before 2026-07" || r.LabelType != Month || r.LabelValue != "2026-07" || r.LabelTemplate != "Balance before {value}" {
			t.Fatalf("paged opening row = %+v", r)
		}
	}
}

func TestOpeningLabelValueRefusals(t *testing.T) {
	z := newZoo(t)
	cases := map[string]struct{ opening, at, want string }{
		"no token":             {`{"label":"Balance before","labelValue":{"param":"asOf"}}`, "groups.0.opening.label", "{value} once in its label"},
		"two tokens":           {`{"label":"{value} to {value}","labelValue":{"param":"asOf"}}`, "groups.0.opening.label", "{value} once in its label"},
		"only the token":       {`{"label":" {value} ","labelValue":{"param":"asOf"}}`, "groups.0.opening.label", "text besides {value}"},
		"token with no value":  {`{"label":"Balance before {value}"}`, "groups.0.opening.label", "needs a labelValue"},
		"bucket with no value": {`{"label":"Balance before","labelBucket":"month"}`, "groups.0.opening.labelBucket", "needs a labelValue"},
		"unknown bucket":       {`{"label":"Before {value}","labelValue":{"param":"asOf"},"labelBucket":"week"}`, "groups.0.opening.labelBucket", "day, month, quarter, year or fiscalYear"},
		"bucket of a text":     {`{"label":"Before {value}","labelValue":{"param":"note"},"labelBucket":"month"}`, "groups.0.opening.labelBucket", "a labelValue that is a day"},
		"bucket of nothing":    {`{"label":"Before {value}","labelValue":{"value":null},"labelBucket":"month"}`, "groups.0.opening.labelBucket", "a labelValue that is a day"},
		"path leaf":            {`{"label":"Before {value}","labelValue":{"path":[{"field":"note"}]}}`, "groups.0.opening.labelValue", "an opening row's label value reads params and values only"},
		"aggregate":            {`{"label":"Before {value}","labelValue":{"op":"sum","args":[{"path":[{"field":"grams"}]}]}}`, "groups.0.opening.labelValue", "params and values only"},
		"column":               {`{"label":"Before {value}","labelValue":{"col":"grams"}}`, "groups.0.opening.labelValue", "params and values only"},
		"yes or no":            {`{"label":"Before {value}","labelValue":{"value":true}}`, "groups.0.opening.labelValue", "an opening row's label value is a text, a day or a number"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			requireSpecError(t, z.validate(t, gramsWithOpening(c.opening)), 400, c.at, c.want)
		})
	}
}

// The refusal of a column heading keeps its own words.
func TestRegressionGuardAHeadingRefusalNamesAHeading(t *testing.T) {
	z := newZoo(t)
	requireSpecError(t, z.validate(t, fmt.Sprintf(zooHeadings, `{"col":"cost"}`)), 400, "columns.3.heading", "a heading reads params and values only")
	requireSpecError(t, z.validate(t, fmt.Sprintf(zooHeadings, `{"value":true}`)), 400, "columns.3.heading", "a heading is a text, a day or a number")
}
