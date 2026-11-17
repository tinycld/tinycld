package reports

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Drill value forms, as a drillPart writes them.
const (
	drillFormula = "formula"
	drillFrom    = "from"
	drillTo      = "to"
	drillGroup   = "group"
	drillParam   = "param"
)

const (
	errGroupKeyOutside   = "groupKey is read only in a drill value"
	errPivotBoundOutside = "pivotBound is read only in a drill's pivotCells"
)

// drillPlan is a drilled column bound to its target: the parts each drilled
// level writes, and the parts its pivot cells lay over them.
type drillPlan struct {
	col    *colPlan
	idx    int // the column's index, which names its cells
	drill  *Drill
	target []ParamDef
	// order is the drilled levels (1 for the first group), ascending.
	order  []int
	levels map[int][]drillPart
	pivot  []drillPart
}

// drillPart is one value a drill writes: param is the target's param, form
// how it is written, node the formula (nil for a copy), source the param a
// copy reads, at where its refusals point, and dp the drill it belongs to.
type drillPart struct {
	param, form, source, at string
	node                    *Node
	dp                      *drillPlan
}

// drillLeaves is a context where a drill value's leaves read: a levelCtx.
type drillLeaves interface {
	groupKey(id, at string) (expr, error)
	pivotBound(bound, at string) (expr, error)
}

// drillLeafState is what a drill value compiles against: for a pivot cell
// part, the bounds of its bucket; blank when its pivot value is no bucket.
type drillLeafState struct {
	pivot, blank bool
	from, to     string
}

func compileDrillLeaf(c fctx, n *Node, at string) (expr, error) {
	d, ok := c.(drillLeaves)
	switch {
	case !ok && n.GroupKey != "":
		return expr{}, invalid(at, errGroupKeyOutside)
	case !ok:
		return expr{}, invalid(at, errPivotBoundOutside)
	case n.GroupKey != "":
		return d.groupKey(n.GroupKey, at)
	}
	return d.pivotBound(n.PivotBound, at)
}

// groupKey is group id's key on this level's rows: a bucket's key as text,
// else the group column as the level reads it (a link's record id).
func (lc *levelCtx) groupKey(id, at string) (expr, error) {
	if lc.drill == nil {
		return expr{}, invalid(at, errGroupKeyOutside)
	}
	i := groupNumber(lc.lv.c.p, id) - 1
	if i < 0 || i >= lc.lv.n {
		return expr{}, invalid(at, fmt.Sprintf("groupKey %s is not a group at or above this level", id))
	}
	g := lc.lv.c.p.groups[i]
	if g.group.Bucket != "" {
		return expr{sql: fmt.Sprintf("a.k%d", i), typ: Text}, nil
	}
	return lc.col(g.col.col.ID, at)
}

// pivotBound is the first or last day of the pivot cell's bucket.
func (lc *levelCtx) pivotBound(bound, at string) (expr, error) {
	if lc.drill == nil || !lc.drill.pivot {
		return expr{}, invalid(at, errPivotBoundOutside)
	}
	// A blank bucket's cell is NULL, so its bound is a literal: binding it
	// would add a param that no SQL reads.
	if lc.drill.blank {
		return expr{sql: "'" + placeholderDay + "'", typ: Date}, nil
	}
	day := lc.drill.from
	if bound == "to" {
		day = lc.drill.to
	}
	return expr{sql: lc.bind(day), typ: Date}, nil
}

// groupNumber is group id's level (1 for the first group), 0 for none.
func groupNumber(p *plan, id string) int {
	for i, g := range p.spec.Groups {
		if g.ID == id {
			return i + 1
		}
	}
	return 0
}

func paramDef(defs []ParamDef, id string) *ParamDef {
	for i := range defs {
		if defs[i].ID == id {
			return &defs[i]
		}
	}
	return nil
}

// isPercentColumn is a column that shows a percent: no rows of its own sit
// behind it to open.
func isPercentColumn(c *Column) bool {
	if c.Format == "percent" {
		return true
	}
	return isPercentPart(c)
}

// isPercentPart is a column whose formula root is a percent part.
func isPercentPart(c *Column) bool {
	return c.Formula != nil && slices.Contains([]string{"percentOfParent", "percentOfTotal", "percentOfSection"}, c.Formula.Op)
}

// percentScale is the places a percent part's column prints: its root node's
// scale. Any other column has none (a percent then prints 1 place).
func percentScale(c *Column) *int {
	if !isPercentPart(c) || c.Formula.Scale == nil {
		return nil
	}
	places := *c.Formula.Scale
	return &places
}

