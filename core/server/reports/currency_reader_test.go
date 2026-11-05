package reports

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// fallbackCurrency is the fixed currency the animals' source gives a reader
// who may not view the office row; a reader who may gets officeCurrency.
const fallbackCurrency = "XTS"

// withOfficeCurrency adds the office row and re-registers zoo.animals with a
// Currency that reads the row's currency for its reader, as a package whose
// home currency lives in a rule-guarded collection does. In this copy fee
// names no currency field, so price and fee are both in the fixed currency.
// ann may view the row; bob may not, so he gets fallbackCurrency. It returns
// the reader of each Currency call, in order. newZoo's cleanup resets the
// registry.
func withOfficeCurrency(t *testing.T, z *zoo) *[]*core.RequestInfo {
	t.Helper()
	row := addOffice(t, z)
	readers := &[]*core.RequestInfo{}
	src := *lookupRegistered("zoo.animals")
	src.Fields = slices.Clone(src.Fields)
	for i := range src.Fields {
		if src.Fields[i].Key == "fee" {
			src.Fields[i].Currency = ""
		}
	}
	src.Currency = func(app core.App, reader *core.RequestInfo) string {
		*readers = append(*readers, reader)
		if reader == nil {
			return fallbackCurrency
		}
		rec, err := FindViewableRecord(app, reader, "zoo_office", row.Id)
		if errors.Is(err, sql.ErrNoRows) {
			return fallbackCurrency
		}
		if err != nil {
			t.Errorf("FindViewableRecord: %v", err)
			return fallbackCurrency
		}
		return rec.GetString("currency")
	}
	RegisterSource(src)
	return readers
}

// animalsWorth groups the animals by species, each group labelled by its
// total price, with two amounts in the fixed currency (price and fee) and an
// optional amount floor on the price.
const animalsWorth = `{"version":2,"source":"zoo.animals",
	"params":[{"id":"over","type":"amount","label":"Over"}],
	"columns":[{"id":"species","path":[{"field":"species"}]},
		{"id":"name","path":[{"field":"name"}]},
		{"id":"price","path":[{"field":"price"}],"aggregate":"sum"},
		{"id":"fee","path":[{"field":"fee"}],"aggregate":"sum"}],
	"filter":{"op":"gte","args":[{"path":[{"field":"price"}]},{"param":"over"}]},
	"groups":[{"id":"g","col":"species","label":{"op":"sum","args":[{"path":[{"field":"price"}]}]}}],
	"sort":[{"col":"name"}]}`

// animalsByPrice pivots the animals' fees over their prices: one column per
// price, labelled by the price and its currency.
const animalsByPrice = `{"version":2,"source":"zoo.animals","columns":[
	{"id":"active","path":[{"field":"active"}]},
	{"id":"fee","path":[{"field":"fee"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"active"}],
	"pivot":{"path":[{"field":"price"}],"cell":"fee"}}`

// feedingsByAnimalPrice labels each feeding's animal by its price: a label
// read from the animals' source, in its fixed currency.
const feedingsByAnimalPrice = `{"version":2,"source":"zoo.feedings","columns":[
	{"id":"animal","path":[{"link":"animal"}],"labelField":"price"},
	{"id":"note","path":[{"field":"note"}]}],"sort":[{"col":"note"}]}`

// animalsPriceOptions is an Animals filter whose options are labelled and
// grouped by the animal's price.
var animalsPriceOptions = strings.Replace(animalsParam, `"label":[{"field":"name"}],"group":[{"field":"species"}]`,
	`"label":[{"field":"price"}],"group":[{"field":"price"}]`, 1)

// currencyReaders is who may view the office row (ann) and who may not (bob):
// the fixed currency each gets and the one each must never get.
var currencyReaders = []struct {
	user, want, never string
}{
	{"ann", officeCurrency, fallbackCurrency},
	{"bob", fallbackCurrency, officeCurrency},
}

// requireNoCurrency fails when code is anywhere in what the result sends.
func requireNoCurrency(t *testing.T, what string, v any, code string) {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), strconv.Quote(code)) || strings.Contains(string(raw), " "+code+`"`) {
		t.Fatalf("%s sends %s: %s", what, code, raw)
	}
}

