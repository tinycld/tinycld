package reports

import (
	"strings"
	"testing"
)

// A lookup on a group or total row is computed once per row: its cell and
// the cell's vmx helper both read the sum lookup's mixed check, which the
// group statement computes once on each level that shows the column (the
// total row and the group rows).
func TestALookupOnAGroupRowIsComputedOnce(t *testing.T) {
	z := newZoo(t)
	seen := observeReads(t)
	spec := `{"version":4,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]},
		{"id":"usdFees","formula":` + mixedFees + `}],"groups":[{"id":"g","col":"name"}],"sort":[{"col":"name"}]}`
	requireMixedFeesBlank(t, z, "group", spec)
	groups := 0
	for _, st := range *seen {
		if !strings.Contains(st.sql, "__level") {
			continue
		}
		groups++
		if n := strings.Count(st.sql, "count(DISTINCT"); n != 2 {
			t.Fatalf("the group statement computes the mixed check %d times, want once on each of its 2 levels:\n%s", n, st.sql)
		}
	}
	if groups == 0 {
		t.Fatal("no group statement was read")
	}
}

// A having over the sums of the level below reads rt<L>, the cells of the
// rows that pass their own having; a lookup there is computed once per row
// too. The column reads an aggregate, so its innermost group row computes
// the formula from the group's rows, lookups and all: each of its two
// lookups (one per branch) has its mixed check once.
func TestALookupThatAHavingAddsUpIsComputedOncePerRow(t *testing.T) {
	z := newZoo(t)
	seen := observeReads(t)
	formula := `{"op":"if","args":[{"op":"gt","args":[{"op":"count","args":[{"path":[{"field":"name"}]}]},{"value":0}]},` + mixedFees + `,` + mixedFees + `]}`
	spec := `{"version":4,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]},{"id":"region","path":[{"field":"region"}]},
		{"id":"usdFees","total":"rows","formula":` + formula + `}],
		"groups":[{"id":"r","col":"region","having":{"op":"isEmpty","args":[{"col":"usdFees"}]}},{"id":"g","col":"name"}],"sort":[{"col":"name"}]}`
	z.run(t, "bob", spec, Options{ExpandAll: true}, nil)
	read := false
	for _, st := range *seen {
		for _, cte := range strings.Split(st.sql, "\nrt") {
			if !strings.HasPrefix(cte, "2 AS (") {
				continue
			}
			read = true
			body, _, _ := strings.Cut(cte, "),\n")
			if n := strings.Count(body, "count(DISTINCT"); n != 2 {
				t.Fatalf("rt2 computes the mixed checks of its 2 lookups %d times:\n%s", n, body)
			}
		}
	}
	if !read {
		t.Fatal("no statement holds rt2")
	}
}
