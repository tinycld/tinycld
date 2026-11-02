package reports

import (
	"fmt"
	"strings"
	"testing"
)

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
	for _, bad := range []string{`"12"`, `{"amount":1.5,"currency":"USD"}`, `{"amount":100,"currency":"usd"}`, `{"amount":100,"rate":2}`,
		`{"currency":"USD"}`, `{"amount":"100"}`, `1e3`, `9007199254740992`, `-9007199254740992`, `{"amount":9007199254740992,"currency":"USD"}`} {
		_, err := z.try(t, "ann", budgetHaving, Options{}, map[string]string{"budget": bad})
		requireSpecError(t, err, 400, "params.budget", "an amount is")
	}
	// The largest amount the shell and a stored amount keep exact is accepted.
	for _, good := range []string{`9007199254740991`, `{"amount":-9007199254740991,"currency":"USD"}`} {
		if _, err := z.try(t, "ann", budgetHaving, Options{}, map[string]string{"budget": good}); err != nil {
			t.Fatalf("%s: %v", good, err)
		}
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

// withEuroLeo adds f7: a EUR feeding of leo that ann reads.
func withEuroLeo(t *testing.T, z *zoo, cost int) {
	t.Helper()
	z.rec(t, "f7", "zoo_feedings", map[string]any{"animal": z.id("leo"), "cost": cost, "currency": "EUR",
		"fed": "2026-01-07 00:00:00.000Z", "grams": 10, "note": "f7", "due": "2026-01-07"})
}

// A comparison compares the amount and the currency: across currencies ne is
// true and every other comparison false, never blank.
const costCompared = `{"version":2,"source":"zoo.feedings","params":[{"id":"floor","type":"amount","label":"Floor"}],"columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"over","formula":{"op":"gte","args":[{"path":[{"field":"cost"}]},{"param":"floor"}]}},
	{"id":"same","formula":{"op":"eq","args":[{"path":[{"field":"cost"}]},{"param":"floor"}]}}],
	%s"sort":[{"col":"note"}]}`

func TestAComparisonAcrossCurrenciesIsDefinite(t *testing.T) {
	z := newZoo(t)
	withEuroLeo(t, z, 1000)
	usd := map[string]string{"floor": `{"amount":1000,"currency":"USD"}`}
	res := z.run(t, "ann", fmt.Sprintf(costCompared, ""), Options{}, usd)
	requireRows(t, res, "label", "f1", "f2", "f3", "f4", "f7", "Total", "Total")
	requireRows(t, res, "over", true, true, false, false, false, nil, nil)
	requireRows(t, res, "same", true, false, false, false, false, nil, nil)
	ne := `"filter":{"op":"ne","args":[{"path":[{"field":"cost"}]},{"param":"floor"}]},`
	notFloor := z.run(t, "ann", fmt.Sprintf(costCompared, ne), Options{}, usd)
	requireRows(t, notFloor, "label", "f2", "f3", "f4", "f7", "Total", "Total")
}

// Two amounts in different currencies compare as unequal, whatever their
// cents: Leo's fee is 700 EUR, his price 500000 USD.
const feeAgainstPrice = `{"version":2,"source":"zoo.animals","params":[{"id":"x","type":"amount"}],"columns":[
	{"id":"name","path":[{"field":"name"}]}],
	"filter":%s,"sort":[{"col":"name"}]}`

func TestAmountsInTwoCurrenciesCompareAsUnequal(t *testing.T) {
	z := newZoo(t)
	lt := z.run(t, "ann", fmt.Sprintf(feeAgainstPrice, `{"op":"lt","args":[{"path":[{"field":"fee"}]},{"path":[{"field":"price"}]}]}`), Options{}, nil)
	requireRows(t, lt, "label", "Kit", "Zed", "Total")
	ne := z.run(t, "ann", fmt.Sprintf(feeAgainstPrice, `{"op":"ne","args":[{"path":[{"field":"fee"}]},{"path":[{"field":"price"}]}]}`), Options{}, nil)
	requireRows(t, ne, "label", "Kit", "Leo", "Zed", "Total")
	// between checks every amount's currency, not only one of them.
	between := fmt.Sprintf(feeAgainstPrice, `{"op":"between","args":[{"param":"x"},{"path":[{"field":"fee"}]},{"path":[{"field":"price"}]}]}`)
	res := z.run(t, "ann", between, Options{}, map[string]string{"x": `{"amount":1000,"currency":"USD"}`})
	requireRows(t, res, "label", "Kit", "Zed", "Total")
}

// Two amount params in different currencies hold against nothing; in one,
// they combine.
const twoParams = `{"version":2,"source":"zoo.feedings","params":[{"id":"a","type":"amount"},{"id":"b","type":"amount"}],"columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"diff","formula":{"op":"sub","args":[{"param":"a"},{"param":"b"}]}},
	{"id":"same","formula":{"op":"eq","args":[{"param":"a"},{"param":"b"}]}}],
	"filter":{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"f1"}]}}`

func TestTwoAmountParamsInTwoCurrencies(t *testing.T) {
	z := newZoo(t)
	apart := z.run(t, "ann", twoParams, Options{}, map[string]string{"a": `{"amount":500,"currency":"USD"}`, "b": `{"amount":500,"currency":"EUR"}`})
	requireRows(t, apart, "diff", nil, nil)
	requireRows(t, apart, "same", false, false)
	one := z.run(t, "ann", twoParams, Options{}, map[string]string{"a": `{"amount":500,"currency":"USD"}`, "b": `{"amount":200,"currency":"USD"}`})
	requireRows(t, one, "diff", int64(300), int64(300))
	requireRows(t, one, "diff@currency", "USD", "USD")
	requireRows(t, one, "same", false, false)
}

// A ratio of an amount to an amount param, and a param inside a total, hold
// only in the param's currency.
const budgetShares = `{"version":2,"source":"zoo.feedings","params":[{"id":"budget","type":"amount"}],"columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"share","formula":{"op":"div","scale":2,"args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"param":"budget"}]}},
	{"id":"left","formula":{"op":"sum","args":[{"op":"sub","args":[{"param":"budget"},{"path":[{"field":"cost"}]}]}]}}],
	"groups":[{"id":"g","col":"species"}],"sort":[{"col":"species"}]}`

func TestARatioAndATotalKeepToTheParamsCurrency(t *testing.T) {
	z := newZoo(t)
	withEuroLeo(t, z, 100)
	res := z.run(t, "ann", budgetShares, Options{}, map[string]string{"budget": `{"amount":1000,"currency":"USD"}`})
	requireRows(t, res, "label", "lion", "Total", "lion", "zebra", "Total")
	requireRows(t, res, "share", nil, nil, 3.5, 1.0, 4.5)
	// Per row: 1000-1000 and 1000-2500 for the lion, 1000-700 and 1000-300 for
	// the zebra; the EUR row's amount is blank, so its total adds nothing.
	requireRows(t, res, "left", int64(0), int64(0), int64(-1500), int64(1000), int64(-500))
}

// An amount param in an exists' where holds against the linked rows'
// currency: Leo's only feeding over 3000 is in EUR.
const animalsFedOver = `{"version":2,"source":"zoo.animals","params":[{"id":"over","type":"amount"}],"columns":[
	{"id":"name","path":[{"field":"name"}]}],
	"filter":{"op":"exists","link":[{"link":"feedings"}],"where":{"op":"gte","args":[{"path":[{"field":"cost"}]},{"param":"over"}]}},
	"sort":[{"col":"name"}]}`

func TestAnExistsWhereKeepsAnAmountParamToTheLinkedRowsCurrency(t *testing.T) {
	z := newZoo(t)
	withEuroLeo(t, z, 5000)
	usd := z.run(t, "ann", animalsFedOver, Options{}, map[string]string{"over": `{"amount":3000,"currency":"USD"}`})
	requireRows(t, usd, "label", "Total")
	eur := z.run(t, "ann", animalsFedOver, Options{}, map[string]string{"over": `{"amount":3000,"currency":"EUR"}`})
	requireRows(t, eur, "label", "Leo", "Total")
}

// A fill's where compares the target's amount in its own currency, and a
// parent group's amount (p.<key>) in the parent row's currency.
func TestAFillWhereComparesAmountsInTheirCurrencies(t *testing.T) {
	z := newZoo(t)
	withEmptyAnimals(t, z)
	feeOver := `{"where":{"op":"gte","args":[{"path":[{"field":"fee"}]},{"param":"over"}]}}`
	spec := strings.Replace(animalFill(feeOver, ""), `"columns"`, `"params":[{"id":"over","type":"amount"}],"columns"`, 1)
	// Kit (50 USD) has no rows ann reads; only a USD floor fills it in.
	usd := z.run(t, "ann", spec, Options{}, map[string]string{"over": `{"amount":40,"currency":"USD"}`})
	requireRows(t, usd, "label", "Kit", "Zed", "Leo", "Total")
	eur := z.run(t, "ann", spec, Options{}, map[string]string{"over": `{"amount":40,"currency":"EUR"}`})
	requireRows(t, eur, "label", "Zed", "Leo", "Total")

	// Under each cost, the animals whose fee is at least that cost, in its
	// currency: Leo's 700 EUR fee is not at least 300 USD.
	byCost := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"cost","path":[{"field":"cost"}]},
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"n","path":[{"field":"id"}],"aggregate":"count"}],
		"groups":[{"id":"c","col":"cost"},{"id":"g","col":"animal","fill":{"where":{"op":"gte","args":[{"path":[{"field":"fee"}]},{"col":"cost"}]}}}],
		"sort":[{"col":"cost"},{"formula":{"record":"g","path":[{"field":"name"}]}}]}`, Options{ExpandAll: true}, nil)
	requireRows(t, byCost, "label", "300", "Zed", "300", "700", "Zed", "700", "1000", "Leo", "1000", "2500", "Leo", "2500", "Total")
	requireRows(t, byCost, "kind", "group", "group", "detail", "group", "group", "detail", "group", "group", "detail", "group", "group", "detail", "total")
}

// A lookup's where compares the looked-up rows' amounts in their currency.
const cheapestOver = `{"version":2,"source":"zoo.feedings","params":[{"id":"over","type":"amount"}],"columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cheapest","formula":{"op":"lookup","source":"zoo.feedings","where":{"op":"gte","args":[{"path":[{"field":"cost"}]},{"param":"over"}]},"value":{"path":[{"field":"cost"}]}}}],
	"filter":{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"f1"}]}}`

func TestALookupKeepsAnAmountParamToTheLookedUpRowsCurrency(t *testing.T) {
	z := newZoo(t)
	withEuroLeo(t, z, 5000)
	usd := z.run(t, "ann", cheapestOver, Options{}, map[string]string{"over": `{"amount":2600,"currency":"USD"}`})
	requireRows(t, usd, "cheapest", nil, nil)
	eur := z.run(t, "ann", cheapestOver, Options{}, map[string]string{"over": `{"amount":2600,"currency":"EUR"}`})
	requireRows(t, eur, "cheapest", int64(5000), int64(5000))
	requireRows(t, eur, "cheapest@currency", "EUR", "EUR")
}

// A layout's computed row combines a section's subtotal with an amount param
// (read by a lookup) only in the param's currency.
func TestALayoutRowKeepsAnAmountParamToTheRowsCurrency(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":2,"source":"zoo.feedings","params":[{"id":"budget","type":"amount"}],"columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"}],
		"layout":[
			{"section":"cats","title":"Lions","key":"lion"},
			{"row":"left","label":"Left","formula":{"op":"sub","args":[{"op":"lookup","source":"zoo.sites","value":{"param":"budget"}},{"subtotal":"cats"}]}}]}`
	usd := z.run(t, "ann", spec, Options{}, map[string]string{"budget": `{"amount":5000,"currency":"USD"}`})
	requireRows(t, usd, "label", "Lions", "Total Lions", "Left")
	requireRows(t, usd, "cost", int64(3500), int64(3500), int64(1500))
	eur := z.run(t, "ann", spec, Options{}, map[string]string{"budget": `{"amount":5000,"currency":"EUR"}`})
	requireRows(t, eur, "cost", int64(3500), int64(3500), nil)
}