// Review Focus (Part 2): the money columns and each group's amount label
// carry the reader's fixed currency; bob, who may not view the office row,
// never gets its code.
func TestAFixedCurrencyShowsOnlyWhatTheReaderMayView(t *testing.T) {
	z := newZoo(t)
	withOfficeCurrency(t, z)
	groups := map[string]int{"ann": 3, "bob": 2} // ann: cat, lion, zebra; bob: monkey, zebra
	for _, c := range currencyReaders {
		res := z.run(t, c.user, animalsWorth, Options{ExpandAll: true}, nil)
		for _, id := range []string{"price", "fee"} {
			i := slices.IndexFunc(res.Columns, func(rc ResultColumn) bool { return rc.ID == id })
			if i < 0 {
				t.Fatalf("%s: no %s column in %+v", c.user, id, res.Columns)
			}
			if rc := res.Columns[i]; rc.Currency != c.want || rc.CurrencyCell != "" {
				t.Fatalf("%s: %s column currency %q, cell %q, want %q", c.user, id, rc.Currency, rc.CurrencyCell, c.want)
			}
		}
		n := 0
		for _, row := range res.Rows {
			if row.Kind != KindGroup {
				continue
			}
			n++
			if row.LabelType != Money || row.LabelCurrency != c.want {
				t.Fatalf("%s: group %s label %v %v %q, want an amount in %s", c.user, row.ID, row.LabelType, row.LabelValue, row.LabelCurrency, c.want)
			}
		}
		if n != groups[c.user] {
			t.Fatalf("%s: %d group rows, want %d", c.user, n, groups[c.user])
		}
		requireNoCurrency(t, c.user+"'s run", res, c.never)
	}
}

// A record label read from the animals' source and a pivot over their prices
// carry the reader's fixed currency, in each cell, row label and column.
func TestAFixedCurrencyInLabelsAndPivotsFollowsTheReader(t *testing.T) {
	z := newZoo(t)
	withOfficeCurrency(t, z)
	feedings := map[string]int{"ann": 4, "bob": 3} // ann: f1-f4; bob: f3-f5
	pivots := map[string]int{"ann": 3, "bob": 2}   // ann: 5000, 120000, 500000; bob: 30000, 120000
	for _, c := range currencyReaders {
		labels := z.run(t, c.user, feedingsByAnimalPrice, Options{}, nil)
		n := 0
		for _, row := range labels.Rows {
			if row.Kind != KindDetail {
				continue
			}
			n++
			if got := row.Cells["animal@labelCurrency"]; got != c.want {
				t.Fatalf("%s: %s animal@labelCurrency = %#v, want %q", c.user, row.Cells["note"], got, c.want)
			}
			if row.LabelType != Money || row.LabelCurrency != c.want {
				t.Fatalf("%s: %s label %v %q, want an amount in %s", c.user, row.Cells["note"], row.LabelType, row.LabelCurrency, c.want)
			}
		}
		if n != feedings[c.user] {
			t.Fatalf("%s: %d feedings, want %d", c.user, n, feedings[c.user])
		}
		requireNoCurrency(t, c.user+"'s labels", labels, c.never)

		pivot := z.run(t, c.user, animalsByPrice, Options{}, nil)
		n = 0
		for _, rc := range pivot.Columns {
			if rc.Pivot == nil {
				continue
			}
			n++
			if rc.Currency != c.want || rc.Pivot.LabelType != Money || rc.Pivot.LabelCurrency != c.want {
				t.Fatalf("%s: pivot column %s currency %q, label %v %q, want %s", c.user, rc.ID, rc.Currency, rc.Pivot.LabelType, rc.Pivot.LabelCurrency, c.want)
			}
		}
		if n != pivots[c.user] {
			t.Fatalf("%s: %d pivot columns, want %d", c.user, n, pivots[c.user])
		}
		requireNoCurrency(t, c.user+"'s pivot", pivot, c.never)
	}
}

// An amount floor compares with the price in the reader's fixed currency: a
// floor in the office's currency keeps ann's animals and none of bob's, a
// floor in the fallback currency the reverse.
func TestAnAmountFloorComparesInTheReadersFixedCurrency(t *testing.T) {
	z := newZoo(t)
	withOfficeCurrency(t, z)
	for _, c := range []struct {
		user, floor string
		kinds       []any
	}{
		{"ann", officeCurrency, []any{"group", "detail", "group", "detail", "group", "detail", "total"}},
		{"ann", fallbackCurrency, []any{"total"}},
		{"bob", officeCurrency, []any{"total"}},
		{"bob", fallbackCurrency, []any{"group", "detail", "group", "detail", "total"}},
	} {
		res := z.run(t, c.user, animalsWorth, Options{ExpandAll: true}, map[string]string{"over": `{"amount":0,"currency":"` + c.floor + `"}`})
		if got := rowsOf(res, "kind"); !slices.Equal(got, c.kinds) {
			t.Fatalf("%s, floor 0 %s: kinds = %v, want %v", c.user, c.floor, got, c.kinds)
		}
	}
}

