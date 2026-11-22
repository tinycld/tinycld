package reports

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// checkHeaderShape decodes a spec's header before Load's strict decode, so a
// shape error names its place: header that is not a list of objects is
// refused at header, and a line that does not decode strictly as a HeaderLine
// (a text that is not a string, a key a line does not have) at header.<i>.
func checkHeaderShape(raw []byte) error {
	var top struct {
		Header json.RawMessage `json:"header"`
	}
	if err := json.Unmarshal(raw, &top); err != nil {
		return invalid("spec", "the spec is not valid: "+err.Error())
	}
	if len(top.Header) == 0 || string(top.Header) == "null" {
		return nil
	}
	const shape = "header is a list of {text, value} lines"
	var lines []json.RawMessage
	if err := json.Unmarshal(top.Header, &lines); err != nil {
		return invalid("header", shape)
	}
	for _, line := range lines {
		if !bytes.HasPrefix(bytes.TrimSpace(line), []byte("{")) {
			return invalid("header", shape)
		}
	}
	for i, line := range lines {
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.DisallowUnknownFields()
		var l HeaderLine
		if err := dec.Decode(&l); err != nil {
			return invalid(fmt.Sprintf("header.%d", i), "a header line is {text, value}: "+err.Error())
		}
	}
	return nil
}

// headerLines checks each spec header line's shape: a text that holds
// HeaderValueToken exactly once, of at most maxLabel characters, and a value.
// The value's formula is checked where it compiles (compiler.headerValues).
func (b *binder) headerLines(p *plan) {
	for i, l := range p.spec.Header {
		at := fmt.Sprintf("header.%d", i)
		switch {
		case strings.Count(l.Text, HeaderValueToken) != 1:
			b.bad(at+".text", "a header line writes its value with "+HeaderValueToken+" exactly once")
		case characters(l.Text) > maxLabel:
			b.bad(at+".text", fmt.Sprintf("a header line has at most %d characters", maxLabel))
		}
		if l.Value == nil {
			b.bad(at+".value", "a header line has a value")
		}
	}
}

// headerValues compiles each spec header line's value as one value for the
// whole run: params, values and lookups (read under the reader's rules), of a
// text, day, number or amount type. A row field, a column, a total or a group
// record is refused at header.<i>.value. A value on an unset param is unset:
// its line is blank.
func (c *compiler) headerValues(s *stmt) ([]expr, error) {
	out := make([]expr, 0, len(c.p.spec.Header))
	for i, l := range c.p.spec.Header {
		h := &headingCtx{c: c, s: s, what: "a header line's value", lookups: true}
		e, err := runValue(h, l.Value, fmt.Sprintf("header.%d.value", i))
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// readSpecHeaderLines gives one line per spec header line, in order: its text
// with the value written as an export writes a cell of its type (an amount
// with its currency), "" where the value is blank, so line i stays the i-th
// spec line. One statement reads every value, inside the run's read.
func (rn *runner) readSpecHeaderLines() ([]string, error) {
	lines := rn.c.p.spec.Header
	if len(lines) == 0 {
		return nil, nil
	}
	s := newStmt(rn.app, rn.c.info)
	plans, err := rn.c.headerValues(s)
	if err != nil {
		return nil, err
	}
	h := &headingCtx{c: rn.c, s: s, lookups: true}
	cells := []string{}
	for i, e := range plans {
		if e.unset {
			continue
		}
		cells = append(cells, fmt.Sprintf("%s AS v%d", e.sql, i))
		if e.typ == Money {
			cells = append(cells, fmt.Sprintf("%s AS c%d", h.currencySQL(e.cur), i))
		}
	}
	out := make([]string, len(lines))
	if len(cells) == 0 {
		return out, nil
	}
	rows, err := rn.query(s, nil, "SELECT "+strings.Join(cells, ", "))
	if err != nil || len(rows) == 0 {
		return out, err
	}
	f := newExportFormatter(defaultDisplay, "", false)
	for i, e := range plans {
		if e.unset {
			continue
		}
		v, typ, ok := shownRunValue(rows[0][fmt.Sprintf("v%d", i)], e.typ)
		if !ok {
			continue
		}
		cur, _ := rows[0][fmt.Sprintf("c%d", i)].(string)
		out[i] = strings.Replace(lines[i].Text, HeaderValueToken, f.typedLabelText(typ, v, cur), 1)
	}
	return out, nil
}

// specHeaderLines are the result's spec header lines that print: those after
// the params' lines, the blank ones left out. The PDF and the Excel file print
// them under the period.
func specHeaderLines(spec *Spec, res *Result) []string {
	params := 0
	for _, d := range spec.Params {
		if d.Header != "" {
			params++
		}
	}
	var out []string
	for i, line := range res.HeaderLines {
		if i >= params && line != "" {
			out = append(out, line)
		}
	}
	return out
}
