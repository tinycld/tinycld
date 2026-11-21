package reports

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
)

// kindIn and stateIn are the filters of these tests: a Select path in a
// literal list (list is the JSON of the list).
func kindIn(list string) string {
	return inOf(`{"path":[{"link":"stall"},{"field":"kind"}]}`, `{"value":`+list+`}`)
}

func stateIn(list string) string {
	return inOf(`{"path":[{"field":"state"}]}`, `{"value":`+list+`}`)
}

func inOf(path, list string) string {
	return `{"op":"in","args":[` + path + `,` + list + `]}`
}

// salesOfKinds is salesNames under "the stall's kind is one of the kinds
// picked", a multiple choice param.
const salesOfKinds = `{"version":4,"source":"zoo.sales",
	"params":[{"id":"kinds","type":"choice","multiple":true,"options":[{"value":"food","label":"Food"},{"value":"gift","label":"Gift"}]}],
	"columns":[{"id":"name","path":[{"field":"name"}]},{"id":"amount","path":[{"field":"amount"}],"aggregate":"sum"}],
	"filter":{"op":"in","args":[{"path":[{"link":"stall"},{"field":"kind"}]},{"param":"kinds"}]},
	"sort":[{"col":"name"}]}`

// salesReads runs raw as ann and returns the statements of the run that read
// zoo_sales, failing when there is none.
func salesReads(t *testing.T, z *zoo, raw string, params map[string]string) []readStatement {
	t.Helper()
	seen := observeReads(t)
	z.run(t, "ann", raw, Options{}, params)
	observeRead = nil
	var out []readStatement
	for _, st := range *seen {
		if strings.Contains(st.sql, "zoo_sales") {
			out = append(out, st)
		}
	}
	if len(out) == 0 {
		t.Fatalf("no statement of the run reads zoo_sales (%d reads)", len(*seen))
	}
	return out
}

// storedIn is the stored-column term selectIn writes: a column in a list of
// placeholders that ends with NULL. Its first group is the placeholders.
var storedIn = regexp.MustCompile("\\[\\[\\w+\\.\\w+\\]\\] IN \\(((?:\\{:rp\\d+\\}, )+)NULL\\)")

// storedInTerms is how many stored-column terms the statements hold.
func storedInTerms(seen []readStatement) int {
	n := 0
	for _, st := range seen {
		n += len(storedIn.FindAllString(st.sql, -1))
	}
	return n
}

// An in of a Select field with a list of its option values is served by a
// plain index on the field's column, as eq is (selectEq): the planner
// searches the stalls by kind, then the sales by the stall, with a plain
// join; or it searches the sales by their state. It never scans the sales.
// Without the stored-column term (legacySQL.noSelectIn) the same report
// scans the sales and left-joins the stalls.
func TestASelectInAListIsServedByTheColumnIndex(t *testing.T) {
	byKind := []planSearch{{"zoo_stalls", "idx_zoo_stalls_kind", "kind=?"}, {"zoo_sales", "idx_zoo_sales_stall", "stall=?"}}
	byState := []planSearch{{"zoo_sales", "idx_zoo_sales_state", "state=?"}}
	cases := []struct {
		name   string
		raw    string
		params map[string]string
		want   []planSearch
	}{
		{"a linked select in a list", salesAmounts(kindIn(`["food","gift"]`)), nil, byKind},
		{"a linked select in a list of one", salesAmounts(kindIn(`["food"]`)), nil, byKind},
		{"a linked select in a param's list", salesOfKinds, map[string]string{"kinds": `["food","gift"]`}, byKind},
		{"a select of the report's own row", salesAmounts(stateIn(`["open","paid"]`)), nil, byState},
	}
	z := newZoo(t)
	withStalls(t, z, true)
	scans := func(steps []string) (scan, leftJoin string) {
		for _, step := range steps {
			if strings.HasPrefix(step, "SCAN zoo_sales") {
				scan = step
			}
			if strings.HasPrefix(step, "SEARCH zoo_stalls") && strings.Contains(step, "LEFT-JOIN") {
				leftJoin = step
			}
		}
		return scan, leftJoin
	}
	for _, stats := range []string{"no statistics", "after ANALYZE"} {
		if stats == "after ANALYZE" {
			analyze(t, z)
		}
		for _, c := range cases {
			t.Run(stats+"/"+c.name, func(t *testing.T) {
				for _, st := range salesReads(t, z, c.raw, c.params) {
					steps := stmtPlan(t, z, st)
					scan, leftJoin := scans(steps)
					if scan != "" {
						t.Errorf("the plan scans the sales (%s):\n%s\n%s", scan, strings.Join(steps, "\n"), st.sql)
					}
					if leftJoin != "" {
						t.Errorf("the plan left-joins the stalls (%s):\n%s\n%s", leftJoin, strings.Join(steps, "\n"), st.sql)
					}
					for _, s := range c.want {
						if !s.in(steps) {
							t.Errorf("no step SEARCH %s USING … %s (%s):\n%s\n%s", s.table, s.index, s.cond, strings.Join(steps, "\n"), st.sql)
						}
					}
				}
			})
		}
		t.Run(stats+"/without the stored-column term the sales are scanned", func(t *testing.T) {
			t.Cleanup(func() { legacySQL.noSelectIn = false })
			legacySQL.noSelectIn = true
			for _, st := range salesReads(t, z, salesAmounts(kindIn(`["food","gift"]`)), nil) {
				steps := stmtPlan(t, z, st)
				scan, leftJoin := scans(steps)
				if scan == "" || leftJoin == "" {
					t.Errorf("scan %q, left join %q: the old form no longer scans, so this test proves nothing:\n%s", scan, leftJoin, strings.Join(steps, "\n"))
				}
			}
		})
	}
}