// Review Focus (Part 2): one run reads a source's fixed currency once, for
// the run's reader, however many money columns, rows, labels, pivot columns
// and comparisons need it, so one result never mixes two values.
func TestARunReadsAFixedCurrencyOnceForItsReader(t *testing.T) {
	z := newZoo(t)
	readers := withOfficeCurrency(t, z)
	spec, info, err := Load([]byte(animalsWorth))
	if err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"x_zoo": "open"}
	params := map[string]json.RawMessage{"over": json.RawMessage(`{"amount":0,"currency":"` + officeCurrency + `"}`)}
	res, err := Run(context.Background(), z.app, Request{Spec: spec, Loaded: info, Params: params, Options: Options{ExpandAll: true}, Auth: z.recs["ann"], Headers: headers})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) < 4 {
		t.Fatalf("rows = %v, want ann's three groups and their animals", rowsOf(res, "kind"))
	}
	if got, want := readerIDs(*readers), []string{z.id("ann")}; !slices.Equal(got, want) {
		t.Fatalf("Currency readers = %v, want %v", got, want)
	}
	if (*readers)[0].Headers["x_zoo"] != "open" {
		t.Fatalf("reader headers = %v, want the run's %v", (*readers)[0].Headers, headers)
	}

	for _, spec := range []string{animalsWorth, animalsByPrice, feedingsByAnimalPrice} {
		for _, user := range []string{"ann", "bob"} {
			*readers = nil
			z.run(t, user, spec, Options{ExpandAll: true}, nil)
			if got, want := readerIDs(*readers), []string{z.id(user)}; !slices.Equal(got, want) {
				t.Fatalf("%s: Currency readers = %v, want %v for\n%s", user, got, want, spec)
			}
		}
	}
}

// One read of a param's options reads the fixed currency once, for its
// reader: each amount label and amount group is in the reader's currency.
func TestParamOptionsReadAFixedCurrencyOnceForTheirReader(t *testing.T) {
	z := newZoo(t)
	readers := withOfficeCurrency(t, z)
	for _, c := range currencyReaders {
		*readers = nil
		res := z.options(t, c.user, animalsPriceOptions, OptionsRequest{Param: "animals"})
		if got, want := readerIDs(*readers), []string{z.id(c.user)}; !slices.Equal(got, want) {
			t.Fatalf("%s: Currency readers = %v, want %v", c.user, got, want)
		}
		if len(res.Options) == 0 || len(res.Groups) == 0 {
			t.Fatalf("%s: options %+v, groups %+v", c.user, res.Options, res.Groups)
		}
		for _, o := range res.Options {
			if o.Type != Money || o.Currency != c.want || !strings.HasSuffix(o.Group, " "+c.want) {
				t.Fatalf("%s: option %+v, want an amount in %s", c.user, o, c.want)
			}
		}
		for _, g := range res.Groups {
			if g.Type != Money || g.Currency != c.want || !strings.HasSuffix(g.Value, " "+c.want) {
				t.Fatalf("%s: group %+v, want an amount in %s", c.user, g, c.want)
			}
		}
		requireNoCurrency(t, c.user+"'s options", res, c.never)
	}
}

// Validate's dry compile has no reader, so it never reads a fixed currency.
func TestValidateNeverReadsAFixedCurrency(t *testing.T) {
	z := newZoo(t)
	readers := withOfficeCurrency(t, z)
	for _, spec := range []string{animalsWorth, animalsByPrice, feedingsByAnimalPrice, animalsPriceOptions,
		zebrasWithZedsPrice, zebraPricesWithF3sCost, sitesByFedAnimalPrice} {
		if err := Validate(z.app, loadSpec(t, spec)); err != nil {
			t.Fatalf("Validate: %v\n%s", err, spec)
		}
	}
	if len(*readers) != 0 {
		t.Fatalf("Validate called Currency %d times, readers %v", len(*readers), readerIDs(*readers))
	}
}

// zebrasWithZedsPrice is the zebras' feedings (f3, f4: 1000 USD) in a
// section, then a computed row with Zed's price: an amount in the animals'
// fixed currency in a column whose currency is each feeding's own, so the
// computed row's currency cell carries the fixed code (layout carriedCell).
// With no sort, detail rows read in record id order, which is random, so the
// spec sorts the feedings by cost (f3 700, then f4 300).
const zebrasWithZedsPrice = `{"version":2,"source":"zoo.feedings","columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"species"}],
	"sort":[{"col":"cost","dir":"desc"}],
	"layout":[{"section":"zebras","title":"Zebras","key":"zebra"},
		{"row":"price","label":"Zed price","formula":{"op":"lookup","source":"zoo.animals",
			"where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Zed"}]},"value":{"path":[{"field":"price"}]}}}]}`

