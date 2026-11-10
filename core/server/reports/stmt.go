package reports

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// stmt is one SQL statement being built: its bound parameters, the rule
// CTE of every collection it reads, and unique aliases. With a nil app it
// is dry: Validate compiles a spec on one to check it, and nothing runs.
type stmt struct {
	app    core.App
	info   *core.RequestInfo
	params dbx.Params
	rules  []string // collections, in first-use order
	ruleOf map[string]string
	// ruleReads holds each rule CTE's own select and params, by CTE name, as
	// with rendered them, so a run finds the topics a rule reads (topics).
	ruleReads map[string]ruleRead
	n         int
}

// ruleRead is a rule CTE's select and its params.
type ruleRead struct {
	sql    string
	params dbx.Params
}

func newStmt(app core.App, info *core.RequestInfo) *stmt {
	return &stmt{app: app, info: info, params: dbx.Params{}, ruleOf: map[string]string{}, ruleReads: map[string]ruleRead{}}
}

// bind adds a value to the statement's parameters and returns its
// placeholder. Values are always bound, never written into the SQL; the one
// literal is a row limit (limitSQL).
func (s *stmt) bind(v any) string {
	name := fmt.Sprintf("rp%d", len(s.params))
	s.params[name] = v
	return "{:" + name + "}"
}

// limitSQL writes a statement's row limit as a literal, never a bound
// parameter. SQLite marks a statement whose LIMIT is bound to be prepared
// again once the value is bound (sqlite3ExprIsInteger sets its varmask), so
// a bound limit costs every run a second prepare of the whole statement, and
// of the EXPLAIN that finds its change topics. A limit is an integer bounded
// by the engine: its own caps, or a request's page size or a spec's
// maxColumns, both typed ints checked against a cap before they get here.
func limitSQL(n int64) string { return strconv.FormatInt(n, 10) }

// jsonList is a list of values as "x IN <list>" reads it: jsonSelect in
// parentheses, or "()" for an empty list, which IN reads as no values.
func jsonList(bind func(any) string, values []any) (string, error) {
	if len(values) == 0 {
		return "()", nil
	}
	sel, err := jsonSelect(bind, values)
	if err != nil {
		return "", err
	}
	return "(" + sel + ")", nil
}

// jsonSelect binds a list as one JSON array and selects it back, a row per
// value: a statement binds one parameter however long the list is and
// however often it reads it, where a placeholder per value would pass
// SQLite's 32,766 variables. "+value" drops the BLOB affinity of
// json_each's value column, so IN applies the left side's affinity to each
// value as it does in "x IN (?, ?)".
func jsonSelect(bind func(any) string, values []any) (string, error) {
	parts := make([]string, len(values))
	for i, v := range values {
		text, err := sqlJSON(v)
		if err != nil {
			return "", err
		}
		parts[i] = text
	}
	return "SELECT +value FROM json_each(" + bind("["+strings.Join(parts, ",")+"]") + ")", nil
}

// texts is a list of text values as jsonList and jsonSelect take it.
func texts(values []string) []any {
	items := make([]any, len(values))
	for i, v := range values {
		items[i] = v
	}
	return items
}

// sqlJSON writes v as JSON that SQLite reads back as the value a bound v
// holds: the same type and, for a number, the same bits. A float keeps a
// point or an exponent, so it does not read back as an integer; an
// infinity is written past a double's range, which SQLite reads as that
// infinity; NaN is null, as SQLite binds it.
func sqlJSON(v any) (string, error) {
	switch x := v.(type) {
	case nil:
		return "null", nil
	case bool:
		return strconv.FormatBool(x), nil
	case string:
		b, err := json.Marshal(x)
		return string(b), err
	case int:
		return strconv.Itoa(x), nil
	case int64:
		return strconv.FormatInt(x, 10), nil
	case float64:
		switch {
		case math.IsNaN(x):
			return "null", nil
		case math.IsInf(x, 1):
			return "9e999", nil
		case math.IsInf(x, -1):
			return "-9e999", nil
		}
		text := strconv.FormatFloat(x, 'g', -1, 64)
		if !strings.ContainsAny(text, ".e") {
			text += ".0"
		}
		return text, nil
	}
	return "", fmt.Errorf("reports: a list holds text, numbers, true, false or null, not %T", v)
}

func (s *stmt) alias(prefix string) string {
	s.n++
	return fmt.Sprintf("%s%d", prefix, s.n)
}

// rule names the CTE that holds the rows of collection the reader may list.
// Every reference to the collection reads it, never the table.
func (s *stmt) rule(collection string) string {
	if name, ok := s.ruleOf[collection]; ok {
		return name
	}
	name := fmt.Sprintf("r%d", len(s.rules))
	s.ruleOf[collection] = name
	s.rules = append(s.rules, collection)
	return name
}

