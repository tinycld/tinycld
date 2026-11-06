package reports

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// The zoo's ledger: lines on the chart of accounts (withAccounts), each an
// amount in the source's fixed currency on one account.
//
//	key  account     amount memo
//	l1   cash        500    Opening
//	l2   cash        250    Sale
//	l3   payables    300    Bill
//	l4   rent        120    May rent
func withLines(t *testing.T, z *zoo) {
	t.Helper()
	withAccounts(t, z)
	accounts, err := z.app.FindCollectionByNameOrId("zoo_accounts")
	if err != nil {
		t.Fatal(err)
	}
	lines := core.NewBaseCollection("zoo_lines")
	lines.ListRule = rule(`@request.auth.id != ""`)
	lines.ViewRule = lines.ListRule
	lines.Fields.Add(
		&core.RelationField{Name: "account", CollectionId: accounts.Id, MaxSelect: 1},
		&core.NumberField{Name: "amount"},
		&core.TextField{Name: "memo"},
	)
	z.save(t, lines)
	for _, l := range []struct {
		key, account, memo string
		amount             int
	}{
		{"l1", "cash", "Opening", 500},
		{"l2", "cash", "Sale", 250},
		{"l3", "payables", "Bill", 300},
		{"l4", "rent", "May rent", 120},
	} {
		z.rec(t, l.key, "zoo_lines", map[string]any{"account": z.id(l.account), "amount": l.amount, "memo": l.memo})
	}
	registerLinesSource()
}

func registerLinesSource() {
	RegisterSource(Source{
		Key: "zoo.lines", Collection: "zoo_lines", Label: "Lines",
		Fields: []Field{
			{Key: "amount", Label: "Amount", Type: Money, Aggregates: SumAll},
			{Key: "memo", Label: "Memo", Type: Text},
		},
		Links:    []Link{{Key: "account", Label: "Account", Target: "zoo.accounts", Cardinality: ToOne, Field: "account"}},
		Currency: func(core.App, *core.RequestInfo) string { return "USD" },
	})
}

// ledgerByAccount is a trial-balance shape: one group per account, labelled
// by the account; No. is the account's number, read from the group's record;
// opened lines are labelled by their memo. NO is the No. column's detail
// member ("" for none).
const ledgerByAccount = `{"version":2,"source":"zoo.lines",
	"columns":[
		{"id":"name","path":[{"link":"account"}]},
		{"id":"no","label":"No.","formula":{"record":"byAccount","path":[{"field":"number"}]}NO},
		{"id":"debit","path":[{"field":"amount"}],"aggregate":"sum"}],
	"groups":[{"id":"byAccount","col":"name","subtotals":true}],
	"sort":[{"col":"no"},{"col":"debit"}],
	"detail":{"label":{"path":[{"field":"memo"}]}}}`

func ledger(no string) string { return strings.Replace(ledgerByAccount, "NO", no, 1) }

const blankOnDetail = `,"detail":"blank"`

// A column blank on detail rows has no cell on an opened account's lines;
// its group, subtotal and total rows keep the cell they have without it.
func TestABlankColumnHasNoCellOnDetailRows(t *testing.T) {
	z := newZoo(t)
	withLines(t, z)
	spec := ledger(blankOnDetail)
	closed := z.run(t, "ann", spec, Options{}, nil)
	requireOrderedRows(t, spec, closed, "label", "Cash", "Payables", "Rent", "Total")
	cash := closed.Rows[0].GroupKey
	res := z.run(t, "ann", spec, Options{Expand: [][]any{cash}}, nil)
	requireRows(t, res, "kind", "group", "detail", "detail", "subtotal", "group", "group", "total")
	requireOrderedRows(t, spec, res, "label", "Cash", "Sale", "Opening", "Total Cash", "Payables", "Rent", "Total")
	// The group record's number on the group and its subtotal, as without the
	// member; no number of its own on the total.
	requireOrderedRows(t, spec, res, "no", int64(1000), nil, nil, int64(1000), int64(2000), int64(5000), nil)
	requireOrderedRows(t, spec, res, "debit", int64(750), int64(250), int64(500), int64(750), int64(300), int64(120), int64(1170))
	for _, r := range res.Rows {
		_, has := r.Cells["no"]
		if has == (r.Kind == KindDetail) {
			t.Fatalf("%s row %q: has a no cell = %v, want %v", r.Kind, r.Label, has, r.Kind != KindDetail)
		}
		if _, ok := r.Cells["name"]; r.Kind == KindDetail && !ok {
			t.Fatalf("detail row %q lost its name cell: %v", r.Label, r.Cells)
		}
	}
}