// The stored-column term changes no result. A sale with no stall (S4), with
// a stall of a blank kind (S3) and with a stall the reader cannot see (S5)
// is in no list and is "not in" every list under not; its condition stays
// unknown, so eq with false does not hold for it and a column shows it
// blank. Each case also gives the result of the old form, byte for byte.
func TestASelectInAListKeepsItsResult(t *testing.T) {
	z := newZoo(t)
	withStalls(t, z, false)
	food, both := kindIn(`["food"]`), kindIn(`["food","gift"]`)
	cases := []struct {
		name   string
		filter string
		want   []any
		// terms is how many stored-column terms the filter writes.
		terms int
	}{
		{"in", both, []any{"S1", "S2"}, 1},
		{"in a list of one", food, []any{"S1"}, 1},
		{"not in", notOf(both), []any{"S3", "S4", "S5"}, 1},
		{"not in a list of one", notOf(food), []any{"S2", "S3", "S4", "S5"}, 1},
		// eq writes IS: an unknown condition is not equal to false, so the
		// sales whose kind reads blank stay out.
		{"in is false", `{"op":"eq","args":[` + food + `,{"value":false}]}`, []any{"S2"}, 1},
		{"or of two lists", `{"op":"or","args":[` + food + `,` + kindIn(`["gift"]`) + `]}`, []any{"S1", "S2"}, 2},
		{"or with another condition", `{"op":"or","args":[` + food + `,` + stateIs(`"paid"`) + `]}`, []any{"S1", "S2", "S5"}, 1},
		{"or with a not", `{"op":"or","args":[` + notOf(both) + `,` + stateIs(`"open"`) + `]}`, []any{"S1", "S3", "S4", "S5"}, 1},
		{"and", `{"op":"and","args":[` + both + `,` + stateIn(`["paid"]`) + `]}`, []any{"S2"}, 2},
		{"the same value two times", kindIn(`["food","food"]`), []any{"S1"}, 1},
		{"an empty list", kindIn(`[]`), nil, 0},
		{"not an empty list", notOf(kindIn(`[]`)), []any{"S1", "S2", "S3", "S4", "S5"}, 0},
		{"a list with a text that is no option value", kindIn(`["food","fo'od"]`), []any{"S1"}, 0},
		{"a list with a blank text", kindIn(`["food",""]`), []any{"S1"}, 0},
		{"a list of numbers", kindIn(`[1,2]`), nil, 0},
		{"a list of a text and a number", kindIn(`["food",2]`), []any{"S1"}, 0},
		{"a select of the report's own row", stateIn(`["open"]`), []any{"S1", "S4"}, 1},
		{"not, on the report's own row", notOf(stateIn(`["open"]`)), []any{"S2", "S3", "S5"}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			spec := salesNames(c.filter, "")
			reads := salesReads(t, z, spec, nil)
			if got := detailCells(z.run(t, "ann", spec, Options{}, nil), "name"); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("sales %#v, want %#v", got, c.want)
			}
			if got := storedInTerms(reads[:1]); got != c.terms {
				t.Fatalf("%d stored-column terms in the first statement, want %d:\n%s", got, c.terms, reads[0].sql)
			}
			requireSameAsLegacy(t, z, "ann", spec, Options{}, nil, &legacySQL.noSelectIn)
		})
	}

	t.Run("a param's list", func(t *testing.T) {
		for _, c := range []struct {
			picked string
			want   []any
			terms  int
		}{
			{`["food","gift"]`, []any{"S1", "S2"}, 1},
			{`["gift"]`, []any{"S2"}, 1},
			// No pick: the condition matches every sale.
			{``, []any{"S1", "S2", "S3", "S4", "S5"}, 0},
		} {
			var params map[string]string
			if c.picked != "" {
				params = map[string]string{"kinds": c.picked}
			}
			reads := salesReads(t, z, salesOfKinds, params)
			if got := detailCells(z.run(t, "ann", salesOfKinds, Options{}, params), "name"); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("picked %s: sales %#v, want %#v", c.picked, got, c.want)
			}
			if got := storedInTerms(reads[:1]); got != c.terms {
				t.Fatalf("picked %s: %d stored-column terms, want %d:\n%s", c.picked, got, c.terms, reads[0].sql)
			}
			requireSameAsLegacy(t, z, "ann", salesOfKinds, Options{}, params, &legacySQL.noSelectIn)
		}
	})

	// A column's formula reads the kind from the report's rows, not from the
	// stored column, so it gets no term; its values are pinned all the same.
	t.Run("a column", func(t *testing.T) {
		spec := salesNames("", `,{"id":"isFood","formula":`+food+`},
			{"id":"flag","formula":{"op":"if","args":[`+food+`,{"value":1},{"value":0}]}}`)
		if n := storedInTerms(salesReads(t, z, spec, nil)); n != 0 {
			t.Fatalf("%d stored-column terms for a column's formula, want 0", n)
		}
		res := z.run(t, "ann", spec, Options{}, nil)
		if got, want := detailCells(res, "name"), []any{"S1", "S2", "S3", "S4", "S5"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("sales %#v, want %#v", got, want)
		}
		// S2's kind is gift (false); S3, S4 and S5 read a blank kind (unknown).
		if got, want := detailCells(res, "isFood"), []any{true, false, nil, nil, nil}; !reflect.DeepEqual(got, want) {
			t.Fatalf("isFood %#v, want %#v", got, want)
		}
		if got, want := detailCells(res, "flag"), []any{int64(1), int64(0), int64(0), int64(0), int64(0)}; !reflect.DeepEqual(got, want) {
			t.Fatalf("flag %#v, want %#v", got, want)
		}
		requireSameAsLegacy(t, z, "ann", spec, Options{}, nil, &legacySQL.noSelectIn)
	})

	t.Run("a lookup's where", func(t *testing.T) {
		spec := `{"version":4,"source":"zoo.stalls","columns":[{"id":"name","path":[{"field":"name"}]},
			{"id":"sales","formula":{"op":"lookup","source":"zoo.sales","where":` + both + `,"value":{"path":[{"field":"amount"}]},"pick":"sum"}}],
			"sort":[{"col":"name"}]}`
		if storedInTerms(salesReads(t, z, spec, nil)) == 0 {
			t.Fatal("the lookup's where holds no stored-column term")
		}
		got := detailCells(z.run(t, "ann", spec, Options{}, nil), "sales")
		if len(got) == 0 {
			t.Fatal("the report has no detail row")
		}
		for i, v := range got {
			if v != int64(3) {
				t.Fatalf("row %d: sales %#v, want 3 (S1 and S2; S5's stall is hidden)", i, v)
			}
		}
		requireSameAsLegacy(t, z, "ann", spec, Options{}, nil, &legacySQL.noSelectIn)
	})

	t.Run("both forms agree on every stored value", func(t *testing.T) {
		const read = `NULLIF({:c}, '') IN (SELECT +value FROM json_each({:list}))`
		for _, stored := range []any{nil, "", "food", "gift", "k2", "fo'od"} {
			var same int
			err := z.app.DB().NewQuery(`SELECT (` + read + ` AND {:c} IN ({:a}, {:b}, NULL)) IS (` + read + `)`).
				Bind(dbx.Params{"c": stored, "list": `["food","gift"]`, "a": "food", "b": "gift"}).Row(&same)
			if err != nil {
				t.Fatal(err)
			}
			if same != 1 {
				t.Fatalf("stored %#v: the two forms differ", stored)
			}
		}
	})
}

