package reports

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// The zoo's chart of accounts: a fictional source whose type select declares
// its options in chart order (Assets, Liabilities, Equity, Income, Expense),
// not A–Z. Two stored types no option names (holding, suspense) and one
// blank type make groups of their own. sort is a number a package keeps to
// order its types; rank is its old key. code is a text some accounts leave
// blank: Equity, Income, holding and suspense have no code at all, and Assets
// has one blank code beside its B.
//
//	key          number name        type       sort code
//	cash         1000   Cash        asset      10   B
//	receivables  1200   Receivables asset      25
//	payables     2000   Payables    liability  4    A
//	card         2100   Card        liability  12
//	overdraft    2200   Cash        liability  30
//	capital      3000   Capital     equity     3
//	sales        4000   Sales       income     9
//	rent         5000   Rent        expense    1    C
//	unapplied    9000   Unapplied   suspense   3
//	escrow       9100   Escrow      holding    50
//	unsorted     9999   Unsorted    (blank)    100  Z
func withAccounts(t *testing.T, z *zoo) {
	t.Helper()
	accounts := core.NewBaseCollection("zoo_accounts")
	accounts.ListRule = rule(`@request.auth.id != ""`)
	accounts.ViewRule = accounts.ListRule
	accounts.Fields.Add(
		&core.NumberField{Name: "number"},
		&core.TextField{Name: "name"},
		&core.SelectField{Name: "type", Values: []string{"asset", "liability", "equity", "income", "expense", "holding", "suspense"}, MaxSelect: 1},
		&core.NumberField{Name: "sort"},
		&core.TextField{Name: "code"},
	)
	z.save(t, accounts)
	for _, a := range []struct {
		key, name, typ, code string
		number, sort         int
	}{
		{"cash", "Cash", "asset", "B", 1000, 10},
		{"receivables", "Receivables", "asset", "", 1200, 25},
		{"payables", "Payables", "liability", "A", 2000, 4},
		{"card", "Card", "liability", "", 2100, 12},
		{"overdraft", "Cash", "liability", "", 2200, 30},
		{"capital", "Capital", "equity", "", 3000, 3},
		{"sales", "Sales", "income", "", 4000, 9},
		{"rent", "Rent", "expense", "C", 5000, 1},
		{"unapplied", "Unapplied", "suspense", "", 9000, 3},
		{"escrow", "Escrow", "holding", "", 9100, 50},
		{"unsorted", "Unsorted", "", "Z", 9999, 100},
	} {
		z.rec(t, a.key, "zoo_accounts", map[string]any{"number": a.number, "name": a.name, "type": a.typ, "sort": a.sort, "code": a.code})
	}
	registerAccountsSource()
}

func registerAccountsSource() {
	RegisterSource(Source{
		Key: "zoo.accounts", Collection: "zoo_accounts", Label: "Accounts",
		Fields: []Field{
			{Key: "number", Label: "No.", Type: Number, Aggregates: MinMax},
			{Key: "name", Label: "Name", Type: Text, Aggregates: MinMax},
			{Key: "type", Label: "Type", Type: Select, Options: []Option{
				{Value: "asset", Label: "Assets"},
				{Value: "liability", Label: "Liabilities"},
				{Value: "equity", Label: "Equity"},
				{Value: "income", Label: "Income"},
				{Value: "expense", Label: "Expense"},
			}},
			{Key: "sort", Label: "Sort", Type: Number, Aggregates: MinMax},
			{Key: "code", Label: "Code", Type: Text, Aggregates: MinMax},
		},
		Aliases: map[string]string{"rank": "sort"},
	})
}

// accountsParam is an accounts report whose Accounts filter lists the
// accounts grouped by type. GROUP is the group path and ORDER the
// optionsFrom's groupOrder member (empty for none).
const accountsParam = `{"version":2,"source":"zoo.accounts",
	"params":[{"id":"accounts","type":"choice","label":"Accounts","multiple":true,
		"optionsFrom":{"source":"zoo.accounts","value":[{"field":"id"}],"label":[{"field":"name"}]GROUPORDER}}],
	"columns":[{"id":"name","path":[{"field":"name"}]}],
	"filter":{"op":"in","args":[{"path":[{"field":"id"}]},{"param":"accounts"}]}}`

// accountsBy is accountsParam grouped by type, with groupOrder order ("" for
// none).
func accountsBy(order string) string {
	return accountsGrouped(`[{"field":"type"}]`, order)
}

