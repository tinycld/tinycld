package reports

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// lookupValue decodes a lookup's value: a formula node over the looked-up
// source. A bound spec decodes each lookup once, through plan.lookupOf.
func lookupValue(n *Node, at string) (*Node, error) {
	if n.Value == nil {
		return nil, invalid(at+".value", "a lookup names the value it reads")
	}
	var v Node
	dec := json.NewDecoder(bytes.NewReader(n.Value))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return nil, invalid(at+".value", "a lookup's value is a formula: "+err.Error())
	}
	return &v, nil
}

// decodedLookup is a lookup's value as plan.lookupOf decoded it, or why it
// could not be.
type decodedLookup struct {
	value *Node
	err   error
}

// lookupOf is a lookup node's decoded value, decoded on first use and kept
// with its error, so the binder, the size counts and the compiler read one
// decode. bind decodes every lookup of the spec first (lookups) and refuses
// a value that does not decode, so a later reader may skip a failed one.
func (p *plan) lookupOf(n *Node, at string) (*Node, error) {
	if d, ok := p.lookups[n]; ok {
		return d.value, d.err
	}
	v, err := lookupValue(n, at)
	p.lookups[n] = decodedLookup{value: v, err: err}
	return v, err
}

// lookups decodes the value of every lookup in the spec's formulas and
// reports one that does not decode where it is.
func (b *binder) lookups(p *plan) {
	var walk func(n *Node, at string)
	walk = func(n *Node, at string) {
		if n == nil {
			return
		}
		if n.Op == "lookup" {
			v, err := p.lookupOf(n, at)
			if se, ok := err.(*SpecError); ok {
				b.merge(se)
			}
			walk(v, at+".value")
		}
		walk(n.Where, at+".where")
		for i, a := range n.Args {
			walk(a, fmt.Sprintf("%s.args.%d", at, i))
		}
	}
	for i, c := range p.spec.Columns {
		walk(c.Formula, fmt.Sprintf("columns.%d.formula", i))
	}
	walk(p.spec.Filter, "filter")
	for i, g := range p.spec.Groups {
		walk(g.Label, fmt.Sprintf("groups.%d.label", i))
		if g.Fill != nil {
			walk(g.Fill.Where, fmt.Sprintf("groups.%d.fill.where", i))
		}
		walk(g.Having, fmt.Sprintf("groups.%d.having", i))
	}
	for i, s := range p.spec.Sort {
		walk(s.Formula, fmt.Sprintf("sort.%d.formula", i))
	}
	for i, item := range p.spec.Layout {
		walk(item.Formula, fmt.Sprintf("layout.%d.formula", i))
	}
	if d := p.spec.Detail; d != nil {
		walk(d.Label, "detail.label")
	}
}

