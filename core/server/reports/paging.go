package reports

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

// cursor is a page cursor: a fingerprint of the group levels and sort keys,
// the open innermost group the last row is in (a grouped report only), then
// the sort values of the last row and its id.
type cursor struct {
	Key    string `json:"k"`
	Group  []any  `json:"g,omitempty"`
	Values []any  `json:"v"`
}

// valueKind is the JSON type a sort key's value has in a cursor: n for a
// number, s for text, a for a column whose type is not known.
func valueKind(k sortKey) string {
	switch {
	case k.name == "__id":
		return "s"
	case k.typ == "":
		return "a"
	case isNumeric(k.typ):
		return "n"
	}
	return "s"
}

// fingerprint ties a cursor to the group levels (groupLevels, "" without
// groups) and the sort columns, directions and value types of the report and
// run that gave it (R9d).
func fingerprint(levels string, keys []sortKey) string {
	h := sha256.New()
	if levels != "" {
		h.Write([]byte("groups:" + levels + ";"))
	}
	for _, k := range keys {
		h.Write([]byte(k.col + "|" + dirOf(k.desc) + "|" + valueKind(k) + ";"))
	}
	return hex.EncodeToString(h.Sum(nil)[:6])
}

// cursorOf is the page cursor after row: its group (nil without groups), its
// sort values, then its id.
func cursorOf(levels string, keys []sortKey, row map[string]any, group []any) string {
	values := make([]any, len(keys))
	for i, k := range keys {
		values[i] = row[k.name]
	}
	b, _ := json.Marshal(cursor{Key: fingerprint(levels, keys), Group: group, Values: values})
	return base64.RawURLEncoding.EncodeToString(b)
}

func badCursor() *SpecError {
	return invalid("options.cursor", "the cursor is not one this report gave")
}

