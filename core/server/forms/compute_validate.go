package forms

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	validation "github.com/pocketbase/ozzo-validation/v4"
)

// Mirror of core/lib/forms/expression.ts (schema), computed-validation.ts
// (cross-field checks) and orderNodes.

var columnKinds = map[string]bool{"text": true, "number": true, "toggle": true, "date": true, "select": true, "computed": true}

func numericKind(kind string) bool {
	return kind == "number" || kind == "computed" || kind == "toggle"
}

type aggRef struct{ table, column string }

type exprRefs struct {
	vars []string
	aggs []aggRef
}

// parseExpr checks the shape of a formula and collects what it reads. It
// returns "" when the formula is valid.
func parseExpr(e any, refs *exprRefs) string {
	switch x := e.(type) {
	case float64:
		return ""
	case map[string]any:
		if len(x) != 1 {
			return "a formula node has exactly one operator"
		}
		for op, arg := range x {
			return parseOp(op, arg, refs)
		}
	}
	return "a formula is a number or an operator"
}

func varName(v any) (string, bool) {
	m, ok := v.(map[string]any)
	if !ok || len(m) != 1 {
		return "", false
	}
	s, ok := m["var"].(string)
	return s, ok && keyRe.MatchString(s)
}

func parseOp(op string, arg any, refs *exprRefs) string {
	if op == "var" {
		s, ok := arg.(string)
		if !ok || !keyRe.MatchString(s) {
			return "var needs a field key"
		}
		refs.vars = append(refs.vars, s)
		return ""
	}
	args, ok := arg.([]any)
	if !ok {
		return op + " needs a list"
	}
	each := func() string {
		for _, a := range args {
			if msg := parseExpr(a, refs); msg != "" {
				return msg
			}
		}
		return ""
	}
	switch op {
	case "add", "mul":
		if len(args) < 2 {
			return op + " needs at least 2 operands"
		}
		return each()
	case "min", "max":
		if len(args) < 1 {
			return op + " needs at least 1 operand"
		}
		return each()
	case "sub", "div":
		if len(args) != 2 {
			return op + " needs exactly 2 operands"
		}
		return each()
	case "round":
		if len(args) != 2 {
			return "round needs exactly 2 operands"
		}
		if msg := parseExpr(args[0], refs); msg != "" {
			return msg
		}
		d, ok := args[1].(float64)
		if !ok || d != math.Trunc(d) || d < 0 || d > 6 {
			return "round decimals must be a whole number from 0 to 6"
		}
		return ""
	case "sumOf":
		if len(args) != 2 {
			return "sumOf needs a table and a column"
		}
		table, ok := varName(args[0])
		col, ok2 := args[1].(string)
		if !ok || !ok2 || !keyRe.MatchString(col) {
			return "sumOf needs a table and a column"
		}
		refs.aggs = append(refs.aggs, aggRef{table, col})
		return ""
	case "count":
		if len(args) != 1 {
			return "count needs a table"
		}
		table, ok := varName(args[0])
		if !ok {
			return "count needs a table"
		}
		refs.aggs = append(refs.aggs, aggRef{table: table})
		return ""
	}
	return "unknown formula operator " + op
}

// orderKeys orders nodes so each follows what it depends on; dependencies that
// are not nodes are ignored. With no order it returns one closed cycle.
func orderKeys(deps map[string][]string) (order []string, cycle []string) {
	state := map[string]int{}
	var visit func(key string, stack []string) bool
	visit = func(key string, stack []string) bool {
		switch state[key] {
		case 2:
			return true
		case 1:
			i := 0
			for j, k := range stack {
				if k == key {
					i = j
					break
				}
			}
			cycle = append(append([]string{}, stack[i:]...), key)
			return false
		}
		state[key] = 1
		for _, d := range deps[key] {
			if _, ok := deps[d]; !ok {
				continue
			}
			if !visit(d, append(append([]string{}, stack...), key)) {
				return false
			}
		}
		state[key] = 2
		order = append(order, key)
		return true
	}
	keys := make([]string, 0, len(deps))
	for k := range deps {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !visit(k, nil) {
			return nil, cycle
		}
	}
	return order, nil
}

// fieldComputeErrors checks one custom computed or table field in isolation.
func fieldComputeErrors(f Field, i int) validation.Errors {
	switch f.Kind {
	case "computed":
		var refs exprRefs
		if msg := parseExpr(f.Expression, &refs); msg != "" {
			return defErr(msg, "fields", i, "expression")
		}
		if f.Decimals < 0 || f.Decimals > 6 {
			return defErr(fmt.Sprintf("decimals %d is out of range. Use 0 to 6.", f.Decimals), "fields", i, "decimals")
		}
	case "table":
		if len(f.Columns) == 0 {
			return defErr("a table needs at least one column", "fields", i, "columns")
		}
		return columnErrors(f.Columns, "fields", i, "columns")
	}
	return nil
}

