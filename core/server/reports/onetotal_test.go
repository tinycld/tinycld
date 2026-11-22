package reports

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// probedByAnimal groups ann's feedings by animal under a having and a
// filter, each an or whose second term reads a sum lookup: with Leo's EUR
// feeding f7 the lookup meets two currencies, so both conditions are probed
// (probeCells) and named in the run's mixedCurrency notice. The cost gives a
// Total row per currency.
const probedByAnimal = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"animal","path":[{"link":"animal"}]},
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
	{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
	"filter":{"op":"or","args":[{"op":"eq","args":[{"value":1},{"value":1}]},{"op":"gt","args":[` + allCosts + `,{"value":0}]}]},
	"groups":[{"id":"g","col":"animal","subtotals":true,"having":{"op":"or","args":[
		{"op":"gt","args":[{"op":"count","args":[{"path":[{"field":"id"}]}]},{"value":0}]},
		{"op":"ne","args":[{"op":"coalesce","args":[` + leoCosts + `,{"value":0}]},{"value":0}]}]}}],
	"sort":[{"col":"note"}]}`

// definesW matches a statement's CTE of the report's rows, w.
var definesW = regexp.MustCompile(`(?m)^w AS (MATERIALIZED )?\(`)

// readsLevelRows is a statement that reads the first level's rows beside
// their path (pagedGroupStatement's legs).
func readsLevelRows(sql string) bool { return strings.Contains(sql, "FROM o1 o JOIN pp1 pp ON") }

// A page of a grouped run, the first one and one read from a cursor, reads
// its level rows and the Total in one statement before the detail
// statement: w and the level CTEs are built once for both.
func TestAPagedRunReadsItsGroupsAndTotalInOneStatement(t *testing.T) {
	z := newZoo(t)
	withF7(t, z)
	// A page of 4: lion's group row, its opening, f1 and f2; then f7, lion's
	// subtotal, zebra's group row and its opening.
	first := z.run(t, "ann", pagedBySpecies, Options{Limit: 4}, nil)
	if first.Page == nil || first.Page.Next == "" {
		t.Fatalf("the first page has no next page: %+v", first.Page)
	}
	for name, opts := range map[string]Options{"the first page": {Limit: 4}, "a cursor page": {Limit: 4, Cursor: first.Page.Next}} {
		t.Run(name, func(t *testing.T) {
			seen := observeReads(t)
			z.run(t, "ann", pagedBySpecies, opts, nil)
			observeRead = nil
			groupReads, totals, ws := 0, 0, 0
			details := false
			for _, st := range *seen {
				switch {
				case isGroupedDetailStatement(st.sql):
					details = true
					continue
				case details:
					t.Fatalf("a statement after the detail statement:\n%s", st.sql)
				}
				if definesW.MatchString(st.sql) {
					ws++
				}
				if isTotalStatement(st.sql) {
					totals++
				}
				if readsLevelRows(st.sql) {
					groupReads++
					if !strings.Contains(st.sql, "FROM o0") {
						t.Errorf("the statement of the level rows does not read the Total (FROM o0):\n%s", st.sql)
					}
				}
			}
			if !details {
				t.Fatalf("no detail statement among the %d reads", len(*seen))
			}
			if groupReads != 1 || totals != 0 {
				t.Errorf("%d statements of level rows and %d Total statements of their own, want 1 and 0", groupReads, totals)
			}
			if ws != 1 {
				t.Errorf("w is built in %d statements before the detail statement, want 1", ws)
			}
		})
	}
}

// probedNotice is the mixedCurrency notice of every run of probedByAnimal:
// the cost's currencies, and both probed conditions.
const probedNotice = "Amounts in different currencies are totalled apart, one row per currency. " +
	"The filter reads a looked-up total that mixes currencies, so that total counts as unknown there. " +
	`The having of the "animal" group reads a looked-up total that mixes currencies, so that total counts as unknown there.`

// keyed writes s with each fixture record's id as its fixture key.
func keyed(z *zoo, s string) string {
	pairs := make([]string, 0, 2*len(z.recs))
	for key, r := range z.recs {
		pairs = append(pairs, r.Id, key)
	}
	return strings.NewReplacer(pairs...).Replace(s)
}

// pageLines is a page as "<kind> <id> <label> cost=<cost> <currency>
// grams=<grams> note=<note>" lines, then "size=<n>" and "next=<the cursor's
// JSON>" (record ids as their fixture keys).
func pageLines(t *testing.T, z *zoo, res *Result) []string {
	t.Helper()
	var out []string
	for _, r := range res.Rows {
		c := r.Cells
		out = append(out, keyed(z, fmt.Sprintf("%s %s %s cost=%v %v grams=%v note=%v", r.Kind, r.ID, r.Label, c["cost"], c["cost@currency"], c["grams"], c["note"])))
	}
	if res.Page == nil {
		return append(out, "no page")
	}
	next := ""
	if res.Page.Next != "" {
		raw, err := base64.RawURLEncoding.DecodeString(res.Page.Next)
		if err != nil {
			t.Fatalf("cursor %q: %v", res.Page.Next, err)
		}
		next = keyed(z, string(raw))
	}
	return append(out, fmt.Sprintf("size=%d", res.Page.Size), "next="+next)
}

// The Total rows (one per currency), their cells, the condition probes'
// notice and each page's cursor are what a paged run gives: closed, and
// every group open in pages of 3 read from each cursor.
func TestAPagedRunKeepsItsTotalAndProbes(t *testing.T) {
	z := newZoo(t)
	withEuroFeeding(t, z)
	const (
		totalEUR = "total total:[\"EUR\"] Total cost=400 EUR grams=10 note=<nil>"
		totalUSD = "total total:[\"USD\"] Total cost=4500 USD grams=1700 note=<nil>"
		fp       = `{"k":"1415ac6cf623",`
	)
	closed := [][]string{{
		`group g:["leo","EUR"] Leo cost=400 EUR grams=10 note=<nil>`,
		totalEUR,
		`group g:["leo","USD"] Leo cost=3500 USD grams=1300 note=<nil>`,
		`group g:["zed","USD"] Zed cost=1000 USD grams=400 note=<nil>`,
		totalUSD,
		"no page",
	}}
	open := [][]string{
		{
			`group g:["leo","EUR"] Leo cost=400 EUR grams=10 note=<nil>`,
			"detail d:f7 Leo cost=400 EUR grams=10 note=f7",
			`subtotal s:["leo","EUR"] Total Leo cost=400 EUR grams=10 note=<nil>`,
			totalEUR, totalUSD,
			"size=1", `next=` + fp + `"g":["leo","EUR"],"p":[[0,"Leo",0,"leo"]],"m":"after"}`,
		},
		{
			totalEUR,
			`group g:["leo","USD"] Leo cost=3500 USD grams=1300 note=<nil>`,
			"detail d:f1 Leo cost=1000 USD grams=500 note=f1",
			"detail d:f2 Leo cost=2500 USD grams=800 note=f2",
			totalUSD,
			"size=2", `next=` + fp + `"g":["leo","USD"],"p":[[0,"Leo",0,"leo"]],"m":"in","v":["f2","f2"]}`,
		},
		{
			`subtotal s:["leo","USD"] Total Leo cost=3500 USD grams=1300 note=<nil>`,
			`group g:["zed","USD"] Zed cost=1000 USD grams=400 note=<nil>`,
			"detail d:f3 Zed cost=700 USD grams=300 note=f3",
			totalUSD,
			"size=1", `next=` + fp + `"g":["zed","USD"],"p":[[0,"Zed",0,"zed"]],"m":"in","v":["f3","f3"]}`,
		},
		{
			"detail d:f4 Zed cost=300 USD grams=100 note=f4",
			`subtotal s:["zed","USD"] Total Zed cost=1000 USD grams=400 note=<nil>`,
			totalUSD,
			"size=1", "next=",
		},
	}
	for _, c := range []struct {
		name string
		opts Options
		want [][]string
	}{
		{"closed", Options{Limit: 3}, closed},
		{"every group open, in pages", Options{Limit: 3, ExpandAll: true}, open},
	} {
		t.Run(c.name, func(t *testing.T) {
			pages := groupedPages(t, z, "ann", probedByAnimal, c.opts)
			if len(pages) != len(c.want) {
				t.Fatalf("%d pages, want %d", len(pages), len(c.want))
			}
			for i, p := range pages {
				if got := pageLines(t, z, p); !slices.Equal(got, c.want[i]) {
					t.Errorf("page %d:\n%s\nwant\n%s", i+1, strings.Join(got, "\n"), strings.Join(c.want[i], "\n"))
				}
				if len(p.Notices) != 1 {
					t.Fatalf("page %d notices %+v, want the one mixedCurrency notice", i+1, p.Notices)
				}
				n := p.Notices[0]
				if n.Code != "mixedCurrency" || n.Message != probedNotice || !slices.Equal(n.Conditions, []string{"filter", "groups.0.having"}) || !slices.Equal(n.Columns, []string{"cost"}) {
					t.Errorf("page %d notice %+v", i+1, n)
				}
			}
		})
	}
}