// The stored-column term is written, and changes no result, on the other
// kinds of source: a Select whose column has another name than its key, a
// Select of a view source, and a Select of a union source. Each case also
// gives the result of the old form, byte for byte.
func TestASelectInAListOnOtherSourcesKeepsItsResult(t *testing.T) {
	in := func(field, list string) string {
		return inOf(`{"path":[{"field":"`+field+`"}]}`, `{"value":`+list+`}`)
	}
	report := func(source, field, filter string) string {
		return `{"version":4,"source":"` + source + `","columns":[{"id":"c","path":[{"field":"` + field + `"}]}],
			"filter":` + filter + `,"sort":[{"col":"c"}]}`
	}
	check := func(t *testing.T, z *zoo, spec string, want []any) {
		t.Helper()
		seen := observeReads(t)
		res := z.run(t, "ann", spec, Options{}, nil)
		observeRead = nil
		if storedInTerms(*seen) == 0 {
			t.Fatalf("no statement holds a stored-column term (%d reads)", len(*seen))
		}
		if got := detailCells(res, "c"); !reflect.DeepEqual(got, want) {
			t.Fatalf("rows %#v, want %#v", got, want)
		}
		requireSameAsLegacy(t, z, "ann", spec, Options{}, nil, &legacySQL.noSelectIn)
	}
	t.Run("a select whose column has another name", func(t *testing.T) {
		z := newZoo(t)
		addZooNotes()
		check(t, z, report("zoo.notes", "kind", in("kind", `["f1","f2"]`)), []any{"f1", "f2"})
		check(t, z, report("zoo.notes", "kind", notOf(in("kind", `["f1"]`))), []any{"f2", "f3", "f4"})
	})
	t.Run("a select of a view source", func(t *testing.T) {
		z := newZoo(t)
		addZooFeedLog(t, z)
		check(t, z, report("zoo.feed_log", "note", in("size", `["big"]`)), []any{"f1", "f2"})
		check(t, z, report("zoo.feed_log", "note", notOf(in("size", `["big"]`))), []any{"f3", "f4", "f5", "f6"})
	})
	t.Run("a select of a union source", func(t *testing.T) {
		z := newUnionZoo(t)
		check(t, z, report("zoo.meals", "kind", in("kind", `["feeding"]`)), []any{"feeding", "feeding", "feeding", "feeding"})
		check(t, z, report("zoo.meals", "kind", notOf(in("kind", `["feeding"]`))), []any{"treat", "treat"})
	})
}

