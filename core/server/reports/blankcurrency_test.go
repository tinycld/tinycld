package reports

import (
	"strings"
	"testing"
)

// requireNoCurrencyWhereBlank wants every row's cell of col that is blank
// to carry no currency, and every row's cell that has a value to carry want
// ("" for none).
func requireNoCurrencyWhereBlank(t *testing.T, res *Result, col, want string) {
	t.Helper()
	for _, r := range res.Rows {
		cur, has := r.Cells[col+"@currency"]
		if r.Cells[col] == nil {
			if has {
				t.Errorf("%s: blank %s carries currency %v", r.ID, col, cur)
			}
			continue
		}
		if want == "" && has {
			t.Errorf("%s: %s = %v carries currency %v", r.ID, col, r.Cells[col], cur)
		}
		if want != "" && cur != want {
			t.Errorf("%s: %s = %v carries currency %v, want %s", r.ID, col, r.Cells[col], cur, want)
		}
	}
}

// A cell blank because a sum lookup met two currencies names no currency:
// bob's fees of zed (USD) and mo (GBP), on detail and total rows.
func TestABlankMixedSumLookupCellCarriesNoCurrency(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "bob", sitesWithSums, Options{}, nil)
	requireRows(t, res, "usdFees", nil, nil, nil)
	requireNoCurrencyWhereBlank(t, res, "usdFees", "")
}

// A cell blank because the currencies it combines differ on its row (R24)
// names no currency, on detail, group and total rows; a cell with a value
// keeps its row's.
func TestABlankMismatchedCellCarriesNoCurrency(t *testing.T) {
	z := newZoo(t)
	f3 := z.recs["f3"]
	f3.Set("currency", "EUR")
	if err := z.app.Save(f3); err != nil {
		t.Fatal(err)
	}
	spec := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"rolled","formula":{"op":"add","args":[` + orZero(lookedUpCost("f1")) + `,{"op":"sum","args":[{"path":[{"field":"cost"}]}]}]}}],
		"groups":[{"id":"bySpecies","col":"species"}],
		"sort":[{"col":"species"},{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
	blank := 0
	for _, r := range res.Rows {
		if r.Cells["rolled"] == nil {
			blank++
		}
	}
	if blank < 3 {
		t.Fatalf("want a blank detail, group and total row, got %d blank rows", blank)
	}
	requireNoCurrencyWhereBlank(t, res, "rolled", "USD")
}

// A formula that reads a row's value is blank on a group and a total row
// (it has no value there), and a blank cell names no currency: the sum of
// the lines' debits added to a stored month looked up by the group's account,
// in the record-leaf and the outer-leaf form. The bank group (USD lines, EUR
// month) is blank on its detail rows too.
func TestAUnreadFormulaCellCarriesNoCurrency(t *testing.T) {
	for name, account := range map[string]string{
		"record leaf": `{"record":"g","path":[{"field":"id"}]}`,
		"outer leaf":  `{"outer":[{"link":"account"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			z := newZoo(t)
			withBalanceZoo(t, z)
			withLedgerMonths(t, z, "zoo.ledger_lines.currency")
			months := strings.ReplaceAll(monthsOf, "ACCOUNT", account)
			spec := `{"version":4,"source":"zoo.ledger_lines","columns":[
				{"id":"acct","path":[{"link":"account"}]},
				{"id":"day","path":[{"field":"date"}]},
				{"id":"rolled","formula":{"op":"add","args":[{"op":"sum","args":[{"path":[{"field":"debit"}]}]},` + months + `]}}],
				"filter":{"op":"eq","args":[{"path":[{"field":"posted"}]},{"value":true}]},
				"groups":[{"id":"g","col":"acct"}],
				"sort":[{"col":"acct"},{"col":"day"}]}`
			res := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
			blankGroups := 0
			for _, r := range res.Rows {
				if r.Kind != KindDetail && r.Cells["rolled"] == nil {
					blankGroups++
				}
			}
			if blankGroups == 0 {
				t.Fatalf("want a blank group or total row: %v", rowsOf(res, "rolled"))
			}
			requireNoCurrencyWhereBlank(t, res, "rolled", "USD")
		})
	}
}

// A count of a sum lookup is a number: it carries no currency on any row,
// and counts the rows whose sum met two currencies.
func TestACountOverAMixedSumLookupCarriesNoCurrency(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":4,"source":"zoo.sites","columns":[
		{"id":"region","path":[{"field":"region"}]},
		{"id":"name","path":[{"field":"name"}]},
		{"id":"n","aggregate":"count","formula":` + mixedFees + `}],
		"groups":[{"id":"g","col":"region"}],
		"layout":[{"section":"north","title":"North","key":"north"},
			{"row":"again","label":"Again","formula":{"subtotal":"north"}}]}`
	res := z.run(t, "bob", spec, Options{ExpandAll: true}, nil)
	counted := 0
	for _, r := range res.Rows {
		if _, has := r.Cells["n@currency"]; has {
			t.Errorf("%s (%s): n = %v carries currency %v", r.ID, r.Kind, r.Cells["n"], r.Cells["n@currency"])
		}
		if r.Kind != KindDetail && r.Cells["n"] != nil {
			counted++
		}
	}
	if counted == 0 {
		t.Fatalf("no group or computed row counts: %v", rowsOf(res, "n"))
	}
}
