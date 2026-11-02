package reports

import (
	"fmt"
	"slices"
	"strings"
)

// level is one level of the group statement over w: level 0 is the total,
// level L the groups of the first L group columns. Each level groups by its
// keys and by the currency keys, so a total never adds two currencies.
//
//	a<L>  aggregates w by the level's keys
//	d<L>_<idx>  a to-many total, once per parent row (aggregate locality)
//	g<L>  a<L> with its to-many totals
//	o<L>  the level's cells, and its rows' order among siblings
type level struct {
	c        *compiler
	r        *rowsCTE
	n        int
	keys     []string // w columns the level groups by
	aggs     []string // a<L> values, "expr AS name"
	aggNames map[string]bool
	tms      []string // d<L> CTEs
	tmCols   []string // g<L> values taken from d<L>
	frame    *windowFrame
	visiting map[string]bool
	// reads memoizes readsRowValue per formula column.
	reads map[string]bool
}

// level prepares level n: its keys, its parent partition, and the order of
// its sibling rows, which running values read.
func (c *compiler) level(r *rowsCTE, n int) (*level, error) {
	lv := &level{c: c, r: r, n: n, aggNames: map[string]bool{}, visiting: map[string]bool{}, reads: map[string]bool{}}
	for i := range n {
		lv.keys = append(lv.keys, fmt.Sprintf("k%d", i))
	}
	lv.keys = append(lv.keys, c.currencies...)
	parent := slices.Clone(c.currencies)
	for i := range max(n-1, 0) {
		parent = append(parent, fmt.Sprintf("k%d", i))
	}
	lv.frame = &windowFrame{parent: prefixed("a.", parent), total: prefixed("a.", c.currencies)}
	order, err := lv.order()
	if err != nil {
		return nil, err
	}
	lv.frame.order = order
	return lv, nil
}

func prefixed(prefix string, names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = prefix + n
	}
	return out
}

// agg adds an aggregate over w to a<L> and returns its reference.
func (lv *level) agg(name, sql string) string {
	if !lv.aggNames[name] {
		lv.aggNames[name] = true
		lv.aggs = append(lv.aggs, sql+" AS "+name)
	}
	return "a." + name
}

// toManyTotal totals a to-many column once per parent row in the group:
// every base row that shares a parent carries the same per-parent total,
// so adding it up per base row would multiply it.
func (lv *level) toManyTotal(cp *colPlan) string {
	idx := lv.c.p.index(cp)
	name := fmt.Sprintf("x%d", idx)
	if lv.aggNames[name] {
		return "a." + name
	}
	lv.aggNames[name] = true
	cols := []string{fmt.Sprintf("p%d", idx), fmt.Sprintf("c%d", idx)}
	var total string
	switch cp.col.Aggregate {
	case Min, Max:
		total = fmt.Sprintf("%s(c%d)", cp.col.Aggregate, idx)
	case Avg:
		cols = append(cols, fmt.Sprintf("cs%d", idx), fmt.Sprintf("cn%d", idx))
		total = fmt.Sprintf("(CASE WHEN COALESCE(sum(cn%[1]d), 0) = 0 THEN NULL ELSE tc_div(sum(cs%[1]d), sum(cn%[1]d), %[2]d) END)", idx, cp.scale)
	default:
		total = fmt.Sprintf("COALESCE(sum(c%d), 0)", idx)
	}
	d := fmt.Sprintf("d%d_%d", lv.n, idx)
	lv.tms = append(lv.tms, fmt.Sprintf("%s AS (SELECT %s FROM (SELECT DISTINCT %s FROM w)%s)",
		d, strings.Join(append(slices.Clone(lv.keys), total+" AS "+name), ", "),
		strings.Join(append(slices.Clone(lv.keys), cols...), ", "), groupBy(lv.keys)))
	lv.tmCols = append(lv.tmCols, d)
	return "a." + name
}

func groupBy(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	return " GROUP BY " + strings.Join(keys, ", ")
}

// cell is a column's value on this level's rows.
func (lv *level) cell(cp *colPlan) (expr, error) {
	ctx := &levelCtx{lv: lv, visiting: lv.visiting}
	return ctx.col(cp.col.ID, cp.at)
}

// order sorts sibling rows: by the spec's sort keys that mean something on
// this level (a total, a formula, this level's own group column), then by
// the level's key.
func (lv *level) order() ([]string, error) {
	var out []string
	for i, s := range lv.c.p.sort {
		e, ok, err := lv.sortValue(s.col, fmt.Sprintf("sort.%d.col", i))
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		dir := ""
		if s.desc {
			dir = " DESC"
		}
		out = append(out, "("+e+" IS NULL)", e+dir)
	}
	if lv.n > 0 {
		key := fmt.Sprintf("a.k%d", lv.n-1)
		out = append(out, "("+key+" IS NULL)", key)
	}
	if len(out) == 0 {
		out = []string{"1"}
	}
	return out, nil
}