// with renders the rule CTEs, then the given CTEs, as one WITH clause.
func (s *stmt) with(ctes ...string) (string, error) {
	parts := make([]string, 0, len(s.rules)+len(ctes))
	for _, name := range s.rules {
		sql := "SELECT 1 AS id WHERE 0"
		if s.app != nil {
			col, err := s.app.FindCachedCollectionByNameOrId(name)
			if err != nil {
				return "", fmt.Errorf("reports: collection %q: %w", name, err)
			}
			ruleSQL, params, err := ruleSelect(s.app, col, s.info)
			if err != nil {
				return "", err
			}
			if err := mergeParams(s.params, params); err != nil {
				return "", err
			}
			sql = ruleSQL
			s.ruleReads[s.ruleOf[name]] = ruleRead{sql: ruleSQL, params: params}
		}
		parts = append(parts, s.ruleOf[name]+" AS ("+sql+")")
	}
	parts = append(parts, ctes...)
	if len(parts) == 0 {
		return "", nil
	}
	return "WITH " + strings.Join(parts, ",\n") + "\n", nil
}

// ruleRef is a read of a rule CTE: a scope reads its collection's rule by
// FROM or JOIN, as "r<n> <alias>".
var ruleRef = regexp.MustCompile(`(?:FROM|JOIN) (r\d+) `)

