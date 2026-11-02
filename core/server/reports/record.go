package reports

import (
	"fmt"
	"strconv"
	"strings"

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

// noName is the label of a visible record whose label field is blank.
const noName = "(no name)"

// isLabelType is true for a field type that reads as text, so a record can
// be shown by it: never an id, an amount, a yes/no or a moment.
func isLabelType(t FieldType) bool {
	return t == Text || t == Select || t == Number || t == Date
}

// labelFieldOf is the field a record is shown by (R31): the chosen field,
// else the first text field, else the first other field that reads as text;
// nil when the source has none (then its own name is shown). A chosen field
// is checked by labelField.
func labelFieldOf(src *Source, chosen string) *Field {
	if chosen != "" {
		return src.field(chosen)
	}
	for _, text := range []bool{true, false} {
		for i := range src.Fields {
			f := &src.Fields[i]
			if (f.Type == Text) == text && isLabelType(f.Type) {
				return f
			}
		}
	}
	return nil
}

// labelValueSQL reads the label of the record a relation value (key) points
// at, through the target's rule CTE: NULL for a blank link or a record the
// reader cannot see, "(no name)" for a visible record whose label field is
// blank, the source's name for a source with no field to show.
func labelValueSQL(s *stmt, target *Source, f *Field, key string) string {
	sc := newScope(s, target)
	shown := s.bind(target.Label)
	if f != nil {
		v := sc.value(&pathPlan{field: f, src: target}).sql
		if f.Type == Select {
			v = selectLabelSQL(s, f, v)
		}
		shown = fmt.Sprintf("COALESCE(NULLIF(CAST(%s AS TEXT), ''), %s)", v, s.bind(noName))
	}
	return fmt.Sprintf("(SELECT %s\n%s\nWHERE %s IS %s)", shown, sc.render(), q(sc.alias, "id"), key)
}

// selectLabelSQL is a select value shown by its option's label; a value no
// option names shows as stored.
func selectLabelSQL(s *stmt, f *Field, value string) string {
	if len(f.Options) == 0 {
		return value
	}
	var b strings.Builder
	b.WriteString("(CASE " + value)
	for _, o := range f.Options {
		b.WriteString(" WHEN " + s.bind(o.Value) + " THEN " + s.bind(o.Label))
	}
	b.WriteString(" ELSE " + value + " END)")
	return b.String()
}

// checkLabelField marks a column whose value points at target as labelled.
// Its chosen label field must be one of the target's fields that reads as
// text, and only such a column chooses one. Every run compiles the detail
// select, which checks every column, before its result is assembled.
func (c *compiler) checkLabelField(cp *colPlan, target *Source) error {
	chosen, at := cp.col.LabelField, cp.at+".labelField"
	if target == nil {
		if chosen != "" {
			return invalid(at, "only a column that links to a record shows a label field")
		}
		return nil
	}
	f := labelFieldOf(target, chosen)
	switch {
	case chosen == "":
	case f == nil:
		return unknownRef(at, "unknown_field", fmt.Sprintf("the source %q has no field %q", target.Key, chosen))
	case !isLabelType(f.Type):
		return invalid(at, "a record is shown by a text, choice, number or date field")
	}
	c.labelled[cp.col.ID] = true
	return nil
}

// labelOf is the label beside a column's relation value e, or NULL for a
// value with no target or a blank one.
func (c *compiler) labelOf(s *stmt, cp *colPlan, e expr) string {
	if e.target == nil || e.sql == "NULL" {
		return "NULL"
	}
	return labelValueSQL(s, e.target, labelFieldOf(e.target, cp.col.LabelField), e.sql)
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