// readCursor decodes a cursor and checks it fits levels and keys: same
// fingerprint, a group exactly when the report has groups (scalars only), one
// value per key, each of the type its key holds, a text id last.
func readCursor(levels string, keys []sortKey, text string) (cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(text)
	if err != nil {
		return cursor{}, badCursor()
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var c cursor
	if err := dec.Decode(&c); err != nil {
		return cursor{}, badCursor()
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return cursor{}, badCursor()
	}
	if c.Key != fingerprint(levels, keys) || len(c.Values) != len(keys) || (levels != "") != (len(c.Group) > 0) {
		return cursor{}, badCursor()
	}
	for i, v := range c.Group {
		switch x := v.(type) {
		case json.Number:
			c.Group[i] = numberValue(x)
		case string, bool, nil:
		default:
			return cursor{}, badCursor()
		}
	}
	if err := readValues(keys, c.Values); err != nil {
		return cursor{}, err
	}
	return c, nil
}

// readValues checks a cursor's detail values against keys, one per key, each
// of the type its key holds, a text id last, and turns numbers into Go numbers.
func readValues(keys []sortKey, values []any) error {
	for i, v := range values {
		kind := valueKind(keys[i])
		switch x := v.(type) {
		case json.Number:
			if kind == "s" {
				return badCursor()
			}
			values[i] = numberValue(x)
		case string:
			if kind == "n" {
				return badCursor()
			}
		case nil:
			if i == len(keys)-1 {
				return badCursor()
			}
		default:
			return badCursor()
		}
	}
	return nil
}

// orderTerm is one term a level orders its rows by: SQL over the level's
// a<L> row, and its direction. A level's "(x IS NULL)" terms are terms too.
type orderTerm struct {
	sql  string
	desc bool
}

const (
	cursorIn    = "in"    // the page ended inside the group: after its row (and opening), or after detail v of it
	cursorAfter = "after" // the page ended after the group's whole subtree
)

// groupCursor is a paged grouped report's cursor (Ruling R6).
type groupCursor struct {
	Key    string  `json:"k"`
	Group  []any   `json:"g"`           // the path's deepest group key: group keys, then currency keys
	Path   [][]any `json:"p"`           // each path level's order values, level 1 first
	Mode   string  `json:"m"`           // cursorIn or cursorAfter
	Values []any   `json:"v,omitempty"` // detail sort values then the id, inside an innermost group only
	Held   []held  `json:"h,omitempty"` // groups whose first details were shown, not yet passed (heldAfter)
}

// held is an open innermost group whose details up to Values (sort values
// then the id) an earlier page showed, and that sorts after where the reader
// is now: it moved later between pages. The pages read none of those details
// again when they reach it at its new place.
type held struct {
	Group  []any `json:"g"`
	Values []any `json:"v"`
}

// maxHeld bounds a cursor's held groups. Each is a group that moved later
// while the reader was inside it; past the bound the oldest is dropped, and
// its first details may show again at its new place.
const maxHeld = 32

// groupPos is where a page ended, before it is written as a cursor.
type groupPos struct {
	key  []any
	path [][]any
	mode string
	held []held
}

// foldPrint names a run's fold state for a group cursor's fingerprint:
// expandAll, the open levels, and the expanded and folded keys in sorted
// order. A cursor from another fold state is refused (J1a); the client then
// reads from page 1, as it does after any fold.
func foldPrint(e *expansion) string {
	var b strings.Builder
	if e.all {
		b.WriteString("all;")
	}
	levels := make([]int, 0, len(e.open))
	for n, ok := range e.open {
		if ok {
			levels = append(levels, n)
		}
	}
	slices.Sort(levels)
	for _, n := range levels {
		fmt.Fprintf(&b, "open%d;", n)
	}
	for _, part := range []struct {
		tag string
		set map[string]bool
	}{{"+", e.set}, {"-", e.closed}} {
		keys := make([]string, 0, len(part.set))
		for k := range part.set {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			b.WriteString(part.tag + k + ";")
		}
	}
	return b.String()
}

// levelShape names each group level's order terms (count and directions), so
// a cursor's path values fit the levels that read them.
func levelShape(levels [][]orderTerm) string {
	var b strings.Builder
	for _, terms := range levels {
		for _, t := range terms {
			b.WriteString(dirOf(t.desc) + ",")
		}
		b.WriteString(";")
	}
	return b.String()
}

// groupFingerprint ties a group cursor to the group levels, each level's
// order shape, the fold state and the detail sort keys (R9d, R6).
func groupFingerprint(levels, shape, fold string, keys []sortKey) string {
	return fingerprint(levels+"|shape:"+shape+"|fold:"+fold, keys)
}

// groupCursorOf writes where a page ended; detail is the page's last detail
// row when it ended on one (Mode in).
func groupCursorOf(fp string, keys []sortKey, at groupPos, detail map[string]any) string {
	c := groupCursor{Key: fp, Group: at.key, Path: at.path, Mode: at.mode, Held: at.held}
	if detail != nil {
		c.Values = make([]any, len(keys))
		for i, k := range keys {
			c.Values[i] = detail[k.name]
		}
	}
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

// readGroupCursor decodes a group cursor and checks it fits the report: the
// same fingerprint, a mode the engine writes, a path of 1..groups levels each
// of its level's width, scalars only (0 or 1 at an "(x IS NULL)" term, as the
// engine writes there), and detail values only inside an
// innermost group after a row.
func readGroupCursor(fp string, shape [][]orderTerm, keys []sortKey, groups, currencies int, text string) (groupCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(text)
	if err != nil {
		return groupCursor{}, badCursor()
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var c groupCursor
	if err := dec.Decode(&c); err != nil {
		return groupCursor{}, badCursor()
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return groupCursor{}, badCursor()
	}
	d := len(c.Path)
	if c.Key != fp || (c.Mode != cursorIn && c.Mode != cursorAfter) || d < 1 || d > groups || d > len(shape) || len(c.Group) != d+currencies {
		return groupCursor{}, badCursor()
	}
	for i, level := range c.Path {
		if len(level) != len(shape[i]) || !scalars(level) {
			return groupCursor{}, badCursor()
		}
		for j, t := range shape[i] {
			if t.blankTerm() && !isBlankFlag(level[j]) {
				return groupCursor{}, badCursor()
			}
		}
	}
	if !scalars(c.Group) {
		return groupCursor{}, badCursor()
	}
	if len(c.Held) > maxHeld {
		return groupCursor{}, badCursor()
	}
	for _, h := range c.Held {
		if len(h.Group) != groups+currencies || !scalars(h.Group) || len(h.Values) != len(keys) {
			return groupCursor{}, badCursor()
		}
		if err := readValues(keys, h.Values); err != nil {
			return groupCursor{}, err
		}
	}
	if len(c.Values) > 0 {
		if c.Mode != cursorIn || d != groups || len(c.Values) != len(keys) {
			return groupCursor{}, badCursor()
		}
		if err := readValues(keys, c.Values); err != nil {
			return groupCursor{}, err
		}
	}
	return c, nil
}

// blankTerm is true for a level's "(x IS NULL)" term, which sorts a key's
// blanks last: its value on a row is 0 or 1.
func (t orderTerm) blankTerm() bool {
	return strings.HasPrefix(t.sql, "(") && strings.HasSuffix(t.sql, " IS NULL)")
}

// isBlankFlag is true for the value an "(x IS NULL)" term gives: 0 or 1.
func isBlankFlag(v any) bool {
	n, ok := v.(int64)
	return ok && (n == 0 || n == 1)
}

// scalars checks every value is a JSON scalar and turns numbers into Go
// numbers in place.
func scalars(values []any) bool {
	for i, v := range values {
		switch x := v.(type) {
		case json.Number:
			values[i] = numberValue(x)
		case string, bool, nil:
		default:
			return false
		}
	}
	return true
}

// afterSQL is true for a row whose terms sort after values, in the terms'
// order and directions, and false (never NULL) otherwise, so NOT afterSQL
// counts a row (the pc count of Task 4). A blank value is matched only by a
// blank (the level's "(x IS NULL)" term before it already put every set
// value first), as keysetCond does. Values are bound, never inlined.
func afterSQL(s *stmt, cols []orderTerm, values []any) string {
	cond := "0"
	for i := len(cols) - 1; i >= 0; i-- {
		if values[i] == nil {
			cond = "(" + cols[i].sql + " IS NULL AND " + cond + ")"
			continue
		}
		v := s.bind(values[i])
		cmp := " > "
		if cols[i].desc {
			cmp = " < "
		}
		// A blank row value against a set value: the "(x IS NULL)" term
		// decided it already; coalesce keeps the term false, not NULL.
		cond = "(coalesce(" + cols[i].sql + cmp + v + ", 0) OR (" + cols[i].sql + " IS " + v + " AND " + cond + "))"
	}
	return cond
}

// keysetAfter selects the rows after a cursor in the detail order of a report
// without groups: a page starts where the last one ended, so a row added or
// removed between pages does not shift the rest.
func keysetAfter(s *stmt, keys []sortKey, text string) (string, error) {
	if text == "" {
		return "1", nil
	}
	c, err := readCursor("", keys, text)
	if err != nil {
		return "", err
	}
	return keysetCond(s, keys, c.Values), nil
}

// keysetCond is true for a row (d) after values in the order of keys. The
// values are bound, never inlined. Group levels carry "IS NULL" terms and use
// afterSQL instead.
func keysetCond(s *stmt, keys []sortKey, values []any) string {
	last := len(keys) - 1
	cond := "d.__id > " + s.bind(values[last])
	for i := last - 1; i >= 0; i-- {
		col := "d." + keys[i].name
		if values[i] == nil {
			// NULLs sort last, so after a NULL come only NULLs.
			cond = "(" + col + " IS NULL AND " + cond + ")"
			continue
		}
		cmp := " > "
		if keys[i].desc {
			cmp = " < "
		}
		v := s.bind(values[i])
		cond = "(" + col + " IS NULL OR " + col + cmp + v + " OR (" + col + " = " + v + " AND " + cond + "))"
	}
	return cond
}
