package reports

import (
	"fmt"
	"slices"
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
	n      int
}

func newStmt(app core.App, info *core.RequestInfo) *stmt {
	return &stmt{app: app, info: info, params: dbx.Params{}, ruleOf: map[string]string{}}
}

// bind adds a value to the statement's parameters and returns its
// placeholder. Values are always bound, never written into the SQL.
func (s *stmt) bind(v any) string {
	name := fmt.Sprintf("rp%d", len(s.params))
	s.params[name] = v
	return "{:" + name + "}"
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
		}
		parts = append(parts, s.ruleOf[name]+" AS ("+sql+")")
	}
	parts = append(parts, ctes...)
	if len(parts) == 0 {
		return "", nil
	}
	return "WITH " + strings.Join(parts, ",\n") + "\n", nil
}

func q(alias, column string) string { return "[[" + alias + "." + column + "]]" }

// fieldSQL reads a field from an alias. A blank date, select or relation is
// NULL, so a blank and a hidden linked row read the same.
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
	return expr{sql: fieldSQL(sc.join(p.hops), p.field), typ: p.field.Type, target: p.target}
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
