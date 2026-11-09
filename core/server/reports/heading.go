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
}

func (h *headingCtx) only(at string) error {
	return invalid(at, "a heading reads params and values only")
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
func (h *headingCtx) bind(v any) string   { return h.s.bind(v) }
func (h *headingCtx) fiscalStart() string { return h.bind(int64(h.c.fiscal)) }
func (h *headingCtx) filter() bool        { return false }
func (h *headingCtx) label(e expr) string { return h.c.textOf(h.s, e) }
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
		e, err := compileFormula(&headingCtx{c: c, s: s}, n, at)
		if err != nil {
			return nil, err
		}
		// A date range or a list is no one value a heading can show; a money
		// heading has no currency to print it in.
		if e.rng || e.list {
			return nil, invalid(at, "a heading is a text, a day or a number")
		}
		switch e.typ {
		case Text, Date, DateTime, Number, "":
		default:
			return nil, invalid(at, "a heading is a text, a day or a number")
		}
		if !e.unset {
			out = append(out, headingPlan{col: cp.col.ID, e: e})
		}
	}
	return out, nil
}

// readHeadings computes the run's headings in one statement.
func (rn *runner) readHeadings() error {
	s := newStmt(rn.app, rn.c.info)
	plans, err := rn.c.headings(s)
	if err != nil || len(plans) == 0 {
		return err
	}
	cells := make([]string, len(plans))
	for i, h := range plans {
		cells[i] = fmt.Sprintf("%s AS h%d", h.e.sql, i)
	}
	rows, err := rn.query(s, nil, "SELECT "+strings.Join(cells, ", "))
	if err != nil || len(rows) == 0 {
		return err
	}
	rn.headings = map[string]*ColumnHeading{}
	for i, h := range plans {
		v := rows[0][fmt.Sprintf("h%d", i)]
		if v == nil {
			continue
		}
		typ := h.e.typ
		if typ == "" {
			typ = Text
		}
		// Whitespace-only counts as blank too: it draws as an empty header.
		if text, ok := v.(string); ok && typ == Text && strings.TrimSpace(text) == "" {
			continue
		}
		if typ == Date {
			// A day reads as YYYY-MM-DD, as a date param's value does.
			if text, ok := v.(string); ok && len(text) >= 10 {
				v = text[:10]
			}
		}
		rn.headings[h.col] = &ColumnHeading{Value: v, Type: typ}
	}
	return nil
}
