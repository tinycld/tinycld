package reports

import (
	"database/sql"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// A fictional balance report with the spec shape the checkpoint brief names,
// run against the whole engine: a chart of accounts, entries and their lines,
// each line a debit or a credit on one account in the line's own currency (a
// per-row currency field): every line is USD.
//
//	zoo_ledger_accounts  number, full_name, type, detail_type, inactive
//	zoo_ledger_entries   number, memo
//	zoo_ledger_lines     entry, account, date (a text day), debit, credit,
//	                     currency, posted, position
//
// Every ledger collection is listed through a grant row in a separate access
// collection (zoo_ledger_access: user, can_read), the rule shape a package
// that keeps its own members writes: PocketBase compiles it to a DISTINCT
// join, so the plan check below covers a rule SQLite cannot read in place.
func withBalanceZoo(t testing.TB, z *zoo) {
	t.Helper()
	access := core.NewBaseCollection("zoo_ledger_access")
	access.Fields.Add(
		&core.RelationField{Name: "user", CollectionId: z.recs["ann"].Collection().Id, MaxSelect: 1},
		&core.BoolField{Name: "can_read"},
	)
	z.save(t, access)
	z.rec(t, "annLedger", "zoo_ledger_access", map[string]any{"user": z.id("ann"), "can_read": true})
	z.rec(t, "bobLedger", "zoo_ledger_access", map[string]any{"user": z.id("bob"), "can_read": false})
	granted := rule(`@request.auth.id != "" && @collection.zoo_ledger_access:me.user ?= @request.auth.id && @collection.zoo_ledger_access:me.can_read ?= true`)
	accounts := core.NewBaseCollection("zoo_ledger_accounts")
	accounts.ListRule, accounts.ViewRule = granted, granted
	accounts.Fields.Add(
		&core.TextField{Name: "number"},
		&core.TextField{Name: "full_name"},
		&core.SelectField{Name: "type", Values: []string{"asset", "liability", "equity", "income", "expense"}, MaxSelect: 1},
		&core.TextField{Name: "detail_type"},
		&core.BoolField{Name: "inactive"},
	)
	z.save(t, accounts)
	entries := core.NewBaseCollection("zoo_ledger_entries")
	entries.ListRule, entries.ViewRule = granted, granted
	entries.Fields.Add(&core.TextField{Name: "number"}, &core.TextField{Name: "memo"})
	z.save(t, entries)
	lines := core.NewBaseCollection("zoo_ledger_lines")
	lines.ListRule, lines.ViewRule = granted, granted
	lines.Fields.Add(
		&core.RelationField{Name: "entry", CollectionId: entries.Id, MaxSelect: 1},
		&core.RelationField{Name: "account", CollectionId: accounts.Id, MaxSelect: 1},
		&core.TextField{Name: "date"},
		&core.NumberField{Name: "debit"},
		&core.NumberField{Name: "credit"},
		&core.TextField{Name: "currency"},
		&core.BoolField{Name: "posted"},
		&core.NumberField{Name: "position"},
	)
	// The indexes a per-line source of this shape declares, so the time test
	// measures the engine, not a full scan per looked-up row.
	lines.AddIndex("idx_zoo_ledger_lines_account_date", false, "account, date", "")
	lines.AddIndex("idx_zoo_ledger_lines_entry", false, "entry, position", "")
	z.save(t, lines)

	for _, a := range []struct {
		key, number, name, typ, detail string
		inactive                       bool
	}{
		{"cash", "1000", "Cash", "asset", "", false},
		{"bank", "1100", "Bank", "asset", "", false},
		{"petty", "1200", "Petty cash", "asset", "", false},
		{"idle", "1300", "Idle", "asset", "", true},
		{"loan", "2000", "Loan", "liability", "", false},
		{"re", "3900", "Retained earnings", "equity", "retained_earnings", false},
		{"sales", "4000", "Sales", "income", "", false},
		{"rent", "5000", "Rent", "expense", "", false},
	} {
		z.rec(t, a.key, "zoo_ledger_accounts", map[string]any{
			"number": a.number, "full_name": a.name, "type": a.typ, "detail_type": a.detail, "inactive": a.inactive,
		})
	}
	for _, e := range []string{"J0", "J1", "J2", "J3"} {
		z.rec(t, e, "zoo_ledger_entries", map[string]any{"number": e})
	}
	for _, l := range []struct {
		key, entry, account, date string
		debit, credit, position   int
		posted                    bool
	}{
		{"j0a", "J0", "bank", "2025-12-15", 2000, 0, 1, true},
		{"j0b", "J0", "sales", "2025-12-15", 0, 2000, 2, true},
		{"j1a", "J1", "cash", "2026-07-01", 0, 10000, 1, true},
		{"j1b", "J1", "rent", "2026-07-01", 10000, 0, 2, true},
		{"j2a", "J2", "cash", "2026-07-02", 3000, 0, 1, true},
		{"j2b", "J2", "loan", "2026-07-02", 0, 4500, 2, true},
		{"j2c", "J2", "sales", "2026-07-02", 1500, 0, 3, true},
		{"j3a", "J3", "cash", "2026-07-03", 99999, 0, 1, false},
		{"j3b", "J3", "rent", "2026-07-03", 0, 99999, 2, false},
	} {
		z.rec(t, l.key, "zoo_ledger_lines", map[string]any{
			"entry": z.id(l.entry), "account": z.id(l.account), "date": l.date,
			"debit": l.debit, "credit": l.credit, "currency": "USD", "position": l.position, "posted": l.posted,
		})
	}

	RegisterSource(Source{
		Key: "zoo.ledger_accounts", Collection: "zoo_ledger_accounts", Label: "Ledger accounts",
		Fields: []Field{
			{Key: "number", Label: "Number", Type: Text, Aggregates: MinMax},
			{Key: "full_name", Label: "Full name", Type: Text, Aggregates: MinMax},
			{Key: "type", Label: "Type", Type: Select, Options: []Option{
				{Value: "asset", Label: "Asset"}, {Value: "liability", Label: "Liability"}, {Value: "equity", Label: "Equity"},
				{Value: "income", Label: "Income"}, {Value: "expense", Label: "Expense"},
			}},
			{Key: "detail_type", Label: "Detail type", Type: Text},
			{Key: "inactive", Label: "Inactive", Type: Bool},
		},
		Links: []Link{{Key: "lines", Label: "Lines", Target: "zoo.ledger_lines", Cardinality: ToMany, BackField: "account"}},
	})
	RegisterSource(Source{
		Key: "zoo.ledger_entries", Collection: "zoo_ledger_entries", Label: "Ledger entries",
		Fields: []Field{
			{Key: "number", Label: "Number", Type: Text, Aggregates: MinMax},
			{Key: "memo", Label: "Memo", Type: Text},
		},
	})
	RegisterSource(Source{
		Key: "zoo.ledger_lines", Collection: "zoo_ledger_lines", Label: "Ledger lines",
		Fields: []Field{
			{Key: "date", Label: "Date", Type: Date, TextDay: true, Aggregates: MinMax},
			{Key: "debit", Label: "Debit", Type: Money, Aggregates: SumAll, Currency: "currency"},
			{Key: "credit", Label: "Credit", Type: Money, Aggregates: SumAll, Currency: "currency"},
			{Key: "currency", Label: "Currency", Type: Text},
			{Key: "posted", Label: "Posted", Type: Bool},
			{Key: "position", Label: "Position", Type: Number, Aggregates: SumAll},
		},
		Links: []Link{
			{Key: "entry", Label: "Entry", Target: "zoo.ledger_entries", Cardinality: ToOne, Field: "entry"},
			{Key: "account", Label: "Account", Target: "zoo.ledger_accounts", Cardinality: ToOne, Field: "account"},
		},
	})
}

// The spec's named pieces, inlined as §5 inlines them. FROM is the period's
// first day; NET a line's debit less credit; PL the profit and loss types.
const (
	glFrom = `{"param":"period","bound":"from"}`
	glNet  = `{"op":"sub","args":[{"path":[{"field":"debit"}]},{"path":[{"field":"credit"}]}]}`
	glPL   = `{"value":["income","expense"]}`
	// opening: the account's posted net before the period (a P&L account only
	// within the period's fiscal year), plus, on retained earnings, every P&L
	// net of the fiscal years before it.
	glOpening = `{"op":"add","args":[
		{"op":"coalesce","args":[{"op":"lookup","source":"zoo.ledger_lines","where":{"op":"and","args":[
			{"op":"eq","args":[{"path":[{"link":"account"}]},{"record":"byChartRow","path":[{"field":"id"}]}]},
			{"op":"eq","args":[{"path":[{"field":"posted"}]},{"value":true}]},
			{"op":"lt","args":[{"path":[{"field":"date"}]},FROM]},
			{"op":"or","args":[
				{"op":"not","args":[{"op":"in","args":[{"path":[{"link":"account"},{"field":"type"}]},PL]}]},
				{"op":"gte","args":[{"op":"fiscalYear","args":[{"path":[{"field":"date"}]}]},{"op":"fiscalYear","args":[FROM]}]}]}]},
			"value":NET,"pick":"sum"},{"value":0}]},
		{"op":"if","args":[
			{"op":"eq","args":[{"record":"byChartRow","path":[{"field":"detail_type"}]},{"value":"retained_earnings"}]},
			{"op":"coalesce","args":[{"op":"lookup","source":"zoo.ledger_lines","where":{"op":"and","args":[
				{"op":"eq","args":[{"path":[{"field":"posted"}]},{"value":true}]},
				{"op":"in","args":[{"path":[{"link":"account"},{"field":"type"}]},PL]},
				{"op":"lt","args":[{"op":"fiscalYear","args":[{"path":[{"field":"date"}]}]},{"op":"fiscalYear","args":[FROM]}]}]},
				"value":NET,"pick":"sum"},{"value":0}]},
			{"value":0}]}]}`
	// byNormal(X): a credit-normal account shows the negated net.
	glByNormal = `{"op":"if","args":[
		{"op":"in","args":[{"record":"byChartRow","path":[{"field":"type"}]},{"value":["liability","equity","income"]}]},
		{"op":"sub","args":[{"value":0},X]},X]}`
	// The other line's account of a 2-line entry, "-Split-" for more.
	glCount = `{"op":"lookup","source":"zoo.ledger_lines",
		"where":{"op":"eq","args":[{"path":[{"link":"entry"}]},{"outer":[{"link":"entry"}]}]},
		"value":{"path":[{"field":"id"}]},"pick":"count"}`
	glSplit = `{"op":"case","args":[
		{"op":"eq","args":[COUNT,{"value":2}]},
		{"op":"lookup","source":"zoo.ledger_lines","where":{"op":"and","args":[
			{"op":"eq","args":[{"path":[{"link":"entry"}]},{"outer":[{"link":"entry"}]}]},
			{"op":"ne","args":[{"path":[{"field":"id"}]},{"outer":[{"field":"id"}]}]}]},
			"value":{"path":[{"link":"account"},{"field":"full_name"}]},"pick":"min"},
		{"op":"gt","args":[COUNT,{"value":2}]},{"value":"-Split-"},
		{"value":null}]}`
	glSpec = `{"version":4,"source":"zoo.ledger_lines",
		"params":[
			{"id":"period","type":"dateRange","label":"Period","default":{"preset":"thisMonthToDate"}},
			{"id":"rows","type":"choice","label":"Rows","default":"active","options":[{"value":"active","label":"With activity"},{"value":"all","label":"All"}]},
			{"id":"accounts","type":"choice","label":"Accounts","multiple":true,
			 "optionsFrom":{"source":"zoo.ledger_accounts","value":[{"field":"id"}],"label":[{"field":"full_name"}]}}],
		"columns":[
			{"id":"account","label":"Account / Date","path":[{"link":"account"}],"labelField":"full_name"},
			{"id":"number","label":"No.","path":[{"link":"entry"},{"field":"number"}]},
			{"id":"split","label":"Split","formula":SPLIT},
			{"id":"debit","label":"Debit","path":[{"field":"debit"}],"aggregate":"sum"},
			{"id":"credit","label":"Credit","path":[{"field":"credit"}],"aggregate":"sum"},
			{"id":"balance","label":"Balance",
			 "formula":BALANCE,
			 "rollup":ROLLUP}],
		"filter":{"op":"and","args":[
			{"op":"eq","args":[{"path":[{"field":"posted"}]},{"value":true}]},
			{"op":"between","args":[{"path":[{"field":"date"}]},{"param":"period"}]},
			{"op":"in","args":[{"col":"account"},{"param":"accounts"}]}]},
		"groups":[{"id":"byChartRow","col":"account","open":true,"subtotals":true,
			"label":{"op":"if","args":[
				{"op":"isEmpty","args":[{"record":"byChartRow","path":[{"field":"number"}]}]},
				{"record":"byChartRow","path":[{"field":"full_name"}]},
				{"op":"concat","args":[{"record":"byChartRow","path":[{"field":"number"}]},{"value":" "},{"record":"byChartRow","path":[{"field":"full_name"}]}]}]},
			"fill":{"where":{"op":"in","args":[{"path":[{"field":"id"}]},{"param":"accounts"}]}},
			"having":{"op":"or","args":[
				{"op":"gt","args":[{"op":"count","args":[{"path":[{"field":"id"}]}]},{"value":0}]},
				{"op":"ne","args":[OPENING,{"value":0}]},
				{"op":"and","args":[
					{"op":"eq","args":[{"param":"rows"},{"value":"all"}]},
					{"op":"eq","args":[{"record":"byChartRow","path":[{"field":"inactive"}]},{"value":false}]}]}]},
			"opening":{"label":"Beginning balance","cells":{"balance":OPENING_BALANCE}}}],
		"sort":[
			{"formula":{"op":"case","args":[
				{"op":"eq","args":[{"record":"byChartRow","path":[{"field":"type"}]},{"value":"asset"}]},{"value":1},
				{"op":"eq","args":[{"record":"byChartRow","path":[{"field":"type"}]},{"value":"liability"}]},{"value":2},
				{"op":"eq","args":[{"record":"byChartRow","path":[{"field":"type"}]},{"value":"equity"}]},{"value":3},
				{"op":"eq","args":[{"record":"byChartRow","path":[{"field":"type"}]},{"value":"income"}]},{"value":4},
				{"value":5}]}},
			{"formula":{"record":"byChartRow","path":[{"field":"number"}]}},
			{"formula":{"record":"byChartRow","path":[{"field":"full_name"}]}},
			{"formula":{"path":[{"field":"date"}]}},
			{"formula":{"path":[{"field":"position"}]}}],
		"detail":{"label":{"path":[{"field":"date"}]}}}`
)

// balanceSpec fills the named pieces into the spec.
func balanceSpec() string {
	opening := strings.NewReplacer("FROM", glFrom, "NET", glNet, "PL", glPL).Replace(glOpening)
	byNormal := func(x string) string { return strings.ReplaceAll(glByNormal, "X", x) }
	running := `{"op":"add","args":[` + opening + `,{"op":"runningSum","args":[` + glNet + `]}]}`
	total := `{"op":"add","args":[` + opening + `,{"op":"coalesce","args":[{"op":"sum","args":[` + glNet + `]},{"value":0}]}]}`
	return strings.NewReplacer(
		"SPLIT", strings.ReplaceAll(glSplit, "COUNT", glCount),
		"OPENING_BALANCE", byNormal(opening),
		"BALANCE", byNormal(running),
		"ROLLUP", byNormal(total),
		"OPENING", opening,
	).Replace(glSpec)
}

// glLine is one row under an account: its kind, label, balance and split.
type glLine struct {
	Kind, Label    string
	Balance, Split any
}

// balanceGroups lists the group rows' labels in order, and each
// group's rows (opening, lines, subtotal) under its label.
func balanceGroups(res *Result) ([]string, map[string][]glLine) {
	var order []string
	byID := map[string]string{}
	rows := map[string][]glLine{}
	for _, r := range res.Rows {
		if r.Kind == KindGroup {
			order = append(order, r.Label)
			byID[r.ID] = r.Label
			continue
		}
		if label, ok := byID[r.Section]; ok {
			rows[label] = append(rows[label], glLine{r.Kind, r.Label, r.Cells["balance"], r.Cells["split"]})
		}
	}
	return order, rows
}

func requireGroupRows(t *testing.T, rows map[string][]glLine, account string, want ...glLine) {
	t.Helper()
	if got := rows[account]; !reflect.DeepEqual(got, want) {
		t.Fatalf("%s rows = %#v\nwant %#v", account, got, want)
	}
}

func glOpeningRow(balance int64) glLine {
	return glLine{KindOpening, "Beginning balance", balance, nil}
}

func glSubtotalRow(account string, balance int64) glLine {
	return glLine{KindSubtotal, "Total " + account, balance, nil}
}

func glDetailRow(day string, balance int64, split any) glLine {
	return glLine{KindDetail, day, balance, split}
}

func runBalances(t *testing.T, z *zoo, params map[string]string) *Result {
	t.Helper()
	return z.run(t, "ann", balanceSpec(), Options{Today: "2026-07-15"}, params)
}

const glJuly = `{"from":"2026-07-01","to":"2026-07-31"}`

func TestABalanceShapedSpecLoadsAndValidates(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	spec, _, err := Load([]byte(balanceSpec()))
	if err != nil {
		t.Fatal(err)
	}
	if spec.Version != CurrentVersion {
		t.Fatalf("version %d", spec.Version)
	}
	if err := Validate(z.app, spec); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

// A debit-normal account credited 100.00 then debited 30.00 shows −100.00,
// −70.00 and ends at −70.00; a credit-normal account credited 45.00 shows
// +45.00; Split names the other account of a 2-line entry and "-Split-" for 3.
func TestABalanceShapedReportRunsBalancesByNormalSide(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	res := runBalances(t, z, map[string]string{"period": glJuly})
	order, rows := balanceGroups(res)
	want := []string{"1000 Cash", "1100 Bank", "2000 Loan", "3900 Retained earnings", "4000 Sales", "5000 Rent"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("accounts = %v\nwant %v", order, want)
	}
	requireGroupRows(t, rows, "1000 Cash",
		glOpeningRow(0),
		glDetailRow("2026-07-01", -10000, "Rent"),
		glDetailRow("2026-07-02", -7000, "-Split-"),
		glSubtotalRow("1000 Cash", -7000))
	requireGroupRows(t, rows, "2000 Loan",
		glOpeningRow(0),
		glDetailRow("2026-07-02", 4500, "-Split-"),
		glSubtotalRow("2000 Loan", 4500))
	// The draft entry J3 is left out.
	requireGroupRows(t, rows, "5000 Rent",
		glOpeningRow(0),
		glDetailRow("2026-07-01", 10000, "Cash"),
		glSubtotalRow("5000 Rent", 10000))
	// Last fiscal year's sale folds into retained earnings, not into Sales.
	requireGroupRows(t, rows, "4000 Sales",
		glOpeningRow(0),
		glDetailRow("2026-07-02", -1500, "-Split-"),
		glSubtotalRow("4000 Sales", -1500))
	requireGroupRows(t, rows, "3900 Retained earnings", glOpeningRow(2000), glSubtotalRow("3900 Retained earnings", 2000))
	requireGroupRows(t, rows, "1100 Bank", glOpeningRow(2000), glSubtotalRow("1100 Bank", 2000))
	// The Total is in no account: its balance reads a record it does not have.
	last := res.Rows[len(res.Rows)-1]
	if last.Kind != KindTotal || last.Cells["balance"] != nil {
		t.Fatalf("last row = %s %#v, want the Total with a blank balance", last.Kind, last.Cells["balance"])
	}
	// Every balance that is not 0 read at least one line, so it carries that
	// line's currency (R24: the first known code of its reads).
	for _, r := range res.Rows {
		if b := r.Cells["balance"]; b != nil && b != int64(0) && r.Cells["balance@currency"] != "USD" {
			t.Fatalf("%s %q balance %#v has currency %#v, want USD", r.Kind, r.Label, b, r.Cells["balance@currency"])
		}
	}
}

func TestABalanceShapedReportCarriesTheBalanceIntoALaterPeriod(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	res := runBalances(t, z, map[string]string{"period": `{"from":"2026-07-02","to":"2026-07-31"}`})
	_, rows := balanceGroups(res)
	requireGroupRows(t, rows, "1000 Cash",
		glOpeningRow(-10000),
		glDetailRow("2026-07-02", -7000, "-Split-"),
		glSubtotalRow("1000 Cash", -7000))
}

func TestABalanceShapedReportUnderAllDatesBeginsAtZero(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	res := runBalances(t, z, map[string]string{"period": `{"preset":"allDates"}`})
	order, rows := balanceGroups(res)
	for _, account := range order {
		if got := rows[account]; len(got) == 0 || !reflect.DeepEqual(got[0], glOpeningRow(0)) {
			t.Fatalf("%s starts with %#v, want a 0.00 beginning balance", account, got)
		}
	}
	requireGroupRows(t, rows, "1000 Cash",
		glOpeningRow(0),
		glDetailRow("2026-07-01", -10000, "Rent"),
		glDetailRow("2026-07-02", -7000, "-Split-"),
		glSubtotalRow("1000 Cash", -7000))
	requireGroupRows(t, rows, "4000 Sales",
		glOpeningRow(0),
		glDetailRow("2025-12-15", 2000, "Bank"),
		glDetailRow("2026-07-02", 500, "-Split-"),
		glSubtotalRow("4000 Sales", 500))
	if _, shown := rows["3900 Retained earnings"]; shown {
		t.Fatalf("retained earnings has nothing before All dates, so the having hides it: %v", order)
	}
}

func TestABalanceShapedReportShowsQuietActiveGroupsOnAll(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	res := runBalances(t, z, map[string]string{"period": glJuly, "rows": `"all"`})
	order, rows := balanceGroups(res)
	requireGroupRows(t, rows, "1200 Petty cash", glOpeningRow(0), glSubtotalRow("1200 Petty cash", 0))
	for _, account := range order {
		if account == "1300 Idle" {
			t.Fatalf("an inactive account with nothing to show is hidden: %v", order)
		}
	}
}

// A reader the joined grant rule refuses sees no ledger line: bob's grant
// row reads can_read false, and cara has no grant row. With the accounts
// listed to every signed-in reader, each account still shows on All, but with
// no line and a 0.00 beginning balance: no opening sums a line he may not
// see (ann's Bank opens at 20.00 from last year's hidden line).
func TestABalanceShapedReportShowsNoLedgerLineToAReaderWithoutAGrant(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	z.user(t, "cara")
	params := map[string]string{"period": glJuly, "rows": `"all"`}
	run := func(user string) ([]string, map[string][]glLine) {
		t.Helper()
		return balanceGroups(z.run(t, user, balanceSpec(), Options{Today: "2026-07-15"}, params))
	}
	for _, user := range []string{"bob", "cara"} {
		if order, rows := run(user); len(order) != 0 || len(rows) != 0 {
			t.Fatalf("%s sees accounts %v and rows %v under the grant rule", user, order, rows)
		}
	}

	accounts, err := z.app.FindCollectionByNameOrId("zoo_ledger_accounts")
	if err != nil {
		t.Fatal(err)
	}
	signedIn := rule(`@request.auth.id != ""`)
	accounts.ListRule, accounts.ViewRule = signedIn, signedIn
	z.save(t, accounts)
	if _, rows := run("ann"); !reflect.DeepEqual(rows["1100 Bank"], []glLine{glOpeningRow(2000), glSubtotalRow("1100 Bank", 2000)}) {
		t.Fatalf("ann's Bank rows = %#v, want a 20.00 opening", rows["1100 Bank"])
	}
	want := []string{"1000 Cash", "1100 Bank", "1200 Petty cash", "2000 Loan", "3900 Retained earnings", "4000 Sales", "5000 Rent"}
	for _, user := range []string{"bob", "cara"} {
		order, rows := run(user)
		if !reflect.DeepEqual(order, want) {
			t.Fatalf("%s sees accounts %v\nwant %v", user, order, want)
		}
		for _, account := range order {
			requireGroupRows(t, rows, account, glOpeningRow(0), glSubtotalRow(account, 0))
		}
	}
}

// An open account cut at a page seam goes on on the next page: its running
// balance carries over the seam, no row is repeated or lost, and the pages
// merged as the client merges them equal the one-page run, filled accounts
// included.
func TestABalanceShapedReportCutAtAPageSeamGoesOnOnTheNextPage(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	params := map[string]string{"period": glJuly, "rows": `"all"`}
	whole := runBalances(t, z, params)
	// A page counts group, opening, detail and subtotal rows (J1a): a page of
	// 3 is Cash's group row, its opening and the 2026-07-01 Rent line.
	opts := Options{Today: "2026-07-15", Limit: 3}
	var pages []*Result
	for range 100 {
		res := z.run(t, "ann", balanceSpec(), opts, params)
		pages = append(pages, res)
		if res.Page == nil || res.Page.Next == "" {
			break
		}
		opts.Cursor = res.Page.Next
	}
	if len(pages) < 2 {
		t.Fatalf("%d pages, want the run cut into pages", len(pages))
	}
	_, first := balanceGroups(pages[0])
	requireGroupRows(t, first, "1000 Cash", glOpeningRow(0), glDetailRow("2026-07-01", -10000, "Rent"))
	// The group row came on page 1, so page 2's first row names it only by
	// its section.
	cash := pages[0].Rows[0]
	next := pages[1].Rows[0]
	got := glLine{next.Kind, next.Label, next.Cells["balance"], next.Cells["split"]}
	if cash.Label != "1000 Cash" || next.Section != cash.ID || !reflect.DeepEqual(got, glDetailRow("2026-07-02", -7000, "-Split-")) {
		t.Fatalf("page 2 starts with %#v in %s, want Cash's 2026-07-02 at -7000 in %s", got, next.Section, cash.ID)
	}
	merged := mergedRows(t, pages)
	if !reflect.DeepEqual(merged, whole.Rows) {
		t.Fatalf("merged pages differ from the one-page run:\n%#v\nwant\n%#v", merged, whole.Rows)
	}
	_, rows := balanceGroups(&Result{Rows: merged})
	requireGroupRows(t, rows, "1000 Cash",
		glOpeningRow(0),
		glDetailRow("2026-07-01", -10000, "Rent"),
		glDetailRow("2026-07-02", -7000, "-Split-"),
		glSubtotalRow("1000 Cash", -7000))
	requireGroupRows(t, rows, "1200 Petty cash", glOpeningRow(0), glSubtotalRow("1200 Petty cash", 0))
}

// J1a: a seam between two account groups (the page ends on a subtotal) goes
// on with the next account's group row; the pages join into the one-page run.
func TestABalanceShapedReportCutBetweenGroupsGoesOnOnTheNextPage(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	params := map[string]string{"period": glJuly, "rows": `"all"`}
	whole := runBalances(t, z, params)
	end := slices.IndexFunc(whole.Rows, func(r Row) bool { return r.Kind == KindSubtotal })
	if end < 0 {
		t.Fatal("no subtotal")
	}
	opts := Options{Today: "2026-07-15", Limit: end + 1 - countEnds(whole.Rows[:end+1])}
	first := z.run(t, "ann", balanceSpec(), opts, params)
	if last := lastCountedRow(first.Rows); last.Kind != KindSubtotal {
		t.Fatalf("page 1 ends in %+v, want the first account's subtotal", last)
	}
	opts.Cursor = first.Page.Next
	second := z.run(t, "ann", balanceSpec(), opts, params)
	if second.Rows[0].Kind != KindGroup || second.Rows[0].ID == first.Rows[0].ID {
		t.Fatalf("page 2 starts with %+v, want the next account's group row", second.Rows[0])
	}
	pages := []*Result{first}
	for o := opts; ; {
		res := z.run(t, "ann", balanceSpec(), o, params)
		pages = append(pages, res)
		if res.Page == nil || res.Page.Next == "" {
			break
		}
		o.Cursor = res.Page.Next
	}
	if got := mergedRows(t, pages); !reflect.DeepEqual(got, whole.Rows) {
		t.Fatalf("merged pages differ from the one-page run")
	}
}

func countEnds(rows []Row) int {
	n := 0
	for _, r := range rows {
		if isEnd(r) {
			n++
		}
	}
	return n
}

func lastCountedRow(rows []Row) Row {
	for i := len(rows) - 1; i >= 0; i-- {
		if !isEnd(rows[i]) {
			return rows[i]
		}
	}
	return Row{}
}

// On a detail row the opening's looked-up code is a subquery the currency
// guard repeats (R24), and SQLite runs a correlated subquery for every row it
// reads: a run over 2,000 lines costs little only while each such subquery
// searches the lines table by an index. Checked on SQLite's query plan of
// every statement the run sends, not on a clock, so a busy machine cannot
// fail it. The lines table is still read whole once per statement (the
// report's own rows, and lookups that read no column of the row), as any
// report over a period must.
func TestABalanceShapedReportOverTwoThousandLinesSearchesLinesPerRow(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	accounts := []string{"cash", "bank", "petty", "loan", "re", "sales", "rent"}
	for i := range 1000 {
		entry := fmt.Sprintf("B%d", i)
		z.rec(t, entry, "zoo_ledger_entries", map[string]any{"number": entry})
		// Even entries fall before July, odd ones in it.
		day := fmt.Sprintf("2026-%02d-%02d", 1+i%6, 1+i%28)
		if i%2 == 1 {
			day = fmt.Sprintf("2026-07-%02d", 1+i%28)
		}
		for j, side := range []struct {
			account       string
			debit, credit int
		}{{accounts[i%7], 100, 0}, {accounts[(i+3)%7], 0, 100}} {
			z.rec(t, fmt.Sprintf("%s-%d", entry, j), "zoo_ledger_lines", map[string]any{
				"entry": z.id(entry), "account": z.id(side.account), "date": day,
				"debit": side.debit, "credit": side.credit, "currency": "USD", "position": j + 1, "posted": true,
			})
		}
	}
	seen := observeReads(t)
	res := runBalances(t, z, map[string]string{"period": glJuly})
	db := readPool(t, z)
	searched := 0
	for _, st := range *seen {
		raw, args := rawStatement(t, st.sql, st.params)
		plan := queryPlan(t, db, raw, args)
		detail := !strings.Contains(st.sql, "__level")
		for _, step := range perRowSteps(plan, ruleReaders(raw, "zoo_ledger_lines")...) {
			if !declaredIndexSearch.MatchString(step.detail) {
				t.Fatalf("a subquery run for every row reads the lines by %q:\n%s", step.detail, strings.Join(plan, "\n"))
			}
			// On a detail row, a read by account reads every line of the
			// row's account, the same for every row of that account: a value
			// of the group, computed once per group.
			if detail && !step.perGroup() && strings.Contains(step.detail, "idx_zoo_ledger_lines_account_date") {
				t.Fatalf("a subquery run for every detail row reads all of an account's lines by %q:\n%s", step.detail, strings.Join(plan, "\n"))
			}
			searched++
		}
	}
	if searched == 0 {
		t.Fatal("no subquery searched the lines table for each row: the plan check checked nothing")
	}
	if len(res.Notices) != 0 {
		t.Fatalf("notices = %+v", res.Notices)
	}
	details := 0
	for _, r := range res.Rows {
		if r.Kind == KindDetail {
			details++
			if r.Cells["balance"] == nil {
				t.Fatalf("detail row %q has no balance", r.Label)
			}
		}
	}
	if details == 0 {
		t.Fatal("the first page has no detail rows")
	}
}

// indexSearch is a plan step that reads a table through an index.
var indexSearch = regexp.MustCompile(`^SEARCH \S+ USING (?:(?:AUTOMATIC )?(?:PARTIAL )?(?:COVERING )?INDEX|INTEGER PRIMARY KEY)`)

// declaredIndexSearch is a plan step that reads a table through an index of
// the table, not one SQLite builds for the statement over a copy of the rows.
var declaredIndexSearch = regexp.MustCompile(`^SEARCH \S+ USING (?:(?:COVERING )?INDEX \S+|INTEGER PRIMARY KEY)`)

// ruleCTE is the start of a rule CTE and the table its first FROM reads.
var ruleCTE = regexp.MustCompile("\\b(r\\d+) AS \\(SELECT .*?\\bFROM `(\\w+)`")

// ruleReaders names what a plan step that reads table's rows calls it in a
// raw statement: the table, where SQLite reads its rule CTE in place, and
// each alias of the rule CTE, where SQLite reads a copy of the rule's rows.
func ruleReaders(raw, table string) []string {
	names := []string{table}
	for _, m := range ruleCTE.FindAllStringSubmatch(raw, -1) {
		if m[2] != table {
			continue
		}
		for _, a := range regexp.MustCompile(`\b`+m[1]+` (t\d+)\b`).FindAllStringSubmatch(raw, -1) {
			names = append(names, a[1])
		}
	}
	return names
}

// queryPlan is SQLite's EXPLAIN QUERY PLAN of stmt, one step a line,
// indented two spaces a level under its parent step.
func queryPlan(t *testing.T, db *sql.DB, stmt string, args []any) []string {
	t.Helper()
	rows, err := db.Query("EXPLAIN QUERY PLAN "+stmt, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	depth := map[int]int{}
	var out []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		depth[id] = depth[parent] + 1
		out = append(out, strings.Repeat("  ", depth[id]-1)+detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// perRowReads lists the plan's steps that read one of tables inside a
// correlated subquery, which SQLite runs again for every row of the query
// around it. A subquery that reads no column of an outer row (a plain SCALAR
// or LIST SUBQUERY, or a MATERIALIZE) runs once per statement, wherever it
// sits, so the steps under it are not listed.
func perRowReads(plan []string, tables ...string) []string {
	var out []string
	for _, s := range perRowSteps(plan, tables...) {
		out = append(out, s.detail)
	}
	return out
}

// planStep is a step of a query plan and the steps above it, nearest last.
type planStep struct {
	detail  string
	parents []string
}

// perRowSteps is perRowReads with each step's parents.
func perRowSteps(plan []string, tables ...string) []planStep {
	var out []planStep
	var stack []string
	var indents []int
	for _, line := range plan {
		detail := strings.TrimLeft(line, " ")
		indent := len(line) - len(detail)
		for len(indents) > 0 && indents[len(indents)-1] >= indent {
			indents, stack = indents[:len(indents)-1], stack[:len(stack)-1]
		}
		if reads := strings.Fields(detail); len(reads) > 1 && slices.Contains(tables, reads[1]) && underCorrelated(stack) {
			out = append(out, planStep{detail: detail, parents: slices.Clone(stack)})
		}
		indents, stack = append(indents, indent), append(stack, detail)
	}
	return out
}

// perGroup is true when the step runs inside a table SQLite computes once
// for the statement that the engine names as one row per group: a
// group-level CTE (a<L>, g<L>, g<L>_<i>, rs<L>, rt<L>, s<L>, f<L>) or the
// table of a value computed once per group key (wg<n>). A correlated
// subquery there runs once per group, not once per detail row. Any other
// table (w, ws, a rule CTE, an unnamed subquery-<n>) may hold the detail
// rows, so it is not.
func (s planStep) perGroup() bool {
	for _, p := range s.parents {
		name, ok := strings.CutPrefix(p, "MATERIALIZE ")
		if ok && perGroupTable.MatchString(name) {
			return true
		}
	}
	return false
}

// perGroupTable is the name of a table the engine computes one row per
// group in.
var perGroupTable = regexp.MustCompile(`^(?:a\d+|g\d+(?:_\d+)?|rs\d+|rt\d+|s\d+|f\d+|wg\d+)$`)

// underCorrelated is true when the nearest subquery among a step's parents
// is a correlated one.
func underCorrelated(parents []string) bool {
	for _, p := range slices.Backward(parents) {
		switch {
		case strings.HasPrefix(p, "CORRELATED "):
			return true
		case strings.HasPrefix(p, "SCALAR SUBQUERY"), strings.HasPrefix(p, "LIST SUBQUERY"), strings.HasPrefix(p, "MATERIALIZE"):
			return false
		}
	}
	return false
}

// perRowReadsOf lists, over every statement seen, the plan steps that read
// table inside a correlated subquery (perRowReads).
func perRowReadsOf(t *testing.T, z *zoo, seen []readStatement, table string) []string {
	t.Helper()
	db := readPool(t, z)
	var out []string
	for _, st := range seen {
		raw, args := rawStatement(t, st.sql, st.params)
		out = append(out, perRowReads(queryPlan(t, db, raw, args), table)...)
	}
	return out
}