// The stored-column term binds one placeholder for each distinct value of
// the list, and no statement of a run holds a value as SQL text, in any
// quoting. A list that names one value 40,000 times binds it once: a
// placeholder for each item would pass SQLite's 32,766 variables.
func TestASelectInAListIsBound(t *testing.T) {
	z := newZoo(t)
	withStalls(t, z, false)
	many := `["gift"` + strings.Repeat(`,"food"`, 40000) + `]`
	cases := []struct {
		name, raw string
		params    map[string]string
		table     string
		values    []string
	}{
		{"a linked select in a list", salesAmounts(kindIn(`["food","gift"]`)), nil, "zoo_stalls", []string{"food", "gift"}},
		{"a linked select in a param's list", salesOfKinds, map[string]string{"kinds": `["gift","food"]`}, "zoo_stalls", []string{"gift", "food"}},
		{"a select of the report's own row", salesAmounts(stateIn(`["open","paid","s2"]`)), nil, "zoo_sales", []string{"open", "paid", "s2"}},
		{"one value 40,000 times", salesAmounts(kindIn(many)), nil, "zoo_stalls", []string{"gift", "food"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reads := salesReads(t, z, c.raw, c.params)
			terms := 0
			for _, st := range reads {
				for _, v := range c.values {
					for _, quoted := range []string{"'" + v + "'", `"` + v + `"`, "`" + v + "`", "[" + v + "]"} {
						if strings.Contains(st.sql, quoted) {
							t.Errorf("a statement holds %s as SQL text:\n%.2000s", quoted, st.sql)
						}
					}
				}
				for _, m := range storedIn.FindAllStringSubmatch(st.sql, -1) {
					terms++
					var bound []string
					for _, mark := range strings.Split(strings.TrimSuffix(m[1], ", "), ", ") {
						v, ok := st.params[strings.Trim(mark, "{:}")].(string)
						if !ok {
							t.Fatalf("the placeholder %s holds %#v, want a text", mark, st.params[strings.Trim(mark, "{:}")])
						}
						bound = append(bound, v)
					}
					if !reflect.DeepEqual(bound, c.values) {
						t.Errorf("the stored-column term binds %q, want %q", bound, c.values)
					}
				}
				if !strings.Contains(st.sql, c.table) {
					t.Errorf("a statement does not name %s:\n%.2000s", c.table, st.sql)
				}
			}
			if terms == 0 {
				t.Fatalf("no statement holds a stored-column term:\n%.2000s", reads[0].sql)
			}
		})
	}
}

// The values of the stored-column term are bound, and SQLite may plan an
// indexed IN by its bound values only when sqlite_stat4 has rows: without
// statistics a statement with the term is prepared once, as every other
// statement of the engine is (TestEveryReportStatementIsPreparedOnce).
func TestASelectInAListIsPreparedOnceWithoutStatistics(t *testing.T) {
	z := newZoo(t)
	withStalls(t, z, true)
	db := readPool(t, z)
	for _, st := range salesReads(t, z, salesAmounts(kindIn(`["food","gift"]`)), nil) {
		raw, args := rawStatement(t, st.sql, st.params)
		if n := reprepares(t, db, raw, args); n != 0 {
			t.Errorf("%d reprepares, want 0:\n%.300s", n, st.sql)
		}
	}
}
