package reports

import (
	"fmt"
	"regexp"
	"slices"
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
	// hoisted are running values and lookups, computed in selects below the
	// one over the frame's rows ("<sql> AS wh<n>"); hoistedAs names them by
	// their SQL.
	hoisted   []hoistLayer
	hoistedAs map[string]string
	// checking is a frame that only compiles a cell to check it: its
	// hoisted values have no select below, so a lookup is not hoisted into
	// it (a running value is refused where it is checked).
	checking bool
}

// hoistLayer is one select below the frame's own: values ("<sql> AS
// wh<n>") computed for each row, or, with keys, once for each distinct value
// of those group key columns and joined back onto the rows by them.
type hoistLayer struct {
	cols  []string
	names []string
	keys  []string
}

// hoist keeps a running value to compute in the select below and returns
// its reference under alias.
func (f *windowFrame) hoist(alias, sql string) string {
	return f.hoistLayer(alias, nil, sql)[0]
}

// hoistLayer adds one layer that computes sqls (once per distinct value of
// keys, when there are keys) and returns their references under alias. A
// value hoisted before keeps its name.
func (f *windowFrame) hoistLayer(alias string, keys []string, sqls ...string) []string {
	if f.hoistedAs == nil {
		f.hoistedAs = map[string]string{}
	}
	refs := make([]string, len(sqls))
	layer := hoistLayer{keys: keys}
	for i, sql := range sqls {
		name, ok := f.hoistedAs[sql]
		if !ok {
			name = fmt.Sprintf("wh%d", len(f.hoistedAs))
			f.hoistedAs[sql] = name
			layer.cols = append(layer.cols, sql+" AS "+name)
			layer.names = append(layer.names, name)
		}
		refs[i] = alias + "." + name
	}
	if len(layer.cols) > 0 {
		f.hoisted = append(f.hoisted, layer)
	}
	return refs
}

// from is what a select over table (as alias) reads: the table, or with
// hoisted values, selects of the table's rows that add them one layer at a
// time under the same alias, so every reference to alias still reads and a
// hoisted value may read one hoisted before it (a lookup of a lookup of a
// running value). A keyed layer computes its values over table's rows
// grouped by its keys, as alias, so its values read the keys as they read
// them on a row; it reads nothing hoisted.
func (f *windowFrame) from(table, alias string) string {
	base := table + " " + alias
	from := base
	if len(f.hoisted) == 0 && table == alias {
		from = table
	}
	for i, h := range f.hoisted {
		if len(h.keys) == 0 {
			from = fmt.Sprintf("(SELECT %s.*, %s FROM %s) %s", alias, strings.Join(h.cols, ", "), from, alias)
			continue
		}
		g := fmt.Sprintf("wg%d", i)
		names := make([]string, len(h.names))
		on := make([]string, len(h.keys))
		for j, n := range h.names {
			names[j] = g + "." + n
		}
		for j, k := range h.keys {
			on[j] = fmt.Sprintf("%s.%s IS %s.%s", g, k, alias, k)
		}
		keys := strings.Join(h.keys, ", ")
		from = fmt.Sprintf("(SELECT %s.*, %s FROM %s LEFT JOIN (SELECT %s, %s FROM %s GROUP BY %s) %s ON %s) %s",
			alias, strings.Join(names, ", "), from, keys, strings.Join(h.cols, ", "), base, keys, g, strings.Join(on, " AND "), alias)
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
	return withCurrency(pathExpr(rc.r.hidden(bound), bound), rc.r.pathCurrency(bound)), nil
}

// record reads the group record's field on a base row: a base row is under
// every group, so its record is its own key. An amount's own currency is the
// currency key valueCurrencies gave it when it is read inside a total, else
// read beside it.
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
	key := fmt.Sprintf("w.k%d", gi)
	e.sql = recordSQL(rc.r.s, target, bound, key)
	id := recordCurrencyID(group, p)
	if ck, ok := c.valueCur[id]; ok {
		e.cur = currency{key: ck, id: id}
	} else {
		e.cur = recordCurrency(rc.r.s, target, bound, key, id)
	}
	return e, nil
}

