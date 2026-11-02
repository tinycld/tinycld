package reports

import (
	"fmt"
	"strings"
)

// Record labels (R31): a value that points at a record is shown by that
// record's label, never by its id. A label is the label field's stored value,
// typed as the field is, so the client formats it as a cell of that type; a
// select shows its option's label and a source with no field its own name,
// both as text. A record is read through its source's rule CTE, so a record
// the reader cannot see has no label.

// noName is the label of a visible record whose label field is blank.
const noName = "(no name)"

// isLabelType is true for a field type a record can be shown by: any but a
// relation, which would show another record's id.
func isLabelType(t FieldType) bool { return t != Relation }

// labelFieldOf is the field a record is shown by: the chosen field, else the
// first text field, else its first other field but its id; nil when the
// source has none (then its own name is shown). checkLabel checks a chosen
// field.
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

// labelTypeOf is the type of a label read from f: the field's own, but a
// select's option label and a source's name are text.
func labelTypeOf(f *Field) FieldType {
	if f == nil || f.Type == Select {
		return Text
	}
	return f.Type
}

// checkLabel refuses a chosen label field (at) that target cannot be shown
// by; no field chosen is the default rule.
func checkLabel(target *Source, chosen, at string) *SpecError {
	if chosen == "" {
		return nil
	}
	f := labelFieldOf(target, chosen)
	switch {
	case f == nil:
		return unknownRef(at, "unknown_field", fmt.Sprintf("the source %q has no field %q", target.Key, chosen))
	case !isLabelType(f.Type):
		return invalid(at, fmt.Sprintf("%q is a record id or a link, which would show an id: a record is shown by one of its other fields", chosen))
	}
	return nil
}

// labelSQL reads f of the record key points at, through the target's rule
// CTE: NULL for a blank key or a record the reader cannot see. A select
// reads its option's label, a source with no field its name; blank, when
// set, is what a visible record whose field is blank reads.
func labelSQL(s *stmt, target *Source, f *Field, key, blank string) string {
	sc := newScope(s, target)
	v := s.bind(target.Label)
	if f != nil {
		v = sc.value(&pathPlan{field: f, src: target}).sql
		if f.Type == Select {
			v = selectLabelSQL(s, f, v)
		}
		if blank != "" {
			v = fmt.Sprintf("COALESCE(NULLIF(%s, ''), %s)", v, s.bind(blank))
		}
	}
	return fmt.Sprintf("(SELECT %s\n%s\nWHERE %s IS %s)", v, sc.render(), q(sc.alias, "id"), key)
}

// labelValueSQL is the label of the record key points at: its label field's
// typed value, "(no name)" for a blank one, NULL for no visible record.
func labelValueSQL(s *stmt, target *Source, f *Field, key string) string {
	return labelSQL(s, target, f, key, noName)
}