// zebraPricesWithF3sCost is the zebras' prices (Zed: 120000) in a section,
// then a computed row with feeding f3's cost (700 USD), which carries its own
// currency: the price column names a currency cell, and every row that
// carries none shows the fixed code there.
const zebraPricesWithF3sCost = `{"version":2,"source":"zoo.animals","columns":[
	{"id":"species","path":[{"field":"species"}]},
	{"id":"price","path":[{"field":"price"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"species"}],
	"layout":[{"section":"zebras","title":"Zebras","key":"zebra"},
		{"row":"f3","label":"f3 cost","formula":{"op":"lookup","source":"zoo.feedings",
			"where":{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"f3"}]},"value":{"path":[{"field":"cost"}]}}}]}`

// sitesByFedAnimalPrice nests each site's animals and their feedings, each
// feeding labelled by its animal's price: a child row's amount label in the
// animals' fixed currency.
const sitesByFedAnimalPrice = `{"version":2,"source":"zoo.sites","columns":[
	{"id":"site","path":[{"field":"name"}]},
	{"id":"name","path":[{"link":"animals","mode":"nest"},{"field":"name"}]},
	{"id":"fed","path":[{"link":"animals","mode":"nest"},{"link":"feedings","mode":"nest"},{"link":"animal"}],"labelField":"price"}],
	"sort":[{"col":"site"}]}`

// Review follow-up (Part 2): a fixed currency carried into a currency cell,
// by a computed row in another currency's column or beside a computed row
// that carries its own, is the reader's; bob never gets the office's code.
// Each run reads it once.
func TestACarriedFixedCurrencyFollowsTheReader(t *testing.T) {
	z := newZoo(t)
	readers := withOfficeCurrency(t, z)
	for _, c := range currencyReaders {
		*readers = nil
		priced := z.run(t, c.user, zebrasWithZedsPrice, Options{ExpandAll: true}, nil)
		requireRows(t, priced, "label", "Zebras", "zebra", "zebra", "Total Zebras", "Zed price")
		requireOrderedRows(t, zebrasWithZedsPrice, priced, "cost", int64(1000), int64(700), int64(300), int64(1000), int64(120000))
		requireRows(t, priced, "cost@currency", "USD", "USD", "USD", "USD", c.want)
		requireNoCurrency(t, c.user+"'s Zed price", priced, c.never)
		if got, want := readerIDs(*readers), []string{z.id(c.user)}; !slices.Equal(got, want) {
			t.Fatalf("%s: Zed price: Currency readers = %v, want %v", c.user, got, want)
		}

		*readers = nil
		costed := z.run(t, c.user, zebraPricesWithF3sCost, Options{ExpandAll: true}, nil)
		requireRows(t, costed, "kind", "group", "detail", "subtotal", "computed")
		requireRows(t, costed, "price", int64(120000), int64(120000), int64(120000), int64(700))
		requireRows(t, costed, "price@currency", c.want, c.want, c.want, "USD")
		if col := costed.Columns[1]; col.ID != "price" || col.CurrencyCell != "price@currency" || col.Currency != "" {
			t.Fatalf("%s: price column = %+v, want the currency cell", c.user, col)
		}
		requireNoCurrency(t, c.user+"'s f3 cost", costed, c.never)
		if got, want := readerIDs(*readers), []string{z.id(c.user)}; !slices.Equal(got, want) {
			t.Fatalf("%s: f3 cost: Currency readers = %v, want %v", c.user, got, want)
		}
	}
}

// Review follow-up (Part 2): a nested child row's amount label is in the
// reader's fixed currency, in its label cell and its row label.
func TestANestedAmountLabelsFixedCurrencyFollowsTheReader(t *testing.T) {
	z := newZoo(t)
	readers := withOfficeCurrency(t, z)
	feedings := map[string]int{"ann": 4, "bob": 3} // ann: f1, f2 (Leo), f3, f4 (Zed); bob: f5 (Mo), f3, f4 (Zed)
	for _, c := range currencyReaders {
		*readers = nil
		res := z.run(t, c.user, sitesByFedAnimalPrice, Options{ExpandAll: true}, nil)
		n := 0
		for _, row := range res.Rows {
			if row.Depth != 2 {
				continue
			}
			n++
			if row.Parent == "" || row.Cells["fed@labelCurrency"] != c.want || row.LabelType != Money || row.LabelCurrency != c.want {
				t.Fatalf("%s: child row %s: parent %q, cell %#v, label %v %q, want an amount in %s",
					c.user, row.ID, row.Parent, row.Cells["fed@labelCurrency"], row.LabelType, row.LabelCurrency, c.want)
			}
		}
		if n != feedings[c.user] {
			t.Fatalf("%s: %d nested feedings, want %d", c.user, n, feedings[c.user])
		}
		requireNoCurrency(t, c.user+"'s nested labels", res, c.never)
		if got, want := readerIDs(*readers), []string{z.id(c.user)}; !slices.Equal(got, want) {
			t.Fatalf("%s: Currency readers = %v, want %v", c.user, got, want)
		}
	}
}