func (rc *rowsCtx) param(id, at string) (expr, error) {
	v, err := paramOf(rc.r.c, id, at)
	if err != nil {
		return expr{}, err
	}
	return paramLeaf(rc.r.c, rc.r.s, rc, id, v)
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
	e = countedArg(op, e)
	sql := e.sql
	switch op {
	case Sum:
		sql = "COALESCE(" + e.sql + ", 0)"
	case Count, CountDistinct:
		sql = "(" + e.sql + " IS NOT NULL)"
	case Avg:
		sql = fmt.Sprintf("tc_round(%s, %d)", e.sql, scale)
	}
	if e.mixed != "" {
		// A sum lookup that met two currencies has no value, so its total
		// over the one row has none either (R3), not 0.
		sql = fmt.Sprintf("(CASE WHEN %s THEN NULL ELSE %s END)", e.mixed, sql)
	}
	return withCurrency(expr{sql: sql, typ: aggregateType(op, e.typ, scale), mixed: e.mixed}, e.cur), nil
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

func (rc *rowsCtx) bind(v any) string   { return rc.r.s.bind(v) }
func (rc *rowsCtx) fiscalStart() string { return rc.bind(int64(rc.r.c.fiscal)) }
func (rc *rowsCtx) filter() bool        { return false }
func (rc *rowsCtx) label(e expr) string { return rc.r.c.textOf(rc.r.s, e) }
func (rc *rowsCtx) lookup(n *Node, at string) (expr, error) {
	e, err := lookupIn(rc, rc.r, n, at)
	if err != nil {
		return e, err
	}
	return rc.hoistLookup(e), nil
}
func (rc *rowsCtx) currencySQL(cur currency) string {
	return currencyCode(rc, rc.r.c, cur, func(key string) string { return "w." + key })
}

// hoist computes a running value a lookup reads below the detail select.
// Only a detail row has one (inside a total a running value is refused).
func (rc *rowsCtx) hoist(sql string) string {
	if rc.detail == nil {
		return sql
	}
	return rc.detail.hoist("w", sql)
}

// hoistLookup computes a lookup on a detail row once per row, in the select
// below the detail select: SQLite runs a correlated subquery again at each
// place the row reads it, and a currency guard (R24) reads a lookup's value,
// code and mixed check many times over. A lookup that holds a total or a
// running value stays where it is.
//
// A lookup that reads of the row only its group keys (a group record, with
// params and constants) has one value for every row of a group: it is
// computed once per group key instead (rowKeysRead).
func (rc *rowsCtx) hoistLookup(e expr) expr {
	if rc.detail == nil || rc.detail.checking || e.agg || e.win {
		return e
	}
	if keys := rowKeysRead(e); len(keys) > 0 {
		// The value keeps its mixed check inline: a keyed layer reads no
		// value hoisted beside it.
		parts := []*string{&e.sql, &e.mixed, &e.cur.sql, &e.shown}
		var sqls []string
		for _, p := range parts {
			if *p != "" {
				sqls = append(sqls, *p)
			}
		}
		refs := rc.detail.hoistLayer("w", keys, sqls...)
		for _, p := range parts {
			if *p != "" {
				*p, refs = refs[0], refs[1:]
			}
		}
		return e
	}
	if e.mixed != "" {
		// A sum lookup's value holds its mixed check: it reads the hoisted
		// one, hoisted before it.
		mixed := rc.hoist(e.mixed)
		e.sql = strings.ReplaceAll(e.sql, e.mixed, mixed)
		e.mixed = mixed
	}
	if e.cur.sql != "" {
		e.cur.sql = rc.hoist(e.cur.sql)
	}
	if e.shown != "" {
		e.shown = rc.hoist(e.shown)
	}
	e.sql = rc.hoist(e.sql)
	return e
}

// rowRef is a lookup's read of the report's row: every value of a base row
// reads it as w.<column>.
var rowRef = regexp.MustCompile(`\bw\.(\w+)`)

// groupKeyCol is a group key column of w (k<group index>).
var groupKeyCol = regexp.MustCompile(`^k\d+$`)

// rowKeysRead is the group key columns a lookup reads of the report's row,
// sorted, when those are all it reads of it; nil when it reads another
// value of the row, or none (a lookup that reads nothing of the row is
// computed once per statement as it is).
func rowKeysRead(e expr) []string {
	var keys []string
	for _, sql := range []string{e.sql, e.mixed, e.cur.sql, e.shown} {
		for _, m := range rowRef.FindAllStringSubmatch(sql, -1) {
			if !groupKeyCol.MatchString(m[1]) {
				return nil
			}
			if !slices.Contains(keys, m[1]) {
				keys = append(keys, m[1])
			}
		}
	}
	slices.Sort(keys)
	return keys
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