// drills binds every drilled column against its target's params; every
// refusal is a 400 at the drill's spec path.
func (b *binder) drills(p *plan) {
	slug, _, _ := strings.Cut(p.spec.Source, ".")
	for i := range p.spec.Columns {
		c := &p.spec.Columns[i]
		if c.Drill == nil {
			continue
		}
		at := fmt.Sprintf("columns.%d.drill", i)
		switch {
		case i == 0:
			b.bad(at, "the first column labels the rows, so it opens no report")
			continue
		case c.Hidden:
			b.bad(at, "a hidden column shows no figure to open")
			continue
		case isPercentColumn(c):
			b.bad(at, "a percent has no rows of its own to open")
			continue
		}
		target, ok := shippedParamDefs(c.Drill.Report)
		if !ok {
			b.bad(at+".report", fmt.Sprintf("no package ships the report %q", c.Drill.Report))
			continue
		}
		if pkg, _, _ := strings.Cut(c.Drill.Report, "."); pkg != slug {
			b.bad(at+".report", "a drill opens a report of the report's own package")
			continue
		}
		dp := &drillPlan{col: p.byID[c.ID], idx: i, drill: c.Drill, target: target, levels: map[int][]drillPart{}}
		b.drillLevels(p, dp, at)
		b.drillPivot(p, dp, at)
		p.drills = append(p.drills, dp)
	}
}

func (b *binder) drillLevels(p *plan, dp *drillPlan, at string) {
	if len(dp.drill.Levels) == 0 {
		b.bad(at+".levels", "a drill opens from at least one group level")
		return
	}
	for _, g := range slices.Sorted(maps.Keys(dp.drill.Levels)) {
		lat := at + ".levels." + g
		n := groupNumber(p, g)
		if n == 0 {
			b.bad(lat, fmt.Sprintf("%s is not a group of the report", g))
			continue
		}
		// Above its blankAbove level (its own, or one a column it reads
		// carries) the column's cell is blank: no figure there to open.
		if blank := p.blankRead(&Node{Col: dp.col.col.ID}, n); blank != nil {
			b.bad(lat, fmt.Sprintf("column %q is blank above group %q, so the %s rows have no figure to open", blank.col.ID, p.spec.Groups[blank.blankLevel-1].ID, g))
			continue
		}
		dp.order = append(dp.order, n)
		dp.levels[n] = b.drillParts(p, dp, dp.drill.Levels[g], lat, n, false)
	}
	slices.Sort(dp.order)
}

func (b *binder) drillPivot(p *plan, dp *drillPlan, at string) {
	if len(dp.drill.PivotCells) == 0 {
		return
	}
	pat := at + ".pivotCells"
	switch {
	case p.pivot == nil:
		b.bad(pat, "the report has no pivot")
	case p.pivot.cell != dp.col:
		b.bad(pat, "pivotCells goes on the pivot's cell column")
	case len(dp.order) > 0:
		// Laid over every drilled level: a groupKey reads a group at or above
		// the highest one.
		dp.pivot = b.drillParts(p, dp, dp.drill.PivotCells, pat, dp.order[0], true)
	}
}

// drillParts checks each value against its target param and splits it into
// the parts it writes.
func (b *binder) drillParts(p *plan, dp *drillPlan, values map[string]DrillValue, at string, level int, pivot bool) []drillPart {
	var parts []drillPart
	for _, id := range slices.Sorted(maps.Keys(values)) {
		v, vat := values[id], at+"."+id
		def := paramDef(dp.target, id)
		if def == nil {
			b.bad(vat, fmt.Sprintf("the report %s has no param %s", dp.drill.Report, id))
			continue
		}
		if msg := drillForm(p, def, v); msg != "" {
			b.bad(vat, msg)
			continue
		}
		for _, part := range valueParts(id, v, vat) {
			part.dp = dp
			b.drillLeafRefs(p, part.node, part.at, level, pivot)
			parts = append(parts, part)
		}
	}
	return parts
}

// drillForm refuses a value form that cannot fit its target param (R3); a
// formula's type is checked when the value compiles.
func drillForm(p *plan, def *ParamDef, v DrillValue) string {
	var fits bool
	switch {
	case v.Range != nil:
		if v.Range.From == nil || v.Range.To == nil {
			return "a range has a from and a to"
		}
		fits = def.Type == ParamDateRange
	case v.Group != nil:
		fits = def.Type == ParamChoice && def.Multiple && def.OptionsFrom != nil && len(def.OptionsFrom.Group) > 0
	case v.Param != "":
		src := paramDef(p.spec.Params, v.Param)
		if src == nil {
			return fmt.Sprintf("the report has no param %s to copy", v.Param)
		}
		fits = src.Type == def.Type && src.Multiple == def.Multiple
	case v.Formula != nil:
		fits = def.Type == ParamDate || def.Type == ParamAsOf || def.Type == ParamChoice
	default:
		return "a drill value is a formula, a range, a group or a param"
	}
	if !fits {
		return fmt.Sprintf("this value does not fit param %s (%s)", def.ID, def.Type)
	}
	return ""
}

