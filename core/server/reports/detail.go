package reports

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
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
	// idText reads a relation inside a text operator as its record's id, not
	// its label: a detail row's open path holds ids (R2).
	idText bool
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
	// compiled holds the formulas compiled on the frame's rows (formula).
	compiled map[*Node]expr
	// recs joins the group records the frame's rows read under its layers;
	// nil for a frame that reads them by subquery.
	recs *recordJoins
	// name prefixes the hoisted values' names (wh when "").
	name string
	// win is true once a running value or a percent (expr.win) is compiled
	// over the frame's rows (rowsCtx.window).
	win bool
	// lookups holds each lookup compiled on the frame's rows by its node, so
	// the copies of one lookup in a formula (an if that reads it in both
	// branches) are computed once (rowsCtx.lookup).
	lookups map[string]detailLookup
	// paged is a frame whose select is cut to a page before its cells are
	// computed (pagedCells): a running value or a percent reads every row, so
	// it is kept in windows and read as w.ww<n>.
	paged   bool
	windows []string
	// wholeReport is set when a percent of the whole report was compiled on
	// the frame: its value on a row reads the rows of every group, so no group
	// may be left out below it (leafRows).
	wholeReport bool
}

// detailLookup is a lookup compiled on a frame's detail rows: its value as
// the frame computes it, what it reads of the row and whether it was hoisted
// (both recorded again in the lookup around a copy), and the wheres it handed
// the detail select to probe (at relative to the lookup's at).
type detailLookup struct {
	e       expr
	reads   *rowReads
	hoisted bool
	at      string
	wheres  []condProbe
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
			prefix := f.name
			if prefix == "" {
				prefix = "wh"
			}
			name = fmt.Sprintf("%s%d", prefix, len(f.hoistedAs))
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
	return f.layers(f.rows(table, alias), table+" "+alias, alias, nil)
}

// rows is table read as alias, with the group records the frame joins.
func (f *windowFrame) rows(table, alias string) string {
	from := table + " " + alias
	if len(f.hoisted) == 0 && table == alias && !f.recs.any() {
		from = table
	}
	// The records first: a hoisted value may read one.
	if f.recs.any() {
		from = f.recs.from(from)
	}
	return from
}