// labelCurrencySQL is the currency of an amount label: the record's own
// currency field, or its source's fixed currency; NULL for any other label.
func labelCurrencySQL(s *stmt, target *Source, f *Field, key string) string {
	switch {
	case f == nil || f.Type != Money:
		return "NULL"
	case f.Currency != "":
		return recordCurrencySQL(s, target, nil, target, f, key)
	case target.Currency != nil && s.app != nil:
		return s.bind(target.Currency(s.app))
	}
	return "NULL"
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

// labelOrder is the label as a sort key: records with no name sort with the
// blank links, after every named record.
func labelOrder(s *stmt, label string) string {
	return "NULLIF(" + label + ", " + s.bind(noName) + ")"
}

// labelRef is a label computed once per row of a frame: its value and its
// currency (NULL for a label that is not an amount).
type labelRef struct {
	value, currency string
}

// labelRefs computes each record label once per row of a frame's select (by
// hoisting it below the select), however many cells, sort keys and window
// orders read it.
type labelRefs struct {
	s     *stmt
	frame *windowFrame
	alias string
	refs  map[string]labelRef
}

// cells are a column's label and its currency in a statement row (vl<idx>,
// vc<idx>), NULL for a column with no label.
func (ref labelRef) cells(idx int) []string {
	value, currency := ref.value, ref.currency
	if value == "" {
		value, currency = "NULL", "NULL"
	}
	return []string{fmt.Sprintf("%s AS vl%d", value, idx), fmt.Sprintf("%s AS vc%d", currency, idx)}
}

func newLabelRefs(s *stmt, frame *windowFrame, alias string) *labelRefs {
	return &labelRefs{s: s, frame: frame, alias: alias, refs: map[string]labelRef{}}
}

// of is the label of the record key points at, read from f.
func (lr *labelRefs) of(target *Source, f *Field, key string) labelRef {
	name := ""
	if f != nil {
		name = f.Key
	}
	id := target.Key + "|" + name + "|" + key
	if ref, ok := lr.refs[id]; ok {
		return ref
	}
	ref := labelRef{value: lr.frame.hoist(lr.alias, labelValueSQL(lr.s, target, f, key)), currency: "NULL"}
	if cur := labelCurrencySQL(lr.s, target, f, key); cur != "NULL" {
		ref.currency = lr.frame.hoist(lr.alias, cur)
	}
	lr.refs[id] = ref
	return ref
}

// labelFieldName is the label field a column's records are shown by: its
// own, else, for a formula that is a column, that column's.
func (p *plan) labelFieldName(cp *colPlan) string {
	for range maxFormulaDepth {
		if cp.col.LabelField != "" {
			return cp.col.LabelField
		}
		n := cp.col.Formula
		if n == nil || n.Col == "" || p.byID[n.Col] == nil {
			return ""
		}
		cp = p.byID[n.Col]
	}
	return ""
}

// checkLabelField refuses a column's label field when it cannot show the
// column's records, and records the type of the column's labels. Every run
// compiles the detail select, which checks every column, before its result
// is assembled.
func (c *compiler) checkLabelField(cp *colPlan, target *Source) error {
	at := cp.at + ".labelField"
	if target == nil {
		if cp.col.LabelField != "" {
			return invalid(at, "only a column that links to a record shows a label field")
		}
		return nil
	}
	if err := checkLabel(target, cp.col.LabelField, at); err != nil {
		return err
	}
	c.labelTypes[cp.col.ID] = labelTypeOf(labelFieldOf(target, c.p.labelFieldName(cp)))
	return nil
}

// columnLabel is the label beside a column's relation value e; false for a
// value with no target or a blank one.
func (c *compiler) columnLabel(lr *labelRefs, cp *colPlan, e expr) (labelRef, bool) {
	if e.target == nil || e.sql == "NULL" {
		return labelRef{}, false
	}
	return lr.of(e.target, labelFieldOf(e.target, c.p.labelFieldName(cp)), e.sql), true
}

// sortLabel is the label a sort key's relation value e sorts by: a column's
// by its label field, a formula's by the label field of the column it reads.
func (c *compiler) sortLabel(lr *labelRefs, s sortPlan, e expr) (labelRef, bool) {
	if s.col != nil {
		return c.columnLabel(lr, s.col, e)
	}
	if e.target == nil || e.sql == "NULL" {
		return labelRef{}, false
	}
	return lr.of(e.target, labelFieldOf(e.target, c.chosenLabel(e)), e.sql), true
}

// chosenLabel is the label field of the column a relation value reads ("" for
// the target's own label field).
func (c *compiler) chosenLabel(e expr) string {
	if cp := c.p.byID[e.col]; cp != nil {
		return c.p.labelFieldName(cp)
	}
	return ""
}

// compileDetailLabel compiles the formula that labels each detail row and
// keeps what it compiled to (c.detailLabel): a value that points at a record
// is shown by the record's label, by the label field of the column it reads;
// any other value keeps its type, and an amount its currency (a carried one
// beside it), so the client formats the label as a cell (R9).
func (c *compiler) compileDetailLabel(rc *rowsCtx, labels *labelRefs, n *Node) (labelRef, error) {
	e, err := compileFormula(rc, n, "detail.label")
	if err != nil {
		return labelRef{}, err
	}
	if e.target == nil {
		lp := labelPlan{typ: e.typ, cur: e.cur}
		ref := labelRef{value: e.sql, currency: "NULL"}
		if e.typ == Money && e.cur.sql != "" {
			ref.currency, lp.recordCurrency = e.cur.sql, true
		}
		c.detailLabel = &lp
		return ref, nil
	}
	f := labelFieldOf(e.target, c.chosenLabel(e))
	c.detailLabel = &labelPlan{typ: labelTypeOf(f), recordCurrency: labelTypeOf(f) == Money}
	if e.sql == "NULL" {
		return labelRef{value: "NULL", currency: "NULL"}, nil
	}
	return labels.of(e.target, f, e.sql), nil
}

// textOf is a relation value read as text, by a text operator: the stored
// value of the record's label field (a select's option label), by the label
// field of the column it reads, and empty text for a blank link or a record
// the reader cannot see.
func (c *compiler) textOf(s *stmt, e expr) string {
	return "COALESCE(" + labelSQL(s, e.target, labelFieldOf(e.target, c.chosenLabel(e)), e.sql, "") + ", '')"
}