func columnErrors(cols []Column, path ...any) validation.Errors {
	seen := map[string]string{}
	for ci, c := range cols {
		at := func(leaf string) []any { return append(append([]any{}, path...), ci, leaf) }
		if !keyRe.MatchString(c.Key) {
			return defErr("invalid column key "+c.Key+". "+keyRule, at("key")...)
		}
		if c.Key == "id" {
			return defErr("id is reserved for the row id", at("key")...)
		}
		if _, dup := seen[c.Key]; dup {
			return defErr("duplicate column key "+c.Key+". Each column key must be unique.", at("key")...)
		}
		seen[c.Key] = c.Kind
		if !columnKinds[c.Kind] {
			return defErr("unknown column kind "+c.Kind+". Kinds: "+strings.Join(sortedBoolKeys(columnKinds), ", ")+".", at("kind")...)
		}
		label := strings.TrimSpace(c.Label)
		if label == "" {
			return defErr("a column needs a label", at("label")...)
		}
		if utf8.RuneCountInString(label) > 120 {
			return defErr("a column label is at most 120 characters", at("label")...)
		}
		// columnSchema reads maxLength on a text column only and drops it elsewhere.
		if c.Kind == "text" && c.MaxLength != nil && (*c.MaxLength < 1 || *c.MaxLength > 10000) {
			return defErr("maxLength is 1 to 10000", at("maxLength")...)
		}
		if c.Kind == "select" {
			if len(c.Options) == 0 {
				return defErr("a select column needs options", at("options")...)
			}
			if errs := duplicateOptionError(c.Options, at("options")...); errs != nil {
				return errs
			}
		}
		if c.Kind == "computed" {
			var refs exprRefs
			if msg := parseExpr(c.Expression, &refs); msg != "" {
				return defErr(msg, at("expression")...)
			}
			if len(refs.aggs) > 0 {
				return defErr("a column formula cannot sum or count a table", at("expression")...)
			}
			if c.Decimals < 0 || c.Decimals > 6 {
				return defErr(fmt.Sprintf("decimals %d is out of range. Use 0 to 6.", c.Decimals), at("decimals")...)
			}
		}
	}
	deps := map[string][]string{}
	for ci, c := range cols {
		if c.Kind != "computed" {
			continue
		}
		var refs exprRefs
		parseExpr(c.Expression, &refs)
		for _, v := range refs.vars {
			kind, ok := seen[v]
			if !ok {
				return defErr("unknown column "+v+". Columns: "+listOrNone(sortedKeys(seen))+".", append(append([]any{}, path...), ci, "expression")...)
			}
			if !numericKind(kind) {
				return defErr(v+" is a "+kind+" column and cannot be used in a formula. Use a number, computed or toggle column.", append(append([]any{}, path...), ci, "expression")...)
			}
		}
		deps[c.Key] = refs.vars
	}
	if _, cyc := orderKeys(deps); cyc != nil {
		idx := 0
		for ci, c := range cols {
			if c.Key == cyc[0] {
				idx = ci
			}
		}
		return defErr("formula cycle: "+strings.Join(cyc, " → "), append(append([]any{}, path...), idx, "expression")...)
	}
	return nil
}

// computeCrossErrors checks the formulas of custom computed fields against the
// whole field list (custom fields and the host's built-ins).
func computeCrossErrors(def *Definition, host *Host) validation.Errors {
	type source struct {
		kind string
		cols []Column
	}
	byKey := map[string]source{}
	for _, f := range def.Fields {
		if f.Kind == "builtin" {
			if b := host.Builtin(f.Key); b != nil {
				byKey[f.Key] = source{b.Kind, b.Columns}
			}
			continue
		}
		byKey[f.Key] = source{f.Kind, f.Columns}
	}
	deps := map[string][]string{}
	index := map[string]int{}
	for i, f := range def.Fields {
		if f.Kind != "computed" {
			continue
		}
		index[f.Key] = i
		var refs exprRefs
		parseExpr(f.Expression, &refs)
		for _, v := range refs.vars {
			s, ok := byKey[v]
			if !ok {
				return defErr("unknown field "+v+". Fields: "+listOrNone(sourceKeys(byKey))+".", "fields", i, "expression")
			}
			if !numericKind(s.kind) {
				return defErr(v+" is a "+s.kind+" field and cannot be used in a formula. Use a number, computed or toggle field.", "fields", i, "expression")
			}
		}
		for _, a := range refs.aggs {
			s, ok := byKey[a.table]
			if !ok {
				return defErr("unknown field "+a.table+". Fields: "+listOrNone(sourceKeys(byKey))+".", "fields", i, "expression")
			}
			if s.kind != "table" {
				return defErr(a.table+" is not a table. sumOf and countOf read a table field.", "fields", i, "expression")
			}
			if a.column == "" {
				continue
			}
			found := false
			for _, c := range s.cols {
				if c.Key != a.column {
					continue
				}
				found = true
				if c.Kind != "number" && c.Kind != "computed" {
					return defErr("column "+a.column+" is not numeric. Use a number or computed column.", "fields", i, "expression")
				}
			}
			if !found {
				return defErr("unknown column "+a.column+". Columns: "+listOrNone(columnKeys(s.cols))+".", "fields", i, "expression")
			}
		}
		deps[f.Key] = refs.vars
	}
	if _, cyc := orderKeys(deps); cyc != nil {
		return defErr("formula cycle: "+strings.Join(cyc, " → "), "fields", index[cyc[0]], "expression")
	}
	return nil
}

func sourceKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func columnKeys(cols []Column) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = c.Key
	}
	return out
}
