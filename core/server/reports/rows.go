package reports

import (
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// compiler compiles one run of a plan: params resolved, fiscal start known.
type compiler struct {
	p      *plan
	app    core.App // nil when dry (Validate)
	info   *core.RequestInfo
	params map[string]paramValue
	fiscal int
	// fixedCur is each source's fixed currency as this read's reader got it
	// (fixedCurrency). It outlives a statement, so rows() leaves it.
	fixedCur map[string]string
	// currencies names w's currency keys; every total groups by them, so
	// amounts in different currencies are never added together.
	currencies []string
	// currencyOf maps a money column's id to its currency: a currency key
	// of w, or a source's fixed currency.
	currencyOf map[string]currency
	// valueCur maps a carried currency's id (a record's or a looked-up row's
	// own currency) to the currency key of w that holds it, for every such
	// amount read inside a total.
	valueCur map[string]string
	// valueCurErr is why a carried currency read inside a total got no key:
	// its record leaf or lookup does not compile.
	valueCurErr map[string]error
	// sectionCur is each column's currency on a first-level group row, which
	// a layout's section subtotal reads.
	sectionCur map[string]currency
	// carried holds the columns whose cells carry their own currency on some
	// row (vcur<idx>, the cell <id>@currency).
	carried map[string]bool
	// detailTypes and levelTypes are each column's type on detail rows
	// and on group, subtotal and total rows.
	detailTypes map[string]FieldType
	levelTypes  map[string]FieldType
	// sectionMixed marks the columns whose first-level cell may be blank
	// because a sum lookup met two currencies (vmx<idx>), and pivotMixed the
	// pivot cells that may be (pvm<j>): a computed layout row reads the
	// helper beside the section's cell.
	sectionMixed map[string]bool
	pivotMixed   bool
	// groupTargets is the source each group's key links to (nil for a group
	// that is not by a link).
	groupTargets []*Source
	// groupOptions are the options of each group whose key is a select's
	// value; groupShown marks a group whose key mixes a select's values with
	// other text, whose shown form w holds as kl<n>. Such groups are shown,
	// and ordered, by their option labels.
	groupOptions [][]Option
	groupShown   []bool
	// labels is each group's label as it compiled: its type and, for an
	// amount, its currency. It outlives a statement (assembly reads it after
	// every pass), so rows() leaves it.
	labels []labelPlan
	// formulaOptions holds the options of each formula column whose value is
	// a select's, so its cells and group labels show option labels.
	formulaOptions map[string][]Option
	// detailLabel is what the report's detail label compiled to (the detail
	// select's __label); nil when the first column labels detail rows.
	detailLabel *labelPlan
	// viewSort are the run's viewer sort keys (options.sort), which order
	// detail rows before the spec's keys and never the window order.
	viewSort []sortPlan
	// sortable holds the columns a viewer may sort by, as detailSelect
	// compiled them.
	sortable map[string]bool
	// labelTypes holds the type of the record label of each column whose
	// values point at a record (vl<idx>, the cell <id>@label).
	labelTypes map[string]FieldType
	// fillSets names the keyset CTE (f<L>) of each level whose group fills in
	// its records (fillKeys); the level below joins its records to its rows.
	fillSets map[int]string
	// expanding holds the params whose group references are being expanded,
	// so options whose where reads such a param back are refused, not
	// expanded forever.
	expanding map[string]bool
	// condProbes are the conditions of the statement being built that read a
	// sum lookup which may meet two currencies (condProbe).
	condProbes []condProbe
	// probed holds each probe condProbes has (at and sql), so one is added
	// once.
	probed map[string]bool
	// detailSortMixed are the spec sort keys of the detail select that read
	// such a sum, and the lookups' wheres on its rows that may (rowsCtx
	// probeWhere): sql names the select's column that is 1 where one did.
	detailSortMixed []condProbe
	// detailWheres are the lookups' wheres the detail select being built
	// probes on each of its rows (whereProber): sql is true on a row where
	// one was unknown.
	detailWheres []condProbe
	// layoutNodes counts every formula node compiled for the layout's
	// computed rows, and layoutCounted what the layout's caps counted
	// (layoutLegs), so a test pins the compile work to the caps.
	layoutNodes, layoutCounted int
}

// labelPlan is what a group's label compiled to. recordCurrency says an
// amount label carries its own currency (a record's, or a looked-up row's),
// which each row holds (__labelcur).
type labelPlan struct {
	typ            FieldType
	cur            currency
	recordCurrency bool
}

// newCompiler reads the fiscal start month for info's reader. A dry compile
// (nil app or nil info) has no reader to read it for, so its start is 1. A
// read of a report builds its compiler through readerCompiler, which reads
// the month once for the presets and the formulas; do not call newCompiler
// on a read path, or the month is read twice and the two can disagree.
func newCompiler(p *plan, app core.App, info *core.RequestInfo, params map[string]paramValue) *compiler {
	fiscal := 1
	if app != nil && info != nil {
		fiscal = fiscalStartOf(p.base, app, info)
	}
	return newCompilerWithFiscal(p, app, info, params, fiscal)
}

// newCompilerWithFiscal is newCompiler with a fiscal start month the caller
// already read, so a read that also resolves period presets uses one value
// for both.
func newCompilerWithFiscal(p *plan, app core.App, info *core.RequestInfo, params map[string]paramValue, fiscal int) *compiler {
	c := &compiler{p: p, app: app, info: info, params: params, fiscal: fiscal, fixedCur: map[string]string{}, currencyOf: map[string]currency{}, valueCur: map[string]string{}, valueCurErr: map[string]error{}, sectionCur: map[string]currency{}, carried: map[string]bool{}, labels: make([]labelPlan, len(p.groups)), labelTypes: map[string]FieldType{}, expanding: map[string]bool{}}
	if params == nil {
		// Dry: each param has its default, as in a run that posts none; bind
		// has checked the defaults.
		c.params, _ = resolveParams(p.spec.Params, nil, anyDay)
	}
	return c
}

// fiscalStartOf is the source's fiscal start month for reader, 1 when it has
// none or gives a month outside 1-12.
func fiscalStartOf(src *Source, app core.App, reader *core.RequestInfo) int {
	if src.FiscalStartMonth == nil {
		return 1
	}
	if m := src.FiscalStartMonth(app, reader); m >= 1 && m <= 12 {
		return m
	}
	return 1
}

// dry is true for a compile with no reader (Validate): no statement runs, and
// no source function may be called.
func (c *compiler) dry() bool {
	return c.app == nil || c.info == nil
}

// fixedCurrency is src's fixed currency for this read's reader, "" when src
// has none or the compile is dry. It calls src.Currency the first time the
// read needs it and keeps the code for the rest of the read (the compiler
// lives for one Run or one ReadParamOptions): read per column, row or label,
// a concurrent change of the package's setting could put two codes in one
// result. The compiler is used from one goroutine only, so the memo has no
// lock.
func (c *compiler) fixedCurrency(src *Source) string {
	if src.Currency == nil || c.dry() {
		return ""
	}
	if code, ok := c.fixedCur[src.Key]; ok {
		return code
	}
	code := src.Currency(c.app, c.info)
	c.fixedCur[src.Key] = code
	return code
}

// rowsCTE is the CTE `w`: one row per base row the reader may list that
// passes the filter, with every value a later pass reads: group keys k<n>,
// currency keys ck<n>, the pivot key pk, path columns c<idx>, hidden path
// values h<n>, to-many parent ids p<idx> (and cs/cn for an average), and
// nest parent ids n<level>.
type rowsCTE struct {
	c      *compiler
	s      *stmt
	root   *scope
	raw    map[string]string // name → its expression inside w
	order  []string
	byExpr map[string]string
	filter string
	// filterMixed is the filter's condMixed: true on a row where a sum
	// lookup the filter reads met two currencies.
	filterMixed string
	// wheres are the lookups' wheres compiled on the report's base rows (in
	// w or in the filter) that may be unknown on one (whereProber); the
	// group statement probes them (probeWheres).
	wheres []rowWhere
}

// rowWhere is a lookup's where to probe over the report's base rows: sql is
// true on a row where it was unknown. inFilter is a lookup in the filter,
// probed over the rows the filter decides on; any other, over the rows it
// keeps.
type rowWhere struct {
	at, sql  string
	inFilter bool
}

// probeWheres adds the probes of the lookups' wheres on the base rows.
func (r *rowsCTE) probeWheres() {
	for _, w := range r.wheres {
		cond := w.sql
		if !w.inFilter && r.filter != "" {
			cond = "(" + r.filter + ") AND " + cond
		}
		r.c.probe(w.at, "SELECT 1 "+r.root.render()+"\nWHERE "+cond)
	}
}

// probeWhere takes a lookup's where on the report's base row; a joined
// row's (a fill's target, an exists' link) is not one, and its lookup is
// left to show no value there.
func (j *joinCtx) probeWhere(at, sql string) bool {
	if j.sc != j.r.root {
		return false
	}
	j.r.wheres = append(j.r.wheres, rowWhere{at: at, sql: sql, inFilter: j.isFilter})
	return true
}

func (c *compiler) rows(s *stmt) (*rowsCTE, error) {
	// Each statement builds its own w; the currency keys come out the same.
	c.currencies, c.currencyOf, c.valueCur, c.valueCurErr = nil, map[string]currency{}, map[string]string{}, map[string]error{}
	r := &rowsCTE{c: c, s: s, root: newScope(s, c.p.base), raw: map[string]string{}, byExpr: map[string]string{}}
	r.add("__id", q(r.root.alias, "id"))
	// The currency keys of the amounts the report reads by path come first, so
	// a group key, the filter and a fill that read one of them have its key
	// (joinCtx.path).
	for _, cp := range c.p.cols {
		if cp.path == nil || cp.nest > 0 {
			continue
		}
		r.colValue(cp)
		if cp.tm < 0 {
			c.currencyOf[cp.col.ID] = r.pathCurrency(cp.path)
		} else if cp.path.field.Type == Money && cp.path.src.Currency != nil {
			// bind refuses a to-many total of amounts with a currency field.
			c.currencyOf[cp.col.ID] = currency{fixed: cp.path.src}
		}
	}
	// A formula's amounts group by their currency as a path column's do. The
	// keys are all known here, before any level groups by them.
	for _, cp := range c.p.cols {
		r.formulaCurrencies(cp.col.Formula)
	}
	for _, g := range c.p.groups {
		r.formulaCurrencies(g.group.Label)
		r.formulaCurrencies(g.group.Having)
	}
	for _, s := range c.p.sort {
		r.formulaCurrencies(s.formula)
	}
	if d := c.p.spec.Detail; d != nil {
		r.formulaCurrencies(d.Label)
	}
	c.groupTargets = make([]*Source, len(c.p.groups))
	c.groupOptions, c.groupShown = make([][]Option, len(c.p.groups)), make([]bool, len(c.p.groups))
	for i, g := range c.p.groups {
		e, err := r.rowValue(g.col, g.at+".col", map[string]bool{})
		if err != nil {
			return nil, err
		}
		if g.group.Bucket != "" {
			if e.typ != Date && e.typ != DateTime {
				return nil, invalid(g.at+".bucket", "only a date column has a bucket")
			}
			e.sql = bucketSQL(r.join(false), g.group.Bucket, e.sql)
		}
		if g.group.Bucket == "" {
			c.groupTargets[i], c.groupOptions[i] = e.target, e.options
			// A key that mixes a select's values with other text is shown, and
			// its groups ordered, by its shown form, which w keeps beside it.
			if e.shown != "" {
				r.add(fmt.Sprintf("kl%d", i), e.shown)
				c.groupShown[i] = true
			}
		}
		// A group by an amount that carries its own currency (a looked-up
		// row's, or a path's read beside it) groups by that currency too: 300
		// USD and 300 EUR are two groups.
		if e.typ == Money && e.cur.sql != "" && e.cur.id != "" {
			r.currencyKey(e.cur.id, e.cur.sql)
		}
		// A skip level's blank key is NULL: a text key of "" is blank as well
		// (fieldSQL leaves a text field's "" as it is), so the tree, the
		// level's own rule, the expansion and the cursor see one blank.
		if g.group.Blank == GroupBlankSkip {
			e.sql = "NULLIF(" + e.sql + ", '')"
		}
		r.add(fmt.Sprintf("k%d", i), e.sql)
	}
	// The filter compiles once the currency keys are known, so an amount it
	// reads by path has the key a column of it has (joinCtx.path).
	if f := c.p.spec.Filter; f != nil {
		e, err := compileFormula(r.join(true), f, "filter")
		if err != nil {
			return nil, err
		}
		if e.typ != Bool {
			return nil, invalid("filter", "the filter is a condition")
		}
		r.filter, r.filterMixed = e.sql, e.condMixed
	}
	r.valueCurrencies()
	r.pivotKey()
	return r, nil
}

// valueCurrencies adds a currency key to w for every record or looked-up
// amount with a per-row currency read inside a total (an aggregate, a running
// value or a percent, directly or through a column it reads): a total groups
// by its currency as by a path amount's (R3). Read outside a total, such an
// amount carries its currency by subquery and needs no key. A record key reads
// the group key's own expression, so the group keys are in w by now.
func (r *rowsCTE) valueCurrencies() {
	seen := map[string]bool{}
	var walk func(n *Node, at string, inTotal bool)
	walk = func(n *Node, at string, inTotal bool) {
		if n == nil {
			return
		}
		switch {
		case inTotal && (n.Record != "" || n.Op == "lookup"):
			r.valueCurrencyKey(n, at)
		case inTotal && n.Col != "" && !seen[n.Col]:
			// A formula column read inside a total is read per base row.
			if cp := r.c.p.byID[n.Col]; cp != nil && cp.col.Formula != nil && cp.col.Aggregate == "" {
				seen[n.Col] = true
				walk(cp.col.Formula, cp.at+".formula", true)
			}
		}
		inTotal = inTotal || aggregateOp(n) || windowOp(n)
		for i, a := range n.Args {
			walk(a, fmt.Sprintf("%s.args.%d", at, i), inTotal)
		}
	}
	// A "rows" column adds its cells up, so it is a total too.
	for _, cp := range r.c.p.cols {
		walk(cp.col.Formula, cp.at+".formula", cp.col.Aggregate != "" || cp.col.Total == TotalRows)
	}
	for _, g := range r.c.p.groups {
		walk(g.group.Label, g.at+".label", false)
		walk(g.group.Having, g.at+".having", false)
	}
	for _, s := range r.c.p.sort {
		walk(s.formula, s.at, false)
	}
	if d := r.c.p.spec.Detail; d != nil {
		walk(d.Label, "detail.label", false)
	}
}

// windowOp is true for an operator that sums the rows a report shows.
func windowOp(n *Node) bool {
	switch n.Op {
	case "runningSum", "percentOfParent", "percentOfTotal":
		return true
	}
	return false
}

// valueCurrencyKey compiles a record leaf or a lookup (at) inside w and keeps
// its carried currency as a currency key. One that does not compile is
// reported by the compile that follows, where it is; its error is kept, so a
// total that then finds no key for its currency names the real cause
// (totalCurrency).
func (r *rowsCTE) valueCurrencyKey(n *Node, at string) {
	id := lookupCurrencyID(n)
	if n.Record != "" {
		id = recordCurrencyID(n.Record, n.Path)
	}
	if _, seen := r.c.valueCur[id]; seen {
		return
	}
	j := r.join(false)
	j.records = true
	e, err := compileFormula(j, n, at)
	if err != nil {
		r.c.valueCurErr[id] = err
		return
	}
	if e.typ == Money && e.cur.sql != "" && e.cur.id == id {
		r.currencyKey(id, e.cur.sql)
	}
}

// currencyKey keeps a carried currency (id) in w as a currency key every
// total groups by, once.
func (r *rowsCTE) currencyKey(id, sql string) {
	if _, seen := r.c.valueCur[id]; seen {
		return
	}
	r.c.valueCur[id] = r.currencyColumn(sql)
}

// currencyColumn is the currency key of w that holds sql, added once: one
// currency read the same way is one key, however it was reached.
func (r *rowsCTE) currencyColumn(sql string) string {
	if name, seen := r.byExpr[sql]; seen {
		return name
	}
	name := fmt.Sprintf("ck%d", len(r.c.currencies))
	r.byExpr[sql] = name
	r.add(name, sql)
	r.c.currencies = append(r.c.currencies, name)
	return name
}

// pathCurrency is the currency of a to-one path's amount: its currency
// field, kept in w as a currency key every total groups by and named for
// combining (currencyField), or its source's fixed currency.
func (r *rowsCTE) pathCurrency(p *pathPlan) currency {
	sql, fixed := amountCurrencyOf(r.root, p.hops, p)
	switch {
	case sql != "":
		return currency{key: r.currencyColumn(sql), field: currencyField(p)}
	case fixed != nil:
		return currency{fixed: fixed}
	}
	return currency{}
}

// currencyField names the currency field of p's amount for combining (R24):
// "<source key>.<currency field key>", "" for a value that is not an amount
// or an amount with no currency field.
func currencyField(p *pathPlan) string {
	if p.field.Type != Money || p.field.Currency == "" {
		return ""
	}
	return p.src.Key + "." + p.field.Currency
}

// amountCurrencyOf is where the currency of p's amount, read in sc after hops,
// comes from: the SQL of its currency field read beside it, or its source's
// fixed currency. Neither for a value that is not an amount, or an amount
// with no currency.
func amountCurrencyOf(sc *scope, hops []hopPlan, p *pathPlan) (string, *Source) {
	f := p.field
	switch {
	case f.Type != Money:
		return "", nil
	case f.Currency != "":
		return fieldSQL(sc.join(hops), p.src.field(f.Currency)), nil
	case p.src.Currency != nil:
		return "", p.src
	}
	return "", nil
}

// formulaCurrencies adds the currency key of every amount a formula reads by
// path. A path that does not bind is reported when the formula compiles. A
// record leaf's path is on its group's target, not on the base source.
func (r *rowsCTE) formulaCurrencies(n *Node) {
	if n == nil {
		return
	}
	if n.Path != nil && n.Record == "" {
		if bound, err := bindValuePath(r.c.p.app, r.c.p.base, n.Path, "", pathValue); err == nil {
			r.pathCurrency(bound)
		}
	}
	for _, a := range n.Args {
		r.formulaCurrencies(a)
	}
}

// add puts a value in w under name, once, and returns its reference.
func (r *rowsCTE) add(name, sql string) string {
	if _, ok := r.raw[name]; !ok {
		r.raw[name] = sql
		r.order = append(r.order, name)
	}
	return "w." + name
}

// colValue puts a path column's value in w and returns its reference. A
// to-many total reads its subquery; a parent with no visible linked rows
// counts and sums to 0.
func (r *rowsCTE) colValue(cp *colPlan) expr {
	idx := r.c.p.index(cp)
	name := fmt.Sprintf("c%d", idx)
	typ := cp.path.field.Type
	if cp.tm >= 0 {
		typ = aggregateType(cp.col.Aggregate, typ, cp.scale)
	}
	ref := withCurrency(expr{sql: "w." + name, typ: typ}, r.c.currencyOf[cp.col.ID])
	if cp.tm < 0 {
		ref.target, ref.options = cp.path.target, optionsOf(cp.path.field)
	}
	if _, ok := r.raw[name]; ok {
		return ref
	}
	if cp.tm < 0 {
		r.add(name, r.root.value(cp.path).sql)
		return ref
	}
	op := cp.col.Aggregate
	value := r.root.total(cp.path, cp.tm, op, cp.scale)
	if op == Count || op == Sum {
		value = "COALESCE(" + value + ", 0)"
	}
	r.add(name, value)
	r.add(fmt.Sprintf("p%d", idx), r.root.parentID(cp.path, cp.tm))
	if op == Avg {
		r.add(fmt.Sprintf("cs%d", idx), r.root.total(cp.path, cp.tm, Sum, 0))
		r.add(fmt.Sprintf("cn%d", idx), r.root.total(cp.path, cp.tm, Count, 0))
	}
	return ref
}

// inner is the expression behind a reference to w, for use inside w's own
// select (its WHERE and its group keys).
func (r *rowsCTE) inner(ref string) string {
	if sql, ok := r.raw[strings.TrimPrefix(ref, "w.")]; ok {
		return sql
	}
	return ref
}

// hidden is a value path a formula reads, kept in w.
func (r *rowsCTE) hidden(p *pathPlan) string {
	key := "path:" + hopsKey(p.hops) + "." + p.field.Key
	if name, ok := r.byExpr[key]; ok {
		return "w." + name
	}
	name := fmt.Sprintf("h%d", len(r.order))
	r.byExpr[key] = name
	return r.add(name, r.root.value(p).sql)
}

// rowValue is a column's value per base row, as an expression inside w: a
// group key, the pivot key, or a column a filter compares.
func (r *rowsCTE) rowValue(cp *colPlan, at string, visiting map[string]bool) (expr, error) {
	if cp.nest > 0 {
		return expr{}, invalid(at, "a child row column is not a value of the report's rows")
	}
	if cp.path != nil {
		e := r.colValue(cp)
		e.sql = r.inner(e.sql)
		return e, nil
	}
	j := r.join(false)
	j.visiting = visiting
	return compileFormula(j, cp.col.Formula, cp.at+".formula")
}

func (r *rowsCTE) join(filter bool) *joinCtx {
	return &joinCtx{r: r, sc: r.root, src: r.c.p.base, isFilter: filter, visiting: map[string]bool{}}
}

func (r *rowsCTE) render() string {
	cols := make([]string, len(r.order))
	for i, name := range r.order {
		cols[i] = r.raw[name] + " AS " + name
	}
	sql := "w AS (SELECT " + strings.Join(cols, ", ") + "\n" + r.root.render()
	if r.filter != "" {
		sql += "\nWHERE " + r.filter
	}
	return sql + ")"
}

// bucketSQL buckets a date: day, month (2026-01), quarter (2026-Q1), year,
// or the base source's fiscal year.
func bucketSQL(c fctx, bucket, day string) string {
	switch bucket {
	case "day":
		return "substr(" + day + ", 1, 10)"
	case "month":
		return "substr(" + day + ", 1, 7)"
	case "quarter":
		return "(substr(" + day + ", 1, 4) || '-Q' || ((CAST(substr(" + day + ", 6, 2) AS INTEGER) + 2) / 3))"
	case "year":
		return "substr(" + day + ", 1, 4)"
	}
	return "tc_fiscal_year(" + day + ", " + c.fiscalStart() + ")"
}

func (p *plan) index(cp *colPlan) int {
	for i, x := range p.cols {
		if x == cp {
			return i
		}
	}
	return -1
}