func (lv *level) sortValue(cp *colPlan, at string) (string, bool, error) {
	if cp.path != nil && cp.col.Aggregate == "" && lv.groupKey(cp) == "" {
		return "", false, nil
	}
	e, err := lv.cell(cp)
	if err != nil {
		return "", false, err
	}
	if e.win {
		return "", false, notASortKey(at)
	}
	return e.sql, true, nil
}

// groupKey is the key of the group column cp on this level, or "".
func (lv *level) groupKey(cp *colPlan) string {
	for i, g := range lv.c.p.groups {
		if i < lv.n && g.col == cp && g.group.Bucket == "" {
			return fmt.Sprintf("a.k%d", i)
		}
	}
	return ""
}

// isGroupColumn is true when cp is any level's group column.
func (lv *level) isGroupColumn(cp *colPlan) bool {
	return slices.ContainsFunc(lv.c.p.groups, func(g *groupPlan) bool { return g.col == cp })
}

// render returns the level's CTEs; o<L> selects the given cells.
func (lv *level) render(cells []string) []string {
	a := fmt.Sprintf("a%d", lv.n)
	keyCols := slices.Clone(lv.keys)
	// __n counts the base rows of the group; it also keeps the select of a
	// level with no aggregate valid.
	ctes := []string{fmt.Sprintf("%s AS (SELECT %s FROM w%s)", a,
		strings.Join(append(append(keyCols, lv.aggs...), "count(*) AS __n"), ", "), groupBy(lv.keys))}
	ctes = append(ctes, lv.tms...)
	g := a
	if len(lv.tmCols) > 0 {
		g = fmt.Sprintf("g%d", lv.n)
		sel := []string{"a.*"}
		joins := []string{}
		for _, d := range lv.tmCols {
			idx := strings.SplitN(d, "_", 2)[1]
			sel = append(sel, d+".x"+idx)
			on := []string{"1"}
			for _, k := range lv.keys {
				on = append(on, fmt.Sprintf("%s.%s IS a.%s", d, k, k))
			}
			joins = append(joins, fmt.Sprintf("LEFT JOIN %s ON %s", d, strings.Join(on, " AND ")))
		}
		ctes = append(ctes, fmt.Sprintf("%s AS (SELECT %s FROM %s a %s)", g, strings.Join(sel, ", "), a, strings.Join(joins, " ")))
	}
	out := []string{fmt.Sprintf("%d AS __level", lv.n), "NULL AS __row"}
	for i := range lv.c.p.groups {
		if i < lv.n {
			out = append(out, fmt.Sprintf("a.k%d AS k%d", i, i))
		} else {
			out = append(out, fmt.Sprintf("NULL AS k%d", i))
		}
	}
	for _, ck := range lv.c.currencies {
		out = append(out, "a."+ck+" AS "+ck)
	}
	out = append(out, cells...)
	out = append(out, fmt.Sprintf("ROW_NUMBER() OVER (%s ORDER BY %s) AS __ord", partitionBy(lv.frame.parent), strings.Join(lv.frame.order, ", ")))
	ctes = append(ctes, fmt.Sprintf("o%d AS (SELECT %s FROM %s a)", lv.n, strings.Join(out, ", "), g))
	return ctes
}

// levelCtx compiles a cell of a group, subtotal or total row from the
// level's aggregates.
type levelCtx struct {
	ctxBase
	lv       *level
	visiting map[string]bool
}

func (lc *levelCtx) col(id, at string) (expr, error) {
	lv := lc.lv
	cp := lv.c.p.byID[id]
	if cp == nil {
		return expr{}, unknownColumn(at, id)
	}
	idx := lv.c.p.index(cp)
	name := fmt.Sprintf("x%d", idx)
	cur := lv.c.currencyOf[id]
	switch {
	case cp.nest > 0:
		return expr{sql: "NULL", typ: cp.path.field.Type}, nil
	case cp.path != nil && cp.tm >= 0:
		return withCurrency(expr{sql: lv.toManyTotal(cp), typ: aggregateType(cp.col.Aggregate, cp.path.field.Type, cp.scale)}, cur), nil
	case cp.path != nil && cp.col.Aggregate != "":
		return withCurrency(expr{sql: lv.agg(name, aggregateSQL(cp.col.Aggregate, fmt.Sprintf("w.c%d", idx), cp.scale)),
			typ: aggregateType(cp.col.Aggregate, cp.path.field.Type, cp.scale)}, cur), nil
	case cp.path != nil:
		sql := "NULL"
		if key := lv.groupKey(cp); key != "" {
			sql = key
		}
		return withCurrency(expr{sql: sql, typ: cp.path.field.Type, target: cp.path.target}, cur), nil
	case cp.col.Aggregate == "" && lv.isGroupColumn(cp):
		return lc.formulaGroupColumn(cp, at)
	}
	done, err := enter(lc.visiting, id, at)
	if err != nil {
		return expr{}, err
	}
	defer done()
	if cp.col.Aggregate == "" {
		e, err := compileFormula(lc, cp.col.Formula, cp.at+".formula")
		if err != nil {
			return expr{}, err
		}
		// A formula that reads a row's value has none for a group of rows;
		// computing it from blanks would show a wrong figure (0).
		if lv.readsRowValue(cp.col.Formula, false, map[string]bool{}) {
			e.sql = "NULL"
		}
		return e, nil
	}
	inner := &rowsCtx{r: lv.r, inAggregate: true, visiting: lc.visiting}
	e, err := compileFormula(inner, cp.col.Formula, cp.at+".formula")
	if err != nil {
		return expr{}, err
	}
	if err := checkAggregate(cp.col.Aggregate, e.typ, cp.scale, cp.at+".aggregate"); err != nil {
		return expr{}, err
	}
	return withCurrency(expr{sql: lv.agg(name, aggregateSQL(cp.col.Aggregate, e.sql, cp.scale)), typ: aggregateType(cp.col.Aggregate, e.typ, cp.scale)}, e.cur), nil
}