// lookupIn compiles a lookup in a context whose rows CTE is r: the smallest
// (pick min, the default) or largest value of the node's value over the rows
// of its source that match its where, NULL when none match. Every row of the
// source is read through its rule CTE, so the reader's list rule applies.
// Inside, a path reads the looked-up source; a column, a group record or a
// param reads the report's row through outer, which makes the subquery
// correlated. SQLite runs an uncorrelated scalar subquery once per statement.
// A column it reads may be a total (one value per row of its level) or a
// running value, which the outer select computes first (windowHoister).
func lookupIn(outer fctx, r *rowsCTE, n *Node, at string) (expr, error) {
	if n.Col != "" || n.Path != nil || n.Param != "" || n.Subtotal != "" || n.Record != "" {
		return expr{}, invalid(at, errNodeKind)
	}
	if len(n.Args) > 0 || n.Link != nil || n.Scale != nil {
		return expr{}, invalid(at, "lookup takes a source, a where, a value and a pick")
	}
	if n.Pick != "" && n.Pick != "min" && n.Pick != "max" {
		return expr{}, invalid(at+".pick", "pick is min or max")
	}
	if n.Source == "" {
		return expr{}, invalid(at+".source", "a lookup names its source")
	}
	src := lookupSource(r.c.p.app, n.Source)
	if src == nil {
		return expr{}, unknownRef(at+".source", "unknown_source", fmt.Sprintf("the source %q does not exist", n.Source))
	}
	valueNode, err := r.c.p.lookupOf(n, at)
	if err != nil {
		return expr{}, err
	}
	sc := newScope(r.s, src)
	v, err := compileFormula(&lookupCtx{outer: outer, r: r, sc: sc, src: src}, valueNode, at+".value")
	if err != nil {
		return expr{}, err
	}
	out := expr{typ: v.typ, target: v.target, cur: v.cur, agg: v.agg, win: v.win}
	where := "1"
	if n.Where != nil {
		w, err := compileWhere(&lookupCtx{outer: outer, r: r, sc: sc, src: src, inWhere: true}, n.Where, at)
		if err != nil {
			return expr{}, err
		}
		if !w.unset {
			where = "COALESCE(" + w.sql + ", 0)"
		}
		out.agg, out.win = out.agg || w.agg, out.win || w.win
	}
	if v.typ == Money && v.cur.sql != "" {
		return pickedAmount(outer, r, n, v, sc, where, out), nil
	}
	pick := "min"
	if n.Pick == "max" {
		pick = "max"
	}
	out.sql = fmt.Sprintf("(SELECT %s(%s)\n%s\nWHERE %s)", pick, v.sql, sc.render(), where)
	return out, nil
}

// pickedAmount is a lookup of an amount that carries its row's currency: the
// matching row with the smallest (min) or largest (max) amount, ties broken
// by currency code, gives both the amount and its currency, so the two always
// come from one row. On a base row inside a total the currency is the key
// valueCurrencies gave the lookup, which the total groups by.
func pickedAmount(outer fctx, r *rowsCTE, n *Node, v expr, sc *scope, where string, out expr) expr {
	dir := "ASC"
	if n.Pick == "max" {
		dir = "DESC"
	}
	from := sc.render()
	// Two correlated subqueries with one ORDER BY, so both pick the same row:
	// a scalar subquery returns one column, and the amount and its currency
	// are read as two cells wherever the lookup is used.
	pickOne := func(col string) string {
		return fmt.Sprintf("(SELECT %s\n%s\nWHERE %s AND %s IS NOT NULL\nORDER BY %s %s, %s LIMIT 1)", col, from, where, v.sql, v.sql, dir, v.cur.sql)
	}
	id := lookupCurrencyID(n)
	out.sql = pickOne(v.sql)
	out.cur = currency{sql: pickOne(v.cur.sql), id: id}
	if ck, ok := r.c.valueCur[id]; ok && !inLevel(outer) {
		out.cur = currency{key: ck, id: id}
	}
	return out
}

// lookupCurrencyID names a looked-up amount's currency for combining.
func lookupCurrencyID(n *Node) string {
	raw, _ := json.Marshal(n)
	return "lk:" + string(raw)
}

// inLevel is true when a formula is compiled on a group, subtotal or total
// row (a levelCtx, or a lookup inside one) or on a layout's computed row:
// there a picked or record currency is read by subquery; on a base row inside
// a total, by its registered key.
func inLevel(f fctx) bool {
	switch x := f.(type) {
	case *levelCtx, *layoutCtx:
		return true
	case *lookupCtx:
		return inLevel(x.outer)
	}
	return false
}

// windowHoister is a context whose select can compute a running value or a
// percent before the rows it reads are narrowed: hoist puts the value's SQL
// in a select below the context's own and returns the reference to it. A
// window function inside a lookup's subquery would run over the looked-up
// rows, so a lookup reads a running value through this.
type windowHoister interface {
	hoist(sql string) string
}

