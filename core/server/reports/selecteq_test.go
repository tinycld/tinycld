package reports

import (
	"reflect"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// withStalls adds two collections and two sources of the zoo: stalls, of a
// Select kind, and the sales made at them, of a Select state. The rule hides
// the stall "Staff" from every reader. With bulk, 100 more stalls (ten of
// each kind) and 5,000 more sales (50 on each of those stalls, their states
// spread evenly) make the tables large enough that a scan and a search plan
// apart.
func withStalls(t *testing.T, z *zoo, bulk bool) {
	t.Helper()
	stalls := core.NewBaseCollection("zoo_stalls")
	stalls.ListRule = rule(`name != "Staff"`)
	stalls.ViewRule = stalls.ListRule
	stalls.Fields.Add(
		&core.TextField{Name: "name"},
		&core.SelectField{Name: "kind", Values: []string{"food", "gift", "k2", "k3", "k4", "k5", "k6", "k7", "k8", "k9"}, MaxSelect: 1},
	)
	stalls.AddIndex("idx_zoo_stalls_kind", false, "kind", "")
	z.save(t, stalls)

	sales := core.NewBaseCollection("zoo_sales")
	sales.ListRule = rule(`@request.auth.id != ""`)
	sales.ViewRule = sales.ListRule
	sales.Fields.Add(
		&core.TextField{Name: "name"},
		&core.RelationField{Name: "stall", CollectionId: stalls.Id, MaxSelect: 1},
		&core.NumberField{Name: "amount"},
		&core.SelectField{Name: "state", Values: []string{"open", "paid", "s2", "s3", "s4", "s5", "s6", "s7", "s8", "s9"}, MaxSelect: 1},
	)
	sales.AddIndex("idx_zoo_sales_stall", false, "stall", "")
	sales.AddIndex("idx_zoo_sales_state", false, "state", "")
	z.save(t, sales)

	food := z.rec(t, "food", "zoo_stalls", map[string]any{"name": "Food", "kind": "food"})
	gifts := z.rec(t, "gifts", "zoo_stalls", map[string]any{"name": "Gifts", "kind": "gift"})
	blank := z.rec(t, "blank", "zoo_stalls", map[string]any{"name": "Blank"})
	staff := z.rec(t, "staff", "zoo_stalls", map[string]any{"name": "Staff", "kind": "food"})
	z.rec(t, "S1", "zoo_sales", map[string]any{"name": "S1", "stall": food.Id, "state": "open", "amount": 1})
	z.rec(t, "S2", "zoo_sales", map[string]any{"name": "S2", "stall": gifts.Id, "state": "paid", "amount": 2})
	z.rec(t, "S3", "zoo_sales", map[string]any{"name": "S3", "stall": blank.Id, "amount": 4})
	z.rec(t, "S4", "zoo_sales", map[string]any{"name": "S4", "state": "open", "amount": 8})
	z.rec(t, "S5", "zoo_sales", map[string]any{"name": "S5", "stall": staff.Id, "state": "paid", "amount": 16})

	RegisterSource(Source{
		Key: "zoo.stalls", Collection: "zoo_stalls", Label: "Stalls",
		Fields: []Field{
			{Key: "name", Label: "Name", Type: Text, Aggregates: MinMax},
			{Key: "kind", Label: "Kind", Type: Select, Options: []Option{
				{Value: "food", Label: "Food"}, {Value: "gift", Label: "Gift"},
				{Value: "k2", Label: "K2"}, {Value: "k3", Label: "K3"}, {Value: "k4", Label: "K4"}, {Value: "k5", Label: "K5"},
				{Value: "k6", Label: "K6"}, {Value: "k7", Label: "K7"}, {Value: "k8", Label: "K8"}, {Value: "k9", Label: "K9"},
			}},
		},
	})
	RegisterSource(Source{
		Key: "zoo.sales", Collection: "zoo_sales", Label: "Sales",
		Fields: []Field{
			{Key: "name", Label: "Name", Type: Text, Aggregates: MinMax},
			{Key: "amount", Label: "Amount", Type: Number, Aggregates: SumAll},
			{Key: "state", Label: "State", Type: Select, Options: []Option{
				{Value: "open", Label: "Open"}, {Value: "paid", Label: "Paid"},
				{Value: "s2", Label: "S2"}, {Value: "s3", Label: "S3"}, {Value: "s4", Label: "S4"}, {Value: "s5", Label: "S5"},
				{Value: "s6", Label: "S6"}, {Value: "s7", Label: "S7"}, {Value: "s8", Label: "S8"}, {Value: "s9", Label: "S9"},
			}},
		},
		Links: []Link{{Key: "stall", Label: "Stall", Target: "zoo.stalls", Cardinality: ToOne, Field: "stall"}},
	})

	if !bulk {
		return
	}
	// Stall i has kind i % 10 (0 food, 1 gift, then k2 to k9); sale j is on
	// stall (j - 1) / 50 + 1 and has state j % 10 (0 open, 1 paid, then s2
	// to s9). One statement each: 5,100 saves would take minutes.
	if _, err := z.app.DB().NewQuery(`WITH RECURSIVE seq(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM seq WHERE i < 100)
		INSERT INTO zoo_stalls (id, name, kind)
		SELECT printf('stall%010d', i), printf('Stall %03d', i),
			CASE i % 10 WHEN 0 THEN 'food' WHEN 1 THEN 'gift' ELSE 'k' || (i % 10) END FROM seq`).Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := z.app.DB().NewQuery(`WITH RECURSIVE seq(j) AS (SELECT 1 UNION ALL SELECT j + 1 FROM seq WHERE j < 5000)
		INSERT INTO zoo_sales (id, name, stall, amount, state)
		SELECT printf('sale%011d', j), printf('B%05d', j), printf('stall%010d', (j - 1) / 50 + 1), 1,
			CASE j % 10 WHEN 0 THEN 'open' WHEN 1 THEN 'paid' ELSE 's' || (j % 10) END FROM seq`).Execute(); err != nil {
		t.Fatal(err)
	}
}

// kindIs and stateIs are the filters of these tests: a Select path equal to
// a text (or to null), with the text on the left when textFirst.
func kindIs(value string, textFirst bool) string {
	return eqOf(`{"path":[{"link":"stall"},{"field":"kind"}]}`, value, textFirst)
}

func stateIs(value string) string {
	return eqOf(`{"path":[{"field":"state"}]}`, value, false)
}

func eqOf(path, value string, textFirst bool) string {
	args := path + `,{"value":` + value + `}`
	if textFirst {
		args = `{"value":` + value + `},` + path
	}
	return `{"op":"eq","args":[` + args + `]}`
}

func notOf(cond string) string { return `{"op":"not","args":[` + cond + `]}` }

// salesAmounts is a report on zoo.sales of the one column amount (summed in
// its total row) and no group, under filter.
func salesAmounts(filter string) string {
	return `{"version":4,"source":"zoo.sales","columns":[{"id":"amount","path":[{"field":"amount"}],"aggregate":"sum"}],
		"filter":` + filter + `}`
}

// salesNames is a report on zoo.sales of the columns name and amount, sorted
// by name, under filter ("" for none) and with more columns.
func salesNames(filter, more string) string {
	f := ""
	if filter != "" {
		f = `,"filter":` + filter
	}
	return `{"version":4,"source":"zoo.sales","columns":[{"id":"name","path":[{"field":"name"}]},
		{"id":"amount","path":[{"field":"amount"}],"aggregate":"sum"}` + more + `]` + f + `,"sort":[{"col":"name"}]}`
}

// salesStatements runs raw as ann and returns the statements of the run
// that read zoo_sales, failing when there is none.
func salesStatements(t *testing.T, z *zoo, raw string) []readStatement {
	t.Helper()
	seen := observeReads(t)
	z.run(t, "ann", raw, Options{}, nil)
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

// stmtPlan is the detail text of each step of the query plan of st.
func stmtPlan(t *testing.T, z *zoo, st readStatement) []string {
	t.Helper()
	rows, err := z.app.DB().NewQuery("EXPLAIN QUERY PLAN " + st.sql).Bind(st.params).Rows()
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var steps []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		steps = append(steps, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return steps
}

// analyze gathers SQLite's statistics of the zoo's tables and requires that
// sqlite_stat4 has rows: a plan read after it is one SQLite chose with them.
func analyze(t *testing.T, z *zoo) {
	t.Helper()
	if _, err := z.app.DB().NewQuery("ANALYZE").Execute(); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := z.app.DB().NewQuery("SELECT count(*) FROM sqlite_stat4").Row(&n); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("sqlite_stat4 has no rows after ANALYZE")
	}
}

// planSearch is a plan step that searches table by an index named index with
// the condition cond.
type planSearch struct{ table, index, cond string }

func (s planSearch) in(steps []string) bool {
	for _, step := range steps {
		if strings.HasPrefix(step, "SEARCH "+s.table+" ") && strings.Contains(step, s.index) && strings.Contains(step, "("+s.cond+")") {
			return true
		}
	}
	return false
}

// An eq of a Select field with one of its option values is served by a
// plain index on the field's column: the planner searches the sales by the
// stall (after it searches the stalls by kind) or by their state, and never
// scans them. A Select reads as NULLIF of its column and the blank text,
// which no index serves.
func TestASelectEqualToATextIsServedByTheColumnIndex(t *testing.T) {
	byKind := []planSearch{{"zoo_stalls", "idx_zoo_stalls_kind", "kind=?"}, {"zoo_sales", "idx_zoo_sales_stall", "stall=?"}}
	byState := []planSearch{{"zoo_sales", "idx_zoo_sales_state", "state=?"}}
	cases := []struct {
		name   string
		filter string
		want   []planSearch
	}{
		{"a linked select equal to a text", kindIs(`"food"`, false), byKind},
		{"the text on the left", kindIs(`"food"`, true), byKind},
		{"a select of the report's own row", stateIs(`"open"`), byState},
	}
	z := newZoo(t)
	withStalls(t, z, true)
	for _, stats := range []string{"no statistics", "after ANALYZE"} {
		if stats == "after ANALYZE" {
			analyze(t, z)
		}
		for _, c := range cases {
			t.Run(stats+"/"+c.name, func(t *testing.T) {
				for _, st := range salesStatements(t, z, salesAmounts(c.filter)) {
					steps := stmtPlan(t, z, st)
					for _, step := range steps {
						if strings.HasPrefix(step, "SCAN zoo_sales") {
							t.Errorf("the plan scans the sales (%s):\n%s\n%s", step, strings.Join(steps, "\n"), st.sql)
						}
					}
					for _, s := range c.want {
						if !s.in(steps) {
							t.Errorf("no step SEARCH %s USING … %s (%s):\n%s\n%s", s.table, s.index, s.cond, strings.Join(steps, "\n"), st.sql)
						}
					}
				}
			})
		}
	}
}

// The stored-column test changes no result: a blank kind, no stall and a
// hidden stall read blank, are not equal to an option value, and are "not
// food" (not unknown) under not; a formula, a lookup's where and an eq the
// change does not touch keep their answers.
func TestASelectEqualToATextKeepsItsResult(t *testing.T) {
	z := newZoo(t)
	withStalls(t, z, false)
	names := func(t *testing.T, raw string) []any {
		t.Helper()
		return detailCells(z.run(t, "ann", raw, Options{}, nil), "name")
	}
	food := kindIs(`"food"`, false)
	cases := []struct {
		name   string
		filter string
		want   []any
	}{
		{"eq", food, []any{"S1"}},
		{"not eq", notOf(food), []any{"S2", "S3", "S4", "S5"}},
		// eq writes IS, so an unknown inner condition is not equal to false
		// (not reads unknown as false and cannot see the difference).
		{"eq false", `{"op":"eq","args":[` + food + `,{"value":false}]}`, []any{"S2", "S3", "S4", "S5"}},
		{"the text on the left", kindIs(`"food"`, true), []any{"S1"}},
		{"or", `{"op":"or","args":[` + food + `,` + kindIs(`"gift"`, false) + `]}`, []any{"S1", "S2"}},
		{"a blank text", kindIs(`""`, false), nil},
		{"null", kindIs(`null`, false), []any{"S3", "S4", "S5"}},
		{"a text that is no option value", kindIs(`"fo'od"`, false), nil},
		{"a select of the report's own row", stateIs(`"open"`), []any{"S1", "S4"}},
		{"not, on the report's own row", notOf(stateIs(`"open"`)), []any{"S2", "S3", "S5"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := names(t, salesNames(c.filter, "")); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("sales %#v, want %#v", got, c.want)
			}
		})
	}

	t.Run("a text that is no option value stays bound", func(t *testing.T) {
		seen := observeReads(t)
		z.run(t, "ann", salesNames(kindIs(`"fo'od"`, false), ""), Options{}, nil)
		observeRead = nil
		if len(*seen) == 0 {
			t.Fatal("no statement was read")
		}
		for _, st := range *seen {
			if strings.Contains(st.sql, "fo'od") || strings.Contains(st.sql, "fo''od") {
				t.Fatalf("a statement holds the text:\n%s", st.sql)
			}
		}
	})

	t.Run("a detail formula", func(t *testing.T) {
		res := z.run(t, "ann", salesNames("", `,{"id":"isFood","formula":{"op":"if","args":[`+food+`,{"value":1},{"value":0}]}}`), Options{}, nil)
		if got, want := detailCells(res, "name"), []any{"S1", "S2", "S3", "S4", "S5"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("sales %#v, want %#v", got, want)
		}
		if got, want := detailCells(res, "isFood"), []any{int64(1), int64(0), int64(0), int64(0), int64(0)}; !reflect.DeepEqual(got, want) {
			t.Fatalf("isFood %#v, want %#v", got, want)
		}
	})

	t.Run("a lookup's where", func(t *testing.T) {
		res := z.run(t, "ann", `{"version":4,"source":"zoo.stalls","columns":[{"id":"name","path":[{"field":"name"}]},
			{"id":"foodSales","formula":{"op":"lookup","source":"zoo.sales","where":`+food+`,"value":{"path":[{"field":"amount"}]},"pick":"sum"}}],
			"sort":[{"col":"name"}]}`, Options{}, nil)
		got := detailCells(res, "foodSales")
		if len(got) == 0 {
			t.Fatal("the report has no detail row")
		}
		for i, v := range got {
			if v != int64(1) {
				t.Fatalf("row %d: food sales %#v, want 1 (S1 only; S5's stall is hidden)", i, v)
			}
		}
	})

	t.Run("both forms agree on every stored value", func(t *testing.T) {
		for _, stored := range []any{nil, "", "food", "gift"} {
			var same int
			err := z.app.DB().NewQuery(`SELECT (NULLIF({:c}, '') IS 'food' AND {:c} = 'food') IS (NULLIF({:c}, '') IS 'food')`).
				Bind(dbx.Params{"c": stored}).Row(&same)
			if err != nil {
				t.Fatal(err)
			}
			if same != 1 {
				t.Fatalf("stored %#v: the two forms differ", stored)
			}
		}
	})
}

// The stored-column term binds its value, as the term it is added to does:
// no statement of a run holds the option value as SQL text, in any quoting.
func TestASelectEqualToATextIsBound(t *testing.T) {
	z := newZoo(t)
	withStalls(t, z, false)
	cases := []struct {
		name, filter, table, value string
	}{
		{"a linked select equal to a text", kindIs(`"food"`, false), "zoo_stalls", "food"},
		{"a select of the report's own row", stateIs(`"open"`), "zoo_sales", "open"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seen := observeReads(t)
			z.run(t, "ann", salesAmounts(c.filter), Options{}, nil)
			observeRead = nil
			named := false
			for _, st := range *seen {
				named = named || strings.Contains(st.sql, c.table)
				for _, quoted := range []string{"'" + c.value + "'", `"` + c.value + `"`, "`" + c.value + "`", "[" + c.value + "]"} {
					if strings.Contains(st.sql, quoted) {
						t.Errorf("a statement holds %s as SQL text:\n%s", quoted, st.sql)
					}
				}
			}
			if !named {
				t.Fatalf("no statement of the run names %s (%d reads)", c.table, len(*seen))
			}
		})
	}
}
