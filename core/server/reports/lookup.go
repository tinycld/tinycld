package reports

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// lookupValue decodes a lookup's value: a formula node over the looked-up
// source.
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

// lookupIn compiles a lookup in a context whose rows CTE is r: the smallest
// (pick min, the default) or largest value of the node's value over the rows
// of its source that match its where, NULL when none match. Every row of the
// source is read through its rule CTE, so the reader's list rule applies.
// Inside, a path reads the looked-up source; a column, a group record or a
// param reads the report's row through outer, which makes the subquery
// correlated. SQLite runs an uncorrelated scalar subquery once per statement.
func lookupIn(outer fctx, r *rowsCTE, n *Node, at string) (expr, error) {
	if len(n.Args) > 0 || n.Link != nil || n.Scale != nil {
		return expr{}, invalid(at, "lookup takes a source, a where, a value and a pick")
	}
	if n.Pick != "" && n.Pick != "min" && n.Pick != "max" {
		return expr{}, invalid(at+".pick", "pick is min or max")
	}
	src := lookupSource(r.c.p.app, n.Source)
	if src == nil {
		return expr{}, unknownRef(at+".source", "unknown_source", fmt.Sprintf("the source %q does not exist", n.Source))
	}
	valueNode, err := lookupValue(n, at)
	if err != nil {
		return expr{}, err
	}
	sc := newScope(r.s, src)
	v, err := compileFormula(&lookupCtx{outer: outer, r: r, sc: sc, src: src}, valueNode, at+".value")
	if err != nil {
		return expr{}, err
	}
	if v.agg || v.win {
		return expr{}, invalid(at+".value", "a lookup reads one value per matching row: totals and running values go outside it")
	}
	where := "1"
	if n.Where != nil {
		w, err := compileWhere(&lookupCtx{outer: outer, r: r, sc: sc, src: src, inWhere: true}, n.Where, at)
		if err != nil {
			return expr{}, err
		}
		if w.agg || w.win {
			return expr{}, invalid(at+".where", "a lookup's where reads one row at a time")
		}
		if !w.unset {
			where = "COALESCE(" + w.sql + ", 0)"
		}
	}
	pick := "min"
	if n.Pick == "max" {
		pick = "max"
	}
	sql := fmt.Sprintf("(SELECT %s(%s)\n%s\nWHERE %s)", pick, v.sql, sc.render(), where)
	return expr{sql: sql, typ: v.typ, target: v.target, cur: v.cur}, nil
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

func (l *lookupCtx) col(id, at string) (expr, error) { return l.outer.col(id, at) }

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

func (l *lookupCtx) path(p Path, at string) (expr, error) {
	bound, err := bindValuePath(l.r.c.p.app, l.src, p, at, pathValue)
	if err != nil {
		return expr{}, err
	}
	if bound.field.Type == Money && bound.field.Currency != "" {
		return expr{}, invalid(at, "this amount has its own row's currency, which totals cannot group by: read it through a column")
	}
	hops := append(append([]hopPlan{}, l.prefix...), bound.hops...)
	e := l.sc.value(&pathPlan{hops: hops, field: bound.field, src: bound.src, target: bound.target})
	if bound.field.Type == Money && bound.src.Currency != nil {
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
