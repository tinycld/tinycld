package reports

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// The table the forms engine (TS and Go) runs too; see
// core/lib/forms/__tests__/formula-cases.test.ts and
// core/server/forms/formula_cases_test.go. A case with a "forms" formula is
// shared: both engines must give its expected value.
const formulaCasesPath = "../../lib/forms/fixtures/formula-cases.json"

type formulaCase struct {
	Name        string               `json:"name"`
	Types       map[string]FieldType `json:"types"`
	Values      map[string]any       `json:"values"`
	Rows        []map[string]any     `json:"rows"`
	LookupTypes map[string]FieldType `json:"lookupTypes"`
	LookupRows  []map[string]any     `json:"lookupRows"`
	FiscalStart int                  `json:"fiscalStart"`
	Formula     *Node                `json:"formula"`
	Forms       any                  `json:"forms"`
	FormsSkip   string               `json:"formsSkip"`
	FormsNote   string               `json:"formsNote"`
	Expect      any                  `json:"expect"`
	Error       string               `json:"error"`
}

func loadFormulaCases(t *testing.T) []formulaCase {
	t.Helper()
	raw, err := os.ReadFile(formulaCasesPath)
	if err != nil {
		t.Fatal(err)
	}
	var table struct {
		Cases []formulaCase `json:"cases"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&table); err != nil {
		t.Fatal(err)
	}
	return table.Cases
}

// tableCtx compiles a case's formula over a one-table query: {col: x} is
// column x of t, and aggregates run over all of t's rows, as a group row's
// aggregates run over the group's rows.
type tableCtx struct {
	types       map[string]FieldType
	params      dbx.Params
	fiscal      int
	lookupTypes map[string]FieldType
	lookupRows  []map[string]any
}

func (c *tableCtx) col(id, at string) (expr, error) {
	typ, ok := c.types[id]
	if !ok {
		return expr{}, invalid(at, "unknown column "+id)
	}
	return expr{sql: "t." + id, typ: typ}, nil
}

func (c *tableCtx) path(Path, string) (expr, error) {
	return expr{}, fmt.Errorf("no paths in the table")
}
func (c *tableCtx) param(string, string) (expr, error) {
	return expr{}, fmt.Errorf("no params in the table")
}
func (c *tableCtx) subtotal(string, string) (expr, error) {
	return expr{}, fmt.Errorf("no subtotals in the table")
}
func (c *tableCtx) window(string, *Node, int, string) (expr, error) {
	return expr{}, fmt.Errorf("no windows in the table")
}
func (c *tableCtx) exists(*Node, string) (expr, error) {
	return expr{}, fmt.Errorf("no links in the table")
}
func (c *tableCtx) filter() bool { return false }

func (c *tableCtx) aggregate(op Aggregate, arg *Node, scale int, at string) (expr, error) {
	e, err := compileFormula(c, arg, at+".args.0")
	if err != nil {
		return expr{}, err
	}
	if err := checkAggregate(op, e.typ, scale, at+".args.0"); err != nil {
		return expr{}, err
	}
	return expr{sql: aggregateSQL(op, e.sql, scale), typ: aggregateType(op, e.typ, scale)}, nil
}

func (c *tableCtx) bind(v any) string {
	name := fmt.Sprintf("p%d", len(c.params))
	c.params[name] = v
	return "{:" + name + "}"
}

func (c *tableCtx) fiscalStart() string {
	if c.fiscal == 0 {
		return c.bind(int64(1))
	}
	return c.bind(int64(c.fiscal))
}

// lookup reads the case's lookupRows as the looked-up source: a path is one
// of their columns, a column the table's own row.
func (c *tableCtx) lookup(n *Node, at string) (expr, error) {
	value, err := lookupValue(n, at)
	if err != nil {
		return expr{}, err
	}
	inner := &tableLookupCtx{outer: c}
	v, err := compileFormula(inner, value, at+".value")
	if err != nil {
		return expr{}, err
	}
	where := "1"
	if n.Where != nil {
		w, err := compileFormula(inner, n.Where, at+".where")
		if err != nil {
			return expr{}, err
		}
		where = "COALESCE(" + w.sql + ", 0)"
	}
	pick := "min"
	if n.Pick == "max" {
		pick = "max"
	}
	return expr{sql: fmt.Sprintf("(SELECT %s(%s) FROM (%s) l WHERE %s)", pick, v.sql, c.lookupTable(), where), typ: v.typ}, nil
}

func (c *tableCtx) lookupTable() string {
	cols := make([]string, 0, len(c.lookupTypes))
	for name := range c.lookupTypes {
		cols = append(cols, name)
	}
	sort.Strings(cols)
	if len(c.lookupRows) == 0 {
		parts := make([]string, len(cols))
		for i, name := range cols {
			parts[i] = "NULL AS " + name
		}
		return "SELECT " + strings.Join(parts, ", ") + " WHERE 0"
	}
	selects := make([]string, 0, len(c.lookupRows))
	for _, row := range c.lookupRows {
		parts := make([]string, len(cols))
		for i, name := range cols {
			parts[i] = c.bind(jsonValue(row[name])) + " AS " + name
		}
		selects = append(selects, "SELECT "+strings.Join(parts, ", "))
	}
	return strings.Join(selects, " UNION ALL ")
}

type tableLookupCtx struct {
	ctxBase
	outer *tableCtx
}

func (l *tableLookupCtx) col(id, at string) (expr, error) { return l.outer.col(id, at) }
func (l *tableLookupCtx) path(p Path, at string) (expr, error) {
	if len(p) != 1 || p[0].Field == "" {
		return expr{}, invalid(at, "a lookup row path is one field")
	}
	typ, ok := l.outer.lookupTypes[p[0].Field]
	if !ok {
		return expr{}, invalid(at, "unknown lookup column "+p[0].Field)
	}
	return expr{sql: "l." + p[0].Field, typ: typ}, nil
}
func (l *tableLookupCtx) param(string, string) (expr, error) { return expr{}, fmt.Errorf("no params") }
func (l *tableLookupCtx) aggregate(Aggregate, *Node, int, string) (expr, error) {
	return expr{}, fmt.Errorf("no totals in a lookup")
}
func (l *tableLookupCtx) window(string, *Node, int, string) (expr, error) {
	return expr{}, fmt.Errorf("no windows")
}
func (l *tableLookupCtx) lookup(n *Node, at string) (expr, error) { return l.outer.lookup(n, at) }
func (l *tableLookupCtx) bind(v any) string                       { return l.outer.bind(v) }
func (l *tableLookupCtx) fiscalStart() string                     { return l.outer.fiscalStart() }
func (l *tableLookupCtx) filter() bool                            { return false }

// jsonValue turns a decoded case value into what SQLite binds: a whole
// number binds as an integer.
func jsonValue(v any) any {
	if n, ok := v.(json.Number); ok {
		return numberValue(n)
	}
	return v
}

func runFormulaCase(t *testing.T, app core.App, c formulaCase) (any, error) {
	t.Helper()
	ctx := &tableCtx{types: c.Types, params: dbx.Params{}, fiscal: c.FiscalStart, lookupTypes: c.LookupTypes, lookupRows: c.LookupRows}
	e, err := compileFormula(ctx, c.Formula, "formula")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	cols := make([]string, 0, len(c.Types))
	for name := range c.Types {
		cols = append(cols, name)
	}
	sort.Strings(cols)
	rows := c.Rows
	if c.Values != nil {
		rows = []map[string]any{c.Values}
	}
	table := "WITH t(_) AS (VALUES (NULL)) "
	if len(cols) > 0 {
		tuples := make([]string, 0, len(rows))
		for i, row := range rows {
			slots := make([]string, len(cols))
			for j, name := range cols {
				key := fmt.Sprintf("r%d_%d", i, j)
				ctx.params[key] = jsonValue(row[name])
				slots[j] = "{:" + key + "}"
			}
			tuples = append(tuples, "("+strings.Join(slots, ", ")+")")
		}
		body := "VALUES " + strings.Join(tuples, ", ")
		if len(tuples) == 0 {
			nulls := strings.TrimSuffix(strings.Repeat("NULL, ", len(cols)), ", ")
			body = "SELECT " + nulls + " WHERE 0"
		}
		table = "WITH t(" + strings.Join(cols, ", ") + ") AS (" + body + ") "
	}
	var out struct {
		V any `db:"v"`
	}
	err = app.DB().NewQuery(table + "SELECT " + e.sql + " AS v FROM t").Bind(ctx.params).One(&out)
	return out.V, err
}

// sameValue compares a SQLite result with a JSON expectation: SQLite has no
// boolean (true is 1) and returns whole numbers as integers.
func sameValue(got, want any) bool {
	switch w := want.(type) {
	case nil:
		return got == nil
	case bool:
		g, ok := got.(int64)
		return ok && (g != 0) == w
	case json.Number:
		switch g := got.(type) {
		case int64:
			return string(w) == fmt.Sprint(g)
		case float64:
			f, err := w.Float64()
			return err == nil && f == g
		}
		return false
	case string:
		return got == w
	}
	return false
}

// onlyFormsOps is true when every operator is one the forms engine has. count is left out: the
// report's count(col) skips blanks and the forms count(rows) counts every row.
func onlyFormsOps(n *Node) bool {
	if n == nil {
		return true
	}
	if n.Op != "" {
		switch n.Op {
		case "add", "sub", "mul", "div", "round", "sum":
		case "min", "max":
			if len(n.Args) < 2 {
				return false
			}
		default:
			return false
		}
	}
	for _, a := range n.Args {
		if !onlyFormsOps(a) {
			return false
		}
	}
	return true
}

// wrapsRoundToWhole is true for a forms formula {"round": [x, 0]}.
func wrapsRoundToWhole(forms any) bool {
	m, ok := forms.(map[string]any)
	if !ok {
		return false
	}
	args, ok := m["round"].([]any)
	if !ok || len(args) != 2 {
		return false
	}
	n, ok := args[1].(json.Number)
	return ok && n.String() == "0"
}

// A case the forms engine can express must run there too, or say why not.
func TestFormulaCasesCarryFormsWhenTheFormsEngineHasTheOperators(t *testing.T) {
	withForms := 0
	for _, c := range loadFormulaCases(t) {
		if c.Forms != nil {
			withForms++
			if c.FormsSkip != "" {
				t.Errorf("%q has both forms and formsSkip", c.Name)
			}
			// The forms engine has no money type: a computed field rounds to
			// its decimals (default 0). A forms formula wrapped in round(…, 0)
			// for a formula that is not itself a round says so.
			if wrapsRoundToWhole(c.Forms) && c.Formula.Op != "round" && c.FormsNote == "" {
				t.Errorf("%q wraps forms in round(…, 0): add a formsNote", c.Name)
			}
		}
		if c.Error != "" || !onlyFormsOps(c.Formula) {
			continue
		}
		if c.Forms == nil && c.FormsSkip == "" {
			t.Errorf("%q uses only operators the forms engine has: add forms or a formsSkip reason", c.Name)
		}
	}
	// Keep equal to the count in the forms Go and TS tests.
	if withForms != 27 {
		t.Fatalf("%d cases carry forms, want 27", withForms)
	}
}

func TestFormulaCasesTheReportCompiler(t *testing.T) {
	app := newPlainApp(t)
	cases := loadFormulaCases(t)
	if len(cases) < 40 {
		t.Fatalf("only %d cases", len(cases))
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			got, err := runFormulaCase(t, app, c)
			if c.Error != "" {
				if err == nil || !strings.Contains(err.Error(), c.Error) {
					t.Fatalf("err = %v, want %q", err, c.Error)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !sameValue(got, c.Expect) {
				t.Fatalf("got %#v, want %v", got, c.Expect)
			}
		})
	}
}