// accountsGrouped is accountsParam grouped by group ("" for no group), with
// groupOrder order ("" for none).
func accountsGrouped(group, order string) string {
	member := ""
	if group != "" {
		member += `,"group":` + group
	}
	if order != "" {
		member += `,"groupOrder":` + order
	}
	return strings.Replace(accountsParam, "GROUPORDER", member, 1)
}

func groupLabels(res *ParamOptions) []string {
	out := []string{}
	for _, g := range res.Groups {
		out = append(out, g.Label)
	}
	return out
}

func optionValues(res *ParamOptions) []string {
	out := []string{}
	for _, o := range res.Options {
		out = append(out, o.Value)
	}
	return out
}

// requireRefusal is a spec refusal at `at` with this status and code. The
// message is the engine's own words, so only where and what kind are checked.
func requireRefusal(t *testing.T, err error, status int, at, code string) {
	t.Helper()
	requireSpecError(t, err, status, at, "")
	if got := err.(*SpecError).Problems[at].Code(); got != code {
		t.Fatalf("code at %q = %q, want %q", at, got, code)
	}
}

// The ways a grouped list orders its groups, and the full first page of
// options each gives: group by group, by label then value inside a group.
var accountOrders = []struct {
	name, order string
	groups      []string
	options     []string // fixture keys, in page order
}{
	// By label, as today: the blank group first, then A–Z by the group's label
	// (a stored value no option names is labelled by that value).
	{"omitted", "", []string{"(none)", "Assets", "Equity", "Expense", "Income", "Liabilities", "holding", "suspense"},
		[]string{"unsorted", "cash", "receivables", "capital", "rent", "sales", "card", "overdraft", "payables", "escrow", "unapplied"}},
	{"label", `{"by":"label"}`, []string{"(none)", "Assets", "Equity", "Expense", "Income", "Liabilities", "holding", "suspense"},
		[]string{"unsorted", "cash", "receivables", "capital", "rent", "sales", "card", "overdraft", "payables", "escrow", "unapplied"}},
	// The select's declared order; values no option names after the named
	// ones, by label; the blank group still first.
	{"declared", `{"by":"declared"}`, []string{"(none)", "Assets", "Liabilities", "Equity", "Income", "Expense", "holding", "suspense"},
		[]string{"unsorted", "cash", "receivables", "card", "overdraft", "payables", "capital", "sales", "rent", "escrow", "unapplied"}},
	// By each group's smallest sort, as a number (Income's 9 before Assets'
	// 10), a tie by label (Equity and suspense are both 3); the blank group
	// first though its sort is the largest.
	{"path", `{"by":"path","path":[{"field":"sort"}]}`, []string{"(none)", "Expense", "Equity", "suspense", "Liabilities", "Income", "Assets", "holding"},
		[]string{"unsorted", "rent", "capital", "unapplied", "card", "overdraft", "payables", "sales", "cash", "receivables", "escrow"}},
	// By each group's smallest code, a blank code being no value (Assets is B,
	// not its blank); the groups whose codes are all blank come after the
	// groups with a code, by label; the blank group still first.
	{"path with blanks", `{"by":"path","path":[{"field":"code"}]}`, []string{"(none)", "Liabilities", "Assets", "Expense", "Equity", "Income", "holding", "suspense"},
		[]string{"unsorted", "card", "overdraft", "payables", "cash", "receivables", "rent", "capital", "sales", "escrow", "unapplied"}},
}

// optionsFrom.groupOrder orders a grouped list's groups, and the options come
// group by group in that same order.
func TestGroupOrderOrdersTheGroupsAndTheOptions(t *testing.T) {
	z := newZoo(t)
	withAccounts(t, z)
	for _, c := range accountOrders {
		res := z.options(t, "ann", accountsBy(c.order), OptionsRequest{Param: "accounts"})
		if got := groupLabels(res); !slices.Equal(got, c.groups) {
			t.Errorf("%s: groups = %v, want %v", c.name, got, c.groups)
		}
		if got, want := optionValues(res), z.keysToIDs(c.options); !slices.Equal(got, want) {
			t.Errorf("%s: options = %v, want %v", c.name, optionLabels(res), z.keysToLabels(t, c.options))
		}
	}
}

// The groups of a search keep the order: only the groups of matching
// options, in declared order.
func TestGroupOrderHoldsForTheGroupsOfASearch(t *testing.T) {
	z := newZoo(t)
	withAccounts(t, z)
	res := z.options(t, "ann", accountsBy(`{"by":"declared"}`), OptionsRequest{Param: "accounts", Search: "ca"})
	if got, want := groupLabels(res), []string{"Assets", "Liabilities", "Equity"}; !slices.Equal(got, want) {
		t.Fatalf("searched groups = %v, want %v", got, want)
	}
	if got, want := optionValues(res), z.keysToIDs([]string{"cash", "card", "overdraft", "capital"}); !slices.Equal(got, want) {
		t.Fatalf("searched options = %v", optionLabels(res))
	}
}

