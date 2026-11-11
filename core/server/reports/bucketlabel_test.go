package reports

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
)

// feedingsPivotBy pivots ann's feeding cost by species and a bucket of fed:
// f1 Jan 5, f2 Feb 10, f3 Jan 20, f4 Mar 3 (2026).
const feedingsPivotBy = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"species"}],
	"pivot":{"path":[{"field":"fed"}],"bucket":"%s","cell":"cost"}}`

// feedingsGroupedBy groups ann's feedings by a bucket of fed, with subtotals.
const feedingsGroupedBy = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"fed","path":[{"field":"fed"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"m","col":"fed","bucket":"%s","subtotals":true}]}`

// pivotLabelTypes is each pivot column's label as "type=value".
func pivotLabelTypes(res *Result) []string {
	var out []string
	for _, c := range res.Columns {
		if c.Pivot != nil {
			out = append(out, fmt.Sprintf("%s=%v", c.Pivot.LabelType, c.Pivot.LabelValue))
		}
	}
	return out
}

// groupLabelTypes is each group and subtotal row's label as
// "label type=value".
func groupLabelTypes(res *Result) []string {
	var out []string
	for _, r := range res.Rows {
		if r.Kind == KindGroup || r.Kind == KindSubtotal {
			out = append(out, fmt.Sprintf("%s %s=%v", r.Label, r.LabelType, r.LabelValue))
		}
	}
	return out
}

func TestABucketedPivotColumnCarriesItsBucketType(t *testing.T) {
	z := newZoo(t)
	for bucket, want := range map[string][]string{
		"month":   {"month=2026-01", "month=2026-02", "month=2026-03"},
		"quarter": {"quarter=2026-Q1"},
		"year":    {"year=2026"},
		"day":     {"day=2026-01-05", "day=2026-01-20", "day=2026-02-10", "day=2026-03-03"},
	} {
		t.Run(bucket, func(t *testing.T) {
			res := z.run(t, "ann", fmt.Sprintf(feedingsPivotBy, bucket), Options{}, nil)
			if got := pivotLabelTypes(res); !slices.Equal(got, want) {
				t.Fatalf("pivot labels = %v, want %v", got, want)
			}
			// The key and the label text stay the raw key (R8).
			for _, c := range res.Columns {
				if c.Pivot != nil && (c.Pivot.Key != c.Pivot.LabelValue || c.Label != c.Pivot.Key) {
					t.Fatalf("column %s: key %#v, label %q, label value %#v", c.ID, c.Pivot.Key, c.Label, c.Pivot.LabelValue)
				}
			}
		})
	}
}

func TestABucketedGroupRowCarriesItsBucketType(t *testing.T) {
	z := newZoo(t)
	for bucket, want := range map[string][]string{
		"month": {
			"2026-01 month=2026-01", "Total 2026-01 month=2026-01",
			"2026-02 month=2026-02", "Total 2026-02 month=2026-02",
			"2026-03 month=2026-03", "Total 2026-03 month=2026-03",
		},
		"quarter": {"2026-Q1 quarter=2026-Q1", "Total 2026-Q1 quarter=2026-Q1"},
		"year":    {"2026 year=2026", "Total 2026 year=2026"},
		"day": {
			"2026-01-05 day=2026-01-05", "Total 2026-01-05 day=2026-01-05",
			"2026-01-20 day=2026-01-20", "Total 2026-01-20 day=2026-01-20",
			"2026-02-10 day=2026-02-10", "Total 2026-02-10 day=2026-02-10",
			"2026-03-03 day=2026-03-03", "Total 2026-03-03 day=2026-03-03",
		},
	} {
		t.Run(bucket, func(t *testing.T) {
			res := z.run(t, "ann", fmt.Sprintf(feedingsGroupedBy, bucket), Options{ExpandAll: true}, nil)
			if got := groupLabelTypes(res); !slices.Equal(got, want) {
				t.Fatalf("group labels = %v\nwant %v", got, want)
			}
		})
	}
	res := z.run(t, "ann", fmt.Sprintf(feedingsGroupedBy, "month"), Options{}, nil)
	// The key is the raw month, then the currency (groups_test.go pins the
	// currency last).
	if key := res.Rows[0].GroupKey; !reflect.DeepEqual(key, []any{"2026-01", "USD"}) {
		t.Fatalf("group key = %#v, want the raw month and USD", key)
	}
}

func TestAFiscalYearBucketIsTypedWithItsEndYear(t *testing.T) {
	z := newZoo(t)
	zooFiscalStart = 7
	t.Cleanup(func() { zooFiscalStart = 1 })
	z.feeding(t, "f9", z.recs["leo"], 400, "2026-08-01", 10)
	// Jan to Mar 2026 fall in the year to June 2026; Aug 2026 in the year to
	// June 2027.
	pivot := z.run(t, "ann", fmt.Sprintf(feedingsPivotBy, "fiscalYear"), Options{}, nil)
	if got, want := pivotLabelTypes(pivot), []string{"fiscalYear=2026", "fiscalYear=2027"}; !slices.Equal(got, want) {
		t.Fatalf("pivot labels = %v, want %v", got, want)
	}
	if k := pivot.Columns[len(pivot.Columns)-1].Pivot.Key; k != int64(2027) {
		t.Fatalf("fiscal key = %#v, want int64 2027", k)
	}
	grouped := z.run(t, "ann", fmt.Sprintf(feedingsGroupedBy, "fiscalYear"), Options{ExpandAll: true}, nil)
	want := []string{"2026 fiscalYear=2026", "Total 2026 fiscalYear=2026", "2027 fiscalYear=2027", "Total 2027 fiscalYear=2027"}
	if got := groupLabelTypes(grouped); !slices.Equal(got, want) {
		t.Fatalf("group labels = %v, want %v", got, want)
	}
}

// A bucket type is a label's type only: no source may declare a field of it.
func TestABucketTypeIsNeverAFieldsType(t *testing.T) {
	for _, typ := range []FieldType{Month, Quarter, Year, FiscalYear, Day} {
		if err := validateField(&Source{}, &Field{Label: "When", Type: typ}); err == nil {
			t.Fatalf("a field of type %q was accepted", typ)
		}
	}
}
