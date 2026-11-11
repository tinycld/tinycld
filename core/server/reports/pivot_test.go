package reports

import (
	"fmt"
	"reflect"
	"testing"
)

const costBySpeciesAndMonth = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"species"}],
	"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"cost"%s}}`

func pivotIDs(res *Result) []string {
	var ids []string
	for _, c := range res.Columns {
		if c.Pivot != nil {
			ids = append(ids, c.ID+"="+c.Label)
		}
	}
	return ids
}

func TestPivotSplitsTheCellByValue(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", fmt.Sprintf(costBySpeciesAndMonth, ""), Options{}, nil)
	if got := pivotIDs(res); !reflect.DeepEqual(got, []string{"cost.p0=2026-01", "cost.p1=2026-02", "cost.p2=2026-03"}) {
		t.Fatalf("pivot columns = %v", got)
	}
	if c := res.Columns[2]; c.Pivot.Cell != "cost" || c.Pivot.Key != "2026-01" || c.Type != Money || c.CurrencyCell != "cost@currency" {
		t.Fatalf("pivot column = %+v", c)
	}
	requireRows(t, res, "label", "lion", "zebra", "Total")
	requireRows(t, res, "cost.p0", int64(1000), int64(700), int64(1700))
	requireRows(t, res, "cost.p1", int64(2500), int64(0), int64(2500))
	requireRows(t, res, "cost.p2", int64(0), int64(300), int64(300))
	// The cell column itself stays as the row's total across the pivot.
	requireRows(t, res, "cost", int64(3500), int64(1000), int64(4500))
	if res.Rows[0].Expanded {
		t.Fatal("a pivot's innermost group has no detail rows to open")
	}
}

func TestPivotStopsAtMaxColumnsAndSaysHowManyAreLeftOut(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", fmt.Sprintf(costBySpeciesAndMonth, `,"maxColumns":2`), Options{}, nil)
	if got := pivotIDs(res); !reflect.DeepEqual(got, []string{"cost.p0=2026-01", "cost.p1=2026-02"}) {
		t.Fatalf("pivot columns = %v", got)
	}
	if len(res.Notices) != 1 || res.Notices[0].Code != "pivotCap" || res.Notices[0].LeftOut != 1 || res.Notices[0].Count != 0 {
		t.Fatalf("notices = %+v", res.Notices)
	}
}

func TestTheRowCapNeverCutsPivotColumns(t *testing.T) {
	z := newZoo(t)
	withLimits(t, func(l *limits) { l.rowCap = 1 })
	res := z.run(t, "ann", fmt.Sprintf(costBySpeciesAndMonth, ""), Options{}, nil)
	if got := len(pivotIDs(res)); got != 3 {
		t.Fatalf("%d pivot columns, want 3", got)
	}
}

func TestPivotCellOfAnAverageAndAYearBucket(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "bob", `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"avg","scale":1}],
		"groups":[{"id":"g","col":"species"}],
		"pivot":{"path":[{"field":"fed"}],"bucket":"year","cell":"grams"}}`, Options{}, nil)
	if got := pivotIDs(res); !reflect.DeepEqual(got, []string{"grams.p0=2026"}) {
		t.Fatalf("pivot columns = %v", got)
	}
	requireRows(t, res, "label", "monkey", "zebra", "Total")
	// Scale 1 keeps a decimal place, so every cell is a REAL.
	requireRows(t, res, "grams.p0", 50.0, 200.0, 150.0)
}

// A pivot's columns read the same whether or not the detail pass ran.
func TestPivotColumnTypesDoNotDependOnWhetherRowsLoaded(t *testing.T) {
	z := newZoo(t)
	spec := fmt.Sprintf(costBySpeciesAndMonth, "")
	collapsed := z.run(t, "ann", spec, Options{}, nil)
	open := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
	if !reflect.DeepEqual(collapsed.Columns, open.Columns) {
		t.Fatalf("columns differ:\n%+v\n%+v", collapsed.Columns, open.Columns)
	}
	if c := collapsed.Columns[len(collapsed.Columns)-1]; c.Type != Money || c.TotalType != "" {
		t.Fatalf("last pivot column = %+v", c)
	}
}

