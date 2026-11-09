package reports

import (
	"regexp"
	"strings"
	"testing"
)

// fillCTE is a fill keyset in a query plan, as SQLite names it.
var fillCTE = regexp.MustCompile(`^(?:MATERIALIZE|CO-ROUTINE) f\d+$`)

// parentLoop is a loop over the parent rows p, or over the level above's
// keyset when SQLite flattens p into the join.
var parentLoop = regexp.MustCompile(`: (?:SCAN|SEARCH) (?:p|f\d+)(?: |$)`)

type planRow struct {
	id, parent int
	detail     string
}

// fillOuterLoops reads the query plan of each statement and returns, for each
// fill keyset in it, the first table its second select loops over: the
// select that joins the parent rows p with the target's records.
func fillOuterLoops(t *testing.T, z *zoo, seen []readStatement) []string {
	t.Helper()
	var outer []string
	for _, st := range seen {
		rows, err := z.app.DB().NewQuery("EXPLAIN QUERY PLAN " + st.sql).Bind(st.params).Rows()
		if err != nil {
			t.Fatal(err)
		}
		var plan []planRow
		for rows.Next() {
			var r planRow
			var unused int
			if err := rows.Scan(&r.id, &r.parent, &unused, &r.detail); err != nil {
				t.Fatal(err)
			}
			plan = append(plan, r)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		children := func(id int) []planRow {
			var out []planRow
			for _, r := range plan {
				if r.parent == id {
					out = append(out, r)
				}
			}
			return out
		}
		for _, f := range plan {
			if !fillCTE.MatchString(f.detail) {
				continue
			}
			// The keyset is a compound select: MERGE (UNION) or COMPOUND QUERY,
			// whose last select is the one of the parent rows and records.
			var branches []planRow
			for _, r := range children(f.id) {
				if strings.HasPrefix(r.detail, "MERGE ") || r.detail == "COMPOUND QUERY" {
					branches = children(r.id)
					break
				}
			}
			if len(branches) < 2 {
				t.Fatalf("%s: plan %v, want a compound select", f.detail, children(f.id))
			}
			loop := ""
			for _, r := range children(branches[len(branches)-1].id) {
				if strings.HasPrefix(r.detail, "SCAN ") || strings.HasPrefix(r.detail, "SEARCH ") {
					loop = r.detail
					break
				}
			}
			outer = append(outer, f.detail+": "+loop)
		}
	}
	return outer
}

// A fill keyset loops over the parent rows p first and probes the target's
// records under each: p is few rows (the currencies, or the expanded rows of
// the level above), and the target's records are a table that can be searched
// by the fill's where. With the target outer, SQLite runs p again for each
// record, a cost of records times base rows. Checked on the plan, not a clock,
// over each shape a fill compiles to.
func TestAFillKeysetLoopsOverItsParentRowsFirst(t *testing.T) {
	cases := []struct {
		name    string
		keysets int
		run     func(t *testing.T, z *zoo)
	}{
		// 3: the page and the Total statement each read f1 (J1a: the Total has a
		// statement of its own).
		{"ledger, 200 accounts", 3, func(t *testing.T, z *zoo) {
			withBalanceZoo(t, z)
			withLedgerScale(t, z, 200)
			if _, err := runLedger(z, map[string]string{"period": ledgerPeriod}, Options{Today: "2025-07-15"}); err != nil {
				t.Fatal(err)
			}
		}},
		{"one level with a where", 1, func(t *testing.T, z *zoo) {
			withEmptyAnimals(t, z)
			z.run(t, "ann", animalFill(`{"where":{"op":"eq","args":[{"path":[{"field":"active"}]},{"value":true}]}}`, ""), Options{}, nil)
		}},
		{"two levels, the deeper where reads its parent", 2, func(t *testing.T, z *zoo) {
			withEmptyAnimals(t, z)
			z.run(t, "ann", sitesThenAnimals, Options{Expand: [][]any{{z.id("east"), "USD"}}}, nil)
		}},
		{"two levels, every parent open", 2, func(t *testing.T, z *zoo) {
			withEmptyAnimals(t, z)
			z.run(t, "ann", sitesThenAnimals, Options{ExpandAll: true}, nil)
		}},
		{"a fill under a group with no fill, its where reads links and the parent record", 1, func(t *testing.T, z *zoo) {
			withEmptyAnimals(t, z)
			z.run(t, "ann", `{"version":4,"source":"zoo.feedings","columns":[
				{"id":"site","path":[{"link":"animal"},{"link":"site"}]},
				{"id":"animal","path":[{"link":"animal"}]},
				{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
				"groups":[
					{"id":"s","col":"site"},
					{"id":"a","col":"animal","fill":{"where":{"op":"eq","args":[{"path":[{"link":"site"},{"field":"name"}]},{"record":"s","path":[{"field":"name"}]}]}}}],
				"sort":[{"formula":{"record":"s","path":[{"field":"name"}]}}]}`, Options{ExpandAll: true}, nil)
		}},
		// 4: the case runs two pages, and each page statement reads f1 and f2;
		// the Total statement (J1a: a statement of its own) reads no fill.
		{"a cursor page that starts inside fill groups", 4, func(t *testing.T, z *zoo) {
			withEmptyAnimals(t, z)
			// Page 1 is the East site's group row alone; page 2 starts at Eli,
			// a fill animal (no feedings) under East.
			opts := Options{ExpandAll: true, Limit: 1}
			first := z.run(t, "ann", sitesThenAnimals, opts, nil)
			if first.Page == nil || first.Page.Next == "" {
				t.Fatal("no second page")
			}
			opts.Cursor = first.Page.Next
			second := z.run(t, "ann", sitesThenAnimals, opts, nil)
			if r := second.Rows[0]; r.Kind != KindGroup || r.Depth != 1 || r.Label != "Eli" {
				t.Fatalf("page 2 starts at %s %q (depth %d), want the fill animal Eli", r.Kind, r.Label, r.Depth)
			}
		}},
		{"three levels of fill", 3, func(t *testing.T, z *zoo) {
			withEmptyAnimals(t, z)
			z.run(t, "ann", threeFills, Options{ExpandAll: true}, nil)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			z := newZoo(t)
			seen := observeReads(t)
			c.run(t, z)
			outer := fillOuterLoops(t, z, *seen)
			if len(outer) != c.keysets {
				t.Fatalf("fill keysets in the plans: %v, want %d", outer, c.keysets)
			}
			for _, o := range outer {
				if !parentLoop.MatchString(o) {
					t.Errorf("%s, want the parent rows p (or the keyset f<n> they are read from) as the outer loop", o)
				}
			}
		})
	}
}
