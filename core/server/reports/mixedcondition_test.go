package reports

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

// allCosts is the sum of every feeding's cost the reader sees, whichever
// currency each is in: with a EUR feeding beside the USD ones it met two
// currencies on every row.
const allCosts = `{"op":"lookup","source":"zoo.feedings","pick":"sum","value":{"path":[{"field":"cost"}]}}`

// withEuroFeeding adds Leo's feeding f7 in EUR, so a sum of Leo's costs (and
// of all costs) meets two currencies.
func withEuroFeeding(t *testing.T, z *zoo) {
	t.Helper()
	z.rec(t, "f7", "zoo_feedings", map[string]any{
		"animal": z.id("leo"), "cost": 400, "currency": "EUR", "fed": "2026-03-10 00:00:00.000Z", "due": "2026-03-10", "grams": 10, "note": "f7",
	})
}

// gramsByAnimalHaving groups the feedings by animal under a having.
func gramsByAnimalHaving(having string) string {
	return `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal","having":` + having + `}],
		"sort":[{"col":"note"}]}`
}

func groupRowLabels(res *Result) []string {
	var out []string
	for _, r := range res.Rows {
		if r.Kind == KindGroup {
			out = append(out, r.Label)
		}
	}
	return out
}

// conditionNotice is the run's mixedCurrency notice when it names the
// condition at, nil when none does.
func conditionNotice(res *Result, at string) *Notice {
	for i, n := range res.Notices {
		if n.Code == "mixedCurrency" && slices.Contains(n.Conditions, at) {
			return &res.Notices[i]
		}
	}
	return nil
}

// A having that reads a sum lookup which met two currencies hides no group
// whose other terms hold: the mixed sum has no value, so the condition on it
// is unknown, and an or with a true term holds (three-valued logic). The run
// names the having in its mixedCurrency notice.
func TestAHavingOverAMixedSumLookupHidesNoGroupItsOtherTermsShow(t *testing.T) {
	z := newZoo(t)
	withEuroFeeding(t, z)
	count := `{"op":"gt","args":[{"op":"count","args":[{"path":[{"field":"id"}]}]},{"value":0}]}`
	res := z.run(t, "ann", gramsByAnimalHaving(count), Options{}, nil)
	if got := groupRowLabels(res); !slices.Equal(got, []string{"Leo", "Zed"}) {
		t.Fatalf("count only: groups = %v, want Leo and Zed", got)
	}
	if conditionNotice(res, "groups.0.having") != nil {
		t.Fatalf("count only: notices = %+v", res.Notices)
	}

	orMixed := `{"op":"or","args":[` + count + `,
		{"op":"ne","args":[{"op":"coalesce","args":[` + leoCosts + `,{"value":0}]},{"value":0}]}]}`
	res = z.run(t, "ann", gramsByAnimalHaving(orMixed), Options{}, nil)
	if got := groupRowLabels(res); !slices.Equal(got, []string{"Leo", "Zed"}) {
		t.Fatalf("count or a mixed sum: groups = %v, want Leo and Zed", got)
	}
	n := conditionNotice(res, "groups.0.having")
	if n == nil {
		t.Fatalf("count or a mixed sum: no notice names the having: %+v", res.Notices)
	}
	if !strings.Contains(n.Message, "reads a looked-up total that mixes currencies") {
		t.Fatalf("message = %q", n.Message)
	}
}