// A blank text and a missing linked row are one (none) column.
func TestPivotFoldsABlankAndAMissingTextKeyIntoOneColumn(t *testing.T) {
	z := newZoo(t)
	blank := z.rec(t, "blank", "zoo_sites", map[string]any{"name": ""})
	z.rec(t, "pip", "zoo_animals", map[string]any{"name": "Pip", "species": "owl", "site": blank.Id, "owner": z.id("ann"), "weight": 7})
	res := z.run(t, "ann", `{"version":4,"source":"zoo.animals","columns":[
		{"id":"species","path":[{"field":"species"}]},
		{"id":"weight","path":[{"field":"weight"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"}],
		"pivot":{"path":[{"link":"site"},{"field":"name"}],"cell":"weight"}}`, Options{}, nil)
	if got := pivotIDs(res); !reflect.DeepEqual(got, []string{"weight.p0=(none)", "weight.p1=North", "weight.p2=South"}) {
		t.Fatalf("pivot columns = %v", got)
	}
	requireRows(t, res, "label", "cat", "lion", "owl", "zebra", "Total")
	requireRows(t, res, "weight.p0", int64(4), int64(0), int64(7), int64(0), int64(11))
}

func TestPivotOverMixedCurrenciesKeepsEachCurrencyApart(t *testing.T) {
	z := newZoo(t)
	z.rec(t, "f7", "zoo_feedings", map[string]any{
		"animal": z.id("zed"), "cost": 400, "currency": "EUR", "fed": "2026-03-10 00:00:00.000Z", "grams": 10, "note": "f7",
	})
	res := z.run(t, "ann", fmt.Sprintf(costBySpeciesAndMonth, ""), Options{}, nil)
	requireRows(t, res, "label", "zebra", "Total", "lion", "zebra", "Total")
	requireRows(t, res, "cost@currency", "EUR", "EUR", "USD", "USD", "USD")
	requireRows(t, res, "cost.p0", int64(0), int64(0), int64(1000), int64(700), int64(1700))
	requireRows(t, res, "cost.p1", int64(0), int64(0), int64(2500), int64(0), int64(2500))
	requireRows(t, res, "cost.p2", int64(400), int64(400), int64(0), int64(300), int64(300))
	for _, c := range res.Columns {
		if c.Pivot != nil && (!c.MixedCurrency || c.CurrencyCell != "cost@currency") {
			t.Fatalf("pivot column = %+v", c)
		}
	}
}

func TestPivotKeyWithQuotesIsOnlyData(t *testing.T) {
	z := newZoo(t)
	const evil = `a'b"; DROP TABLE x;--`
	z.rec(t, "f7", "zoo_feedings", map[string]any{
		"animal": z.id("zed"), "cost": 400, "currency": "USD", "fed": "2026-03-10 00:00:00.000Z", "grams": 10, "note": evil,
	})
	res := z.run(t, "ann", `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"}],
		"pivot":{"path":[{"field":"note"}],"cell":"cost"}}`, Options{}, nil)
	want := []string{"cost.p0=" + evil, "cost.p1=f1", "cost.p2=f2", "cost.p3=f3", "cost.p4=f4"}
	if got := pivotIDs(res); !reflect.DeepEqual(got, want) {
		t.Fatalf("pivot columns = %v", got)
	}
	requireRows(t, res, "cost.p0", int64(0), int64(400), int64(400))
}

func TestPivotCapNoticeCountsEveryLeftOutValue(t *testing.T) {
	z := newZoo(t)
	for i, day := range []string{"04", "05", "06"} {
		z.feeding(t, fmt.Sprintf("x%d", i), z.recs["leo"], 10, "2026-04-"+day, 1)
	}
	res := z.run(t, "ann", `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"}],
		"pivot":{"path":[{"field":"fed"}],"bucket":"day","cell":"cost","maxColumns":2}}`, Options{}, nil)
	if len(res.Notices) != 1 || res.Notices[0].Code != "pivotCap" || res.Notices[0].LeftOut != 5 {
		t.Fatalf("notices = %+v", res.Notices)
	}
}

// A feeding with no day makes a (none) bucket column: no bucket key, so its
// label stays plain text, not a month the client would write in words.
func TestPivotBucketOfARowWithNoDayHasNoTypedLabel(t *testing.T) {
	z := newZoo(t)
	leo := z.recs["leo"]
	z.rec(t, "f9", "zoo_feedings", map[string]any{"animal": leo.Id, "cost": 40, "currency": "USD", "grams": 5, "note": "f9"})
	res := z.run(t, "ann", fmt.Sprintf(costBySpeciesAndMonth, ""), Options{}, nil)
	var none *ResultColumn
	for i, c := range res.Columns {
		if c.Pivot != nil && c.Pivot.Key == nil {
			none = &res.Columns[i]
		}
	}
	if none == nil {
		t.Fatalf("no pivot column for a missing day: %v", pivotIDs(res))
	}
	if none.Label != "(none)" || none.Pivot.LabelType != "" || none.Pivot.LabelValue != nil {
		t.Fatalf("(none) column = %+v", none.Pivot)
	}
	for _, c := range res.Columns {
		if c.Pivot != nil && c.Pivot.Key != nil && c.Pivot.LabelType != Month {
			t.Fatalf("%s label type = %q", c.ID, c.Pivot.LabelType)
		}
	}
}
