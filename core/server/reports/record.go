package reports

import (
	"fmt"
	"strconv"

	"github.com/pocketbase/pocketbase/core"
)

// groupIndex is the index of the group with id, or -1.
func (c *compiler) groupIndex(id string) int {
	for i, g := range c.p.groups {
		if g.group.ID == id {
			return i
		}
	}
	return -1
}

// recordOf finds the group a record leaf names and the source its key
// links to.
func (c *compiler) recordOf(group, at string) (int, *Source, error) {
	gi := c.groupIndex(group)
	if gi < 0 {
		return 0, nil, unknownRef(at, "unknown_group", fmt.Sprintf("the report has no group %q", group))
	}
	if gi >= len(c.groupTargets) || c.groupTargets[gi] == nil {
		return 0, nil, invalid(at, fmt.Sprintf("the group %q is not grouped by a link: it has no record", group))
	}
	return gi, c.groupTargets[gi], nil
}

// bindRecordPath binds a record leaf's path on the group's target: to-one
// links, then a field. An amount needs its source's fixed currency here; Task
// 5a lifts that (Q5: the amount carries its own currency).
func bindRecordPath(app core.App, target *Source, p Path, at string) (*pathPlan, error) {
	if len(p) == 0 {
		return nil, invalid(at, "a record leaf names a field of the record")
	}
	bound, err := bindValuePath(app, target, p, at+".path", pathValue)
	if err != nil {
		return nil, err
	}
	if bound.field.Type == Money && bound.field.Currency != "" {
		return nil, invalid(at+".path", "this amount has its own row's currency, which totals cannot group by: read it through a column")
	}
	return bound, nil
}

// recordExpr types a record leaf; sql is filled in by its context.
func recordExpr(bound *pathPlan) expr {
	e := expr{typ: bound.field.Type, target: bound.target}
	if bound.field.Type == Money && bound.src.Currency != nil {
		e.cur = currency{fixed: bound.src}
	}
	return e
}

// recordSQL reads a group record's field: a subquery on the target's rule
// CTE, keyed by the group key, so a record the reader cannot see reads
// blank.
func recordSQL(s *stmt, target *Source, bound *pathPlan, key string) string {
	sc := newScope(s, target)
	v := sc.value(bound)
	return fmt.Sprintf("(SELECT %s\n%s\nWHERE %s IS %s)", v.sql, sc.render(), q(sc.alias, "id"), key)
}

// firstTextField is the field a record group is labelled by when it names
// no label.
func firstTextField(src *Source) *Field {
	for i := range src.Fields {
		if src.Fields[i].Type == Text && src.Fields[i].Key != idField.Key {
			return &src.Fields[i]
		}
	}
	return nil
}

// labelText is a label value as row text: (none) for a blank.
func labelText(v any) string {
	switch x := v.(type) {
	case nil:
		return blankLabel
	case string:
		if x == "" {
			return blankLabel
		}
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(x, 10)
	}
	return fmt.Sprint(v)
}