func valueParts(id string, v DrillValue, at string) []drillPart {
	switch {
	case v.Range != nil:
		return []drillPart{
			{param: id, form: drillFrom, node: v.Range.From, at: at + ".from"},
			{param: id, form: drillTo, node: v.Range.To, at: at + ".to"},
		}
	case v.Group != nil:
		return []drillPart{{param: id, form: drillGroup, node: v.Group, at: at + ".group"}}
	case v.Param != "":
		return []drillPart{{param: id, form: drillParam, source: v.Param, at: at}}
	}
	return []drillPart{{param: id, form: drillFormula, node: v.Formula, at: at}}
}

// drillLeafRefs refuses a groupKey of no group or of a level below the
// drilled one, and a pivotBound outside pivotCells or without a bucketed
// pivot.
func (b *binder) drillLeafRefs(p *plan, node *Node, at string, level int, pivot bool) {
	walkDrillNode(node, at, func(n *Node, at string) {
		if n.GroupKey != "" {
			switch g := groupNumber(p, n.GroupKey); {
			case g == 0:
				b.bad(at, fmt.Sprintf("%s is not a group of the report", n.GroupKey))
			case g > level:
				b.bad(at, fmt.Sprintf("groupKey %s is below the level the drill opens from", n.GroupKey))
			}
		}
		if n.PivotBound != "" {
			switch {
			case !pivot:
				b.bad(at, errPivotBoundOutside)
			case n.PivotBound != "from" && n.PivotBound != "to":
				b.bad(at, `pivotBound is "from" or "to"`)
			case p.pivot == nil || p.pivot.pivot.Bucket == "":
				b.bad(at, "pivotBound needs a pivot by a bucket of a date")
			}
		}
	})
}

func walkDrillNode(n *Node, at string, visit func(*Node, string)) {
	if n == nil {
		return
	}
	visit(n, at)
	walkDrillNode(n.Where, at+".where", visit)
	for i, a := range n.Args {
		walkDrillNode(a, fmt.Sprintf("%s.args.%d", at, i), visit)
	}
}

// drillNodes visits every formula of the spec's drills with its spec path,
// so each formula walk (aliases, lookups, sizes, section percents) reaches
// them.
func (s *Spec) drillNodes(visit func(n *Node, at string)) {
	for i := range s.Columns {
		d := s.Columns[i].Drill
		if d == nil {
			continue
		}
		at := fmt.Sprintf("columns.%d.drill", i)
		for _, g := range slices.Sorted(maps.Keys(d.Levels)) {
			drillValueNodes(d.Levels[g], at+".levels."+g, visit)
		}
		drillValueNodes(d.PivotCells, at+".pivotCells", visit)
	}
}

func drillValueNodes(values map[string]DrillValue, at string, visit func(*Node, string)) {
	for _, id := range slices.Sorted(maps.Keys(values)) {
		v, vat := values[id], at+"."+id
		switch {
		case v.Range != nil:
			visit(v.Range.From, vat+".from")
			visit(v.Range.To, vat+".to")
		case v.Group != nil:
			visit(v.Group, vat+".group")
		case v.Formula != nil:
			visit(v.Formula, vat)
		}
	}
}

// resolveDrills reads, once per run, which drilled columns' targets the
// reader may open.
func (rn *runner) resolveDrills() error {
	drills := rn.c.p.drills
	if len(drills) == 0 {
		return nil
	}
	keys := make([]string, 0, len(drills))
	for _, dp := range drills {
		keys = append(keys, dp.drill.Report)
	}
	targets, err := drillTargets(rn.app, rn.c.info, keys)
	if err != nil {
		return err
	}
	rn.drillTargets = map[string]*DrillTarget{}
	for _, dp := range drills {
		if t := targets[dp.drill.Report]; t != nil {
			rn.drillTargets[dp.col.col.ID] = t
		}
	}
	return nil
}

// pivotDrill is the target the pivot columns drill to: the cell column's,
// only when its drill has pivotCells, as only then do its rows draw a
// <cell>.p<j>@drill.
func (rn *runner) pivotDrill(cell *colPlan) *DrillTarget {
	for _, dp := range rn.c.p.drills {
		if dp.col.col.ID == cell.col.ID && len(dp.pivot) > 0 {
			return rn.drillTargets[cell.col.ID]
		}
	}
	return nil
}

