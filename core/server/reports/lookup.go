package reports

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
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

// lookupPicks are a lookup's picks: the smallest or largest matching value,
// their sum, or the number of matching rows with a value.
var lookupPicks = []string{"min", "max", "sum", "count"}

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
		walk(c.Rollup, fmt.Sprintf("columns.%d.rollup", i))
	}
	walk(p.spec.Filter, "filter")
	for i, g := range p.spec.Groups {
		walk(g.Label, fmt.Sprintf("groups.%d.label", i))
		if g.Fill != nil {
			walk(g.Fill.Where, fmt.Sprintf("groups.%d.fill.where", i))
		}
		walk(g.Having, fmt.Sprintf("groups.%d.having", i))
		if g.Opening != nil {
			for _, id := range g.Opening.cellIDs() {
				walk(g.Opening.Cells[id], fmt.Sprintf("groups.%d.opening.cells.%s", i, id))
			}
		}
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
// of its source that match its where, their sum (NULL when none match) or the
// number of them with a value (0 when none match). Every row of the
// source is read through its rule CTE, so the reader's list rule applies.
// Inside, a path reads the looked-up source; a column, a group record or a
// param reads the report's row through outer, which makes the subquery
// correlated. SQLite runs an uncorrelated scalar subquery once per statement.
// A column it reads may be a total (one value per row of its level) or a
// running value, which the outer select computes first (windowHoister).
func lookupIn(outer fctx, r *rowsCTE, n *Node, at string) (expr, error) {
	if n.Col != "" || n.Path != nil || n.Outer != nil || n.Param != "" || n.Subtotal != "" || n.Record != "" || n.Bound != "" {
		return expr{}, invalid(at, errNodeKind)
	}
	if len(n.Args) > 0 || n.Link != nil || n.Scale != nil {
		return expr{}, invalid(at, "lookup takes a source, a where, a value and a pick")
	}
	if n.Pick != "" && !slices.Contains(lookupPicks, n.Pick) {
		return expr{}, invalid(at+".pick", "pick is min, max, sum or count")
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
	v.sql = ownRows(sc, v.sql)
	if v.cur.sql != "" {
		v.cur.sql = ownRows(sc, v.cur.sql)
	}
	out := expr{typ: v.typ, target: v.target, cur: v.cur, agg: v.agg, win: v.win, options: v.options}
	where, unknown := "1", ""
	if n.Where != nil {
		w, err := compileWhere(&lookupCtx{outer: outer, r: r, sc: sc, src: src, inWhere: true}, n.Where, at)
		if err != nil {
			return expr{}, err
		}
		// WHERE drops a NULL condition as it drops a false one, and a bare
		// condition lets SQLite read each of its AND terms, so an index on
		// the looked-up source can serve one (linkedEq).
		if !w.unset {
			where = "(" + w.sql + ")"
		}
		out.agg, out.win = out.agg || w.agg, out.win || w.win
		if w.condMixed != "" {
			unknown = whereUnknown(outer, sc, w, at+".where")
		}
	}
	e, err := pickedLookup(outer, r, n, v, sc, where, out, at)
	if err != nil || unknown == "" || n.Pick == "count" {
		return e, err
	}
	// The context cannot name the where (whereProber): the lookup has no
	// value where its where was unknown, as a sum that met two currencies
	// has none, and the notice names its column or condition. A count is
	// never blank for currencies; it counts the rows the where held for.
	if e.unguarded == "" {
		return e.guarded(unknown, e.sql), nil
	}
	return e.guarded(orSQL(e.mixed, unknown), e.unguarded), nil
}

// whereProber is a context that probes, over the rows it compiles for,
// whether a lookup's where was unknown on one of them (a condition in it
// read a sum lookup that met two currencies), so the run's mixedCurrency
// notice names that where among its conditions. probeWhere reports false
// where it cannot probe the rows.
type whereProber interface {
	probeWhere(at, sql string) bool
}

// whereUnknown hands a lookup's where whose condition may read a sum lookup
// that met two currencies to the context the lookup is compiled for (the
// report's row a lookup inside a lookup reads is the outer one's). unknown
// is SQL true on a report row where the where was unknown on a looked-up
// row, rather than false, unless another term decided it; it returns that
// SQL when the context cannot probe it, "" when it took it.
func whereUnknown(outer fctx, sc *scope, w expr, at string) string {
	unknown := fmt.Sprintf("EXISTS (SELECT 1\n%s\nWHERE %s AND (%s) IS NULL)", sc.render(), w.condMixed, w.sql)
	row := outer
	for {
		inner, ok := row.(*lookupCtx)
		if !ok {
			break
		}
		row = inner.outer
	}
	if p, ok := row.(whereProber); ok && p.probeWhere(at, unknown) {
		return ""
	}
	return unknown
}

// pickedLookup is the lookup's value by its pick over the matching rows.
func pickedLookup(outer fctx, r *rowsCTE, n *Node, v expr, sc *scope, where string, out expr, at string) (expr, error) {
	switch n.Pick {
	case "count":
		// count skips a blank value; with no match it is 0. A value blank
		// because a sum lookup in it met two currencies has a value, only not
		// one to show, so it is counted (countedArg); the count names no
		// currency.
		counted := v.sql
		if v.mixed != "" {
			counted = fmt.Sprintf("(CASE WHEN %s THEN 1 ELSE %s END)", v.mixed, v.sql)
		}
		return expr{sql: fmt.Sprintf("(SELECT count(%s)\n%s\nWHERE %s)", counted, sc.render(), where), typ: Number, agg: out.agg, win: out.win}, nil
	case "sum":
		if v.typ != Number && v.typ != Money {
			return expr{}, invalid(at+".value", "pick sum adds numbers or amounts")
		}
		return summedLookup(outer, r, n, v, sc, where, out), nil
	}
	if v.typ == Money && v.cur.sql != "" {
		return pickedAmount(outer, r, n, v, sc, where, out), nil
	}
	pick := "min"
	if n.Pick == "max" {
		pick = "max"
	}
	from := sc.render()
	value := fmt.Sprintf("(SELECT %s(%s)\n%s\nWHERE %s)", pick, v.sql, from, where)
	if v.shown != "" {
		out.shown = pickedShown(n, v, from, where)
	}
	return out.guarded(innerMixed(v, from, where), value), nil
}

// innerMixed is SQL that is true where a matching row's value is blank
// because a sum lookup inside it met two currencies, "" when none can: the
// lookup over those values has no value there either (R3), since the blank
// one would be left out of it without a word.
func innerMixed(v expr, from, where string) string {
	if v.mixed == "" {
		return ""
	}
	return fmt.Sprintf("EXISTS (SELECT 1\n%s\nWHERE %s AND %s)", from, where, v.mixed)
}

// guarded is e with value as its value: blank where mixed holds, when it
// can. unguarded keeps value, so a hoist computes the value and its mixed
// check once each and builds the blank from the two (rowsCtx.hoistLookup).
func (e expr) guarded(mixed, value string) expr {
	e.mixed, e.unguarded, e.sql = mixed, "", value
	if mixed != "" {
		e.unguarded = value
		e.sql = fmt.Sprintf("(CASE WHEN %s THEN NULL ELSE %s END)", mixed, value)
	}
	return e
}

// ownRows ties a lookup's value to the rows of its source. SQLite counts an
// aggregate whose argument reads no column of its own query (a value that is
// a lookup reading only the report's row) as an aggregate of the query
// around it, which folds the report into one row.
func ownRows(sc *scope, sql string) string {
	return fmt.Sprintf("(CASE WHEN %s.id IS NOT NULL THEN %s END)", sc.alias, sql)
}

// summedLookup adds the value over the matching rows, blank when none match.
// An amount whose currency each row carries is added only in one currency:
// where the matching rows hold two, the sum is blank and mixed says so (R3).
// Its currency is the rows' one currency, read beside it; on a base row
// inside a total, the key valueCurrencies gave the lookup, as pickedAmount's.
func summedLookup(outer fctx, r *rowsCTE, n *Node, v expr, sc *scope, where string, out expr) expr {
	from := sc.render()
	out.target, out.options, out.shown = nil, nil, ""
	sum := fmt.Sprintf("(SELECT sum(%s)\n%s\nWHERE %s)", v.sql, from, where)
	// A matching value that is itself a sum over two currencies is blank, so
	// it adds nothing below; the sum over it has no value either (R3).
	inner := innerMixed(v, from, where)
	if v.typ != Money || v.cur.sql == "" {
		return out.guarded(inner, sum)
	}
	matched := where + " AND " + v.sql + " IS NOT NULL"
	mixed := orSQL(fmt.Sprintf("((SELECT count(DISTINCT %s)\n%s\nWHERE %s) > 1)", v.cur.sql, from, matched), inner)
	out = out.guarded(mixed, sum)
	if inner == "" {
		// One read of the matching rows: sum of no rows is NULL, count 0 and
		// min NULL, as the three subqueries above give.
		hit := "CASE WHEN " + v.sql + " IS NOT NULL THEN " + v.cur.sql + " END"
		out.scan = fmt.Sprintf("SELECT sum(%s) AS u, (count(DISTINCT %s) > 1) AS m, min(%s) AS c\n%s\nWHERE %s",
			v.sql, hit, hit, from, where)
	}
	id := lookupCurrencyID(n)
	out.cur = currency{sql: fmt.Sprintf("(SELECT min(%s)\n%s\nWHERE %s)", v.cur.sql, from, matched), id: id, field: v.cur.field}
	if ck, ok := r.c.valueCur[id]; ok && !inLevel(outer) {
		out.cur = currency{key: ck, id: id, field: v.cur.field}
	}
	return out
}

// pickedShown is the shown form of a looked-up value that mixes a select's
// values with other text, read from the row whose stored value the lookup
// picks: the smallest (min) or largest (max) one, as min() and max() pick it,
// ties broken by the shown form.
func pickedShown(n *Node, v expr, from, where string) string {
	dir := "ASC"
	if n.Pick == "max" {
		dir = "DESC"
	}
	return fmt.Sprintf("(SELECT %s\n%s\nWHERE %s AND %s IS NOT NULL\nORDER BY %s %s, %s LIMIT 1)", v.shown, from, where, v.sql, v.sql, dir, v.shown)
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
	out = out.guarded(innerMixed(v, from, where), pickOne(v.sql))
	out.cur = currency{sql: pickOne(v.cur.sql), id: id, field: v.cur.field}
	if ck, ok := r.c.valueCur[id]; ok && !inLevel(outer) {
		out.cur = currency{key: ck, id: id, field: v.cur.field}
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
	return paramLeaf(l.r.c, l.r.s, l, id, v)
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
	// A currency field is the looked-up row's own currency, read beside the
	// amount; lookupIn picks the two from one row. The column names it for
	// combining.
	if l.inWhere && bound.endsAtLink {
		if col := l.storedLinkCol(hops); col != "" {
			e.link = storedLink{col: col, sql: e.sql, sc: l.sc}
		}
	}
	switch cur, fixed := amountCurrencyOf(l.sc, hops, bound); {
	case cur != "":
		e.cur = currency{sql: cur, id: "row:" + cur, field: currencyField(bound)}
	case fixed != nil:
		e.cur = currency{fixed: fixed}
	}
	return e, nil
}

// storedLinkCol is the stored relation column a one-hop link path follows,
// on the lookup scope's own base row, or "" where that column may hold NULL:
// a view's, or one read through a join (a longer path), which is NULL when
// the middle row is blank or hidden while the path's value is blank. A base
// collection stores a blank relation as ”.
func (l *lookupCtx) storedLinkCol(hops []hopPlan) string {
	if len(hops) != 1 {
		return ""
	}
	col, err := l.r.c.p.app.FindCachedCollectionByNameOrId(hops[0].from.Collection)
	if err != nil {
		// Without the bound the lookup is still right, only slower.
		log.Warn("lookup link column not bounded", "collection", hops[0].from.Collection, "err", err)
		return ""
	}
	if col.IsView() {
		return ""
	}
	return q(l.sc.alias, hops[0].link.Field)
}

// When one side of eq is a link path in a lookup's where, eq adds a bound:
// the stored relation column lies between the other side and itself, which SQLite serves from an index on that column, so a lookup
// correlated with the report's row searches the looked-up source instead of
// scanning it for every row. The link path reads the linked row's id through
// a join on that column, blank when the reader cannot see the row, so the
// bound holds wherever the equality does: a value V gives the column V, and a
// blank V gives every column (” up to any blob, which sorts after all text).
//
// linkedEq is eq with that bound, "" when neither side reads through a
// stored column. The other side's value V is read three times; where V
// reads nothing of the looked-up row, it is computed once per report row in
// a one-row table of the lookup's scope (oneValue), and the three read it
// there. A V that reads the looked-up row is one of its fields (a path) or
// a condition on it, which a table in the same FROM cannot read.
func linkedEq(n *Node, args []expr) string {
	for i, a := range args[:2] {
		if a.link.col == "" || a.link.sql != a.sql {
			continue
		}
		v := args[1-i].sql
		if !readsLookedUpRow(n.Args[1-i]) {
			v = a.link.sc.oneValue(v)
		}
		return fmt.Sprintf("(%s IS %s AND %s >= COALESCE(%s, '') AND %s <= COALESCE(%s, x'ff'))", a.sql, v, a.link.col, v, a.link.col, v)
	}
	return ""
}

// oneValue computes sql once for each row the scope's query is run for, in
// a one-row table joined to the scope, and returns the reference to it. One
// row never changes the scope's rows. SQLite runs the table as a co-routine
// first, so an index on the scope's table still serves a bound on it.
func (sc *scope) oneValue(sql string) string {
	alias := sc.s.alias("lb")
	sc.sql = append(sc.sql, fmt.Sprintf("JOIN (SELECT %s AS v) %s", sql, alias))
	return alias + ".v"
}

// readsLookedUpRow is true when a node in a lookup's where reads the
// looked-up row: a path, or an exists over a link of it. A lookup inside it
// reads its own source, and every other leaf the report's row.
func readsLookedUpRow(n *Node) bool {
	switch {
	case n == nil, n.Op == "lookup":
		return false
	case n.Path != nil, n.Link != nil, n.Op == "exists", n.Op == "notExists":
		return true
	}
	return slices.ContainsFunc(n.Args, readsLookedUpRow)
}

// outerPath reads a path on the report's row: what a path means in the
// context the outermost lookup sits in (a detail row's value, a joined row
// in the filter, blank on a group, subtotal or total row). It is bound on
// the report's source, to one row (R4).
func (l *lookupCtx) outerPath(p Path, at string) (expr, error) {
	base := l.r.c.p.base
	if outerToMany(base, p) {
		return expr{}, invalid(at+".outer", "outer reads one value of the report's row: a to-many link is not one")
	}
	if _, err := bindValuePath(l.r.c.p.app, base, p, at+".outer", pathValue); err != nil {
		return expr{}, err
	}
	var row fctx = l
	for {
		inner, ok := row.(*lookupCtx)
		if !ok {
			break
		}
		row = inner.outer
	}
	return row.path(p, at+".outer")
}

// outerToMany is true when an outer path follows a to-many link from the
// report's source. A link it cannot follow is left to bindValuePath.
func outerToMany(base *Source, p Path) bool {
	src := base
	for _, h := range p {
		if h.Link == "" || src == nil {
			continue
		}
		l := src.link(h.Link)
		if l == nil {
			return false
		}
		if l.Cardinality != ToOne {
			return true
		}
		src = lookupRegistered(l.Target)
	}
	return false
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

// currencySQL reads a currency key on the report's row, as a column does:
// through outer.
func (l *lookupCtx) currencySQL(cur currency) string { return l.outer.currencySQL(cur) }

// compiled is told of each node compiled in the lookup (leafTracker). A
// lookup is computed once per group key only when every node it compiled is
// of a kind known to read nothing of the report's row but the group record
// (rowFreeNode); any other node, of a kind known to read the row or of a kind
// not known here, counts as a read of the row, whether or not its leaf
// records one.
func (l *lookupCtx) compiled(n *Node) {
	if rowFreeNode(n) {
		return
	}
	var row fctx = l
	for {
		inner, ok := row.(*lookupCtx)
		if !ok {
			break
		}
		row = inner.outer
	}
	if rc, ok := row.(*rowsCtx); ok {
		rc.reads.value()
	}
}

// rowFreeNode is true for a node inside a lookup that reads nothing of the
// report's row but its group record (which records its key): a record, a
// param, a constant, a path (on the looked-up source), a lookup or an exists
// (whose own nodes are told apart), and an operator over its arguments.
func rowFreeNode(n *Node) bool {
	switch {
	case n.Op == "min" || n.Op == "max":
		// One value is a total; two or more, the smallest or largest.
		return len(n.Args) != 1
	case n.Op != "":
		return rowFreeOps[n.Op]
	case n.Record != "", n.Param != "", n.Path != nil:
		return true
	}
	return n.Value != nil && n.Col == "" && n.Outer == nil && n.Subtotal == ""
}

// rowFreeOps are the operators that compute from their arguments alone.
var rowFreeOps = map[string]bool{
	"lookup": true, "exists": true, "notExists": true,
	"add": true, "sub": true, "mul": true, "div": true, "round": true,
	"eq": true, "ne": true, "lt": true, "lte": true, "gt": true, "gte": true,
	"between": true, "in": true, "and": true, "or": true, "not": true,
	"concat": true, "upper": true, "lower": true, "contains": true, "startsWith": true,
	"year": true, "quarter": true, "month": true, "fiscalYear": true, "fiscalPeriod": true,
	"addDays": true, "addMonths": true, "diffDays": true,
	"if": true, "case": true, "coalesce": true, "isEmpty": true,
}
