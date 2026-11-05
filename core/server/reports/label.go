package reports

import (
	"encoding/hex"
	"encoding/json"
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
// currency field, or its source's fixed currency for c's reader; NULL for any
// other label, and for a fixed currency when dry.
func (c *compiler) labelCurrencySQL(s *stmt, target *Source, f *Field, key string) string {
	switch {
	case f == nil || f.Type != Money:
		return "NULL"
	case f.Currency != "":
		return recordCurrencySQL(s, target, nil, target, f, key)
	case target.Currency != nil && !c.dry():
		return s.bind(c.fixedCurrency(target))
	}
	return "NULL"
}

// selectLabelSQL is a select value shown by its option's label; a value no
// option names shows as stored.
func selectLabelSQL(s *stmt, f *Field, value string) string {
	return optionLabelSQL(s.bind, f.Options, value)
}

// optionLabelSQL is a select's stored value shown by its option's label; a
// value no option names shows as stored.
//
// The options are one JSON parameter however many there are (a CASE with a
// WHEN per option binds two per option and per read, past SQLite's 32,766
// variables for a long list). The object's keys are "k" and the hex of each
// value's UTF-8 bytes, so any value is one key that a JSON path can name;
// hex(value) builds the same key in SQL. The value stays outside any
// subquery, so an aggregate or a window function in it keeps its meaning.
//
// It matches as "CASE value WHEN ? THEN ? … ELSE value END" does: a value
// has no affinity at every caller (a select's NULLIF read, a group key from
// one, an if/case/coalesce result) and no collation, so a WHEN matches
// exactly a text value with the same bytes; a number, a blank or any other
// type matches none. The first option of a value wins, as the first WHEN
// does.
func optionLabelSQL(bind func(any) string, options []Option, value string) string {
	if len(options) == 0 {
		return value
	}
	labels := bind(optionLabelsJSON(options))
	return fmt.Sprintf("(CASE WHEN typeof(%[1]s) = 'text' THEN COALESCE(json_extract(%[2]s, '$.k' || hex(%[1]s)), %[1]s) ELSE %[1]s END)", value, labels)
}

// optionLabelsJSON is {"k<HEX of value>": label, …}, the first option of each
// value only.
func optionLabelsJSON(options []Option) string {
	seen := map[string]bool{}
	parts := make([]string, 0, len(options))
	for _, o := range options {
		if seen[o.Value] {
			continue
		}
		seen[o.Value] = true
		label, _ := json.Marshal(o.Label) // a string always encodes
		parts = append(parts, `"k`+strings.ToUpper(hex.EncodeToString([]byte(o.Value)))+`":`+string(label))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// optionsOf is a select field's options; nil for any other field.
func optionsOf(f *Field) []Option {
	if f == nil || f.Type != Select {
		return nil
	}
	return f.Options
}

// pathExpr is a path's field value, read by sql: typed as the field, a
// relation with the source it points at, a select with its options.
func pathExpr(sql string, p *pathPlan) expr {
	return expr{sql: sql, typ: p.field.Type, target: p.target, options: optionsOf(p.field)}
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
	c     *compiler
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

func newLabelRefs(c *compiler, s *stmt, frame *windowFrame, alias string) *labelRefs {
	return &labelRefs{c: c, s: s, frame: frame, alias: alias, refs: map[string]labelRef{}}
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
	if cur := lr.c.labelCurrencySQL(lr.s, target, f, key); cur != "NULL" {
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
// is shown by the record's label, by the label field of the column it reads,
// and a select by its option's label; any other value keeps its type, and an
// amount its currency (a carried one beside it), so the client formats the
// label as a cell (R9).
func (c *compiler) compileDetailLabel(rc *rowsCtx, labels *labelRefs, n *Node) (labelRef, error) {
	e, err := compileFormula(rc, n, "detail.label")
	if err != nil {
		return labelRef{}, err
	}
	e = asShown(rc.bind, e)
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