// A filter that reads a sum lookup which met two currencies drops no row
// whose other terms keep it, and the run names the filter in its notice.
func TestAFilterOverAMixedSumLookupDropsNoRowItsOtherTermsKeep(t *testing.T) {
	z := newZoo(t)
	withEuroFeeding(t, z)
	spec := `{"version":4,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]}],
		"filter":{"op":"or","args":[
			{"op":"eq","args":[{"value":1},{"value":1}]},
			{"op":"gt","args":[` + allCosts + `,{"value":0}]}]},
		"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireRows(t, res, "label", "f1", "f2", "f3", "f4", "f7", "Total")
	n := conditionNotice(res, "filter")
	if n == nil {
		t.Fatalf("no notice names the filter: %+v", res.Notices)
	}
	if !strings.HasPrefix(n.Message, "The filter reads a looked-up total that mixes currencies") {
		t.Fatalf("message = %q", n.Message)
	}
}

// A filter whose sum lookup is in one currency names nothing.
func TestAFilterOverASumLookupInOneCurrencyHasNoNotice(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":4,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]}],
		"filter":{"op":"gt","args":[` + allCosts + `,{"value":0}]},
		"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireRows(t, res, "label", "f1", "f2", "f3", "f4", "Total")
	if len(res.Notices) != 0 {
		t.Fatalf("notices = %+v", res.Notices)
	}
}

// A fill's where that reads a sum lookup which met two currencies is named
// in the notice too.
func TestAFillWhereOverAMixedSumLookupIsNamed(t *testing.T) {
	z := newZoo(t)
	withEuroFeeding(t, z)
	spec := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal","fill":{"where":{"op":"or","args":[
			{"op":"eq","args":[{"value":1},{"value":1}]},
			{"op":"gt","args":[` + allCosts + `,{"value":0}]}]}}}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	// ann sees Leo, Zed and Kit: Kit has no feeding she sees but f6 is
	// Kit's, which she does not keep; the fill shows every animal she sees.
	if got := groupRowLabels(res); !slices.Contains(got, "Kit") || !slices.Contains(got, "Leo") {
		t.Fatalf("groups = %v, want the fill's records", got)
	}
	if conditionNotice(res, "groups.0.fill.where") == nil {
		t.Fatalf("no notice names the fill: %+v", res.Notices)
	}
}

// withMixedCash posts two lines on Cash before July, one in USD and one in
// EUR, so Cash's opening meets two currencies; and two on Petty cash, which
// has no line in July.
func withMixedCash(t *testing.T, z *zoo) {
	t.Helper()
	for _, l := range []struct{ key, account, currency string }{
		{"m1", "cash", "USD"}, {"m2", "cash", "EUR"}, {"m3", "petty", "USD"}, {"m4", "petty", "EUR"},
	} {
		z.rec(t, l.key, "zoo_ledger_lines", map[string]any{
			"entry": z.id("J0"), "account": z.id(l.account), "date": "2026-06-10",
			"debit": 500, "credit": 0, "currency": l.currency, "position": 9, "posted": true,
		})
	}
}

// The General Ledger's having (count > 0 OR opening != 0 OR …) keeps an
// account whose earlier lines mix currencies and that has lines in the
// period: its opening has no value, its activity still shows. The notice
// names the having and the balance column, whose opening cell is blank.
func TestABalanceShapedHavingKeepsAnAccountWhoseOpeningMixesCurrencies(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	withMixedCash(t, z)
	res := runBalances(t, z, map[string]string{"period": glJuly})
	order, rows := balanceGroups(res)
	if !slices.Contains(order, "1000 Cash") {
		t.Fatalf("accounts = %v, want 1000 Cash shown", order)
	}
	cash := rows["1000 Cash"]
	if len(cash) < 3 || cash[0].Kind != KindOpening || cash[0].Balance != nil || cash[1].Kind != KindDetail {
		t.Fatalf("Cash rows = %#v, want a blank opening and its July lines", cash)
	}
	n := conditionNotice(res, "groups.0.having")
	if n == nil || !slices.Contains(n.Columns, "balance") {
		t.Fatalf("notices = %+v, want one naming the having and the balance column", res.Notices)
	}
}

// A notice names only the columns a mixed cell of this page is in: a page
// whose rows show no mixed cell names none, a later page that shows Petty
// cash's blank opening names the balance column.
func TestAMixedCellNoticeNamesOnlyColumnsBlankOnThisPage(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	// Only Petty cash mixes: Cash's lines stay in USD.
	for _, l := range []struct{ key, currency string }{{"m3", "USD"}, {"m4", "EUR"}} {
		z.rec(t, l.key, "zoo_ledger_lines", map[string]any{
			"entry": z.id("J0"), "account": z.id("petty"), "date": "2026-06-10",
			"debit": 500, "credit": 0, "currency": l.currency, "position": 9, "posted": true,
		})
	}
	params := map[string]string{"period": glJuly, "rows": `"all"`}
	opts := Options{Today: "2026-07-15", Limit: 1}
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
	namesBalance := func(res *Result) bool {
		return slices.ContainsFunc(res.Notices, func(n Notice) bool {
			return n.Code == "mixedCurrency" && slices.Contains(n.Columns, "balance")
		})
	}
	if namesBalance(pages[0]) {
		t.Fatalf("page 1 (%v) names balance: %+v", groupRowLabels(pages[0]), pages[0].Notices)
	}
	shown := false
	for i, p := range pages {
		_, rows := balanceGroups(p)
		petty := rows["1200 Petty cash"]
		opening := len(petty) > 0 && petty[0].Kind == KindOpening
		if opening != namesBalance(p) {
			t.Fatalf("page %d shows Petty cash's opening %v, names balance %v: %+v", i+1, opening, namesBalance(p), p.Notices)
		}
		shown = shown || opening
	}
	if !shown {
		t.Fatal("no page shows Petty cash's opening")
	}
}

// keyedLayer is a select below the detail select that computes values once
// per group key (windowFrame.from).
var keyedLayer = regexp.MustCompile(`\) wg\d+ ON `)

// The General Ledger's opening lookup is computed once per group in the
// detail select, however many times the run compiles the column that reads
// it: a viewer sort on the balance and the detail label compile it again.
func TestABalanceShapedReportHoistsItsOpeningLookupOnce(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	params := map[string]string{"period": glJuly}
	opening := strings.NewReplacer("FROM", glFrom, "NET", glNet, "PL", glPL).Replace(glOpening)
	dateLabel := `"detail":{"label":{"path":[{"field":"date"}]}}`
	if !strings.Contains(balanceSpec(), dateLabel) {
		t.Fatal("the balance spec's detail label moved")
	}
	openingLabel := strings.Replace(balanceSpec(), dateLabel, `"detail":{"label":`+opening+`}`, 1)
	hoisted := func(spec string, opts Options) int {
		seen := observeReads(t)
		z.run(t, "ann", spec, opts, params)
		observeRead = nil
		for _, st := range *seen {
			if !strings.Contains(st.sql, "__level") && strings.Contains(st.sql, "AS wh") {
				return len(keyedLayer.FindAllString(st.sql, -1))
			}
		}
		t.Fatal("no detail statement hoists a value")
		return 0
	}
	plain := hoisted(balanceSpec(), Options{Today: "2026-07-15"})
	// The label reads the opening the balance column reads; a viewer sort on
	// the first column sorts by the label.
	labelled := hoisted(openingLabel, Options{Today: "2026-07-15"})
	sorted := hoisted(openingLabel, Options{Today: "2026-07-15", Sort: []ViewSort{{Col: "account"}}})
	if labelled != plain+1 || sorted != labelled {
		t.Fatalf("keyed layers: %d plain, %d with the opening as the label, %d sorted by it; want the label's one more, the sort's none",
			plain, labelled, sorted)
	}
}