// eachDrillCell calls cell for each drill cell in the union's order: its
// name, the drilled level it is compiled on, and its part (j is -1 for a
// level part). A copy writes a posted value, so it has no cell.
func (p *plan) eachDrillCell(pivotCount int, cell func(name string, dp *drillPlan, n, j int, part drillPart) error) error {
	for _, dp := range p.drills {
		for _, n := range dp.order {
			for k, part := range dp.levels[n] {
				if part.form == drillParam {
					continue
				}
				if err := cell(fmt.Sprintf("dv%d_%d_%d", dp.idx, n, k), dp, n, -1, part); err != nil {
					return err
				}
			}
			for j := 0; j < pivotCount && len(dp.pivot) > 0; j++ {
				for k, part := range dp.pivot {
					if part.form == drillParam {
						continue
					}
					if err := cell(fmt.Sprintf("dvp%d_%d_%d_%d", dp.idx, n, j, k), dp, n, j, part); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// drillCells are the level's drill cells: compiled on the drilled level,
// NULL on every other, so each leg of the group statement has the same.
func (lv *level) drillCells(pivot []pivotValue) ([]string, error) {
	var out []string
	err := lv.c.p.eachDrillCell(len(pivot), func(name string, dp *drillPlan, n, j int, part drillPart) error {
		sql := "NULL"
		if n == lv.n {
			st := &drillLeafState{}
			if j >= 0 {
				st.pivot = true
				st.from, st.to, st.blank = lv.pivotBounds(pivot[j])
			}
			s, err := lv.drillPart(dp, part, st)
			if err != nil {
				return err
			}
			// A blank bucket's part compiles only so Validate refuses a
			// misfit. What its other leaves bound stays: a level memoizes
			// its lookups, so a later cell may read those params.
			if !st.blank {
				sql = s
			}
		}
		out = append(out, sql+" AS "+name)
		return nil
	})
	return out, err
}

// placeholderDay stands for the bounds of a pivot value that is no bucket
// (a dry compile's, a blank date's): the part still compiles, so Validate
// refuses a misfit, and its cell is NULL (R6). pivotBound writes it as a
// literal, never a bound param.
const placeholderDay = "0001-01-01"

// pivotBounds is pivot value v's first and last day, cut to the pivot's
// range; blank when v is no bucket.
func (lv *level) pivotBounds(v pivotValue) (from, to string, blank bool) {
	pv := lv.c.p.pivot
	from, to, ok := bucketDays(pv.pivot.Bucket, v.key, lv.c.fiscal)
	if !ok {
		return placeholderDay, placeholderDay, true
	}
	if pv.pivot.Range != "" {
		r := lv.c.params[pv.pivot.Range]
		if r.from != "" && r.from > from {
			from = r.from
		}
		if r.to != "" && r.to < to {
			to = r.to
		}
	}
	return from, to, false
}

// drillPart compiles one drill value on this level, refusing a type that
// cannot fit its target param (R3). A value that reads one row's value has
// none on a group row, as an opening cell has none.
func (lv *level) drillPart(dp *drillPlan, part drillPart, st *drillLeafState) (string, error) {
	wheres := len(lv.wheres)
	e, err := compileFormula(&levelCtx{lv: lv, visiting: lv.visiting, rows: lv.frame, drill: st}, part.node, part.at)
	if err != nil {
		return "", err
	}
	def := paramDef(dp.target, part.param)
	if !drillValueFits(def, part.form, e.typ) {
		return "", invalid(part.at, fmt.Sprintf("this value does not fit param %s (%s): it is %s", def.ID, def.Type, e.typ))
	}
	if lv.readsRowValue(part.node, false, map[string]bool{}) {
		lv.unread(e, wheres)
		return "NULL", nil
	}
	return cellSQL(e), nil
}

func drillValueFits(def *ParamDef, form string, typ FieldType) bool {
	isDate := typ == Date || typ == DateTime
	isChoice := typ == Text || typ == Select || typ == Relation
	switch form {
	case drillFrom, drillTo:
		return isDate
	case drillGroup:
		return isChoice
	}
	switch def.Type {
	case ParamDate, ParamAsOf:
		return isDate
	case ParamChoice:
		return isChoice
	}
	return false
}

// blankDrillCells are every drill cell, NULL: a computed layout row's.
func blankDrillCells(p *plan, pivotCount int) []string {
	var out []string
	// The callback never fails, so neither does the walk.
	_ = p.eachDrillCell(pivotCount, func(name string, _ *drillPlan, _, _ int, _ drillPart) error {
		out = append(out, "NULL AS "+name)
		return nil
	})
	return out
}

// DrillParams is a <col>@drill cell: the target's param values by param
// id, each in the run request's JSON form.
type DrillParams map[string]json.RawMessage

// givenParam is a source param as the run was given it (R4): its posted
// JSON, else its default, else null (the target clears it).
func (c *compiler) givenParam(id string) json.RawMessage {
	d := paramDef(c.p.spec.Params, id)
	raw, ok := c.given[id]
	if !ok && d != nil {
		raw, ok = d.Default, d.Default != nil
	}
	if !ok {
		return json.RawMessage("null")
	}
	if d != nil && d.Type == ParamAmount {
		return amountJSON(raw)
	}
	return raw
}

// amountJSON writes an amount posted as bare cents in its object form: the
// URL reads a bare number as units, not cents, so a copy must not keep it.
func amountJSON(raw json.RawMessage) json.RawMessage {
	// A pointer, because null unmarshals into an int64 as 0.
	var cents *int64
	if json.Unmarshal(raw, &cents) != nil || cents == nil {
		return raw
	}
	return jsonOf(map[string]any{"amount": *cents, "currency": ""})
}

// drillCellsOf writes the <col>@drill cells of a group or subtotal row of
// level n, for each column drilled from that level whose target the reader
// may view.
func (rn *runner) drillCellsOf(row map[string]any, n int, out map[string]any) {
	for _, dp := range rn.c.p.drills {
		parts, ok := dp.levels[n]
		if !ok || rn.drillTargets[dp.col.col.ID] == nil {
			continue
		}
		values, ok := rn.drillParams(parts, func(k int) any { return row[fmt.Sprintf("dv%d_%d_%d", dp.idx, n, k)] })
		if !ok {
			continue
		}
		out[dp.col.col.ID+"@drill"] = values
		// Each pivot cell drills with the level's values and its pivot
		// values laid over them; the cell column, the pivot's total, with the
		// level's values alone.
		for j := range rn.pivotValues {
			if len(dp.pivot) == 0 {
				break
			}
			over, ok := rn.drillParams(dp.pivot, func(k int) any { return row[fmt.Sprintf("dvp%d_%d_%d_%d", dp.idx, n, j, k)] })
			if !ok {
				continue
			}
			merged := maps.Clone(values)
			maps.Copy(merged, over)
			out[fmt.Sprintf("%s.p%d@drill", dp.col.col.ID, j)] = merged
		}
	}
}

// drillParams writes parts' values; false when one is blank (R5): a NULL,
// an empty text, a day that is not a day, or a range that ends before it
// starts.
func (rn *runner) drillParams(parts []drillPart, value func(k int) any) (DrillParams, bool) {
	out := DrillParams{}
	ranges := map[string]*[2]string{}
	for k, part := range parts {
		if part.form == drillParam {
			out[part.param] = rn.c.givenParam(part.source)
			continue
		}
		v := value(k)
		if v == nil {
			return nil, false
		}
		text := textOf(v)
		if text == "" {
			return nil, false
		}
		def := paramDef(part.dp.target, part.param)
		switch {
		case part.form == drillFrom || part.form == drillTo:
			day, ok := dayOf(text)
			if !ok {
				return nil, false
			}
			if ranges[part.param] == nil {
				ranges[part.param] = &[2]string{}
			}
			i := 0
			if part.form == drillTo {
				i = 1
			}
			ranges[part.param][i] = day
		case part.form == drillGroup:
			out[part.param] = jsonOf([]map[string]string{{"group": text}})
		case def.Type == ParamDate || def.Type == ParamAsOf:
			day, ok := dayOf(text)
			if !ok {
				return nil, false
			}
			out[part.param] = jsonOf(day)
		case def.Multiple:
			out[part.param] = jsonOf([]string{text})
		default:
			out[part.param] = jsonOf(text)
		}
	}
	for id, r := range ranges {
		if r[0] > r[1] {
			return nil, false
		}
		out[id] = jsonOf(map[string]string{"from": r[0], "to": r[1]})
	}
	return out, true
}

// dayOf is a stored date's day (its first 10 characters).
func dayOf(s string) (string, bool) {
	if len(s) < 10 || !isDay(s[:10]) {
		return "", false
	}
	return s[:10], true
}

// jsonOf marshals a text, a list of texts or a map of texts, which cannot
// fail.
func jsonOf(v any) json.RawMessage {
	raw, _ := json.Marshal(v)
	return raw
}
