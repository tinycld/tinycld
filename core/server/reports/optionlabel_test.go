package reports

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// codeName is a label formula over the zoo's accounts: "<code> <name>", the
// name alone when the account has no code, as a package labels a numbered
// account "1000 Business Checking".
const codeName = `{"op":"if","args":[
	{"op":"isEmpty","args":[{"path":[{"field":"code"}]}]},
	{"path":[{"field":"name"}]},
	{"op":"concat","args":[{"path":[{"field":"code"}]},{"value":" "},{"path":[{"field":"name"}]}]}]}`

// accountsFrom is an accounts report whose Accounts filter reads its options
// with these optionsFrom members after source and value (label, labelFormula,
// order, group, …), and prints the picks' labels in a header line.
func accountsFrom(members string) string {
	return `{"version":4,"source":"zoo.accounts",
		"params":[{"id":"accounts","type":"choice","label":"Accounts","multiple":true,"header":"Accounts: {label}",
			"optionsFrom":{"source":"zoo.accounts","value":[{"field":"id"}],` + members + `}}],
		"columns":[{"id":"name","path":[{"field":"name"}]}],
		"filter":{"op":"in","args":[{"path":[{"field":"id"}]},{"param":"accounts"}]}}`
}

// readAllOptions reads every page of a list at this page size.
func readAllOptions(t *testing.T, z *zoo, spec string, limit int) []string {
	t.Helper()
	var got []string
	req := OptionsRequest{Param: "accounts", Limit: limit}
	for range 20 {
		res := z.options(t, "ann", spec, req)
		got = append(got, optionLabels(res)...)
		if res.Next == "" {
			return got
		}
		req.After = res.Next
	}
	t.Fatalf("limit %d: the list does not end", limit)
	return nil
}

// A label formula labels each option with text made of several fields, and
// the list orders by that label, as a path label orders.
func TestOptionsLabelFormulaLabelsTheOptions(t *testing.T) {
	z := newZoo(t)
	withAccounts(t, z)
	spec := accountsFrom(`"labelFormula":` + codeName)
	want := []string{"A Payables", "B Cash", "C Rent", "Capital", "Card", "Cash", "Escrow", "Receivables", "Sales", "Unapplied", "Z Unsorted"}
	res := z.options(t, "ann", spec, OptionsRequest{Param: "accounts"})
	if got := optionLabels(res); !slices.Equal(got, want) {
		t.Fatalf("labels = %q, want %q", got, want)
	}
	for _, o := range res.Options {
		if o.Type != "" || o.Currency != "" {
			t.Fatalf("option %q: type %q currency %q, want a plain text label", o.Label, o.Type, o.Currency)
		}
	}
	// A search reads the label as the picker shows it.
	searched := z.options(t, "ann", spec, OptionsRequest{Param: "accounts", Search: "b c"})
	if got := optionLabels(searched); !slices.Equal(got, []string{"B Cash"}) {
		t.Fatalf("search %q = %q, want B Cash", "b c", got)
	}
	// Asked values are labelled by the formula too.
	asked := z.options(t, "ann", spec, OptionsRequest{Param: "accounts", Values: []string{z.id("rent"), z.id("capital")}})
	if got := optionLabels(asked); !slices.Equal(got, []string{"C Rent", "Capital"}) {
		t.Fatalf("asked labels = %q", got)
	}
}

// The header line names the picks by the label formula.
func TestOptionsLabelFormulaNamesThePicksInTheHeader(t *testing.T) {
	z := newZoo(t)
	withAccounts(t, z)
	picks := fmt.Sprintf(`[%q,%q]`, z.id("cash"), z.id("capital"))
	res := z.run(t, "ann", accountsFrom(`"labelFormula":`+codeName), Options{}, map[string]string{"accounts": picks})
	if want := []string{"Accounts: B Cash, Capital"}; !slices.Equal(res.HeaderLines, want) {
		t.Fatalf("headerLines = %q, want %q", res.HeaderLines, want)
	}
	ordered := accountsFrom(`"labelFormula":` + codeName + `,"order":[{"field":"number"}]`)
	res = z.run(t, "ann", ordered, Options{}, map[string]string{"accounts": picks})
	if want := []string{"Accounts: B Cash, Capital"}; !slices.Equal(res.HeaderLines, want) {
		t.Fatalf("ordered headerLines = %q, want %q", res.HeaderLines, want)
	}
	res = z.run(t, "ann", ordered, Options{}, map[string]string{"accounts": fmt.Sprintf(`[%q,%q]`, z.id("rent"), z.id("payables"))})
	if want := []string{"Accounts: A Payables, C Rent"}; !slices.Equal(res.HeaderLines, want) {
		t.Fatalf("ordered headerLines = %q, want %q", res.HeaderLines, want)
	}
}

