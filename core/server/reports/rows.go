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
	// currencies names w's currency keys; every total groups by them, so
	// amounts in different currencies are never added together.
	currencies []string
	// currencyOf maps a money column's id to its currency: a currency key
	// of w, or a source's fixed currency.
	currencyOf map[string]currency
	// detailTypes and levelTypes are each column's type on detail rows
	// and on group, subtotal and total rows.
	detailTypes map[string]FieldType
	levelTypes  map[string]FieldType
	// groupTargets is the source each group's key links to (nil for a group
	// that is not by a link).
	groupTargets []*Source
	// labels is each group's label formula as it compiled: its type and,
	// for an amount, its currency. It outlives a statement (assembly reads
	// it after every pass), so rows() leaves it.
	labels []labelPlan
	// labelled holds the columns whose values point at a record, which each
	// carry that record's label (vl<idx>, the cell <id>@label).
	labelled map[string]bool
}

// labelPlan is what a group's label formula compiled to.
type labelPlan struct {
	typ FieldType
	cur currency
}

func newCompiler(p *plan, app core.App, info *core.RequestInfo, params map[string]paramValue) *compiler {
	c := &compiler{p: p, app: app, info: info, params: params, fiscal: 1, currencyOf: map[string]currency{}, labels: make([]labelPlan, len(p.groups)), labelled: map[string]bool{}}
	if app != nil {
		c.fiscal = fiscalStartOf(p.base, app)
	}
	if params == nil {
		// Dry: each param has its default, as in a run that posts none; bind
		// has checked the defaults.
		c.params, _ = resolveParams(p.spec.Params, nil, anyDay)
	}
	return c
}

// fiscalStartOf is the source's fiscal start month, 1 when it has none.
func fiscalStartOf(src *Source, app core.App) int {
	if src.FiscalStartMonth == nil {
		return 1
	}
	if m := src.FiscalStartMonth(app); m >= 1 && m <= 12 {
		return m
	}
	return 1
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
}

func (c *compiler) rows(s *stmt) (*rowsCTE, error) {
	// Each statement builds its own w; the currency keys come out the same.
	c.currencies, c.currencyOf = nil, map[string]currency{}
	r := &rowsCTE{c: c, s: s, root: newScope(s, c.p.base), raw: map[string]string{}, byExpr: map[string]string{}}
	r.add("__id", q(r.root.alias, "id"))
	if f := c.p.spec.Filter; f != nil {
		e, err := compileFormula(r.join(true), f, "filter")
		if err != nil {
			return nil, err
		}
		if e.typ != Bool {
			return nil, invalid("filter", "the filter is a condition")
		}
		r.filter = e.sql
	}
	c.groupTargets = make([]*Source, len(c.p.groups))
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
			c.groupTargets[i] = e.target
		}
		r.add(fmt.Sprintf("k%d", i), e.sql)
	}
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
	}
	r.pivotKey()
	return r, nil
}

// pathCurrency is the currency of a to-one path's amount: its currency
// field, kept in w as a currency key every total groups by, or its source's
// fixed currency.
func (r *rowsCTE) pathCurrency(p *pathPlan) currency {
	f := p.field
	switch {
	case f.Type != Money:
		return currency{}
	case f.Currency != "":
		sql := fieldSQL(r.root.join(p.hops), p.src.field(f.Currency))
		name, seen := r.byExpr[sql]
		if !seen {
			name = fmt.Sprintf("ck%d", len(r.c.currencies))
			r.byExpr[sql] = name
			r.add(name, sql)
			r.c.currencies = append(r.c.currencies, name)
		}
		return currency{key: name}
	case p.src.Currency != nil:
		return currency{fixed: p.src}
	}
	return currency{}
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
		ref.target = cp.path.target
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