// The member changes nothing but the detail rows' cell: every other row, and
// every other cell of a detail row, is what the same report gives without it.
func TestABlankColumnLeavesEveryOtherCell(t *testing.T) {
	z := newZoo(t)
	withLines(t, z)
	shown := z.run(t, "ann", ledger(""), Options{ExpandAll: true}, nil)
	blank := z.run(t, "ann", ledger(blankOnDetail), Options{ExpandAll: true}, nil)
	if len(shown.Rows) != len(blank.Rows) {
		t.Fatalf("rows: %d shown, %d blank", len(shown.Rows), len(blank.Rows))
	}
	details := 0
	for i, want := range shown.Rows {
		got := blank.Rows[i]
		if want.Kind == KindDetail {
			details++
			if _, ok := want.Cells["no"]; !ok {
				t.Fatalf("without the member, detail row %q has no number: %v", want.Label, want.Cells)
			}
			want.Cells = maps.Clone(want.Cells)
			delete(want.Cells, "no")
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("row %d:\n got  %+v\n want %+v", i, got, want)
		}
	}
	if details != 4 {
		t.Fatalf("detail rows = %d, want 4", details)
	}
	if !reflect.DeepEqual(blank.Columns, shown.Columns) {
		t.Fatalf("columns:\n got  %+v\n want %+v", blank.Columns, shown.Columns)
	}
}

// feedingsCostBlank is feedings grouped by animal, the cost (in its row's
// currency) blank on detail rows.
const feedingsCostBlank = `{"version":2,"source":"zoo.feedings","columns":[
	{"id":"animal","path":[{"link":"animal"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum","detail":"blank"},
	{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"animal","subtotals":true}],"sort":[{"col":"grams"}]}`

// An amount blank on detail rows leaves out its currency cell there too; the
// group, subtotal and total rows keep both.
func TestABlankAmountHasNoCurrencyCellOnDetailRows(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", feedingsCostBlank, Options{ExpandAll: true}, nil)
	requireRows(t, res, "kind", "group", "detail", "detail", "subtotal", "group", "detail", "detail", "subtotal", "total")
	requireOrderedRows(t, feedingsCostBlank, res, "grams", int64(400), int64(100), int64(300), int64(400), int64(1300), int64(500), int64(800), int64(1300), int64(1700))
	requireOrderedRows(t, feedingsCostBlank, res, "cost", int64(1000), nil, nil, int64(1000), int64(3500), nil, nil, int64(3500), int64(4500))
	if res.Columns[1].CurrencyCell != "cost@currency" {
		t.Fatalf("cost column = %+v", res.Columns[1])
	}
	for _, r := range res.Rows {
		_, hasCost := r.Cells["cost"]
		cur, hasCur := r.Cells["cost@currency"]
		if r.Kind == KindDetail {
			if hasCost || hasCur {
				t.Fatalf("detail row %q: %v", r.Label, r.Cells)
			}
			continue
		}
		if !hasCost || cur != "USD" {
			t.Fatalf("%s row %q: %v", r.Kind, r.Label, r.Cells)
		}
	}
}

