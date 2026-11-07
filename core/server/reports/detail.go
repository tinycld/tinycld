package reports

import (
	"fmt"
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
	// reads records what the lookup being compiled reads of the row; nil
	// outside a lookup.
	reads *rowReads
}

// rowReads is what a lookup on a detail row reads of the report's row,
// recorded by the row's leaves while the lookup compiles: the group key
// columns its group records read, and whether it reads any other value of
// the row (a column, a path, a currency key, a running value). Each leaf of
// rowsCtx that reads the row records its read, so the facts do not depend on
// how the SQL names the row.
type rowReads struct {
	keys  []string
	other bool
}

func (r *rowReads) key(k string) {
	if r != nil && !slices.Contains(r.keys, k) {
		r.keys = append(r.keys, k)
	}
}

func (r *rowReads) value() {
	if r != nil {
		r.other = true
	}
}

// add records an inner lookup's reads in the lookup around it.
func (r *rowReads) add(inner *rowReads) {
	if r == nil {
		return
	}
	for _, k := range inner.keys {
		r.key(k)
	}
	if inner.other {
		r.value()
	}
}

// groupKeys is the group key columns read, sorted, when those are all the
// lookup reads of the row; nil when it reads another value of the row, or
// none (a lookup that reads nothing of the row is computed once per
// statement as it is).
func (r *rowReads) groupKeys() []string {
	if r.other || len(r.keys) == 0 {
		return nil
	}
	keys := slices.Clone(r.keys)
	slices.Sort(keys)
	return keys
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
		rc.reads.value()
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
	rc.reads.value()
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
	k := fmt.Sprintf("k%d", gi)
	rc.reads.key(k)
	key := "w." + k
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
	// The argument compiles in a context of its own, which records nothing.
	rc.reads.value()
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
	rc.reads.value()
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

// lookup compiles a lookup on the row, recording what it reads of the row
// (hoistLookup reads that). A lookup inside it, through a column's formula,
// adds its reads to this one's; one hoisted is a value of the row.
func (rc *rowsCtx) lookup(n *Node, at string) (expr, error) {
	around := rc.reads
	reads := &rowReads{}
	rc.reads = reads
	e, err := lookupIn(rc, rc.r, n, at)
	rc.reads = around
	if err != nil {
		return e, err
	}
	around.add(reads)
	e, hoisted := rc.hoistLookup(e, reads)
	if hoisted {
		around.value()
	}
	return e, nil
}

func (rc *rowsCtx) currencySQL(cur currency) string {
	return currencyCode(rc, rc.r.c, cur, func(key string) string {
		rc.reads.value()
		return "w." + key
	})
}

// hoist computes a running value a lookup reads below the detail select.
// Only a detail row has one (inside a total a running value is refused).
func (rc *rowsCtx) hoist(sql string) string {
	rc.reads.value()
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
// computed once per group key instead (rowReads.groupKeys). hoisted is true
// when the lookup now reads a value hoisted below.
func (rc *rowsCtx) hoistLookup(e expr, reads *rowReads) (_ expr, hoisted bool) {
	if rc.detail == nil || rc.detail.checking || e.agg || e.win {
		return e, false
	}
	if keys := reads.groupKeys(); len(keys) > 0 {
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
		return e, true
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
	return e, true
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
