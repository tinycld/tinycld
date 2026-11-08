package reports

import (
	"strings"
	"testing"
)

// sortsAsBlank is the sentence a notice gives a sort key over a mixed sum.
const sortsAsBlank = "reads a looked-up total that mixes currencies, so it sorts as blank there."

// A detail sort key that reads a sum lookup which met two currencies sorts
// as blank, and the run names it in its mixedCurrency notice as it names a
// filter.
func TestADetailSortOverAMixedSumLookupIsNamed(t *testing.T) {
	z := newZoo(t)
	withEuroFeeding(t, z)
	spec := `{"version":4,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]}],
		"sort":[{"formula":` + allCosts + `},{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireRows(t, res, "label", "f1", "f2", "f3", "f4", "f7", "Total")
	n := conditionNotice(res, "sort.0.formula")
	if n == nil {
		t.Fatalf("no notice names the sort key: %+v", res.Notices)
	}
	if n.Message != "The formula of sort key 1 "+sortsAsBlank {
		t.Fatalf("message = %q", n.Message)
	}
}

// A sort key over a mixed sum on the group rows alone (every group shut, so
// no detail row is read) is named too.
func TestAGroupSortOverAMixedSumLookupIsNamed(t *testing.T) {
	z := newZoo(t)
	withEuroFeeding(t, z)
	spec := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal"}],
		"sort":[{"formula":` + leoCosts + `}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	for _, r := range res.Rows {
		if r.Kind == KindDetail {
			t.Fatalf("a detail row %q is shown; the test needs the group rows alone", r.Label)
		}
	}
	n := conditionNotice(res, "sort.0.formula")
	if n == nil {
		t.Fatalf("no notice names the sort key: %+v", res.Notices)
	}
	if !strings.Contains(n.Message, sortsAsBlank) {
		t.Fatalf("message = %q", n.Message)
	}
}

// A sort by a column whose sum lookup met two currencies names the column
// by its label.
func TestASortByAMixedColumnNamesTheColumn(t *testing.T) {
	z := newZoo(t)
	withEuroFeeding(t, z)
	spec := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"all","label":"All costs","formula":` + allCosts + `}],
		"sort":[{"col":"all"},{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	n := conditionNotice(res, "sort.0.col")
	if n == nil {
		t.Fatalf("no notice names the sort key: %+v", res.Notices)
	}
	if !strings.Contains(n.Message, `The sort by "All costs" `+sortsAsBlank) {
		t.Fatalf("message = %q", n.Message)
	}
}

// A sort key whose sum lookup is in one currency names nothing.
func TestASortOverASumLookupInOneCurrencyHasNoNotice(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":4,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]}],
		"sort":[{"formula":` + allCosts + `},{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	if len(res.Notices) != 0 {
		t.Fatalf("notices = %+v", res.Notices)
	}
}

// A condition as a sort key over a mixed sum is unknown there, so it sorts
// as blank and is named as well.
func TestAConditionSortKeyOverAMixedSumLookupIsNamed(t *testing.T) {
	z := newZoo(t)
	withEuroFeeding(t, z)
	spec := `{"version":4,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]}],
		"sort":[{"col":"note"},{"formula":{"op":"gt","args":[` + allCosts + `,{"value":0}]}}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	n := conditionNotice(res, "sort.1.formula")
	if n == nil {
		t.Fatalf("no notice names the sort key: %+v", res.Notices)
	}
	if n.Message != "The formula of sort key 2 "+sortsAsBlank {
		t.Fatalf("message = %q", n.Message)
	}
}

// A condition as a sort key that an `or` term decides wherever the mixed sum
// leaves it unknown has a value on every row, so it sorts by that value and
// names nothing: on detail rows and on group rows.
func TestAConditionSortKeyThatATrueTermDecidesIsNotNamed(t *testing.T) {
	z := newZoo(t)
	withEuroFeeding(t, z)
	decided := func(costs string) string {
		return `{"op":"or","args":[{"op":"gt","args":[` + costs + `,{"value":0}]},{"value":true}]}`
	}
	detail := `{"version":4,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]}],
		"sort":[{"col":"note"},{"formula":` + decided(allCosts) + `}]}`
	if res := z.run(t, "ann", detail, Options{}, nil); len(res.Notices) != 0 {
		t.Fatalf("detail sort: notices = %+v", res.Notices)
	}
	group := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal"}],
		"sort":[{"formula":` + decided(leoCosts) + `}]}`
	if res := z.run(t, "ann", group, Options{}, nil); len(res.Notices) != 0 {
		t.Fatalf("group sort: notices = %+v", res.Notices)
	}
}
