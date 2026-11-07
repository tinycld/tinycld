package reports

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
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
	for i, v := range c.Values {
		kind := valueKind(keys[i])
		switch x := v.(type) {
		case json.Number:
			if kind == "s" {
				return cursor{}, badCursor()
			}
			c.Values[i] = numberValue(x)
		case string:
			if kind == "n" {
				return cursor{}, badCursor()
			}
		case nil:
			if i == len(keys)-1 {
				return cursor{}, badCursor()
			}
		default:
			return cursor{}, badCursor()
		}
	}
	return c, nil
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
// values are bound, never inlined.
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