// layers adds the hoisted layers that pick keeps (every layer when pick is
// nil) to from, in the order they were hoisted. base is the rows a keyed
// layer groups.
func (f *windowFrame) layers(from, base, alias string, pick func(int) bool) string {
	for i, h := range f.hoisted {
		if pick != nil && !pick(i) {
			continue
		}
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
	return rc.formula(cp.col.Formula, cp.at+".formula")
}

// formula compiles n on a detail row once for the frame: a column, a sort
// key and the detail label that read one formula share one compile. Each
// compile binds its own names, so a lookup compiled twice would be hoisted
// and computed twice. Inside a lookup a formula compiles again, so the
// lookup records what it reads of the row; a running value too, since it
// reads the frame's order, which grows while the sort keys compile.
func (rc *rowsCtx) formula(n *Node, at string) (expr, error) {
	f := rc.detail
	if f == nil || f.checking || rc.reads != nil {
		return compileFormula(rc, n, at)
	}
	if e, ok := f.compiled[n]; ok {
		return e, nil
	}
	e, err := compileFormula(rc, n, at)
	if err == nil && !e.win {
		if f.compiled == nil {
			f.compiled = map[*Node]expr{}
		}
		f.compiled[n] = e
	}
	return e, err
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
	id := recordCurrencyID(group, p)
	read := func(p *pathPlan) string { return recordSQL(rc.r.s, target, p, key) }
	// On a detail row the record is joined under the frame once, however many
	// cells, sort keys and window orders read it. Inside a lookup it stays a
	// subquery on the group key, which lets the lookup be computed once per
	// group key (rowReads.groupKeys).
	if f := rc.detail; f != nil && f.recs != nil && !f.checking && rc.reads == nil {
		read = f.recs.reader(gi, target)
	}
	e.sql = read(bound)
	if ck, ok := c.valueCur[id]; ok {
		e.cur = currency{key: ck, id: id}
	} else {
		e.cur = recordCurrencyBy(bound, key, id, read)
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
	e, err := rc.aggregateArg(rc.r, rc.visiting, rc.idText, op, arg, scale, at)
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
	blank := e.blankFor()
	if blank != "" {
		// A sum lookup that met two currencies has no value, so its total
		// over the one row has none either (R3), not 0.
		sql = fmt.Sprintf("(CASE WHEN %s THEN NULL ELSE %s END)", blank, sql)
	}
	return withCurrency(expr{sql: sql, typ: aggregateType(op, e.typ, scale), mixed: blank}, e.cur), nil
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
	w, err := windowSQL(op, e, scale, rc.detail, rc.bind, at)
	rc.detail.win = rc.detail.win || w.win
	return w, err
}

func (rc *rowsCtx) bind(v any) string              { return rc.r.s.bind(v) }
func (rc *rowsCtx) fiscalStart() string            { return rc.bind(int64(rc.r.c.fiscal)) }
func (rc *rowsCtx) readerDay(day string) *zonedDay { return rc.r.c.readerDay(day) }
func (rc *rowsCtx) filter() bool                   { return false }
func (rc *rowsCtx) label(e expr) string {
	if rc.idText {
		return e.sql
	}
	return rc.r.c.textOf(rc.r.s, e)
}

// lookup compiles a lookup on the row, recording what it reads of the row
// (hoistLookup reads that). A lookup inside it, through a column's formula,
// adds its reads to this one's; one hoisted is a value of the row.
func (rc *rowsCtx) lookup(n *Node, at string) (expr, error) {
	f := rc.detail
	memo := f != nil && !f.checking
	key := lookupCurrencyID(n)
	if m, ok := f.lookupOf(key); memo && ok {
		rc.reads.add(m.reads)
		if m.hoisted {
			rc.reads.value()
		}
		for _, w := range m.wheres {
			rc.probeWhere(at+strings.TrimPrefix(w.at, m.at), w.sql)
		}
		return m.e, nil
	}
	around := rc.reads
	reads := &rowReads{}
	rc.reads = reads
	wheres := len(rc.r.c.detailWheres)
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
	if memo {
		if f.lookups == nil {
			f.lookups = map[string]detailLookup{}
		}
		f.lookups[key] = detailLookup{e: e, reads: reads, hoisted: hoisted, at: at, wheres: slices.Clone(rc.r.c.detailWheres[wheres:])}
	}
	return e, nil
}

// window keeps a running value or a percent of a paged frame to compute over
// every row, below the page cut, and returns its reference; any other frame
// computes it in place.
func (f *windowFrame) window(sql string) string {
	if !f.paged {
		return sql
	}
	i := slices.Index(f.windows, sql)
	if i < 0 {
		i = len(f.windows)
		f.windows = append(f.windows, sql)
	}
	return fmt.Sprintf("w.ww%d", i)
}

// windowRef is a reference to a paged frame's running value or percent.
var windowRef = regexp.MustCompile(`\bw\.ww(\d+)\b`)

// inline writes each running value or percent back in the place that reads
// it: the form of a select that is not cut before its cells.
func (f *windowFrame) inline(sql string) string {
	return windowRef.ReplaceAllStringFunc(sql, func(ref string) string {
		n, _ := strconv.Atoi(strings.TrimPrefix(ref, "w.ww"))
		return f.windows[n]
	})
}

// readsHoisted reports whether sql reads the frame's hoisted value name on a
// row (w.wh12 does not read w.wh1).
func readsHoisted(sql, name string) bool {
	ref := "w." + name
	for at := 0; ; {
		i := strings.Index(sql[at:], ref)
		if i < 0 {
			return false
		}
		start, end := at+i, at+i+len(ref)
		before := start == 0 || !isNamePart(sql[start-1])
		after := end == len(sql) || !isNamePart(sql[end])
		if before && after {
			return true
		}
		at = end
	}
}

func isNamePart(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

// earlyLayers marks the hoisted layers a paged select computes before its
// page is cut: a keyed layer (it reads every row of its group), a layer that
// one of texts reads (a sort key, a running value, a probe mark), and a layer
// that an early layer reads. The other layers are computed for the page's
// rows only.
func (f *windowFrame) earlyLayers(texts []string) []bool {
	early := make([]bool, len(f.hoisted))
	read := slices.Clone(texts)
	for i, h := range f.hoisted {
		if len(h.keys) > 0 {
			early[i] = true
			read = append(read, h.cols...)
		}
	}
	for changed := true; changed; {
		changed = false
		for i, h := range f.hoisted {
			if early[i] || !slices.ContainsFunc(h.names, func(name string) bool {
				return slices.ContainsFunc(read, func(sql string) bool { return readsHoisted(sql, name) })
			}) {
				continue
			}
			early[i], changed = true, true
			read = append(read, h.cols...)
		}
	}
	return early
}

// lookupOf is the lookup compiled on the frame's rows under key, if any.
func (f *windowFrame) lookupOf(key string) (detailLookup, bool) {
	if f == nil {
		return detailLookup{}, false
	}
	m, ok := f.lookups[key]
	return m, ok
}

// probeWhere takes a lookup's where on a detail row, which the detail
// select probes on each row it shows, or on a row of w inside a total,
// which the group statement probes over w. One compiled only to check a
// column is never run.
func (rc *rowsCtx) probeWhere(at, sql string) bool {
	switch {
	case rc.detail == nil:
		rc.r.c.probe(at, "SELECT 1 FROM w WHERE "+sql)
	case !rc.detail.checking && !slices.Contains(rc.r.c.detailWheres, condProbe{at: at, sql: sql}):
		rc.r.c.detailWheres = append(rc.r.c.detailWheres, condProbe{at: at, sql: sql})
	}
	return true
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
	return hoistedLookup(rc.detail, "w", reads.groupKeys(), e), true
}

// hoistedLookup computes a lookup's parts in one layer of frame (once per
// distinct value of keys, when there are keys) and reads them under alias.
// A value with a mixed check is hoisted as its two parts, the value before
// the check and the check, and the blank is built from the two above the
// layer, so each is computed once.
func hoistedLookup(frame *windowFrame, alias string, keys []string, e expr) expr {
	return hoistLookupParts(e, func(parts []lookupPart) []string {
		sqls := make([]string, len(parts))
		for i, p := range parts {
			sqls[i] = p.sql
		}
		return frame.hoistLayer(alias, keys, sqls...)
	})
}

// lookupPart is one part of a lookup that a context computes once: col names
// its role (u the value before the mixed check, s the value when there is no
// such check, m the check, c the currency, sh the shown value).
type lookupPart struct{ col, sql string }

// hoistLookupParts computes a lookup's parts once, through hoist, which
// returns a reference for each part in order, and rebuilds the lookup over
// the references. A value with a mixed check is computed as its two parts
// and the blank is built from the two references, so each is computed once.
// The detail and level frames (hoistedLookup) and the layout
// (layoutCtx.lookup) share this rule, so they cannot drift apart.
func hoistLookupParts(e expr, hoist func([]lookupPart) []string) expr {
	if e.mixed != "" && e.unguarded == "" {
		log.Warn("a lookup with a mixed check has no unguarded value; its check is computed twice")
	}
	value, valueCol := e.sql, "s"
	if e.unguarded != "" {
		value, valueCol = e.unguarded, "u"
	}
	fields := []*string{&value, &e.mixed, &e.cur.sql, &e.shown}
	cols := []string{valueCol, "m", "c", "sh"}
	var parts []lookupPart
	for i, f := range fields {
		if *f != "" {
			parts = append(parts, lookupPart{col: cols[i], sql: *f})
		}
	}
	refs := hoist(parts)
	for _, f := range fields {
		if *f != "" {
			*f, refs = refs[0], refs[1:]
		}
	}
	if e.unguarded == "" {
		e.sql = value
		return e
	}
	return e.guarded(e.mixed, value)
}

// windowSQL is a running value or a percent over a frame. A running sum
// adds in the frame's order within the parent; a percent divides by the sum
// over the parent or over the whole report, with no figure when that sum is
// zero. bind binds the percent's scale on the statement the value is in.
func windowSQL(op string, e expr, scale int, f *windowFrame, bind func(any) string, at string) (expr, error) {
	if !isNumeric(e.typ) || e.win {
		return expr{}, invalid(at, op+" needs a number that is not itself a running value")
	}
	if op == "percentOfSection" {
		if e.typ == Bool {
			return expr{}, invalid(at, op+" needs a number, not a yes/no value")
		}
		// A placeholder: the figure needs the section's total with its within
		// rows, which only the layout's legs hold, so assemble fills it
		// (sectionPercentCell).
		return expr{sql: "NULL", typ: Percent, win: true}, nil
	}
	switch op {
	case "runningSum":
		over := "OVER (" + partitionBy(f.parent) + " ORDER BY " + strings.Join(f.order, ", ") + " ROWS UNBOUNDED PRECEDING)"
		return withCurrency(expr{sql: f.window("SUM(" + num(e) + ") " + over), typ: sumType([]expr{e}), win: true}, e.cur), nil
	case "percentOfParent":
		return expr{sql: f.window(fmt.Sprintf("tc_pct(%s, SUM(%s) OVER (%s), %s)", num(e), num(e), partitionBy(f.parent), bind(int64(scale)))), typ: Percent, win: true}, nil
	}
	f.wholeReport = true
	return expr{sql: f.window(fmt.Sprintf("tc_pct(%s, SUM(%s) OVER (%s), %s)", num(e), num(e), partitionBy(f.total), bind(int64(scale)))), typ: Percent, win: true}, nil
}

func partitionBy(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	return "PARTITION BY " + strings.Join(keys, ", ")
}

// openPathRe is the characters an open path may hold: no scheme, query,
// fragment, dot or space reaches the router.
var openPathRe = regexp.MustCompile(`^[A-Za-z0-9_/-]+$`)

// openPath keeps a detail row's open value when it is a path under the
// report's package: "<slug>/" then segments that are none of them empty.
func openPath(slug, v string) (string, bool) {
	rest, ok := strings.CutPrefix(v, slug+"/")
	if !ok || rest == "" || !openPathRe.MatchString(v) || strings.Contains(v, "//") || strings.HasSuffix(v, "/") {
		return "", false
	}
	return v, true
}