// A group by a formula over an amount groups by its currency too, and a group
// key's comparison with an amount param holds in the param's currency.
func TestAGroupKeyKeepsAmountsToTheirCurrency(t *testing.T) {
	z := newZoo(t)
	withEuroLeo(t, z, 1000)
	byAmount := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"amount","formula":{"path":[{"field":"cost"}]}},
		{"id":"n","path":[{"field":"id"}],"aggregate":"count"}],
		"groups":[{"id":"g","col":"amount"}],"sort":[{"col":"amount"}]}`, Options{}, nil)
	requireRows(t, byAmount, "n", int64(1), int64(1), int64(1), int64(1), int64(1), int64(1), int64(4))
	big := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","params":[{"id":"over","type":"amount"}],"columns":[
		{"id":"big","formula":{"op":"gte","args":[{"path":[{"field":"cost"}]},{"param":"over"}]}},
		{"id":"n","path":[{"field":"id"}],"aggregate":"count"}],
		"groups":[{"id":"g","col":"big"}],"sort":[{"col":"big"}]}`, Options{}, map[string]string{"over": `{"amount":1000,"currency":"USD"}`})
	requireRows(t, big, "big", false, nil, false, true, nil)
	requireRows(t, big, "n", int64(1), int64(1), int64(2), int64(2), int64(4))
}
