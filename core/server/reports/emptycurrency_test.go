package reports

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// homeCurrency is the currency withFeedingsCurrency gives zoo.feedings. No
// feeding carries it, so a cell in it can only come from the source.
const homeCurrency = "GBP"

// withFeedingsCurrency re-registers zoo.feedings with a Currency func, as a
// package whose amounts name a per-row currency field also gives its home
// currency. newZoo's cleanup resets the registry.
func withFeedingsCurrency(t *testing.T) {
	t.Helper()
	src := *lookupRegistered("zoo.feedings")
	src.Currency = func(core.App, *core.RequestInfo) string { return homeCurrency }
	RegisterSource(src)
}

// noFeedings filters every feeding out.
const noFeedings = `"filter":{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"no such note"}]},`

// cellCurrencies is each row's id, label and cost currency.
func cellCurrencies(res *Result) []any {
	var got []any
	for _, r := range res.Rows {
		got = append(got, r.ID, r.Label, r.Cells["cost@currency"])
	}
	return got
}

// With no rows at all, an empty section's group and subtotal rows and the
// computed rows over them read the source's currency: no row gives one.
func TestAnEmptyLayoutReadsTheSourceCurrency(t *testing.T) {
	z := newZoo(t)
	withFeedingsCurrency(t)
	res := z.run(t, "ann", fmt.Sprintf(zooStatement, noFeedings), Options{}, nil)
	requireRows(t, res, "cost", int64(0), int64(0), int64(0), int64(0), int64(0), int64(0), int64(0), int64(0))
	want := []any{
		`g:["lion","GBP"]`, "Lions", homeCurrency, `s:["lion","GBP"]`, "Total Lions", homeCurrency,
		`g:["zebra","GBP"]`, "Zebras", homeCurrency, `s:["zebra","GBP"]`, "Total Zebras", homeCurrency,
		`g:["monkey","GBP"]`, "Monkeys", homeCurrency, `s:["monkey","GBP"]`, "Total Monkeys", homeCurrency,
		`c:gap:["GBP"]`, "Lions less zebras", homeCurrency, `c:all:["GBP"]`, "All animals", homeCurrency,
	}
	if got := cellCurrencies(res); !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %#v\nwant %#v", got, want)
	}
}

// Each currency key reads its own source's currency over no rows: a feeding's
// cost the feedings', its animal's fee the animals'.
func TestEachCurrencyKeyReadsItsOwnSourceCurrency(t *testing.T) {
	z := newZoo(t)
	withFeedingsCurrency(t)
	spec := strings.Replace(layoutOf(lions+`,{"row":"r","label":"R","formula":{"subtotal":"cats"}}`),
		`"aggregate":"sum"}]`, `"aggregate":"sum"},{"id":"fee","path":[{"link":"animal"},{"field":"fee"}],"aggregate":"sum"}]`, 1)
	spec = strings.Replace(spec, `"groups"`, noFeedings+`"groups"`, 1)
	res := z.run(t, "ann", spec, Options{}, nil)
	requireRows(t, res, "kind", "group", "subtotal", "computed")
	requireRows(t, res, "cost@currency", homeCurrency, homeCurrency, homeCurrency)
	requireRows(t, res, "fee@currency", "USD", "USD", "USD")
}

// With no rows the fill records sit under one total in the source's
// currency, and each reads 0 in it: on the paged path (no pivot), on the
// unpaged one (a pivot), and on the unpaged one when the row cap cuts fill
// records.
func TestFillWithNoRowsIsInTheSourceCurrency(t *testing.T) {
	none := `{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"none"}]}`
	pivot := strings.Replace(animalFill(`{}`, none), `"sort"`, `"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"cost"},"sort"`, 1)
	for name, tc := range map[string]struct {
		spec   string
		cap    int
		labels []any
	}{
		"paged":  {animalFill(`{}`, none), 0, []any{"Eli", "Kit", "Leo", "Lia", "Zed", "Total"}},
		"pivot":  {pivot, 0, []any{"Eli", "Kit", "Leo", "Lia", "Zed", "Total"}},
		"capped": {pivot, 2, []any{"Eli", "Kit", "Total"}},
	} {
		t.Run(name, func(t *testing.T) {
			z := newZoo(t)
			withEmptyAnimals(t, z)
			withFeedingsCurrency(t)
			if tc.cap > 0 {
				withLimits(t, func(l *limits) { l.rowCap = tc.cap })
			}
			res := z.run(t, "ann", tc.spec, Options{}, nil)
			requireRows(t, res, "label", tc.labels...)
			for _, r := range res.Rows {
				if !strings.HasSuffix(r.ID, `"GBP"]`) {
					t.Fatalf("row id %q, want it in GBP", r.ID)
				}
				if r.Kind == KindGroup && (r.Cells["cost"] != int64(0) || r.Cells["cost@currency"] != homeCurrency) {
					t.Fatalf("%s: cost %#v %#v, want 0 GBP", r.Label, r.Cells["cost"], r.Cells["cost@currency"])
				}
			}
		})
	}
}

