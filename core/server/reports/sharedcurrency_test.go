package reports

import (
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// zoo_ledger_months is a stored summary of zoo_ledger_lines: one row for an
// account and a month, with that month's debit and credit totals and their
// currency. Its source's currency field holds the same currency as the
// lines', which it declares with SameCurrencyAs (sameAs "" declares nothing).
//
//	cash   2026-06  debit 500  USD   (its lines are USD)
//	bank   2026-06  debit 900  EUR   (its line is USD: two codes on one row)
//	petty  2026-06  debit 250  GBP   (it has no line)
//
// loan, sales and rent have lines and no month; idle and re have neither.
func withLedgerMonths(t testing.TB, z *zoo, sameAs string) {
	t.Helper()
	accounts, err := z.app.FindCollectionByNameOrId("zoo_ledger_accounts")
	if err != nil {
		t.Fatal(err)
	}
	months := core.NewBaseCollection("zoo_ledger_months")
	months.ListRule, months.ViewRule = accounts.ListRule, accounts.ViewRule
	months.Fields.Add(
		&core.RelationField{Name: "account", CollectionId: accounts.Id, MaxSelect: 1},
		&core.TextField{Name: "first_day"},
		&core.NumberField{Name: "debit_total"},
		&core.NumberField{Name: "credit_total"},
		&core.TextField{Name: "currency"},
	)
	z.save(t, months)
	for _, m := range []struct {
		key, account, currency string
		debit                  int
	}{
		{"cashJune", "cash", "USD", 500},
		{"bankJune", "bank", "EUR", 900},
		{"pettyJune", "petty", "GBP", 250},
	} {
		z.rec(t, m.key, "zoo_ledger_months", map[string]any{
			"account": z.id(m.account), "first_day": "2026-06-01", "debit_total": m.debit, "credit_total": 0, "currency": m.currency,
		})
	}
	RegisterSource(ledgerMonthsSource(sameAs))
}

func ledgerMonthsSource(sameAs string) Source {
	return Source{
		Key: "zoo.ledger_months", Collection: "zoo_ledger_months", Label: "Ledger months",
		Fields: []Field{
			{Key: "first_day", Label: "First day", Type: Date, TextDay: true, Aggregates: MinMax},
			{Key: "debit_total", Label: "Debit total", Type: Money, Aggregates: SumAll, Currency: "currency"},
			{Key: "credit_total", Label: "Credit total", Type: Money, Aggregates: SumAll, Currency: "currency"},
			{Key: "currency", Label: "Currency", Type: Text, SameCurrencyAs: sameAs},
		},
		Links: []Link{{Key: "account", Label: "Account", Target: "zoo.ledger_accounts", Cardinality: ToOne, Field: "account"}},
	}
}

// monthsOf and linesOf are the stored months' net and the posted lines' net
// of the account whose id is ACCOUNT, 0 where it has none.
const (
	monthsOf = `{"op":"coalesce","args":[{"op":"lookup","source":"zoo.ledger_months",
		"where":{"op":"eq","args":[{"path":[{"link":"account"}]},ACCOUNT]},
		"value":{"op":"sub","args":[{"path":[{"field":"debit_total"}]},{"path":[{"field":"credit_total"}]}]},"pick":"sum"},{"value":0}]}`
	linesOf = `{"op":"coalesce","args":[{"op":"lookup","source":"zoo.ledger_lines",
		"where":{"op":"and","args":[
			{"op":"eq","args":[{"path":[{"link":"account"}]},ACCOUNT]},
			{"op":"eq","args":[{"path":[{"field":"posted"}]},{"value":true}]}]},
		"value":` + glNet + `,"pick":"sum"},{"value":0}]}`
)

// monthsAndLines is a report with one row per ledger account: its number and
// the stored months' net added to the lines' net.
func monthsAndLines() string {
	account := strings.NewReplacer("ACCOUNT", `{"outer":[{"field":"id"}]}`)
	return `{"version":4,"source":"zoo.ledger_accounts","columns":[
		{"id":"number","path":[{"field":"number"}]},
		{"id":"net","formula":{"op":"add","args":[` + account.Replace(monthsOf) + `,` + account.Replace(linesOf) + `]}}],
		"sort":[{"col":"number"}]}`
}

// Two sources' amounts do not combine when neither says its currency is the
// other's: R24 holds for one currency field only.
func TestAmountsOfTwoSourcesDoNotCombineWithNoDeclaration(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	withLedgerMonths(t, z, "")
	spec, _, err := Load([]byte(monthsAndLines()))
	if err != nil {
		t.Fatal(err)
	}
	err = Validate(z.app, spec)
	if err == nil || !strings.Contains(err.Error(), "amounts in different currencies cannot be combined") {
		t.Fatalf("Validate = %v, want the refusal to combine two currencies", err)
	}
}

// With the declaration the months' and the lines' amounts add up. On each row
// the codes are checked: one code on both sides gives the sum in it, a side
// with no row leaves the other side's amount and code, two codes give a blank
// with no code and the run's mixedCurrency notice, and no row on either side
// gives 0 with no code.
func TestAmountsOfTwoSourcesThatShareACurrencyCombine(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	withLedgerMonths(t, z, "zoo.ledger_lines.currency")
	if err := CheckSources(z.app); err != nil {
		t.Fatalf("CheckSources: %v", err)
	}
	res := z.run(t, "ann", monthsAndLines(), Options{}, nil)
	type cell struct{ net, currency any }
	got := map[string]cell{}
	for _, r := range res.Rows {
		if r.Kind == KindDetail {
			got[r.Cells["number"].(string)] = cell{r.Cells["net"], r.Cells["net@currency"]}
		}
	}
	want := map[string]cell{
		"1000": {int64(-6500), "USD"}, // cash: month 500, lines -7000
		"1100": {nil, nil},            // bank: month EUR, line USD
		"1200": {int64(250), "GBP"},   // petty: a month and no line
		"1300": {int64(0), nil},       // idle: neither
		"2000": {int64(-4500), "USD"}, // loan: lines and no month
		"3900": {int64(0), nil},       // retained earnings: neither
		"4000": {int64(-500), "USD"},  // sales: lines and no month
		"5000": {int64(10000), "USD"}, // rent: lines and no month
	}
	if len(got) != len(want) {
		t.Fatalf("rows = %v\nwant %v", got, want)
	}
	for number, w := range want {
		if got[number] != w {
			t.Errorf("%s = %#v, want %#v", number, got[number], w)
		}
	}
	if !slices.ContainsFunc(res.Notices, func(n Notice) bool { return n.Code == "mixedCurrency" && slices.Contains(n.Columns, "net") }) {
		t.Fatalf("notices = %+v, want mixedCurrency naming net", res.Notices)
	}
}

// The same holds where one amount is the base row's own and the other a
// looked-up row's of the other source: a line's debit added to its account's
// stored months. The bank line (USD) meets the bank month (EUR) and is blank.
func TestABaseRowsAmountCombinesWithASharedCurrencyLookup(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	withLedgerMonths(t, z, "zoo.ledger_lines.currency")
	months := strings.ReplaceAll(monthsOf, "ACCOUNT", `{"outer":[{"link":"account"}]}`)
	spec := `{"version":4,"source":"zoo.ledger_lines","columns":[
		{"id":"account","path":[{"link":"account"},{"field":"number"}]},
		{"id":"day","path":[{"field":"date"}]},
		{"id":"both","formula":{"op":"add","args":[{"path":[{"field":"debit"}]},` + months + `]}}],
		"filter":{"op":"and","args":[
			{"op":"eq","args":[{"path":[{"field":"posted"}]},{"value":true}]},
			{"op":"in","args":[{"col":"account"},{"value":["1000","1100","2000"]}]}]},
		"sort":[{"col":"account"},{"col":"day"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	var got [][3]any
	for _, r := range res.Rows {
		if r.Kind == KindDetail {
			got = append(got, [3]any{r.Cells["account"], r.Cells["both"], r.Cells["both@currency"]})
		}
	}
	want := [][3]any{
		{"1000", int64(500), "USD"},  // j1a: debit 0 + month 500
		{"1000", int64(3500), "USD"}, // j2a: debit 3000 + month 500
		{"1100", nil, nil},           // j0a: USD line, EUR month
		{"2000", int64(0), "USD"},    // j2b: debit 0, no month
	}
	if !slices.Equal(got, want) {
		t.Fatalf("rows = %#v\nwant %#v", got, want)
	}
}

// currencyField gives one name to a currency field and to a field that
// declares it holds the same currency; a field with no declaration keeps its
// own name, so every report that names no such field compiles as before.
func TestCurrencyFieldNamesTheDeclaredField(t *testing.T) {
	ResetForTesting()
	t.Cleanup(ResetForTesting)
	months := ledgerMonthsSource("zoo.ledger_lines.currency")
	alone := ledgerMonthsSource("")
	for name, c := range map[string]struct {
		src  *Source
		key  string
		want string
	}{
		"declared":       {&months, "debit_total", "zoo.ledger_lines.currency"},
		"not declared":   {&alone, "credit_total", "zoo.ledger_months.currency"},
		"not an amount":  {&months, "first_day", ""},
		"the code field": {&months, "currency", ""},
	} {
		if got := currencyField(&pathPlan{src: c.src, field: c.src.field(c.key)}); got != c.want {
			t.Errorf("%s: currencyField = %q, want %q", name, got, c.want)
		}
	}
}

func TestRegisterSourceRefusesAMalformedSameCurrency(t *testing.T) {
	cases := map[string]struct {
		edit func(*Source)
		want string
	}{
		"on a field no money field names": {func(s *Source) { s.Fields[0].SameCurrencyAs = "zoo.ledger_lines.currency" }, "only a currency field of a money field"},
		"no source key":                   {func(s *Source) { s.Fields[3].SameCurrencyAs = "currency" }, "<source key>.<field key>"},
		"no field key":                    {func(s *Source) { s.Fields[3].SameCurrencyAs = "zoo.ledger_lines." }, "<source key>.<field key>"},
		"its own source":                  {func(s *Source) { s.Fields[3].SameCurrencyAs = "zoo.ledger_months.currency" }, "is a field of this source"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			ResetForTesting()
			t.Cleanup(ResetForTesting)
			src := ledgerMonthsSource("")
			c.edit(&src)
			defer func() {
				r := recover()
				if msg, _ := r.(string); r == nil || !strings.Contains(msg, c.want) {
					t.Fatalf("panic %v does not mention %q", r, c.want)
				}
			}()
			RegisterSource(src)
		})
	}
}

// What needs the other source is checked when every source is registered:
// CheckSources refuses the source that declares, and leaves the other usable.
func TestCheckSourcesRefusesASameCurrencyThatNamesNoCurrencyField(t *testing.T) {
	cases := map[string]struct {
		sameAs, want string
		chain        bool
	}{
		"unregistered source":   {"zoo.ledger_weeks.currency", `same-currency source "zoo.ledger_weeks" is not registered`, false},
		"unknown field":         {"zoo.ledger_lines.code", `"zoo.ledger_lines.code" is not the currency field of a money field`, false},
		"not a currency field":  {"zoo.ledger_lines.date", `"zoo.ledger_lines.date" is not the currency field of a money field`, false},
		"a field that declares": {"zoo.ledger_years.currency", `"zoo.ledger_years.currency" names "zoo.ledger_lines.currency" itself: name that field`, true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			z := newZoo(t)
			withBalanceZoo(t, z)
			withLedgerMonths(t, z, c.sameAs)
			if c.chain {
				years := ledgerMonthsSource("zoo.ledger_lines.currency")
				years.Key = "zoo.ledger_years"
				RegisterSource(years)
			}
			err := CheckSources(z.app)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("CheckSources = %v, want it to contain %q", err, c.want)
			}
			if lookupSource(z.app, "zoo.ledger_months") != nil {
				t.Fatal("the refused source is still usable")
			}
			if lookupSource(z.app, "zoo.ledger_lines") == nil {
				t.Fatal("a sound source was refused")
			}
		})
	}
}
