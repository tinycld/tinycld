package reports

import (
	"database/sql/driver"
	"fmt"
)

// sectionPercent is a column whose formula is percentOfSection: each row's
// value of arg as a percent of the named section's total of arg, at scale.
// Its cells are filled after the statements (sectionPercentCell): a
// section's total holds its within rows, which only the layout's union legs
// compute, so no statement can read it beside the rows it divides (R1).
type sectionPercent struct {
	col, arg *colPlan
	section  string
	scale    int
}

// sectionPercents binds every percentOfSection column. It refuses the
// operator anywhere but a column's whole formula, and any formula, group or
// cell that reads such a column: its value is not in the statement, so a
// reader there would see blank (R2).
func (b *binder) sectionPercents(p *plan) {
	p.percents = map[string]*sectionPercent{}
	sections := map[string]bool{}
	for i := range p.spec.Layout {
		if s := p.spec.Layout[i].Section; s != "" {
			sections[s] = true
		}
	}
	for _, cp := range p.cols {
		if n := cp.col.Formula; n != nil && n.Op == "percentOfSection" {
			b.sectionPercent(p, cp, sections)
		}
	}
	walkSpecFormulas(p, func(n *Node, at string, columnRoot bool) {
		switch {
		case n.Op == "percentOfSection" && !columnRoot:
			b.bad(at, "percentOfSection is a column's whole formula, not a part of one")
		case n.Col != "" && p.percents[n.Col] != nil:
			b.bad(at, fmt.Sprintf("no formula reads %q: a percent of a section is filled after the report's totals", n.Col))
		}
	})
	for i := range p.spec.Groups {
		if p.percents[p.spec.Groups[i].Col] != nil {
			b.bad(fmt.Sprintf("groups.%d.col", i), "a report does not group by a percent of a section")
		}
	}
	for i := range p.spec.Layout {
		for _, id := range p.spec.Layout[i].cellIDs() {
			if p.percents[id] != nil {
				b.bad(fmt.Sprintf("layout.%d.cells.%s", i, id), "a percent of a section divides the row's own cell: it takes no cell formula")
			}
		}
	}
}

func (b *binder) sectionPercent(p *plan, cp *colPlan, sections map[string]bool) {
	n, at := cp.col.Formula, cp.at+".formula"
	switch {
	case len(p.spec.Layout) == 0:
		b.bad(at, "percentOfSection divides by a layout section's total: only a report with a layout has sections")
		return
	case n.Subtotal == "":
		b.bad(at+".subtotal", "percentOfSection names its section in subtotal")
		return
	case !sections[n.Subtotal]:
		b.missing(at+".subtotal", "unknown_subtotal", fmt.Sprintf("the layout has no section %q", n.Subtotal))
		return
	case cp.col.Aggregate != "":
		b.bad(cp.at+".aggregate", "a percent of a section has no aggregate: each row shows its own share")
		return
	// A null argument decodes to a nil node; refuse it here, before compile.
	case len(n.Args) != 1 || n.Args[0] == nil || n.Args[0].Col == "" || nodeKinds(n.Args[0]) != 1:
		b.bad(at+".args.0", `percentOfSection takes one column, {"col": <id>}`)
		return
	}
	arg := p.byID[n.Args[0].Col]
	if arg == nil {
		b.merge(unknownColumn(at+".args.0", n.Args[0].Col))
		return
	}
	// A missing or bad scale is refused where the formula compiles (needScale).
	scale := 0
	if n.Scale != nil {
		scale = *n.Scale
	}
	p.percents[cp.col.ID] = &sectionPercent{col: cp, arg: arg, section: n.Subtotal, scale: scale}
}

// readSectionBases reads, for one currency block, each percent column's
// section total of its argument from the row the section shows, so a
// folded section divides by the same total.
func (rn *runner) readSectionBases(t *tree, total map[string]any) {
	if len(rn.c.p.percents) == 0 {
		return
	}
	ck := rn.currencyKey(total)
	byKey := rn.sectionsByKey(t, ck)
	for i := range rn.c.p.layout {
		item := rn.c.p.layout[i]
		if item.Section == "" {
			continue
		}
		row := rn.sectionRow(t, item, total, byKey)
		for id, sp := range rn.c.p.percents {
			if sp.section == item.Section {
				rn.sectionBases[id+"|"+ck] = row[fmt.Sprintf("v%d", rn.c.p.index(sp.arg))]
			}
		}
	}
}

// sectionPercentCell is a row's own argument cell as a percent of its
// currency block's section total, rounded as tc_pct rounds: blank for a
// blank cell or a zero or blank total.
func (rn *runner) sectionPercentCell(sp *sectionPercent, row map[string]any) any {
	part := row[fmt.Sprintf("v%d", rn.c.p.index(sp.arg))]
	whole := rn.sectionBases[sp.col.col.ID+"|"+rn.currencyKey(row)]
	v, err := sqlPct(nil, []driver.Value{part, whole, int64(sp.scale)})
	if err != nil {
		if rn.assembleErr == nil {
			rn.assembleErr = mapRunError(err)
		}
		return nil
	}
	return v
}

// walkSpecFormulas calls visit on every node of every formula the spec
// holds, with where it is. columnRoot is true only for the root node of a
// column's formula.
func walkSpecFormulas(p *plan, visit func(n *Node, at string, columnRoot bool)) {
	var walk func(n *Node, at string, root bool)
	walk = func(n *Node, at string, root bool) {
		if n == nil {
			return
		}
		visit(n, at, root)
		for i, a := range n.Args {
			walk(a, fmt.Sprintf("%s.args.%d", at, i), false)
		}
		walk(n.Where, at+".where", false)
		if l, ok := p.lookups[n]; ok {
			walk(l.value, at+".value", false)
		}
	}
	s := p.spec
	walk(s.Filter, "filter", false)
	for i := range s.Columns {
		c, at := &s.Columns[i], fmt.Sprintf("columns.%d", i)
		walk(c.Formula, at+".formula", true)
		walk(c.Rollup, at+".rollup", false)
		walk(c.Heading, at+".heading", false)
	}
	for i := range s.Sort {
		walk(s.Sort[i].Formula, fmt.Sprintf("sort.%d.formula", i), false)
	}
	for i := range s.Groups {
		g, at := &s.Groups[i], fmt.Sprintf("groups.%d", i)
		walk(g.Label, at+".label", false)
		walk(g.Having, at+".having", false)
		if g.Fill != nil {
			walk(g.Fill.Where, at+".fill.where", false)
		}
		if g.Opening != nil {
			for _, id := range g.Opening.cellIDs() {
				walk(g.Opening.Cells[id], at+".opening.cells."+id, false)
			}
		}
	}
	for i := range s.Layout {
		item, at := &s.Layout[i], fmt.Sprintf("layout.%d", i)
		walk(item.Formula, at+".formula", false)
		for _, id := range item.cellIDs() {
			walk(item.Cells[id], at+".cells."+id, false)
		}
	}
	if s.Detail != nil {
		walk(s.Detail.Label, "detail.label", false)
	}
	for i := range s.Params {
		if o := s.Params[i].OptionsFrom; o != nil {
			walk(o.Where, fmt.Sprintf("params.%d.optionsFrom.where", i), false)
		}
	}
}
