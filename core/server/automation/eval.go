package automation

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

type Condition struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value"`
}

type ConditionGroup struct {
	Match      string      `json:"match"`
	Conditions []Condition `json:"conditions"`
}

type ConditionsAST struct {
	Match  string           `json:"match"`
	Groups []ConditionGroup `json:"groups"`
}

// DecodeConditions round-trips through JSON because Record.Get on a json
// column yields types.JSONRaw (see notify_batcher.go's hard-won comment) and
// the client stores plain objects — one path handles both.
func DecodeConditions(raw any) (ConditionsAST, error) {
	var ast ConditionsAST
	if raw == nil {
		return ast, nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return ast, err
	}
	if len(b) == 0 || string(b) == "null" || string(b) == `""` {
		return ast, nil
	}
	return ast, json.Unmarshal(b, &ast)
}

// normalize mirrors audit.fieldToString: one canonical string per value.
func normalize(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []string:
		return strings.Join(t, ",")
	default:
		return fmt.Sprintf("%v", t)
	}
}

func toFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		var f float64
		_, err := fmt.Sscanf(t, "%g", &f)
		return f, err == nil
	default:
		return 0, false
	}
}

func stringValues(record *core.Record, field string) []string {
	if vs := record.GetStringSlice(field); len(vs) > 0 {
		return vs
	}
	if s := record.GetString(field); s != "" {
		return []string{s}
	}
	return nil
}

func evalCondition(c Condition, record *core.Record) bool {
	switch c.Op {
	case "contains", "not_contains", "equals", "starts_with":
		field := strings.ToLower(normalize(record.Get(c.Field)))
		want := strings.ToLower(normalize(c.Value))
		var hit bool
		switch c.Op {
		case "contains":
			hit = strings.Contains(field, want)
		case "not_contains":
			return !strings.Contains(field, want)
		case "equals":
			hit = field == want
		case "starts_with":
			hit = strings.HasPrefix(field, want)
		}
		return hit
	case "eq", "neq", "gt", "lt":
		have := record.GetFloat(c.Field)
		want, ok := toFloat(c.Value)
		if !ok {
			return false
		}
		switch c.Op {
		case "eq":
			return have == want
		case "neq":
			return have != want
		case "gt":
			return have > want
		case "lt":
			return have < want
		}
	case "is_true":
		return record.GetBool(c.Field)
	case "is_false":
		return !record.GetBool(c.Field)
	case "before", "after", "within_last_days":
		have := record.GetDateTime(c.Field).Time()
		if have.IsZero() {
			return false
		}
		if c.Op == "within_last_days" {
			days, ok := toFloat(c.Value)
			if !ok {
				return false
			}
			return have.After(time.Now().Add(-time.Duration(days*24) * time.Hour))
		}
		want, err := time.Parse("2006-01-02 15:04:05.000Z", normalize(c.Value))
		if err != nil {
			// Date-only form from the builder ("2026-08-11")
			want, err = time.Parse("2006-01-02", normalize(c.Value))
			if err != nil {
				return false
			}
		}
		if c.Op == "before" {
			return have.Before(want)
		}
		return have.After(want)
	case "is", "is_not":
		want := normalize(c.Value)
		hit := false
		for _, v := range stringValues(record, c.Field) {
			if v == want {
				hit = true
				break
			}
		}
		if c.Op == "is" {
			return hit
		}
		return !hit
	case "is_empty":
		return len(stringValues(record, c.Field)) == 0 && normalize(record.Get(c.Field)) == ""
	}
	// Unknown operator: a rule authored against a newer core than this one.
	// Fail closed (no match) rather than erroring the whole dispatch.
	return false
}

func evalGroup(g ConditionGroup, record *core.Record) bool {
	if len(g.Conditions) == 0 {
		return true
	}
	for _, c := range g.Conditions {
		hit := evalCondition(c, record)
		if g.Match == "any" && hit {
			return true
		}
		if g.Match != "any" && !hit {
			return false
		}
	}
	return g.Match != "any"
}

// EvaluateConditions applies the one-level AND/OR AST. No conditions = match.
func EvaluateConditions(ast ConditionsAST, record *core.Record) bool {
	if len(ast.Groups) == 0 {
		return true
	}
	for _, g := range ast.Groups {
		hit := evalGroup(g, record)
		if ast.Match == "any" && hit {
			return true
		}
		if ast.Match != "any" && !hit {
			return false
		}
	}
	return ast.Match != "any"
}

// WatchChanged reports whether any watched field differs from the record's
// original values. Empty watch = fire on every update.
func WatchChanged(record *core.Record, watch []string) bool {
	if len(watch) == 0 {
		return true
	}
	original := record.Original()
	for _, f := range watch {
		if normalize(record.Get(f)) != normalize(original.Get(f)) {
			return true
		}
	}
	return false
}
