package reports

import (
	"fmt"
	"slices"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// filterBounds are bounds on the base table's stored columns that the report
// filter implies, joined with AND, "" when it implies none. The filter reads
// a link through the joined row's id (blank when the reader cannot see the
// row) and a day through substr(), and no index serves either form, so w read
// every base row. A bound names the stored column itself, which an index on
// it serves. The filter stays as it is and still decides each row: a bound
// only leaves out rows the filter refuses.
//
// Only a term of the filter's top-level and (or the filter itself) gives a
// bound: a row the filter keeps passes every such term. The base must be a
// collection that is not a view and has no union parts, as storedLinkCol
// requires of a lookup's source.
func (c *compiler) filterBounds(r *rowsCTE) (string, error) {
	f := c.p.spec.Filter
	// A dry compile (Validate) has no database to read the collection from.
	if f == nil || legacySQL.noFilterBounds || c.dry() || len(c.p.base.Parts) > 0 {
		return "", nil
	}
	col, err := c.app.FindCachedCollectionByNameOrId(c.p.base.Collection)
	if err != nil {
		// Without the bounds the rows are still right, only slower.
		log.Warn("filter not bounded", "collection", c.p.base.Collection, "err", err)
		return "", nil
	}
	if col.IsView() {
		return "", nil
	}
	// stored is true when the collection keeps column name as one of kinds:
	// a bound compares the stored value as text.
	stored := func(name string, kinds ...string) bool {
		field := col.Fields.GetByName(name)
		if field == nil || !slices.Contains(kinds, field.Type()) {
			return false
		}
		// A multi-select relation holds a JSON array, never an id.
		rel, isRel := field.(*core.RelationField)
		return !isRel || !rel.IsMultiple()
	}
	terms := []*Node{f}
	if f.Op == "and" {
		terms = f.Args
	}
	var bounds []string
	for i, term := range terms {
		if term == nil || len(term.Args) != 2 {
			continue
		}
		at := "filter"
		if f.Op == "and" {
			at = fmt.Sprintf("filter.args.%d", i)
		}
		var bound string
		switch term.Op {
		case "in":
			bound, err = r.listBound(term, at, stored)
		case "between", "lt", "lte", "gt", "gte":
			bound, err = r.dayBound(term, at, stored)
		}
		if err != nil {
			return "", err
		}
		if bound != "" {
			bounds = append(bounds, bound)
		}
	}
	return strings.Join(bounds, " AND "), nil
}

// basePath is the path a filter term's first value reads on the base row: a
// path leaf, or a column that is a path and not a to-many total or a child
// row's. nil for any other value.
func (r *rowsCTE) basePath(n *Node, at string) (*pathPlan, error) {
	switch {
	case n.Op != "" || n.Value != nil || n.Param != "":
		return nil, nil
	case n.Col != "":
		cp := r.c.p.byID[n.Col]
		if cp == nil || cp.path == nil || cp.tm >= 0 || cp.nest > 0 {
			return nil, nil
		}
		return cp.path, nil
	case n.Path != nil:
		return bindValuePath(r.c.p.app, r.c.p.base, n.Path, at, pathValue)
	}
	return nil, nil
}

// rowFreeLeaf is a value that reads nothing of a row: a param (or a bound of
// one) or a constant.
func rowFreeLeaf(n *Node) bool {
	return n.Op == "" && n.Col == "" && n.Path == nil && n.Outer == nil && n.Record == "" && n.Subtotal == "" && (n.Param != "" || n.Value != nil)
}

// listBound is the bound of in(X, list) where X ends at one to-one link of the
// base row: the row's stored relation column is in the list. The filter reads
// the linked row's id through a join on that column, so where it holds the
// column holds that id.
func (r *rowsCTE) listBound(term *Node, at string, stored func(string, ...string) bool) (string, error) {
	p, err := r.basePath(term.Args[0], at+".args.0")
	if err != nil || p == nil || !p.endsAtLink || len(p.hops) != 1 || !rowFreeLeaf(term.Args[1]) {
		return "", err
	}
	if !stored(p.hops[0].link.Field, core.FieldTypeRelation) {
		return "", nil
	}
	list, err := compileFormula(r.join(true), term.Args[1], at+".args.1")
	if err != nil || !list.list || list.unset || list.sql == "()" {
		return "", err
	}
	return linkBound(q(r.root.alias, p.hops[0].link.Field), list.sql), nil
}

// dayBound is the bound of between(D, range) and of lt, lte, gt or gte(D,
// day), where D is a Date field of the base row itself and the other value a
// param's day (a day typed as a constant is text to the compiler and gives no
// bound). The column must hold text or a PocketBase date.
func (r *rowsCTE) dayBound(term *Node, at string, stored func(string, ...string) bool) (string, error) {
	p, err := r.basePath(term.Args[0], at+".args.0")
	if err != nil || p == nil || len(p.hops) != 0 || p.endsAtLink || p.field.Type != Date || !rowFreeLeaf(term.Args[1]) {
		return "", err
	}
	if !stored(p.field.Column, core.FieldTypeText, core.FieldTypeDate, core.FieldTypeAutodate) {
		return "", nil
	}
	v, err := compileFormula(r.join(true), term.Args[1], at+".args.1")
	if err != nil || v.unset || v.all {
		return "", err
	}
	col := q(r.root.alias, p.field.Column)
	switch {
	case term.Op == "between" && v.rng:
		return dayBounds(col, v.lo, v.hi), nil
	case term.Op == "between" || v.rng || v.typ != Date:
		return "", nil
	case term.Op == "lt" || term.Op == "lte":
		return dayBounds(col, "", v.sql), nil
	}
	return dayBounds(col, v.sql, ""), nil
}

// linkBound bounds a stored relation column by a list of ids.
func linkBound(col, list string) string {
	return col + " IN " + list
}

// dayBounds bounds a stored day column by the first (lo) and last (hi) day
// its day may be; "" leaves that side open. The column holds a day
// (2026-07-01) or a day with a time after it (2026-07-01 10:00:00.000Z), and
// its day is its first 10 characters. A day at or after lo is a text at or
// after lo. A day at or before hi is a text at or before hi followed by the
// byte ff, which sorts after every character a time can hold; so the bounds
// hold wherever the comparison of the days does, also for lt and gt, whose
// bounds are only wider.
func dayBounds(col, lo, hi string) string {
	var parts []string
	if lo != "" {
		parts = append(parts, col+" >= "+lo)
	}
	if hi != "" {
		parts = append(parts, col+" <= ("+hi+" || x'ff')")
	}
	return strings.Join(parts, " AND ")
}
