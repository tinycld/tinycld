package reports

import "testing"

const budgetHaving = `{"version":2,"source":"zoo.feedings","params":[{"id":"budget","type":"amount","label":"Budget"}],"columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
	{"id":"left","formula":{"op":"sub","args":[{"param":"budget"},{"op":"sum","args":[{"path":[{"field":"cost"}]}]}]}}],
	"groups":[{"id":"g","col":"species","having":{"op":"gte","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"param":"budget"}]}}],
	"sort":[{"col":"species"}]}`

// An amount param compares and combines only with amounts of its currency.
func TestAnAmountParamKeepsToItsCurrency(t *testing.T) {
	z := newZoo(t)
	z.rec(t, "f7", "zoo_feedings", map[string]any{"animal": z.id("leo"), "cost": 100, "currency": "EUR",
		"fed": "2026-01-07 00:00:00.000Z", "grams": 10, "note": "f7", "due": "2026-01-07"})
	all := z.run(t, "ann", budgetHaving, Options{}, nil)
	requireRows(t, all, "label", "lion", "Total", "lion", "zebra", "Total")

	usd := z.run(t, "ann", budgetHaving, Options{}, map[string]string{"budget": `{"amount":1500,"currency":"USD"}`})
	requireRows(t, usd, "label", "Total", "lion", "Total")
	requireRows(t, usd, "left", nil, int64(-2000), int64(-3000))

	eur := z.run(t, "ann", budgetHaving, Options{}, map[string]string{"budget": `{"amount":50,"currency":"EUR"}`})
	requireRows(t, eur, "label", "lion", "Total", "Total")
	requireRows(t, eur, "left", int64(-50), int64(-50), nil)

	any := z.run(t, "ann", budgetHaving, Options{}, map[string]string{"budget": `1500`})
	requireRows(t, any, "label", "Total", "lion", "Total")
}

func TestAnAmountParamValueIsChecked(t *testing.T) {
	z := newZoo(t)
	for _, bad := range []string{`"12"`, `{"amount":1.5,"currency":"USD"}`, `{"amount":100,"currency":"usd"}`, `{"amount":100,"rate":2}`} {
		_, err := z.try(t, "ann", budgetHaving, Options{}, map[string]string{"budget": bad})
		requireSpecError(t, err, 400, "params.budget", "an amount is")
	}
}

// A filter compares an amount param with each row's amount in the row's own
// currency: a USD floor never keeps a EUR row because its cents are larger.
const costFloor = `{"version":2,"source":"zoo.feedings","params":[{"id":"floor","type":"amount","label":"Floor"}],"columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cost","path":[{"field":"cost"}]}],
	"filter":{"op":"gte","args":[{"path":[{"field":"cost"}]},{"param":"floor"}]},
	"sort":[{"col":"note"}]}`

func TestAFilterKeepsAnAmountParamToTheRowsCurrency(t *testing.T) {
	z := newZoo(t)
	z.rec(t, "f7", "zoo_feedings", map[string]any{"animal": z.id("leo"), "cost": 5000, "currency": "EUR",
		"fed": "2026-01-07 00:00:00.000Z", "grams": 10, "note": "f7", "due": "2026-01-07"})
	usd := z.run(t, "ann", costFloor, Options{}, map[string]string{"floor": `{"amount":1000,"currency":"USD"}`})
	requireRows(t, usd, "label", "f1", "f2", "Total")
	eur := z.run(t, "ann", costFloor, Options{}, map[string]string{"floor": `{"amount":1000,"currency":"EUR"}`})
	requireRows(t, eur, "label", "f7", "Total")
	anyCurrency := z.run(t, "ann", costFloor, Options{}, map[string]string{"floor": `1000`})
	requireRows(t, anyCurrency, "label", "f1", "f2", "f7", "Total", "Total")
	unset := z.run(t, "ann", costFloor, Options{}, nil)
	requireRows(t, unset, "label", "f1", "f2", "f3", "f4", "f7", "Total", "Total")
}

// A column that shows an amount param shows it in the param's currency, and
// the resolved params echo the amount and its currency.
const budgetColumn = `{"version":2,"source":"zoo.feedings","params":[{"id":"budget","type":"amount","label":"Budget"}],"columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"budget","formula":{"param":"budget"}}],
	"filter":{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"f1"}]}}`

func TestAnAmountParamShowsItsCurrency(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", budgetColumn, Options{}, map[string]string{"budget": `{"amount":150000,"currency":"USD"}`})
	requireRows(t, res, "budget", int64(150000), int64(150000))
	requireRows(t, res, "budget@currency", "USD", "USD")
	if len(res.Params) != 1 {
		t.Fatalf("params = %+v", res.Params)
	}
	got, ok := res.Params[0].Value.(map[string]any)
	if !ok || got["amount"] != int64(150000) || got["currency"] != "USD" {
		t.Fatalf("resolved budget = %#v", res.Params[0].Value)
	}
}

// An amount a filter reads with no column of it carries its currency field
// read beside it; a source's fixed currency is that source's code.
const animalsOver = `{"version":2,"source":"zoo.animals","params":[{"id":"over","type":"amount","label":"Over"}],"columns":[
	{"id":"name","path":[{"field":"name"}]}],
	"filter":{"op":"or","args":[
		{"op":"gte","args":[{"path":[{"field":"fee"}]},{"param":"over"}]},
		{"op":"gte","args":[{"path":[{"field":"price"}]},{"param":"over"}]}]},
	"sort":[{"col":"name"}]}`

func TestAFilterKeepsAnAmountParamToAFieldOrFixedCurrency(t *testing.T) {
	z := newZoo(t)
	all := z.run(t, "ann", animalsOver, Options{}, nil)
	requireRows(t, all, "label", "Kit", "Leo", "Zed", "Total")
	eur := z.run(t, "ann", animalsOver, Options{}, map[string]string{"over": `{"amount":100,"currency":"EUR"}`})
	requireRows(t, eur, "label", "Leo", "Total")
	usd := z.run(t, "ann", animalsOver, Options{}, map[string]string{"over": `{"amount":200000,"currency":"USD"}`})
	requireRows(t, usd, "label", "Leo", "Total")
	usdFee := z.run(t, "ann", animalsOver, Options{}, map[string]string{"over": `{"amount":5000,"currency":"USD"}`})
	requireRows(t, usdFee, "label", "Kit", "Leo", "Zed", "Total")
}
