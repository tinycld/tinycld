package reports

import "fmt"

// optionsSelect selects a choice param's options: the rows of its source
// the reader may list that pass its where, one row per value (v) with its
// label (l) and group (g) as text. A where on another param reads that
// param's value for the request.
func (c *compiler) optionsSelect(s *stmt, op *optionsPlan) (string, error) {
	sc := newScope(s, op.src)
	value := "CAST(" + sc.value(op.value).sql + " AS TEXT)"
	label := "CAST(" + optionLabelOf(s, sc.value(op.label)) + " AS TEXT)"
	group := "NULL"
	if op.group != nil {
		group = "CAST(" + sc.value(op.group).sql + " AS TEXT)"
	}
	where := "1"
	if w := op.def.OptionsFrom.Where; w != nil {
		// A rows CTE with no root: a where reads its source's paths, never a
		// report column (joinCtx.col refuses outside the root scope).
		r := &rowsCTE{c: c, s: s}
		e, err := compileWhere(&joinCtx{r: r, sc: sc, src: op.src, isFilter: true, visiting: map[string]bool{}}, w, op.at)
		if err != nil {
			return "", err
		}
		if !e.unset {
			where = "COALESCE(" + e.sql + ", 0)"
		}
	}
	return fmt.Sprintf("SELECT %s AS v, min(COALESCE(%s, '')) AS l, min(%s) AS g\n%s\nWHERE %s AND %s IS NOT NULL\nGROUP BY 1",
		value, label, group, sc.render(), where, value), nil
}

// optionLabelOf is an option's label as the picker shows it (R31): a value
// that points at a record by that record's label, through its rule CTE, so a
// record the reader cannot see has none; a select by its option's label.
func optionLabelOf(s *stmt, e expr) string {
	if e.target != nil {
		return labelValueSQL(s, e.target, labelFieldOf(e.target, ""), e.sql)
	}
	return asShown(s.bind, e).sql
}
