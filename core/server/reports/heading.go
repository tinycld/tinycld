package reports

import (
	"fmt"
	"strings"
)

// headingCtx compiles a column's heading: params, values and scalar
// operators only, so the heading is one value for the whole run.
type headingCtx struct {
	c *compiler
	s *stmt
	// what names the value in a refusal: "a heading", or an opening row's
	// label value, which is one value for the whole run too.
	what string
}

func (h *headingCtx) only(at string) error {
	return invalid(at, h.what+" reads params and values only")
}
func (h *headingCtx) col(_, at string) (expr, error) { return expr{}, h.only(at) }

// compileFormula hands path the leaf's ".path" key; the refusal names the
// leaf itself, the node a heading may not hold.
func (h *headingCtx) path(_ Path, at string) (expr, error) {
	return expr{}, h.only(strings.TrimSuffix(at, ".path"))
}
func (h *headingCtx) subtotal(_, at string) (expr, error)     { return expr{}, h.only(at) }
func (h *headingCtx) exists(_ *Node, at string) (expr, error) { return expr{}, h.only(at) }
func (h *headingCtx) lookup(_ *Node, at string) (expr, error) { return expr{}, h.only(at) }
func (h *headingCtx) record(_ string, _ Path, at string) (expr, error) {
	return expr{}, h.only(at)
}
func (h *headingCtx) outerPath(_ Path, at string) (expr, error) { return expr{}, h.only(at) }
func (h *headingCtx) aggregate(_ Aggregate, _ *Node, _ int, at string) (expr, error) {
	return expr{}, h.only(at)
}
func (h *headingCtx) window(_ string, _ *Node, _ int, at string) (expr, error) {
	return expr{}, h.only(at)
}
func (h *headingCtx) param(id, at string) (expr, error) {
	v, err := paramOf(h.c, id, at)
	if err != nil {
		return expr{}, err
	}
	return paramLeaf(h.c, h.s, h, id, v)
}
func (h *headingCtx) bind(v any) string              { return h.s.bind(v) }
func (h *headingCtx) fiscalStart() string            { return h.bind(int64(h.c.fiscal)) }
func (h *headingCtx) readerDay(day string) *zonedDay { return h.c.readerDay(day) }
func (h *headingCtx) filter() bool                   { return false }
func (h *headingCtx) label(e expr) string            { return h.c.textOf(h.s, e) }
func (h *headingCtx) currencySQL(cur currency) string {
	return currencyCode(h, h.c, cur, func(string) string { return "NULL" })
}

// headingPlan is one column's compiled heading.
type headingPlan struct {
	col string
	e   expr
}

// headings compiles every column's heading; a heading on an unset param is
// left out, so the column shows its label.
func (c *compiler) headings(s *stmt) ([]headingPlan, error) {
	var out []headingPlan
	for _, cp := range c.p.cols {
		n := cp.col.Heading
		if n == nil {
			continue
		}
		at := cp.at + ".heading"
		e, err := runValue(&headingCtx{c: c, s: s, what: "a heading"}, n, at)
		if err != nil {
			return nil, err
		}
		if !e.unset {
			out = append(out, headingPlan{col: cp.col.ID, e: e})
		}
	}
	return out, nil
}

// runValue compiles a formula that is one value for the whole run. A date
// range or a list is no one value a view can show; an amount has no currency
// to print it in.
func runValue(h *headingCtx, n *Node, at string) (expr, error) {
	e, err := compileFormula(h, n, at)
	if err != nil {
		return expr{}, err
	}
	ok := !e.rng && !e.list
	switch e.typ {
	case Text, Date, DateTime, Number, "":
	default:
		ok = false
	}
	if !ok {
		return expr{}, invalid(at, h.what+" is a text, a day or a number")
	}
	return e, nil
}

// openingLabelPlan is one group level's compiled opening label value.
type openingLabelPlan struct {
	level int
	e     expr
	typ   FieldType
}

// openingLabels compiles every opening row's label value; one on an unset
// param is left out, so the row's label has no value.
func (c *compiler) openingLabels(s *stmt) ([]openingLabelPlan, error) {
	var out []openingLabelPlan
	for i, g := range c.p.groups {
		op := g.group.Opening
		if op == nil || op.LabelValue == nil {
			continue
		}
		h := &headingCtx{c: c, s: s, what: "an opening row's label value"}
		e, err := runValue(h, op.LabelValue, g.at+".opening.labelValue")
		if err != nil {
			return nil, err
		}
		typ := e.typ
		if op.LabelBucket != "" {
			// A date-time's day depends on the reader's zone; only a day has
			// one bucket for every reader.
			if e.typ != Date {
				return nil, invalid(g.at+".opening.labelBucket", "a labelBucket needs a labelValue that is a day")
			}
			e.sql, typ = bucketSQL(h, op.LabelBucket, e.sql), bucketType(op.LabelBucket)
		}
		if !e.unset {
			out = append(out, openingLabelPlan{level: i + 1, e: e, typ: typ})
		}
	}
	return out, nil
}

// shownRunValue is a run value as a view shows it, false for a blank one.
func shownRunValue(v any, typ FieldType) (any, FieldType, bool) {
	if v == nil {
		return nil, "", false
	}
	if typ == "" {
		typ = Text
	}
	// Whitespace-only counts as blank too: it draws as an empty header.
	if text, ok := v.(string); ok && typ == Text && strings.TrimSpace(text) == "" {
		return nil, "", false
	}
	if typ == Date {
		// A day reads as YYYY-MM-DD, as a date param's value does.
		if text, ok := v.(string); ok && len(text) >= 10 {
			v = text[:10]
		}
	}
	return v, typ, true
}

// readHeadings computes the run's headings and its opening rows' label
// values in one statement.
func (rn *runner) readHeadings() error {
	s := newStmt(rn.app, rn.c.info)
	plans, err := rn.c.headings(s)
	if err != nil {
		return err
	}
	labels, err := rn.c.openingLabels(s)
	if err != nil || len(plans)+len(labels) == 0 {
		return err
	}
	cells := make([]string, 0, len(plans)+len(labels))
	for i, h := range plans {
		cells = append(cells, fmt.Sprintf("%s AS h%d", h.e.sql, i))
	}
	for i, l := range labels {
		cells = append(cells, fmt.Sprintf("%s AS o%d", l.e.sql, i))
	}
	rows, err := rn.query(s, nil, "SELECT "+strings.Join(cells, ", "))
	if err != nil || len(rows) == 0 {
		return err
	}
	rn.headings = map[string]*ColumnHeading{}
	for i, h := range plans {
		if v, typ, ok := shownRunValue(rows[0][fmt.Sprintf("h%d", i)], h.e.typ); ok {
			rn.headings[h.col] = &ColumnHeading{Value: v, Type: typ}
		}
	}
	rn.openingLabels = map[int]*ColumnHeading{}
	for i, l := range labels {
		if v, typ, ok := shownRunValue(rows[0][fmt.Sprintf("o%d", i)], l.typ); ok {
			rn.openingLabels[l.level] = &ColumnHeading{Value: v, Type: typ}
		}
	}
	return nil
}
