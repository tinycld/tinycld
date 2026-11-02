package reports

import (
	"fmt"
	"strings"
)

// rowsCtx compiles over w, one row per base row: a detail row's cells, and
// the value inside an aggregate. Inside an aggregate it reads no to-many
// total (a total of per-parent totals double-counts) and no other total.
type rowsCtx struct {
	ctxBase
	r           *rowsCTE
	inAggregate bool
	// detail orders and partitions the detail rows for running values;
	// nil inside an aggregate.
	detail   *windowFrame
	visiting map[string]bool
}

// windowFrame is where a running value or a percent reads its rows: the
// partition of the row's parent, the partition of the whole report, and the
// row order.
type windowFrame struct {
	parent []string
	total  []string
	order  []string
	// hoisted are running values a lookup reads, computed in a select below
	// the one over the frame's rows ("<sql> AS wh<n>"); hoistedAs names them
	// by their SQL.
	hoisted   []string
	hoistedAs map[string]string
}

// hoist keeps a running value to compute in the select below and returns
// its reference under alias.
func (f *windowFrame) hoist(alias, sql string) string {
	if name, ok := f.hoistedAs[sql]; ok {
		return alias + "." + name
	}
	if f.hoistedAs == nil {
		f.hoistedAs = map[string]string{}
	}
	name := fmt.Sprintf("wh%d", len(f.hoisted))
	f.hoisted = append(f.hoisted, sql+" AS "+name)
	f.hoistedAs[sql] = name
	return alias + "." + name
}

// from is what a select over table (as alias) reads: the table, or with
// hoisted values, selects of the table's rows that add them one at a time
// under the same alias, so every reference to alias still reads and a
// hoisted value may read one hoisted before it (a lookup of a lookup of a
// running value).
func (f *windowFrame) from(table, alias string) string {
	from := table + " " + alias
	if len(f.hoisted) == 0 && table == alias {
		from = table
	}
	for _, h := range f.hoisted {
		from = fmt.Sprintf("(SELECT %s.*, %s FROM %s) %s", alias, h, from, alias)
	}
	return from
}

func (rc *rowsCtx) col(id, at string) (expr, error) {
	cp := rc.r.c.p.byID[id]
	if cp == nil {
		return expr{}, unknownColumn(at, id)
	}
	if cp.nest > 0 {
		return expr{}, invalid(at, "a formula does not read child row columns")
	}
	if cp.path != nil {
		if cp.tm >= 0 && rc.inAggregate {
			return expr{}, invalid(at, fmt.Sprintf("a total of %q, itself a total across a to-many link, would double-count: use the column's own aggregate", id))
		}
		return rc.r.colValue(cp), nil
	}
	done, err := enter(rc.visiting, id, at)
	if err != nil {
		return expr{}, err
	}
	defer done()
	return compileFormula(rc, cp.col.Formula, cp.at+".formula")
}

func (rc *rowsCtx) path(p Path, at string) (expr, error) {
	bound, err := bindValuePath(rc.r.c.p.app, rc.r.c.p.base, p, at, pathValue)
	if err != nil {
		return expr{}, err
	}
	return withCurrency(expr{sql: rc.r.hidden(bound), typ: bound.field.Type, target: bound.target}, rc.r.pathCurrency(bound)), nil
}

// record reads the group record's field on a base row: a base row is under
// every group, so its record is its own key.
func (rc *rowsCtx) record(group string, p Path, at string) (expr, error) {
	c := rc.r.c
	gi, target, err := c.recordOf(group, at)
	if err != nil {
		return expr{}, err
	}
	bound, err := bindRecordPath(c.p.app, target, p, at)
	if err != nil {
		return expr{}, err
	}
	e := recordExpr(bound)
	e.sql = recordSQL(rc.r.s, target, bound, fmt.Sprintf("w.k%d", gi))
	return e, nil
}

func (rc *rowsCtx) param(id, at string) (expr, error) {
	v, err := paramOf(rc.r.c, id, at)
	if err != nil {
		return expr{}, err
	}
	return paramExpr(rc, v), nil
}

// aggregate on a detail row is the aggregate of one row: sum(x) is x.
func (rc *rowsCtx) aggregate(op Aggregate, arg *Node, scale int, at string) (expr, error) {
	if rc.inAggregate {
		return expr{}, invalid(at, "a total inside a total")
	}
	e, err := rc.aggregateArg(rc.r, rc.visiting, op, arg, scale, at)
	if err != nil {
		return expr{}, err
	}
	sql := e.sql
	switch op {
	case Sum:
		sql = "COALESCE(" + e.sql + ", 0)"
	case Count, CountDistinct:
		sql = "(" + e.sql + " IS NOT NULL)"
	case Avg:
		sql = fmt.Sprintf("tc_round(%s, %d)", e.sql, scale)
	}
	return withCurrency(expr{sql: sql, typ: aggregateType(op, e.typ, scale)}, e.cur), nil
}

func (rc *rowsCtx) window(op string, arg *Node, scale int, at string) (expr, error) {
	if rc.detail == nil {
		return expr{}, invalid(at, op+" cannot be inside a total")
	}
	e, err := compileFormula(rc, arg, at+".args.0")
	if err != nil {
		return expr{}, err
	}
	return windowSQL(op, e, scale, rc.detail, at)
}

func (rc *rowsCtx) bind(v any) string                       { return rc.r.s.bind(v) }
func (rc *rowsCtx) fiscalStart() string                     { return rc.bind(int64(rc.r.c.fiscal)) }
func (rc *rowsCtx) filter() bool                            { return false }
func (rc *rowsCtx) lookup(n *Node, at string) (expr, error) { return lookupIn(rc, rc.r, n, at) }

// hoist computes a running value a lookup reads below the detail select.
// Only a detail row has one (inside a total a running value is refused).
func (rc *rowsCtx) hoist(sql string) string {
	if rc.detail == nil {
		return sql
	}
	return rc.detail.hoist("w", sql)
}

// windowSQL is a running value or a percent over a frame. A running sum
// adds in the frame's order within the parent; a percent divides by the sum
// over the parent or over the whole report, with no figure when that sum is
// zero.
func windowSQL(op string, e expr, scale int, f *windowFrame, at string) (expr, error) {
	if !isNumeric(e.typ) || e.win {
		return expr{}, invalid(at, op+" needs a number that is not itself a running value")
	}
	switch op {
	case "runningSum":
		over := "OVER (" + partitionBy(f.parent) + " ORDER BY " + strings.Join(f.order, ", ") + " ROWS UNBOUNDED PRECEDING)"
		return withCurrency(expr{sql: "SUM(" + num(e) + ") " + over, typ: sumType([]expr{e}), win: true}, e.cur), nil
	case "percentOfParent":
		return expr{sql: fmt.Sprintf("tc_pct(%s, SUM(%s) OVER (%s), %d)", num(e), num(e), partitionBy(f.parent), scale), typ: Percent, win: true}, nil
	}
	return expr{sql: fmt.Sprintf("tc_pct(%s, SUM(%s) OVER (%s), %d)", num(e), num(e), partitionBy(f.total), scale), typ: Percent, win: true}, nil
}

func partitionBy(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	return "PARTITION BY " + strings.Join(keys, ", ")
}