// The order path orders the options by its value, a tie by label then
// value, and an option with no value after those with one. Pages only
// append, at any page size.
func TestOptionsOrderPathOrdersTheOptions(t *testing.T) {
	z := newZoo(t)
	withAccounts(t, z)
	for _, c := range []struct {
		name, members string
		want          []string
	}{
		{"no order: by label", `"label":[{"field":"name"}]`,
			[]string{"Capital", "Card", "Cash", "Cash", "Escrow", "Payables", "Receivables", "Rent", "Sales", "Unapplied", "Unsorted"}},
		{"by a number", `"label":[{"field":"name"}],"order":[{"field":"number"}]`,
			[]string{"Cash", "Receivables", "Payables", "Card", "Cash", "Capital", "Sales", "Rent", "Unapplied", "Escrow", "Unsorted"}},
		// Capital and Unapplied tie at 3: by label.
		{"a tie by label", `"label":[{"field":"name"}],"order":[{"field":"sort"}]`,
			[]string{"Rent", "Capital", "Unapplied", "Payables", "Sales", "Cash", "Card", "Receivables", "Cash", "Escrow", "Unsorted"}},
		// A blank code is no value: those accounts come last, by label.
		{"blanks last", `"label":[{"field":"name"}],"order":[{"field":"code"}]`,
			[]string{"Payables", "Cash", "Rent", "Unsorted", "Capital", "Card", "Cash", "Escrow", "Receivables", "Sales", "Unapplied"}},
		{"with a label formula", `"labelFormula":` + codeName + `,"order":[{"field":"number"}]`,
			[]string{"B Cash", "Receivables", "A Payables", "Card", "Cash", "Capital", "Sales", "C Rent", "Unapplied", "Escrow", "Z Unsorted"}},
	} {
		spec := accountsFrom(c.members)
		if got := optionLabels(z.options(t, "ann", spec, OptionsRequest{Param: "accounts"})); !slices.Equal(got, c.want) {
			t.Errorf("%s: labels = %q, want %q", c.name, got, c.want)
		}
		for _, limit := range []int{1, 2, 3} {
			if got := readAllOptions(t, z, spec, limit); !slices.Equal(got, c.want) {
				t.Errorf("%s limit %d: pages = %q, want %q", c.name, limit, got, c.want)
			}
		}
	}
}

// In a grouped list the order path orders the options inside each group;
// the groups keep their own order.
func TestOptionsOrderPathOrdersInsideGroups(t *testing.T) {
	z := newZoo(t)
	withAccounts(t, z)
	grouped := accountsFrom(`"label":[{"field":"name"}],"order":[{"field":"sort"}],"group":[{"field":"type"}],"groupOrder":{"by":"declared"}`)
	res := z.options(t, "ann", grouped, OptionsRequest{Param: "accounts"})
	if got, want := groupLabels(res), []string{"(none)", "Assets", "Liabilities", "Equity", "Income", "Expense", "holding", "suspense"}; !slices.Equal(got, want) {
		t.Fatalf("groups = %q, want %q", got, want)
	}
	// Liabilities by sort: Payables 4, Card 12, Cash 30.
	want := []string{"Unsorted", "Cash", "Receivables", "Payables", "Card", "Cash", "Capital", "Sales", "Rent", "Escrow", "Unapplied"}
	if got := optionLabels(res); !slices.Equal(got, want) {
		t.Fatalf("labels = %q, want %q", got, want)
	}
	for _, limit := range []int{1, 2, 3} {
		if got := readAllOptions(t, z, grouped, limit); !slices.Equal(got, want) {
			t.Errorf("limit %d: pages = %q, want %q", limit, got, want)
		}
	}
}

// What a label formula and an order path are refused for, by where and what
// kind: the words are the engine's.
func TestOptionsLabelFormulaAndOrderRefusals(t *testing.T) {
	app := withZooRegistry(t)
	registerAccountsSource()
	for _, c := range []struct {
		name, members, at, code string
		status                  int
	}{
		{"a label and a label formula", `"label":[{"field":"name"}],"labelFormula":` + codeName,
			"params.0.optionsFrom.label", "invalid", 400},
		{"no label at all", `"order":[{"field":"number"}]`,
			"params.0.optionsFrom.label", "invalid", 400},
		{"a label formula that is not text", `"labelFormula":{"path":[{"field":"number"}]}`,
			"params.0.optionsFrom.labelFormula", "invalid", 400},
		{"a label formula over an unknown field", `"labelFormula":{"op":"concat","args":[{"path":[{"field":"nope"}]},{"value":"x"}]}`,
			"params.0.optionsFrom.labelFormula.args.0.path.0.field", "unknown_field", 422},
		{"a label formula that totals", `"labelFormula":{"op":"concat","args":[{"op":"sum","args":[{"path":[{"field":"number"}]}]},{"value":"x"}]}`,
			"params.0.optionsFrom.labelFormula.args.0", "invalid", 400},
		{"a label formula that reads a column", `"labelFormula":{"op":"concat","args":[{"col":"name"},{"value":"x"}]}`,
			"params.0.optionsFrom.labelFormula.args.0", "invalid", 400},
		{"an order that does not bind", `"label":[{"field":"name"}],"order":[{"field":"nope"}]`,
			"params.0.optionsFrom.order.0.field", "unknown_field", 422},
		{"an order that ends at a link", `"label":[{"field":"name"}],"order":[{"link":"nope"}]`,
			"params.0.optionsFrom.order.0.link", "unknown_link", 422},
	} {
		t.Run(c.name, func(t *testing.T) {
			requireRefusal(t, Validate(app, loadSpec(t, accountsFrom(c.members))), c.status, c.at, c.code)
		})
	}
}

// An order path and a label formula's renamed keys load as their current
// keys.
func TestOptionsOrderAndLabelFormulaResolveAliases(t *testing.T) {
	registerAccountsSource()
	raw := accountsFrom(`"labelFormula":{"op":"concat","args":[{"path":[{"field":"rank"}]},{"value":" "},{"path":[{"field":"name"}]}]},"order":[{"field":"rank"}]`)
	spec, info, err := Load([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	of := spec.Params[0].OptionsFrom
	if of.Order[0].Field != "sort" || of.LabelFormula.Args[0].Path[0].Field != "sort" {
		t.Fatalf("order = %+v, labelFormula = %+v", of.Order, of.LabelFormula.Args[0])
	}
	if n := strings.Count(fmt.Sprint(info.Aliased), "rank"); n != 2 {
		t.Fatalf("aliased = %v, want rank twice", info.Aliased)
	}
}