// lookupCtx compiles inside a lookup: paths on the looked-up source (after
// prefix, inside an exists of the where), everything else through outer.
type lookupCtx struct {
	ctxBase
	outer   fctx
	r       *rowsCTE
	sc      *scope
	src     *Source
	prefix  []hopPlan
	inWhere bool
}

// col reads the report's row; a running value is computed in the outer
// select first (a lookup inside a lookup leaves that to the outermost).
func (l *lookupCtx) col(id, at string) (expr, error) {
	e, err := l.outer.col(id, at)
	if err != nil || !e.win {
		return e, err
	}
	if h, ok := l.outer.(windowHoister); ok {
		e.sql = h.hoist(e.sql)
	}
	return e, nil
}

// A param is the run's value wherever the lookup sits (a layout row's outer
// context reads no params, so a lookup there resolves them itself).
func (l *lookupCtx) param(id, at string) (expr, error) {
	v, err := paramOf(l.r.c, id, at)
	if err != nil {
		return expr{}, err
	}
	return paramExpr(l, v), nil
}

func (l *lookupCtx) lookup(n *Node, at string) (expr, error) { return lookupIn(l, l.r, n, at) }

// record reads the report row's group record, as a column does: through
// outer, which makes the subquery correlated.
func (l *lookupCtx) record(group string, p Path, at string) (expr, error) {
	return l.outer.record(group, p, at)
}

// subtotal reads a layout's section or earlier row at the cell being
// compiled, as the computed row's own formula does; elsewhere outer refuses
// it.
func (l *lookupCtx) subtotal(id, at string) (expr, error) { return l.outer.subtotal(id, at) }

func (l *lookupCtx) path(p Path, at string) (expr, error) {
	bound, err := bindValuePath(l.r.c.p.app, l.src, p, at, pathValue)
	if err != nil {
		return expr{}, err
	}
	hops := append(append([]hopPlan{}, l.prefix...), bound.hops...)
	e := l.sc.value(&pathPlan{hops: hops, field: bound.field, src: bound.src, target: bound.target})
	switch {
	case bound.field.Type != Money:
	case bound.field.Currency != "":
		// The looked-up row's own currency, read beside the amount; lookupIn
		// picks the two from one row. The column names it for combining.
		cur := l.sc.value(&pathPlan{hops: hops, field: bound.src.field(bound.field.Currency), src: bound.src}).sql
		e.cur = currency{sql: cur, id: "row:" + cur}
	case bound.src.Currency != nil:
		e.cur = currency{fixed: bound.src}
	}
	return e, nil
}

func (l *lookupCtx) aggregate(op Aggregate, _ *Node, _ int, at string) (expr, error) {
	return expr{}, invalid(at, fmt.Sprintf("%s totals rows: a lookup reads one value per matching row", op))
}

func (l *lookupCtx) window(op string, _ *Node, _ int, at string) (expr, error) {
	return expr{}, invalid(at, op+" reads the rows a report shows: use it in a column")
}

func (l *lookupCtx) exists(n *Node, at string) (expr, error) {
	link, err := bindValuePath(l.r.c.p.app, l.src, n.Link, at+".link", pathLinks)
	if err != nil {
		return expr{}, err
	}
	mk := func(sc *scope, hops []hopPlan) fctx {
		src := sc.src
		if len(hops) > 0 {
			src = hops[len(hops)-1].to
		}
		return &lookupCtx{outer: l.outer, r: l.r, sc: sc, src: src, prefix: hops, inWhere: true}
	}
	return l.sc.exists(l.prefix, link, n.Where, mk, at)
}

func (l *lookupCtx) bind(v any) string   { return l.r.s.bind(v) }
func (l *lookupCtx) fiscalStart() string { return l.bind(int64(l.r.c.fiscal)) }
func (l *lookupCtx) filter() bool        { return l.inWhere }
func (l *lookupCtx) label(e expr) string { return l.r.c.textOf(l.r.s, e) }