// formulaGroupColumn is a formula column that is also a group column: on a
// row of the level that groups by it, its key (every row of the group has
// the same value); elsewhere, like a path group column, blank. The formula
// itself is evaluated per base row in w, where the key is computed.
func (lc *levelCtx) formulaGroupColumn(cp *colPlan, at string) (expr, error) {
	e, err := lc.lv.r.rowValue(cp, at, map[string]bool{})
	if err != nil {
		return expr{}, err
	}
	e.sql = "NULL"
	if key := lc.lv.groupKey(cp); key != "" {
		e.sql = key
	}
	return e, nil
}

// aggregateOp is true for an operator that totals rows (min and max with one
// argument; with more they compare values).
func aggregateOp(n *Node) bool {
	switch n.Op {
	case "sum", "count", "countDistinct", "avg":
		return true
	case "min", "max":
		return len(n.Args) == 1
	}
	return false
}

// readsRowValue is true when a formula reads, outside an aggregate, a value
// that exists only per row on this level: a {path}, a path column with no
// aggregate, a group column on a level where it is not a key, or a formula
// column that reads one. Such a formula is blank as a whole on the level's
// rows.
func (lv *level) readsRowValue(n *Node, inAggregate bool, seen map[string]bool) bool {
	if n == nil {
		return false
	}
	if n.Path != nil {
		return !inAggregate
	}
	if n.Col != "" && !inAggregate && !seen[n.Col] {
		cp := lv.c.p.byID[n.Col]
		if cp == nil || cp.col.Aggregate != "" || lv.groupKey(cp) != "" {
			return false
		}
		if cp.path != nil {
			return cp.tm < 0
		}
		if lv.isGroupColumn(cp) {
			return true
		}
		// Memoized: a chain of columns that each read the one before twice
		// would otherwise be walked once per path through it.
		if reads, ok := lv.reads[n.Col]; ok {
			return reads
		}
		seen[n.Col] = true
		defer delete(seen, n.Col)
		reads := lv.readsRowValue(cp.col.Formula, false, seen)
		lv.reads[n.Col] = reads
		return reads
	}
	inAggregate = inAggregate || aggregateOp(n)
	for _, a := range n.Args {
		if lv.readsRowValue(a, inAggregate, seen) {
			return true
		}
	}
	return false
}

// path is blank on a group, subtotal or total row, as a path column with no
// aggregate is: the rows of a group have no single value. The path is still
// bound, so an unknown field or link is an error.
func (lc *levelCtx) path(p Path, at string) (expr, error) {
	bound, err := bindValuePath(lc.lv.c.p.app, lc.lv.c.p.base, p, at, pathValue)
	if err != nil {
		return expr{}, err
	}
	return withCurrency(expr{sql: "NULL", typ: bound.field.Type, target: bound.target}, lc.lv.r.pathCurrency(bound)), nil
}

func (lc *levelCtx) param(id, at string) (expr, error) {
	v, err := paramOf(lc.lv.c, id, at)
	if err != nil {
		return expr{}, err
	}
	return paramExpr(lc, v), nil
}

func (lc *levelCtx) aggregate(op Aggregate, arg *Node, scale int, at string) (expr, error) {
	e, err := lc.aggregateArg(lc.lv.r, lc.visiting, op, arg, scale, at)
	if err != nil {
		return expr{}, err
	}
	name := fmt.Sprintf("y%d", len(lc.lv.aggs))
	return withCurrency(expr{sql: lc.lv.agg(name, aggregateSQL(op, e.sql, scale)), typ: aggregateType(op, e.typ, scale)}, e.cur), nil
}

func (lc *levelCtx) window(op string, arg *Node, scale int, at string) (expr, error) {
	e, err := compileFormula(lc, arg, at+".args.0")
	if err != nil {
		return expr{}, err
	}
	return windowSQL(op, e, scale, lc.lv.frame, at)
}

func (lc *levelCtx) bind(v any) string   { return lc.lv.r.s.bind(v) }
func (lc *levelCtx) fiscalStart() string { return lc.bind(int64(lc.lv.c.fiscal)) }
func (lc *levelCtx) filter() bool        { return false }