// A column may be blank on detail rows when another column or the detail
// label names those rows, at any place, even first.
func TestValidateAcceptsABlankColumn(t *testing.T) {
	app := withZooRegistry(t)
	registerAccountsSource()
	registerLinesSource()
	for name, spec := range map[string]string{
		"the No. column": ledger(blankOnDetail),
		"the amount":     feedingsCostBlank,
		"the first column, labelled by memo": `{"version":2,"source":"zoo.lines",
			"columns":[{"id":"no","path":[{"link":"account"},{"field":"number"}],"detail":"blank"},
				{"id":"name","path":[{"link":"account"}]}],
			"groups":[{"id":"byAccount","col":"name"}],
			"detail":{"label":{"path":[{"field":"memo"}]}}}`,
		"a formula over the first column labels the rows": `{"version":2,"source":"zoo.lines",
			"columns":[{"id":"memo","path":[{"field":"memo"}],"detail":"blank"},
				{"id":"name","path":[{"link":"account"}]}],
			"groups":[{"id":"byAccount","col":"name"}],
			"detail":{"label":{"op":"concat","args":[{"col":"memo"},{"value":"!"}]}}}`,
	} {
		if err := Validate(app, loadSpec(t, spec)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// What a detail member is refused for, by where and what kind: the words are
// the engine's.
func TestValidateRefusesABlankColumn(t *testing.T) {
	app := withZooRegistry(t)
	registerAccountsSource()
	registerLinesSource()
	for _, c := range []struct{ name, spec, at string }{
		// Only "blank" is a detail member.
		{"another value", ledger(`,"detail":"hide"`), "columns.1.detail"},
		// A report with no groups has no group row to show the value on.
		{"no groups", `{"version":2,"source":"zoo.lines","columns":[
			{"id":"memo","path":[{"field":"memo"}]},
			{"id":"no","path":[{"link":"account"},{"field":"number"}],"detail":"blank"}]}`, "columns.1.detail"},
		// The first column labels detail rows when no detail label is set.
		{"the first column with no detail label", `{"version":2,"source":"zoo.lines","columns":[
			{"id":"name","path":[{"link":"account"}],"detail":"blank"},
			{"id":"debit","path":[{"field":"amount"}],"aggregate":"sum"}],
			"groups":[{"id":"byAccount","col":"name"}]}`, "columns.0.detail"},
		// A detail label of exactly {col} is that column's cell.
		{"the column the detail label is", strings.Replace(ledger(blankOnDetail),
			`"detail":{"label":{"path":[{"field":"memo"}]}}`, `"detail":{"label":{"col":"no"}}`, 1), "columns.1.detail"},
		// A pivot report has no detail rows, so a blank there does nothing.
		{"a pivot report", pivotGramsBlank, "columns.2.detail"},
		// A nest level's first column labels that level's child rows.
		{"the first column of a nest level", feedingsFedBlank, "columns.2.detail"},
	} {
		t.Run(c.name, func(t *testing.T) {
			requireRefusal(t, Validate(app, loadSpec(t, c.spec)), 400, c.at, "invalid")
		})
	}
}

// pivotGramsBlank is feedings grouped by species, cost pivoted by month, and
// grams blank on detail rows, which a pivot report never has.
const pivotGramsBlank = `{"version":2,"source":"zoo.feedings","columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
	{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum","detail":"blank"}],
	"groups":[{"id":"g","col":"species"}],
	"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"cost"}}`

// feedingsFedBlank is animalsWithFeedings with fed, the first column of the
// nested level and so the label of its child rows, blank on detail rows.
var feedingsFedBlank = strings.Replace(strings.Replace(animalsWithFeedings, "COST", "", 1),
	`{"field":"fed"}]}`, `{"field":"fed"}],"detail":"blank"}`, 1)

// twoNestLevels nests each feeding's animal's feedings under the feeding: a
// second nest level, labelled by its own first column (again). AGAIN and
// AGAINCOST are the detail members of the second level's two columns.
const twoNestLevels = `{"version":2,"source":"zoo.animals","columns":[
	{"id":"name","path":[{"field":"name"}]},
	{"id":"site","path":[{"link":"site"}]},
	{"id":"fed","path":[{"link":"feedings","mode":"nest"},{"field":"fed"}]},
	{"id":"cost","path":[{"link":"feedings","mode":"nest"},{"field":"cost"}]},
	{"id":"again","path":[{"link":"feedings","mode":"nest"},{"link":"animal"},{"link":"feedings","mode":"nest"},{"field":"fed"}]AGAIN},
	{"id":"againCost","path":[{"link":"feedings","mode":"nest"},{"link":"animal"},{"link":"feedings","mode":"nest"},{"field":"cost"}]AGAINCOST}],
	"groups":[{"id":"bySite","col":"site"}]}`

func nestLevels(again, againCost string) string {
	return strings.NewReplacer("AGAINCOST", againCost, "AGAIN", again).Replace(twoNestLevels)
}

// The second nest level's first column labels that level's child rows too.
func TestValidateRefusesABlankOnASecondNestLevelsFirstColumn(t *testing.T) {
	app := withZooRegistry(t)
	requireRefusal(t, Validate(app, loadSpec(t, nestLevels(blankOnDetail, ""))), 400, "columns.4.detail", "invalid")
}

// Without the member, both refused shapes are valid reports, and a nest
// level's later column may be blank: each refusal is the member's alone.
func TestValidateAcceptsThePivotAndNestShapesWithNoBlank(t *testing.T) {
	app := withZooRegistry(t)
	for name, spec := range map[string]string{
		"the pivot":                          strings.Replace(pivotGramsBlank, blankOnDetail, "", 1),
		"the nested level":                   strings.Replace(feedingsFedBlank, blankOnDetail, "", 1),
		"a nest level's second column blank": strings.Replace(animalsWithFeedings, "COST", blankOnDetail, 1),
		"two nest levels":                    nestLevels("", ""),
		"the second level's second column":   nestLevels("", blankOnDetail),
	} {
		if err := Validate(app, loadSpec(t, spec)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// A run refuses what Validate refuses: a blank label leaves a row with no name.
func TestRunRefusesABlankLabelColumn(t *testing.T) {
	z := newZoo(t)
	withLines(t, z)
	spec := `{"version":2,"source":"zoo.lines","columns":[
		{"id":"name","path":[{"link":"account"}],"detail":"blank"},
		{"id":"debit","path":[{"field":"amount"}],"aggregate":"sum"}],
		"groups":[{"id":"byAccount","col":"name"}]}`
	_, err := z.try(t, "ann", spec, Options{ExpandAll: true}, nil)
	requireRefusal(t, err, 400, "columns.0.detail", "invalid")
}

// animalsWithFeedings is animals grouped by site, each animal's feedings
// nested under it as child rows (fed labels them); the feeding's cost, in its
// row's currency, is a child row column. COST is its detail member.
const animalsWithFeedings = `{"version":2,"source":"zoo.animals","columns":[
	{"id":"name","path":[{"field":"name"}]},
	{"id":"site","path":[{"link":"site"}]},
	{"id":"fed","path":[{"link":"feedings","mode":"nest"},{"field":"fed"}]},
	{"id":"cost","path":[{"link":"feedings","mode":"nest"},{"field":"cost"}]COST}],
	"groups":[{"id":"bySite","col":"site","subtotals":true}],
	"sort":[{"col":"name"}]}`

// A child row (a nested level's row) is a detail row too: a blank column of
// its level has no cell there, nor a currency cell. Every other row and cell
// is what the same report gives without the member.
func TestABlankColumnHasNoCellOnChildRows(t *testing.T) {
	z := newZoo(t)
	shownSpec := strings.Replace(animalsWithFeedings, "COST", "", 1)
	blankSpec := strings.Replace(animalsWithFeedings, "COST", blankOnDetail, 1)
	shown := z.run(t, "ann", shownSpec, Options{ExpandAll: true}, nil)
	blank := z.run(t, "ann", blankSpec, Options{ExpandAll: true}, nil)
	// The sort orders animals, not sites, so only which rows come is checked;
	// the two runs read the same rows in the same order.
	labels := []string{}
	for _, r := range blank.Rows {
		labels = append(labels, r.Label)
	}
	slices.Sort(labels)
	if want := []string{"(none)", "2026-01-05", "2026-01-20", "2026-02-10", "2026-03-03", "Kit", "Leo", "North", "South",
		"Total", "Total (none)", "Total North", "Total South", "Zed"}; !slices.Equal(labels, want) {
		t.Fatalf("labels = %v\nwant     %v", labels, want)
	}
	for _, r := range blank.Rows {
		_, hasCost := r.Cells["cost"]
		_, hasCur := r.Cells["cost@currency"]
		if r.Parent != "" && (hasCost || hasCur) {
			t.Fatalf("child row %q: %v", r.Label, r.Cells)
		}
	}
	if len(shown.Rows) != len(blank.Rows) {
		t.Fatalf("rows: %d shown, %d blank", len(shown.Rows), len(blank.Rows))
	}
	children := 0
	for i, want := range shown.Rows {
		got := blank.Rows[i]
		if want.Parent != "" {
			children++
			// Without the member a child row has the cost and its currency.
			if want.Cells["cost"] == nil || want.Cells["cost@currency"] != "USD" {
				t.Fatalf("child row %q without the member: %v", want.Label, want.Cells)
			}
		}
		if want.Kind == KindDetail {
			want.Cells = maps.Clone(want.Cells)
			delete(want.Cells, "cost")
			delete(want.Cells, "cost@currency")
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("row %d:\n got  %+v\n want %+v", i, got, want)
		}
	}
	if children != 4 {
		t.Fatalf("child rows = %d, want 4", children)
	}
}
