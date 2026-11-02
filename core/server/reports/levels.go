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
//	f<L>  a fill's keyset: the level's base keys and its fill records
//	a<L>  aggregates w by the level's keys, then adds a fill's records with
//	      no rows (fillRows)
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
	tmCols   []tmCol  // g<L> values taken from d<L>
	// fill is the level's keyset when its group fills in the records with no
	// rows (fillKeys), "" otherwise; render emits it as the CTE f<L>.
	fill  string
	frame *windowFrame
	// labels computes the record labels the level's rows read once per row.
	labels *labelRefs
	// label is the level's row label (labelSQL) and its currency; labelKey
	// sorts by it: a record's label with no-name records last, or "" for a
	// level whose rows are not records.
	label    labelRef
	labelKey string
	visiting map[string]bool
	// reads memoizes readsRowValue per formula column.
	reads map[string]bool
}

// level prepares level n: its keys, its parent partition, and the order of
// its sibling rows, which running values read.
func (c *compiler) level(r *rowsCTE, n int, expand *expansion) (*level, error) {
	lv := &level{c: c, r: r, n: n, aggNames: map[string]bool{}, visiting: map[string]bool{}, reads: map[string]bool{}}
	for i := range n {
		lv.keys = append(lv.keys, fmt.Sprintf("k%d", i))
	}
	lv.keys = append(lv.keys, c.currencies...)
	if n >= 1 && c.p.groups[n-1].group.Fill != nil {
		fill, err := c.fillKeys(r, n, expand, c.p.groups[n-1].at+".fill")
		if err != nil {
			return nil, err
		}
		lv.fill = fill
		c.fillSets[n] = fmt.Sprintf("f%d", n)
	}
	parent := slices.Clone(c.currencies)
	for i := range max(n-1, 0) {
		parent = append(parent, fmt.Sprintf("k%d", i))
	}
	lv.frame = &windowFrame{parent: prefixed("a.", parent), total: prefixed("a.", c.currencies)}
	lv.labels = newLabelRefs(r.s, lv.frame, "a")
	if err := lv.labelSQL(); err != nil {
		return nil, err
	}
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
	// A group with no base rows (a fill record) has no d<L> row; its sum and
	// count are 0, as a total over no parents reads.
	zero := false
	switch cp.col.Aggregate {
	case Min, Max:
		total = fmt.Sprintf("%s(c%d)", cp.col.Aggregate, idx)
	case Avg:
		cols = append(cols, fmt.Sprintf("cs%d", idx), fmt.Sprintf("cn%d", idx))
		total = fmt.Sprintf("(CASE WHEN COALESCE(sum(cn%[1]d), 0) = 0 THEN NULL ELSE tc_div(sum(cs%[1]d), sum(cn%[1]d), %[2]d) END)", idx, cp.scale)
	default:
		total = fmt.Sprintf("COALESCE(sum(c%d), 0)", idx)
		zero = true
	}
	d := fmt.Sprintf("d%d_%d", lv.n, idx)
	lv.tms = append(lv.tms, fmt.Sprintf("%s AS (SELECT %s FROM (SELECT DISTINCT %s FROM w)%s)",
		d, strings.Join(append(slices.Clone(lv.keys), total+" AS "+name), ", "),
		strings.Join(append(slices.Clone(lv.keys), cols...), ", "), groupBy(lv.keys)))
	lv.tmCols = append(lv.tmCols, tmCol{d: d, name: name, zero: zero})
	return "a." + name
}