// topics returns the change-signal topics of every collection a statement
// built on s reads (sql, as with rendered it): the topics of the rule CTEs it
// reads, each found by explaining the rule's own select once per run (cache
// holds them by select). Every collection a statement reads is read through
// its rule CTE, so the statement is not compiled a second time for them. The
// result may name a collection SQLite's plan of the statement leaves out (a
// join no value reads): that topic only signals a change that cannot change
// the result, which costs a client one more run, never an old result; do not
// go back to explaining the whole statement to drop it.
func (s *stmt) topics(ctx context.Context, app core.App, sql string, cache map[string][]string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, m := range ruleRef.FindAllStringSubmatch(sql, -1) {
		name := m[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		r, ok := s.ruleReads[name]
		if !ok {
			return nil, fmt.Errorf("reports: the statement reads %s, which it holds no rule for", name)
		}
		key := r.sql + "\x00" + fmt.Sprint(r.params)
		topics, ok := cache[key]
		if !ok {
			var err error
			if topics, err = topicsRead(ctx, app, r.sql, r.params); err != nil {
				return nil, err
			}
			cache[key] = topics
		}
		for _, t := range topics {
			if !slices.Contains(out, t) {
				out = append(out, t)
			}
		}
	}
	slices.Sort(out)
	return out, nil
}

func q(alias, column string) string { return "[[" + alias + "." + column + "]]" }

// fieldSQL reads a field from an alias. A blank date, select or relation is
// NULL, so a blank and a hidden linked row read the same. A day is compared
// through substr(), which no index serves. Measured 2026-11
// (BenchmarkDayComparison, 30,000 lines): a bare-column form with a date index
// made `between` about 40% faster and `gte` about 20% faster, but not `lte`
// (the Balance Sheet read), so the form was not changed; see
// docs/plans/2026-11-09-report-layout-perf.md, Results.
func fieldSQL(alias string, f *Field) string {
	c := q(alias, f.Column)
	switch f.Type {
	case Date:
		return "NULLIF(substr(" + c + ", 1, 10), '')"
	case DateTime, Select, Relation:
		return "NULLIF(" + c + ", '')"
	}
	return c
}

// scope is a FROM clause and its joins: the rows of a statement, a to-many
// subquery or an exists subquery. Paths that share a prefix share a join.
type scope struct {
	s     *stmt
	src   *Source
	from  string // "r0 t1"
	alias string
	joins map[string]string
	sql   []string
	tms   []*tmSub
}

// newScope reads src's rule CTE as "FROM r<n> <alias>" (and join as
// "JOIN r<n> <alias>"): stmt.topics finds the collections a statement reads by
// that form (ruleRef).
func newScope(s *stmt, src *Source) *scope {
	alias := s.alias("t")
	return &scope{
		s: s, src: src, alias: alias,
		from:  s.rule(src.Collection) + " " + alias,
		joins: map[string]string{},
	}
}

// join follows to-one hops from the scope's base and returns the alias of
// the last. A to-one link left-joins (inner with with: required): a linked
// row the reader cannot see leaves empty cells and keeps the row.
func (sc *scope) join(hops []hopPlan) string {
	alias := sc.alias
	for i := range hops {
		key := hopsKey(hops[:i+1])
		if next, ok := sc.joins[key]; ok {
			alias = next
			continue
		}
		h := hops[i]
		next := sc.s.alias("t")
		kind := "LEFT JOIN"
		if h.required {
			kind = "JOIN"
		}
		sc.sql = append(sc.sql, fmt.Sprintf("%s %s %s ON %s = %s",
			kind, sc.s.rule(h.to.Collection), next, q(next, "id"), q(alias, h.link.Field)))
		sc.joins[key] = next
		alias = next
	}
	return alias
}

// value reads a to-one path's field in this scope.
func (sc *scope) value(p *pathPlan) expr {
	return pathExpr(fieldSQL(sc.join(p.hops), p.field), p)
}

// sql renders the FROM clause, its joins and its to-many subqueries.
func (sc *scope) render() string {
	parts := append([]string{"FROM " + sc.from}, sc.sql...)
	for _, tm := range sc.tms {
		parts = append(parts, tm.render())
	}
	return strings.Join(parts, "\n")
}

// tmSub is one to-many path totalled per parent row: a subquery grouped by
// the back relation, joined onto the parent. Every column that totals the
// same to-many path reads one subquery.
type tmSub struct {
	key      string
	parent   string // the parent row's alias in the outer scope
	required bool
	alias    string
	child    *scope
	back     string
	values   []string
}

// total adds a per-parent aggregate to the subquery of a column's to-many
// path and returns the SQL that reads it on the parent row.
func (sc *scope) total(p *pathPlan, at int, op Aggregate, scale int) string {
	prefix := p.hops[:at+1]
	key := hopsKey(prefix)
	i := slices.IndexFunc(sc.tms, func(t *tmSub) bool { return t.key == key })
	if i < 0 {
		h := prefix[at]
		tm := &tmSub{
			key: key, parent: sc.join(prefix[:at]), required: h.required,
			alias: sc.s.alias("m"), child: newScope(sc.s, h.to), back: h.link.BackField,
		}
		sc.tms = append(sc.tms, tm)
		i = len(sc.tms) - 1
	}
	tm := sc.tms[i]
	value := tm.child.value(&pathPlan{hops: p.hops[at+1:], field: p.field})
	name := fmt.Sprintf("v%d", len(tm.values))
	tm.values = append(tm.values, aggregateSQL(op, value.sql, scale)+" AS "+name)
	return q(tm.alias, name)
}

// parentID is the parent row's id the subquery of a to-many path joins on;
// group rows total a to-many column once per parent.
func (sc *scope) parentID(p *pathPlan, at int) string {
	return q(sc.join(p.hops[:at]), "id")
}

func (tm *tmSub) render() string {
	kind := "LEFT JOIN"
	if tm.required {
		kind = "JOIN"
	}
	back := q(tm.child.alias, tm.back)
	return fmt.Sprintf("%s (SELECT %s AS pk, %s\n%s\nGROUP BY %s) %s ON %s = %s",
		kind, back, strings.Join(tm.values, ", "), tm.child.render(), back, tm.alias, q(tm.alias, "pk"), q(tm.parent, "id"))
}

// exists compiles exists over a link path from the scope's base (after
// prefix): to-one hops join in this scope, and the first to-many hop opens a
// semi-join, so a match never multiplies the row.
func (sc *scope) exists(prefix []hopPlan, link *pathPlan, where *Node, mk func(*scope, []hopPlan) fctx, at string) (expr, error) {
	hops := append(slices.Clone(prefix), link.hops...)
	i := slices.IndexFunc(hops, func(h hopPlan) bool { return h.link.Cardinality == ToMany })
	if i < 0 {
		cond := q(sc.join(hops), "id") + " IS NOT NULL"
		if where != nil {
			w, err := compileWhere(mk(sc, hops), where, at)
			if err != nil {
				return expr{}, err
			}
			// A where on unset params filters nothing, so the exists does
			// not either: in a filter it matches every row, as notExists does.
			if w.unset {
				return unsetCondition, nil
			}
			// A blank where field makes the condition NULL, which NOT would
			// keep NULL: exists is 0 or 1, like SQL EXISTS.
			cond += " AND COALESCE(" + w.sql + ", 0)"
		}
		return expr{sql: "(" + cond + ")", typ: Bool}, nil
	}
	parent := sc.join(hops[:i])
	child := newScope(sc.s, hops[i].to)
	rest := &pathPlan{hops: hops[i+1:]}
	inner, err := child.exists(nil, rest, where, mk, at)
	if err != nil || inner.unset {
		return inner, err
	}
	return expr{sql: fmt.Sprintf("EXISTS (SELECT 1\n%s\nWHERE %s = %s AND %s)",
		child.render(), q(child.alias, hops[i].link.BackField), q(parent, "id"), inner.sql), typ: Bool}, nil
}

func compileWhere(c fctx, where *Node, at string) (expr, error) {
	e, err := compileFormula(c, where, at+".where")
	if err != nil {
		return expr{}, err
	}
	if e.typ != Bool {
		return expr{}, invalid(at+".where", "where is a condition")
	}
	return e, nil
}

// scanRows reads every row as column name to value, keeping SQLite's types
// (int64, float64, string, nil). It closes rows.
func scanRows(rows *dbx.Rows) ([]map[string]any, error) {
	defer rows.Close()
	names, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for rows.Next() {
		values := make([]any, len(names))
		ptrs := make([]any, len(names))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(names))
		for i, name := range names {
			if b, ok := values[i].([]byte); ok {
				values[i] = string(b)
			}
			row[name] = values[i]
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
