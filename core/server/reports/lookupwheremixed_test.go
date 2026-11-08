package reports

import (
	"slices"
	"strings"
	"testing"
)

// unknownWhere is a count of the animals whose where reads bob's mixed sum
// of fees (USD and GBP): the where is unknown on every animal, so the count
// matches none, and the run names that where in its mixedCurrency notice.
const unknownWhere = `{"op":"lookup","source":"zoo.animals","pick":"count","value":{"path":[{"field":"id"}]},
	"where":{"op":"gt","args":[` + mixedFees + `,{"value":0}]}}`

// requireWhereNamed runs spec as bob, collapsed and expanded, and wants one
// mixedCurrency notice whose conditions are want, each told in a sentence.
func requireWhereNamed(t *testing.T, z *zoo, spec string, want ...string) {
	t.Helper()
	for _, opts := range []Options{{}, {ExpandAll: true}} {
		res := z.run(t, "bob", spec, opts, nil)
		i := slices.IndexFunc(res.Notices, func(n Notice) bool { return n.Code == "mixedCurrency" })
		if i < 0 {
			t.Fatalf("expand %v: no mixedCurrency notice: %+v", opts.ExpandAll, res.Notices)
		}
		n := res.Notices[i]
		if !slices.Equal(n.Conditions, want) {
			t.Fatalf("expand %v: conditions = %v, want %v", opts.ExpandAll, n.Conditions, want)
		}
		if strings.Count(n.Message, "The where of a lookup in ") != len(want) {
			t.Fatalf("expand %v: message = %q", opts.ExpandAll, n.Message)
		}
	}
}

func TestALookupWhereOverAMixedSumIsNamedOnDetailAndTotalRows(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":4,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]},
		{"id":"fed","formula":` + unknownWhere + `}],"sort":[{"col":"name"}]}`
	requireWhereNamed(t, z, spec, "columns.1.formula.where")
	res := z.run(t, "bob", spec, Options{}, nil)
	for i, v := range rowsOf(res, "fed") {
		if v != int64(0) {
			t.Fatalf("row %d fed = %#v, want 0", i, v)
		}
	}
	want := `The where of a lookup in the "fed" column reads a looked-up total that mixes currencies, so that total counts as unknown there.`
	if len(res.Notices) != 1 || res.Notices[0].Message != want {
		t.Fatalf("notices = %+v, want one: %q", res.Notices, want)
	}
}

func TestALookupWhereOverAMixedSumIsNamedOnGroupRows(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":4,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]},{"id":"region","path":[{"field":"region"}]},
		{"id":"fed","formula":` + unknownWhere + `}],"groups":[{"id":"r","col":"region"}],"sort":[{"col":"name"}]}`
	requireWhereNamed(t, z, spec, "columns.2.formula.where")
}

func TestALookupWhereOverAMixedSumIsNamedInTheFilter(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":4,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]}],
		"filter":{"op":"gte","args":[` + unknownWhere + `,{"value":0}]},"sort":[{"col":"name"}]}`
	requireWhereNamed(t, z, spec, "filter.args.0.where")
}

func TestALookupWhereOverAMixedSumIsNamedInAHaving(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":4,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]},{"id":"region","path":[{"field":"region"}]}],
		"groups":[{"id":"r","col":"region","having":{"op":"gte","args":[` + unknownWhere + `,{"value":0}]}}],"sort":[{"col":"name"}]}`
	requireWhereNamed(t, z, spec, "groups.0.having.args.0.where")
}

func TestALookupWhereOverAMixedSumIsNamedOnAComputedRow(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":4,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]},{"id":"region","path":[{"field":"region"}]},
		{"id":"n","formula":{"value":1},"total":"rows"}],"groups":[{"id":"r","col":"region"}],
		"layout":[{"section":"north","title":"North","key":"north"},{"row":"fed","label":"Fed","formula":` + unknownWhere + `}],"sort":[{"col":"name"}]}`
	requireWhereNamed(t, z, spec, "layout.1.formula.where")
}

// A lookup whose where reads the report's row has no value on a group row,
// so its where is probed on the detail rows only: a collapsed report shows
// none and names nothing; opened, its detail rows name the where.
func TestALookupWhereThatReadsTheRowIsNamedOnDetailRowsOnly(t *testing.T) {
	z := newZoo(t)
	where := `{"op":"and","args":[{"op":"gt","args":[` + mixedFees + `,{"value":0}]},{"op":"ne","args":[{"path":[{"link":"site"}]},{"outer":[{"field":"id"}]}]}]}`
	spec := `{"version":4,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]},{"id":"region","path":[{"field":"region"}]},
		{"id":"fed","formula":{"op":"lookup","source":"zoo.animals","pick":"count","value":{"path":[{"field":"id"}]},"where":` + where + `}}],
		"groups":[{"id":"r","col":"region"}],"sort":[{"col":"name"}]}`
	if res := z.run(t, "bob", spec, Options{}, nil); len(res.Notices) != 0 {
		t.Fatalf("collapsed: notices = %+v, want none", res.Notices)
	}
	res := z.run(t, "bob", spec, Options{ExpandAll: true}, nil)
	if len(res.Notices) != 1 || !slices.Equal(res.Notices[0].Conditions, []string{"columns.2.formula.where"}) {
		t.Fatalf("expanded: notices = %+v", res.Notices)
	}
}