// tmCol is a to-many total g<L> takes from its d<L> CTE: the CTE, the
// total's name, and whether a group with no d<L> row reads 0 (a sum or a
// count) rather than blank.
type tmCol struct {
	d, name string
	zero    bool
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
// this level (a total, a formula, this level's own group column), then, for
// a record group, by its label, then by the level's key (the record's id), so
// records with no sort key read in label order and the same label keeps one
// order.
func (lv *level) order() ([]string, error) {
	var out []string
	for _, s := range lv.c.p.sort {
		e, ok, err := lv.sortKey(s)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		// A record sorts by its label's typed value, then by its id.
		if ref, ok := lv.c.sortLabel(lv.labels, s, e); ok {
			key := labelOrder(lv.r.s, ref.value)
			out = append(out, "("+key+" IS NULL)", key+dirOf(s.desc))
		}
		out = append(out, "("+e.sql+" IS NULL)", e.sql+dirOf(s.desc))
	}
	if lv.n > 0 {
		if lv.c.groupTargets[lv.n-1] != nil && lv.labelKey != "" {
			out = append(out, "("+lv.labelKey+" IS NULL)", lv.labelKey)
		}
		key := fmt.Sprintf("a.k%d", lv.n-1)
		out = append(out, "("+key+" IS NULL)", key)
	}
	if len(out) == 0 {
		out = []string{"1"}
	}
	return out, nil
}

// sortKey is a sort key's value on this level, or ok false where it means
// nothing (a path, or a formula that reads a row value).
func (lv *level) sortKey(s sortPlan) (expr, bool, error) {
	if s.formula == nil {
		return lv.sortValue(s.col, s.at)
	}
	if lv.readsRowValue(s.formula, false, map[string]bool{}) {
		return expr{}, false, nil
	}
	e, err := compileFormula(&levelCtx{lv: lv, visiting: lv.visiting}, s.formula, s.at)
	if err != nil {
		return expr{}, false, err
	}
	if e.win {
		return expr{}, false, notASortKey(s.at)
	}
	return e, true, nil
}

func (lv *level) sortValue(cp *colPlan, at string) (expr, bool, error) {
	if cp.path != nil && cp.col.Aggregate == "" && lv.groupKey(cp) == "" {
		return expr{}, false, nil
	}
	e, err := lv.cell(cp)
	if err != nil {
		return expr{}, false, err
	}
	if e.win {
		return expr{}, false, notASortKey(at)
	}
	return e, true, nil
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

// render returns the level's CTEs; o<L> selects the given cells, then
// __own, the row's label and its currency, and its count of base rows.
func (lv *level) render(cells []string, own string) []string {
	a := fmt.Sprintf("a%d", lv.n)
	keyCols := slices.Clone(lv.keys)
	// __n counts the base rows of the group; it also keeps the select of a
	// level with no aggregate valid.
	rows := fmt.Sprintf("SELECT %s FROM w%s", strings.Join(append(append(keyCols, lv.aggs...), "count(*) AS __n"), ", "), groupBy(lv.keys))
	var ctes []string
	if lv.fill != "" {
		// The keyset is one CTE, which the level below reads by name.
		ctes = append(ctes, fmt.Sprintf("%s AS (%s)", lv.c.fillSets[lv.n], lv.fill))
		rows += "\nUNION ALL\n" + lv.fillRows()
	}
	ctes = append(ctes, fmt.Sprintf("%s AS (%s)", a, rows))
	ctes = append(ctes, lv.tms...)
	g := a
	if len(lv.tmCols) > 0 {
		g = fmt.Sprintf("g%d", lv.n)
		sel := []string{"a.*"}
		joins := []string{}
		for _, tm := range lv.tmCols {
			value := tm.d + "." + tm.name
			if tm.zero {
				value = "COALESCE(" + value + ", 0)"
			}
			sel = append(sel, value+" AS "+tm.name)
			on := []string{"1"}
			for _, k := range lv.keys {
				on = append(on, fmt.Sprintf("%s.%s IS a.%s", tm.d, k, k))
			}
			joins = append(joins, fmt.Sprintf("LEFT JOIN %s ON %s", tm.d, strings.Join(on, " AND ")))
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
	out = append(out, own+" AS __own", lv.label.value+" AS __label", lv.label.currency+" AS __labelcur", "a.__n AS __n")
	ctes = append(ctes, fmt.Sprintf("o%d AS (SELECT %s FROM %s)", lv.n, strings.Join(out, ", "), lv.frame.from(g, "a")))
	return ctes
}

// own is 1 for a row of this level that passes its group's having.
func (lv *level) own() (string, error) {
	if lv.n == 0 {
		return "1", nil
	}
	g := lv.c.p.groups[lv.n-1]
	if g.group.Having == nil {
		return "1", nil
	}
	at := g.at + ".having"
	e, err := compileFormula(&levelCtx{lv: lv, visiting: lv.visiting, cond: true}, g.group.Having, at)
	if err != nil {
		return "", err
	}
	if e.win {
		return "", invalid(at, "a running value or a percent is not a condition on a group")
	}
	if e.typ != Bool && e.typ != "" {
		return "", invalid(at, "having is a condition")
	}
	// A row's value is blank on a group of rows, so such a having would hide
	// every group.
	if lv.readsRowValue(g.group.Having, false, map[string]bool{}) {
		return "", invalid(at, "having reads a group's totals, keys and records, not a value of one row")
	}
	if e.unset {
		return "1", nil
	}
	return "COALESCE(" + e.sql + ", 0)", nil
}

// labelSQL sets the label of the level's rows: the group's label formula
// (the record's label when its value points at one), a record group's
// record label, or none (the key labels the row).
func (lv *level) labelSQL() error {
	lv.label = labelRef{value: "NULL", currency: "NULL"}
	if lv.n == 0 {
		return nil
	}
	g := lv.c.p.groups[lv.n-1]
	target, chosen, key := lv.c.groupTargets[lv.n-1], lv.c.p.labelFieldName(g.col), fmt.Sprintf("a.k%d", lv.n-1)
	if g.group.Label != nil {
		e, err := compileFormula(&levelCtx{lv: lv, visiting: lv.visiting}, g.group.Label, g.at+".label")
		if err != nil {
			return err
		}
		if e.win {
			return invalid(g.at+".label", "a running value or a percent is not a label")
		}
		if at, inner := lv.innerRecord(g.group.Label, g.at+".label", false, map[string]bool{}); inner != "" {
			return invalid(at, fmt.Sprintf("the label of group %q reads the record of group %q, which is grouped inside it: a %q row has no one %q record", g.group.ID, inner, g.group.ID, inner))
		}
		lv.c.labels[lv.n-1] = labelPlan{typ: e.typ, cur: e.cur}
		// A label that reads a row's value has none for a group of rows.
		if lv.readsRowValue(g.group.Label, false, map[string]bool{}) {
			return nil
		}
		if e.target == nil {
			lv.label.value, lv.labelKey = e.sql, e.sql
			// An amount that carries its own currency labels the row with it.
			if e.typ == Money && e.cur.sql != "" {
				lv.label.currency = e.cur.sql
				lv.c.labels[lv.n-1].recordCurrency = true
			}
			return nil
		}
		// A label that points at a record shows the record's label, by the
		// label field of the column it reads.
		target, chosen, key = e.target, "", e.sql
		if cp := lv.c.p.byID[g.group.Label.Col]; cp != nil {
			chosen = lv.c.p.labelFieldName(cp)
		}
	}
	if target == nil {
		return nil
	}
	f := labelFieldOf(target, chosen)
	lv.c.labels[lv.n-1] = labelPlan{typ: labelTypeOf(f), recordCurrency: labelTypeOf(f) == Money}
	if key == "NULL" {
		return nil
	}
	lv.label = lv.labels.of(target, f, key)
	lv.labelKey = labelOrder(lv.r.s, lv.label.value)
	return nil
}

// innerRecord finds where a formula on this level reads, outside a total,
// the record of a group nested inside the level's group (directly, through a
// column it reads or through a lookup), and that group's id; "" when it reads
// none. Such a read has no value on the level's rows.
func (lv *level) innerRecord(n *Node, at string, inAggregate bool, seen map[string]bool) (string, string) {
	if n == nil {
		return "", ""
	}
	if n.Record != "" {
		if gi := lv.c.groupIndex(n.Record); !inAggregate && gi >= lv.n {
			return at, n.Record
		}
		return "", ""
	}
	if cp := lv.c.p.byID[n.Col]; cp != nil && cp.col.Formula != nil && cp.col.Aggregate == "" && !seen[n.Col] {
		seen[n.Col] = true
		if _, inner := lv.innerRecord(cp.col.Formula, cp.at+".formula", inAggregate, seen); inner != "" {
			return at, inner
		}
		return "", ""
	}
	if n.Op == "lookup" {
		// bind has refused a value that does not decode.
		if v, err := lv.c.p.lookupOf(n, at); err == nil {
			if at, inner := lv.innerRecord(v, at+".value", inAggregate, seen); inner != "" {
				return at, inner
			}
		}
	}
	if at, inner := lv.innerRecord(n.Where, at+".where", inAggregate, seen); inner != "" {
		return at, inner
	}
	inAggregate = inAggregate || aggregateOp(n)
	for i, a := range n.Args {
		if at, inner := lv.innerRecord(a, fmt.Sprintf("%s.args.%d", at, i), inAggregate, seen); inner != "" {
			return at, inner
		}
	}
	return "", ""
}

// levelCtx compiles a cell of a group, subtotal or total row from the
// level's aggregates.
type levelCtx struct {
	ctxBase
	lv       *level
	visiting map[string]bool
	// cond is compiling a having: a condition on an unset param matches every
	// group.
	cond bool
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
		return expr{sql: "NULL", typ: cp.path.field.Type, target: cp.path.target}, nil
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
		// A column has the value its cell shows wherever it is read: a having
		// that reads it does not make its conditions on an unset param match,
		// as a filter that reads a column does not (rowValue).
		e, err := compileFormula(&levelCtx{lv: lv, visiting: lc.visiting}, cp.col.Formula, cp.at+".formula")
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
	totalCur, err := lv.c.totalCurrency(e, cp.at+".formula")
	if err != nil {
		return expr{}, err
	}
	return withCurrency(expr{sql: lv.agg(name, aggregateSQL(cp.col.Aggregate, e.sql, cp.scale)), typ: aggregateType(cp.col.Aggregate, e.typ, cp.scale)}, totalCur), nil
}

// totalCurrency is the currency of a total of e. A total groups by its
// amounts' currencies, so a carried currency read inside it is a currency key
// of w (valueCurrencies); one read by subquery has no base row to read it on
// here. When its key was not made because the value does not compile, that
// is the error.
func (c *compiler) totalCurrency(e expr, at string) (currency, error) {
	if e.cur.sql == "" {
		return e.cur, nil
	}
	if err := c.valueCurErr[e.cur.id]; err != nil {
		return currency{}, err
	}
	return currency{}, invalid(at, "this amount's currency is read per row, and the total has no currency key to group it by")
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
	// The value compiled inside w, so a currency it carries is read there:
	// rows() kept it as the currency key the levels group by.
	if e.cur.sql != "" {
		ck, ok := lc.lv.c.valueCur[e.cur.id]
		if !ok {
			return expr{}, invalid(at, "this amount's currency is read per row, and the group has no currency key to group it by")
		}
		e.cur = currency{key: ck, id: e.cur.id}
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
	if n.Op == "lookup" {
		return lv.lookupReadsRowValue(n, inAggregate, seen)
	}
	// A record leaf has one value on its group's level and the levels
	// below it; above them the rows have no one record.
	if n.Record != "" {
		gi := lv.c.groupIndex(n.Record)
		return !inAggregate && gi >= 0 && lv.n < gi+1
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

// lookupReadsRowValue is readsRowValue for a lookup. Its paths read the
// looked-up source, never the report's rows; the report's values it reads
// ({col}) make it correlated, and blank where one of them is. A lookup that
// reads none is one value for the whole report, shown on every row.
func (lv *level) lookupReadsRowValue(n *Node, inAggregate bool, seen map[string]bool) bool {
	var walk func(n *Node) bool
	walk = func(n *Node) bool {
		switch {
		case n == nil:
			return false
		case n.Col != "" || n.Record != "":
			return lv.readsRowValue(n, inAggregate, seen)
		case n.Path != nil:
			return false
		}
		if n.Op == "lookup" {
			// bind has refused a value that does not decode.
			if v, err := lv.c.p.lookupOf(n, ""); err == nil && walk(v) {
				return true
			}
		}
		return walk(n.Where) || slices.ContainsFunc(n.Args, walk)
	}
	return walk(n)
}

// record reads the group record's field on the group's level and the levels
// below it, keyed by the group's key.
func (lc *levelCtx) record(group string, p Path, at string) (expr, error) {
	c := lc.lv.c
	gi, target, err := c.recordOf(group, at)
	if err != nil {
		return expr{}, err
	}
	bound, err := bindRecordPath(c.p.app, target, p, at)
	if err != nil {
		return expr{}, err
	}
	e := recordExpr(bound)
	// Outside the group (a level above it, the total) there is no one record,
	// and so no amount and no currency.
	key := "NULL"
	e.sql = "NULL"
	if lc.lv.n >= gi+1 {
		key = fmt.Sprintf("a.k%d", gi)
		e.sql = recordSQL(lc.lv.r.s, target, bound, key)
	}
	e.cur = recordCurrency(lc.lv.r.s, target, bound, key, recordCurrencyID(group, p))
	return e, nil
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
	cur, err := lc.lv.c.totalCurrency(e, at+".args.0")
	if err != nil {
		return expr{}, err
	}
	name := fmt.Sprintf("y%d", len(lc.lv.aggs))
	return withCurrency(expr{sql: lc.lv.agg(name, aggregateSQL(op, e.sql, scale)), typ: aggregateType(op, e.typ, scale)}, cur), nil
}

func (lc *levelCtx) window(op string, arg *Node, scale int, at string) (expr, error) {
	e, err := compileFormula(lc, arg, at+".args.0")
	if err != nil {
		return expr{}, err
	}
	return windowSQL(op, e, scale, lc.lv.frame, at)
}

func (lc *levelCtx) bind(v any) string                       { return lc.lv.r.s.bind(v) }
func (lc *levelCtx) fiscalStart() string                     { return lc.bind(int64(lc.lv.c.fiscal)) }
func (lc *levelCtx) filter() bool                            { return lc.cond }
func (lc *levelCtx) label(e expr) string                     { return lc.lv.c.textOf(lc.lv.r.s, e) }
func (lc *levelCtx) lookup(n *Node, at string) (expr, error) { return lookupIn(lc, lc.lv.r, n, at) }

// hoist computes a running value a lookup reads below the level's select.
func (lc *levelCtx) hoist(sql string) string { return lc.lv.frame.hoist("a", sql) }
