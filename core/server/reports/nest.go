package reports

import (
	"fmt"
	"strings"
)

// nestParent puts in w, as n0, the id of the row the first nest level
// hangs from: the row the hops before the nest link reach.
func (r *rowsCTE) nestParent() string {
	hops := r.c.p.nest[0].hops
	return r.add("n0", q(r.root.join(hops[:len(hops)-1]), "id"))
}

// nestPass runs one query per nest level, each keyed by the ids of the
// level above: the base rows of the page, then each level's own rows.
// Linked rows the reader cannot see are left out.
func (rn *runner) nestPass(details []map[string]any) ([]map[any][]map[string]any, error) {
	p := rn.c.p
	if len(p.nest) == 0 {
		return nil, nil
	}
	var out []map[any][]map[string]any
	parents := distinct(details, "__n0")
	for level := range p.nest {
		rows, err := rn.nestLevel(level, parents)
		if err != nil {
			return nil, err
		}
		byParent := map[any][]map[string]any{}
		for _, row := range rows {
			byParent[row["__parent"]] = append(byParent[row["__parent"]], row)
		}
		out = append(out, byParent)
		parents = distinct(rows, "__next")
	}
	return out, nil
}

func distinct(rows []map[string]any, key string) []any {
	seen := map[any]bool{}
	var out []any
	for _, row := range rows {
		if v := row[key]; v != nil && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// maxNestParents is the most parent ids one nest statement binds. SQLite
// refuses more than 32,766 variables; the rest are left for the statement's
// other parameters. The default row cap and page sizes stay under it.
const maxNestParents = 30000

// nestLevel reads one level's child rows from the child collection's rule
// CTE (through newScope), under the run's time limit (through rn.query).
// The level keeps its first rowCap rows in parent id order, so at the cap
// the children of the highest parent ids are the ones left out.
func (rn *runner) nestLevel(level int, parents []any) ([]map[string]any, error) {
	p := rn.c.p
	if len(parents) == 0 {
		return nil, nil
	}
	if len(parents) > maxNestParents {
		return nil, invalid("columns", fmt.Sprintf("the report nests child rows under %d rows; the most is %d: filter the report or lower its row limit", len(parents), maxNestParents))
	}
	nl := p.nest[level]
	link := nl.hops[len(nl.hops)-1]
	s := newStmt(rn.app, rn.c.info)
	sc := newScope(s, link.to)
	back := q(sc.alias, link.link.BackField)
	sel := []string{back + " AS __parent", q(sc.alias, "id") + " AS __id"}
	var order []string
	for _, cp := range nl.cols {
		rest := &pathPlan{hops: cp.path.hops[len(nl.hops):], field: cp.path.field}
		v := sc.value(rest).sql
		sel = append(sel, fmt.Sprintf("%s AS v%d", v, p.index(cp)))
		if rn.c.labelled[cp.col.ID] {
			label := labelValueSQL(s, cp.path.target, labelFieldOf(cp.path.target, cp.col.LabelField), v)
			sel = append(sel, fmt.Sprintf("%s AS vl%d", label, p.index(cp)))
		}
		order = append(order, "("+v+" IS NULL)", v)
		if cur := p.nestCurrency(cp); cur.key != "" {
			field := cp.path.src.field(cp.path.field.Currency)
			sel = append(sel, sc.value(&pathPlan{hops: rest.hops, field: field}).sql+" AS "+cur.key)
		}
	}
	if level+1 < len(p.nest) {
		next := p.nest[level+1].hops
		sel = append(sel, q(sc.join(next[len(nl.hops):len(next)-1]), "id")+" AS __next")
	}
	ids := make([]string, len(parents))
	for i, id := range parents {
		ids[i] = s.bind(id)
	}
	body := fmt.Sprintf("SELECT %s\n%s\nWHERE %s IN (%s)\nORDER BY __parent, %s LIMIT %s",
		strings.Join(sel, ", "), sc.render(), back, strings.Join(ids, ", "),
		strings.Join(append(order, "__id"), ", "), s.bind(int64(runLimits.rowCap+1)))
	rows, err := rn.query(s, nil, body)
	if err != nil {
		return nil, err
	}
	if len(rows) > runLimits.rowCap {
		rows = rows[:runLimits.rowCap]
		rn.noticeOnce(Notice{Code: "nestCap", Message: fmt.Sprintf("The report shows the first %d child rows.", runLimits.rowCap), Count: runLimits.rowCap})
	}
	return rows, nil
}

// nestCurrency is a child row amount's currency: its currency field, which
// nestLevel selects as u<idx>, or its source's fixed currency.
func (p *plan) nestCurrency(cp *colPlan) currency {
	f := cp.path.field
	switch {
	case f.Type != Money:
		return currency{}
	case f.Currency != "":
		return currency{key: fmt.Sprintf("u%d", p.index(cp))}
	case cp.path.src.Currency != nil:
		return currency{fixed: cp.path.src}
	}
	return currency{}
}

// nestedRows emits the child rows of one parent, each followed by its own
// children.
func (rn *runner) nestedRows(t *tree, level int, parentKey any, parentID, section string, depth int) {
	if level >= len(t.nested) || parentKey == nil {
		return
	}
	cols := rn.c.p.nest[level].cols
	for _, row := range t.nested[level][parentKey] {
		id := fmt.Sprintf("%s/%v", parentID, row["__id"])
		cells := map[string]any{}
		for _, cp := range cols {
			cells[cp.col.ID] = rn.cellValue(cp, row[fmt.Sprintf("v%d", rn.c.p.index(cp))], false)
			if cur := rn.c.p.nestCurrency(cp); cur.key != "" {
				cells[cp.col.ID+"@currency"] = row[cur.key]
			}
			rn.labelCell(cells, cp, row)
		}
		rn.push(Row{ID: id, Kind: KindDetail, Depth: depth, Label: rn.firstLabel(cols, row), Section: section, Parent: parentID, Cells: cells})
		rn.nestedRows(t, level+1, row["__next"], id, section, depth+1)
	}
}