// A source with no Currency func reads no currency over no rows, as before.
func TestAnEmptyLayoutWithoutASourceCurrencyHasNone(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", fmt.Sprintf(zooStatement, noFeedings), Options{}, nil)
	for _, r := range res.Rows {
		if cur, has := r.Cells["cost@currency"]; has && cur != nil {
			t.Fatalf("%s: currency %#v, want none", r.Label, cur)
		}
	}
	if res.Rows[6].ID != `c:gap:[null]` {
		t.Fatalf("computed row id = %q", res.Rows[6].ID)
	}
}

// A grouped report with no rows ends in a total in the source's currency. It
// totals nothing, so its amount stays blank.
func TestATotalOverNoRowsIsInTheSourceCurrency(t *testing.T) {
	z := newZoo(t)
	withFeedingsCurrency(t)
	spec := strings.Replace(layoutOf(lions), `,"layout":[`+lions+`]`, "", 1)
	spec = strings.Replace(spec, `"groups"`, noFeedings+`"groups"`, 1)
	res := z.run(t, "ann", spec, Options{}, nil)
	requireRows(t, res, "kind", "total")
	requireRows(t, res, "cost", nil)
	if res.Rows[0].ID != `total:["GBP"]` {
		t.Fatalf("total id = %q", res.Rows[0].ID)
	}
}

// Rows in their own currencies keep them: an empty section reads 0 in each
// currency the rows have, and the source's currency, which no row has,
// makes no row of its own.
func TestRowsKeepTheirCurrencyOverTheSourceCurrency(t *testing.T) {
	z := newZoo(t)
	withFeedingsCurrency(t)
	z.rec(t, "f7", "zoo_feedings", map[string]any{
		"animal": z.id("zed"), "cost": 400, "currency": "EUR", "fed": "2026-03-10 00:00:00.000Z", "grams": 10, "note": "f7",
	})
	res := z.run(t, "ann", fmt.Sprintf(zooStatement, ""), Options{}, nil)
	var ids []string
	for _, r := range res.Rows {
		ids = append(ids, r.ID)
		if r.Cells["cost@currency"] == homeCurrency || strings.Contains(r.ID, homeCurrency) {
			t.Fatalf("%s %s: in the source's currency", r.ID, r.Label)
		}
	}
	want := []string{
		`g:["lion","EUR"]`, `s:["lion","EUR"]`, `g:["zebra","EUR"]`, `s:["zebra","EUR"]`, `g:["monkey","EUR"]`, `s:["monkey","EUR"]`, `c:gap:["EUR"]`, `c:all:["EUR"]`,
		`g:["lion","USD"]`, `s:["lion","USD"]`, `g:["zebra","USD"]`, `s:["zebra","USD"]`, `g:["monkey","USD"]`, `s:["monkey","USD"]`, `c:gap:["USD"]`, `c:all:["USD"]`,
	}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("ids = %#v\nwant %#v", ids, want)
	}
	requireRows(t, res, "cost@currency",
		"EUR", "EUR", "EUR", "EUR", "EUR", "EUR", "EUR", "EUR",
		"USD", "USD", "USD", "USD", "USD", "USD", "USD", "USD")
}

// A row whose currency field is blank is a row with no currency: the
// source's currency is for no rows only, never for a row that names none.
func TestARowWithABlankCurrencyGetsNoSourceCurrency(t *testing.T) {
	z := newZoo(t)
	withFeedingsCurrency(t)
	z.rec(t, "f7", "zoo_feedings", map[string]any{
		"animal": z.id("zed"), "cost": 400, "currency": "", "fed": "2026-03-10 00:00:00.000Z", "grams": 10, "note": "f7",
	})
	only := strings.Replace(layoutOf(lions), `"groups"`, `"filter":{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"f7"}]},"groups"`, 1)
	res := z.run(t, "ann", only, Options{}, nil)
	for _, r := range res.Rows {
		if cur := r.Cells["cost@currency"]; cur == homeCurrency {
			t.Fatalf("%s %s: currency %#v, want none from the source", r.ID, r.Label, cur)
		}
	}
}