// Pages of a grouped list only append: read page after page, at any page
// size, they are the one full page, group by group. A label in two groups
// (Cash) is split across a page boundary without a loss or a repeat.
func TestGroupedOptionsPageInGroupOrder(t *testing.T) {
	z := newZoo(t)
	withAccounts(t, z)
	for _, c := range accountOrders {
		spec := accountsBy(c.order)
		want := z.keysToIDs(c.options)
		for _, limit := range []int{1, 2, 3} {
			var got []string
			req := OptionsRequest{Param: "accounts", Limit: limit}
			for range len(want) + 1 {
				res := z.options(t, "ann", spec, req)
				if len(res.Options) > limit {
					t.Fatalf("%s limit %d: a page of %d", c.name, limit, len(res.Options))
				}
				got = append(got, optionValues(res)...)
				if res.Next == "" {
					break
				}
				req.After = res.Next
			}
			if !slices.Equal(got, want) {
				t.Errorf("%s limit %d: pages = %v, want %v", c.name, limit, z.idsToLabels(t, got), z.keysToLabels(t, c.options))
			}
		}
	}
}

// A declared order reads the options of a select at the end of a link too.
func TestGroupOrderDeclaredThroughALink(t *testing.T) {
	app := withZooRegistry(t)
	spec := loadSpec(t, withGroup(`[{"link":"site"},{"field":"region"}],"groupOrder":{"by":"declared"}`))
	if err := Validate(app, spec); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

// What a groupOrder is refused for, by where and what kind: the words are
// the engine's.
func TestGroupOrderRefusals(t *testing.T) {
	app := withZooRegistry(t)
	registerAccountsSource()
	for _, c := range []struct {
		name, spec, at, code string
		status               int
	}{
		{"declared on a text group", accountsGrouped(`[{"field":"name"}]`, `{"by":"declared"}`),
			"params.0.optionsFrom.groupOrder", "invalid", 400},
		{"declared on a group that ends at a link", withGroup(`[{"link":"site"}],"groupOrder":{"by":"declared"}`),
			"params.0.optionsFrom.groupOrder", "invalid", 400},
		{"an unknown by", accountsBy(`{"by":"sideways"}`),
			"params.0.optionsFrom.groupOrder.by", "invalid", 400},
		{"no by", accountsBy(`{}`),
			"params.0.optionsFrom.groupOrder.by", "invalid", 400},
		{"an order with no group", accountsGrouped("", `{"by":"label"}`),
			"params.0.optionsFrom.groupOrder", "invalid", 400},
		{"a path that does not bind", accountsBy(`{"by":"path","path":[{"field":"nope"}]}`),
			"params.0.optionsFrom.groupOrder.path.0.field", "unknown_field", 422},
		{"by path with no path", accountsBy(`{"by":"path"}`),
			"params.0.optionsFrom.groupOrder.path", "invalid", 400},
		// A path orders only by path.
		{"a path with by label", accountsBy(`{"by":"label","path":[{"field":"sort"}]}`),
			"params.0.optionsFrom.groupOrder.path", "invalid", 400},
		{"a path with by declared", accountsBy(`{"by":"declared","path":[{"field":"sort"}]}`),
			"params.0.optionsFrom.groupOrder.path", "invalid", 400},
		{"a to-many path", withGroup(`[{"field":"species"}],"groupOrder":{"by":"path","path":[{"link":"feedings"},{"field":"grams"}]}`),
			"params.0.optionsFrom.groupOrder.path.0", "invalid", 400},
	} {
		t.Run(c.name, func(t *testing.T) {
			requireRefusal(t, Validate(app, loadSpec(t, c.spec)), c.status, c.at, c.code)
		})
	}
}

// A groupOrder path's renamed key loads as its current key, and orders as it.
func TestGroupOrderPathResolvesAliases(t *testing.T) {
	z := newZoo(t)
	withAccounts(t, z)
	aliased := accountsBy(`{"by":"path","path":[{"field":"rank"}]}`)
	spec, info, err := Load([]byte(aliased))
	if err != nil {
		t.Fatal(err)
	}
	order := spec.Params[0].OptionsFrom.GroupOrder
	if order.By != GroupOrderPath || order.Path[0].Field != "sort" ||
		!slices.Contains(info.Aliased, Alias{Source: "zoo.accounts", From: "rank", To: "sort"}) {
		t.Fatalf("groupOrder = %+v, aliased %v", order, info.Aliased)
	}
	byPath := accountOrders[3]
	res := z.options(t, "ann", aliased, OptionsRequest{Param: "accounts"})
	if got := groupLabels(res); !slices.Equal(got, byPath.groups) {
		t.Fatalf("aliased groups = %v, want %v", got, byPath.groups)
	}
}

// A declared order over a select with many options pages as a short one
// does: 11,000 options, read up to three times by a cursor page (a column,
// the order and the cursor's condition), would bind 33,000 values, past
// SQLite's 32,766 variables, if each option were a variable of its own (as
// optionLabelSQL's one JSON parameter avoids). The bins' stored kinds are
// declared Zinc, Moss, Alder, the reverse of A–Z, among the others.
func TestGroupOrderDeclaredPagesOverALongOptionList(t *testing.T) {
	z := newZoo(t)
	bins := core.NewBaseCollection("zoo_bins")
	bins.ListRule = rule(`@request.auth.id != ""`)
	bins.Fields.Add(&core.TextField{Name: "name"}, &core.SelectField{Name: "kind", Values: []string{"alder", "moss", "zinc"}, MaxSelect: 1})
	z.save(t, bins)
	var options []Option
	for i := range 11000 {
		switch i {
		case 5:
			options = append(options, Option{Value: "zinc", Label: "Zinc"})
		case 20:
			options = append(options, Option{Value: "moss", Label: "Moss"})
		case 10000:
			options = append(options, Option{Value: "alder", Label: "Alder"})
		default:
			options = append(options, Option{Value: fmt.Sprintf("x%d", i), Label: fmt.Sprintf("X %d", i)})
		}
	}
	RegisterSource(Source{
		Key: "zoo.bins", Collection: "zoo_bins", Label: "Bins",
		Fields: []Field{
			{Key: "name", Label: "Name", Type: Text, Aggregates: MinMax},
			{Key: "kind", Label: "Kind", Type: Select, Options: options},
		},
	})
	for _, b := range []struct{ key, kind string }{
		{"a1", "alder"}, {"a2", "alder"}, {"m1", "moss"}, {"m2", "moss"}, {"z1", "zinc"}, {"z2", "zinc"},
	} {
		z.rec(t, b.key, "zoo_bins", map[string]any{"name": strings.ToUpper(b.key), "kind": b.kind})
	}
	want := []string{"Z1", "Z2", "M1", "M2", "A1", "A2"}
	spec := `{"version":2,"source":"zoo.bins",
		"params":[{"id":"bins","type":"choice","multiple":true,
			"optionsFrom":{"source":"zoo.bins","value":[{"field":"id"}],"label":[{"field":"name"}],"group":[{"field":"kind"}],"groupOrder":{"by":"declared"}}}],
		"columns":[{"id":"name","path":[{"field":"name"}]}],
		"filter":{"op":"in","args":[{"path":[{"field":"id"}]},{"param":"bins"}]}}`
	loaded, _, err := Load([]byte(spec))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	req := OptionsRequest{Spec: loaded, Param: "bins", Limit: 2, Auth: z.recs["ann"]}
	for page := 1; page <= len(want); page++ {
		res, err := ReadParamOptions(context.Background(), z.app, req)
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		if page == 1 {
			if labels := groupLabels(res); !slices.Equal(labels, []string{"Zinc", "Moss", "Alder"}) {
				t.Fatalf("groups = %v, want Zinc, Moss, Alder", labels)
			}
		}
		got = append(got, optionLabels(res)...)
		if res.Next == "" {
			break
		}
		req.After = res.Next
	}
	if !slices.Equal(got, want) {
		t.Fatalf("pages = %v, want %v", got, want)
	}
}

// The spec names each order by a constant.
func TestGroupOrderConstants(t *testing.T) {
	if GroupOrderLabel != "label" || GroupOrderDeclared != "declared" || GroupOrderPath != "path" {
		t.Fatalf("orders = %q %q %q", GroupOrderLabel, GroupOrderDeclared, GroupOrderPath)
	}
}

func (z *zoo) keysToIDs(keys []string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, z.id(k))
	}
	return out
}

func (z *zoo) keysToLabels(t *testing.T, keys []string) []string {
	t.Helper()
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, z.recs[k].GetString("name"))
	}
	return out
}

func (z *zoo) idsToLabels(t *testing.T, ids []string) []string {
	t.Helper()
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		for _, r := range z.recs {
			if r.Id == id {
				out = append(out, r.GetString("name"))
			}
		}
	}
	return out
}
