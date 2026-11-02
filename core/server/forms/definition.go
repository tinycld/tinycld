package forms

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"

	validation "github.com/pocketbase/ozzo-validation/v4"
)

// Wire mirror of core/lib/forms/definition.ts. Only what the server reads;
// unknown field properties pass through untouched.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type Field struct {
	Key           string   `json:"key"`
	Kind          string   `json:"kind"`
	Page          string   `json:"page"`
	Label         string   `json:"label"`
	Width         string   `json:"width,omitempty"`
	Required      bool     `json:"required"`
	Hidden        bool     `json:"hidden"`
	Options       []Option `json:"options,omitempty"`
	Multiple      bool     `json:"multiple,omitempty"`
	Collaborative bool     `json:"collaborative,omitempty"`
	Min           *float64 `json:"min,omitempty"`
	Max           *float64 `json:"max,omitempty"`
	MaxLength     *int     `json:"maxLength,omitempty"`
	Expression    any      `json:"expression,omitempty"`
	Decimals      int      `json:"decimals,omitempty"`
	Columns       []Column `json:"columns,omitempty"`
	Accept        []string `json:"accept,omitempty"`
	MaxSizeMb     *float64 `json:"maxSizeMb,omitempty"`
}

// Column is a table field's column; a mirror of columnSchema in
// core/lib/forms/definition.ts.
type Column struct {
	Key        string   `json:"key"`
	Kind       string   `json:"kind"`
	Label      string   `json:"label"`
	Required   bool     `json:"required"`
	Hidden     bool     `json:"hidden"`
	Options    []Option `json:"options,omitempty"`
	Expression any      `json:"expression,omitempty"`
	Decimals   int      `json:"decimals,omitempty"`
	Min        *float64 `json:"min,omitempty"`
	Max        *float64 `json:"max,omitempty"`
	MaxLength  *int     `json:"maxLength,omitempty"`
}

type Page struct {
	Key   string `json:"key"`
	Title string `json:"title"`
}

type Condition struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value,omitempty"`
}

type Action struct {
	Action  string   `json:"action"`
	Field   string   `json:"field,omitempty"`
	Page    string   `json:"page,omitempty"`
	Options []Option `json:"options,omitempty"`
	Value   any      `json:"value,omitempty"`
}

// MarshalJSON writes a set_value's value even when it is null, as the client
// does (its schema defaults the value to null), so both seeds write the same
// JSON (core/lib/forms/fixtures/layout-definition.json). Every other action
// has no value and leaves it out.
func (a Action) MarshalJSON() ([]byte, error) {
	type plain Action
	if a.Action != "set_value" {
		return json.Marshal(plain(a))
	}
	return json.Marshal(struct {
		plain
		Value any `json:"value"`
	}{plain(a), a.Value})
}

type Rule struct {
	Key     string      `json:"key"`
	Match   string      `json:"match"`
	When    []Condition `json:"when"`
	Actions []Action    `json:"actions"`
}

type Submit struct {
	Required []string `json:"required"`
}

type Definition struct {
	Version  int            `json:"version"`
	Pages    []Page         `json:"pages"`
	Fields   []Field        `json:"fields"`
	Rules    []Rule         `json:"rules"`
	Defaults map[string]any `json:"defaults"`
	Submit   Submit         `json:"submit"`
}

func ParseDefinition(raw []byte) (*Definition, error) {
	var def Definition
	if err := json.Unmarshal(raw, &def); err != nil {
		return nil, fmt.Errorf("forms: parse definition: %w", err)
	}
	if def.Defaults == nil {
		def.Defaults = map[string]any{}
	}
	return &def, nil
}

func (d *Definition) Field(key string) *Field {
	for i := range d.Fields {
		if d.Fields[i].Key == key {
			return &d.Fields[i]
		}
	}
	return nil
}

var keyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

// Mirror of core/lib/forms/definition.ts OPERATORS_BY_KIND.
var operatorsByKind = map[string][]string{
	"text":      {"eq", "neq", "contains", "starts_with", "is_empty", "not_empty"},
	"textarea":  {"eq", "neq", "contains", "starts_with", "is_empty", "not_empty"},
	"richtext":  {"is_empty", "not_empty"},
	"markdown":  {"is_empty", "not_empty"},
	"number":    {"eq", "neq", "gt", "lt", "gte", "lte", "is_empty", "not_empty"},
	"date":      {"before", "after", "is_empty", "not_empty"},
	"toggle":    {"is_true", "is_false"},
	"select":    {"is", "is_not", "is_any_of", "is_empty", "not_empty"},
	"user":      {"is", "is_not", "is_any_of", "is_empty", "not_empty"},
	"checklist": {"all_done", "any_open", "is_empty"},
	"file":      {"is_empty", "not_empty"},
	"custom":    {"is_empty", "not_empty"},
	"computed":  {"eq", "neq", "gt", "lt", "gte", "lte", "is_empty", "not_empty"},
	"table":     {"is_empty", "not_empty"},
	"section":   {},
}

func opAllowed(kind, op string) bool {
	for _, o := range operatorsByKind[kind] {
		if o == op {
			return true
		}
	}
	return false
}

const invalidDefinition = "validation_invalid_definition"

// dateRe is the stored shape of a date value: a day, or a day and a time of
// day. Mirrored in core/lib/forms/definition.ts DATE_RE.
var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}(T\d{2}:\d{2})?$`)

// defErr reports a definition problem as a field error keyed
// `definition.<path>`, where path is the dot-joined location inside the
// definition, the same path definitionSchema and validateDefinitionForHost
// give the zod issue in core/lib/forms/definition.ts. Examples:
// `definition.fields.3.key`, `definition.rules.0.when.1.op`,
// `definition.defaults.due`. With no path the key is `definition`.
func defErr(msg string, path ...any) validation.Errors {
	parts := []string{"definition"}
	for _, p := range path {
		parts = append(parts, fmt.Sprint(p))
	}
	return validation.Errors{strings.Join(parts, "."): validation.NewError(invalidDefinition, msg)}
}

// ValidateDefinition mirrors definitionSchema + validateDefinitionForHost in
// core/lib/forms. It returns the first problem as a field error keyed by its
// path inside the definition (see defErr), so the builder shows it on the
// part of the form that is wrong.
func ValidateDefinition(def *Definition, host *Host) validation.Errors {
	if def.Version != 1 {
		return defErr("unsupported definition version", "version")
	}
	if len(def.Pages) == 0 {
		return defErr("a form needs at least one page", "pages")
	}
	pages := map[string]bool{}
	for i, p := range def.Pages {
		if !keyRe.MatchString(p.Key) {
			return defErr("invalid page key "+p.Key, "pages", i, "key")
		}
		if pages[p.Key] {
			return defErr("duplicate page key "+p.Key, "pages", i, "key")
		}
		pages[p.Key] = true
	}
	kinds := map[string]string{}
	seenBuiltin := map[string]bool{}
	// requiredOnPage lists, per page, the host-required built-ins placed on it.
	requiredOnPage := map[string][]string{}
	for i, f := range def.Fields {
		if !keyRe.MatchString(f.Key) {
			return defErr("invalid field key "+f.Key, "fields", i, "key")
		}
		if _, dup := kinds[f.Key]; dup {
			return defErr("duplicate field key "+f.Key, "fields", i, "key")
		}
		if !pages[f.Page] {
			return defErr("unknown page "+f.Page, "fields", i, "page")
		}
		if f.Kind != "builtin" && len(host.Kinds) > 0 && !slices.Contains(host.Kinds, f.Kind) {
			return defErr(f.Kind+" fields are not allowed in "+host.Label, "fields", i, "kind")
		}
		if f.Kind == "builtin" {
			b := host.Builtin(f.Key)
			if b == nil {
				return defErr(f.Key+" is not a built-in field of "+host.Label, "fields", i, "key")
			}
			if b.Required && f.Hidden {
				return defErr(f.Key+" is required by the record and cannot be hidden", "fields", i, "hidden")
			}
			if b.Required {
				requiredOnPage[f.Page] = append(requiredOnPage[f.Page], f.Key)
			}
			kinds[f.Key] = b.Kind
			seenBuiltin[f.Key] = true
			continue
		}
		if _, ok := operatorsByKind[f.Kind]; !ok || f.Kind == "custom" || f.Kind == "section" {
			return defErr("unknown field kind "+f.Kind, "fields", i, "kind")
		}
		if host.Builtin(f.Key) != nil {
			return defErr(f.Key+" is a built-in field and cannot be a custom field", "fields", i, "key")
		}
		if host.HasColumn(f.Key) {
			return defErr(f.Key+" is a column of the record and cannot be a custom field", "fields", i, "key")
		}
		if errs := duplicateOptionError(f.Options, "fields", i, "options"); errs != nil {
			return errs
		}
		if f.Kind == "number" && f.Min != nil && f.Max != nil && *f.Min > *f.Max {
			return defErr("minimum is above maximum", "fields", i, "min")
		}
		if f.Collaborative && host.Collab == nil {
			return defErr(host.Label+" does not support collaborative fields", "fields", i, "collaborative")
		}
		if errs := fieldComputeErrors(f, i); errs != nil {
			return errs
		}
		kinds[f.Key] = f.Kind
	}
	for _, b := range host.Builtins {
		if !seenBuiltin[b.Key] {
			return defErr("missing built-in field "+b.Key, "fields")
		}
	}
	if errs := computeCrossErrors(def, host); errs != nil {
		return errs
	}
	ruleKeys := map[string]bool{}
	for ri, r := range def.Rules {
		if !keyRe.MatchString(r.Key) {
			return defErr("invalid rule key "+r.Key, "rules", ri, "key")
		}
		if ruleKeys[r.Key] {
			return defErr("duplicate rule key "+r.Key, "rules", ri, "key")
		}
		ruleKeys[r.Key] = true
		if len(r.When) == 0 {
			return defErr("rule "+r.Key+" needs a condition and an action", "rules", ri, "when")
		}
		if len(r.Actions) == 0 {
			return defErr("rule "+r.Key+" needs a condition and an action", "rules", ri, "actions")
		}
		for ci, c := range r.When {
			kind, ok := kinds[c.Field]
			if !ok {
				return defErr("unknown field "+c.Field, "rules", ri, "when", ci, "field")
			}
			if !opAllowed(kind, c.Op) {
				return defErr(c.Op+" is not valid for a "+kind+" field", "rules", ri, "when", ci, "op")
			}
			if msg := conditionValueError(c); msg != "" {
				return defErr(msg, "rules", ri, "when", ci, "value")
			}
		}
		for ai, a := range r.Actions {
			if errs := actionError(a, kinds, pages, requiredOnPage, host, "rules", ri, "actions", ai); errs != nil {
				return errs
			}
		}
	}
	for k, v := range def.Defaults {
		if _, ok := kinds[k]; !ok {
			return defErr("unknown field "+k, "defaults", k)
		}
		if !validExpression(v) {
			return defErr("the default of "+k+" is not a value or a supported expression", "defaults", k)
		}
	}
	for i, k := range def.Submit.Required {
		if _, ok := kinds[k]; !ok {
			return defErr("unknown field "+k, "submit", "required", i)
		}
	}
	return nil
}

// duplicateOptionError mirrors the schema's check on a select's options and a
// set_options action's options: each value appears once.
func duplicateOptionError(options []Option, path ...any) validation.Errors {
	seen := map[string]bool{}
	for oi, o := range options {
		if seen[o.Value] {
			return defErr("duplicate option value "+o.Value, append(append([]any{}, path...), oi, "value")...)
		}
		seen[o.Value] = true
	}
	return nil
}

func actionError(a Action, kinds map[string]string, pages map[string]bool, requiredOnPage map[string][]string, host *Host, path ...any) validation.Errors {
	at := func(leaf string) []any { return append(append([]any{}, path...), leaf) }
	switch a.Action {
	case "show", "hide":
		if a.Page != "" {
			if !pages[a.Page] {
				return defErr("unknown page "+a.Page, at("page")...)
			}
			if held := requiredOnPage[a.Page]; a.Action == "hide" && len(held) > 0 {
				return defErr("page "+a.Page+" holds required field "+held[0], at("page")...)
			}
			return nil
		}
		if _, ok := kinds[a.Field]; !ok {
			return defErr("unknown field "+a.Field, at("field")...)
		}
		if b := host.Builtin(a.Field); a.Action == "hide" && b != nil && b.Required {
			return defErr("cannot hide required field "+a.Field, at("field")...)
		}
	case "require", "set_options", "set_value":
		if _, ok := kinds[a.Field]; !ok {
			return defErr("unknown field "+a.Field, at("field")...)
		}
		if a.Action == "set_options" && len(a.Options) == 0 {
			return defErr("set_options needs at least one option", at("options")...)
		}
		if errs := duplicateOptionError(a.Options, at("options")...); errs != nil {
			return errs
		}
		if a.Action == "set_value" && !validExpression(a.Value) {
			return defErr("the value of "+a.Field+" is not a value or a supported expression", at("value")...)
		}
	default:
		return defErr("unknown action "+a.Action, at("action")...)
	}
	return nil
}

// validExpression mirrors expressionSchema in core/lib/forms/definition.ts:
// a literal (text, number, true/false, null or a list of text), or one of the
// picker's expressions {today:[]}, {me:[]} and {addDays:[base, days]} whose
// base is a date literal or {today:[]} and whose days is a whole number.
func validExpression(v any) bool {
	switch x := v.(type) {
	case nil, string, float64, bool:
		return true
	case []any:
		for _, it := range x {
			if _, ok := it.(string); !ok {
				return false
			}
		}
		return true
	case map[string]any:
		if len(x) != 1 {
			return false
		}
		if isNoArgOp(x, "today") || isNoArgOp(x, "me") {
			return true
		}
		args, ok := x["addDays"].([]any)
		if !ok || len(args) != 2 {
			return false
		}
		days, ok := args[1].(float64)
		if !ok || days != math.Trunc(days) {
			return false
		}
		if s, ok := args[0].(string); ok {
			return dateRe.MatchString(s)
		}
		base, ok := args[0].(map[string]any)
		return ok && len(base) == 1 && isNoArgOp(base, "today")
	}
	return false
}

func isNoArgOp(m map[string]any, op string) bool {
	args, ok := m[op].([]any)
	return ok && len(args) == 0
}

// conditionValueError checks the value shapes the engine cannot coerce.
func conditionValueError(c Condition) string {
	switch c.Op {
	case "contains", "starts_with":
		if _, ok := c.Value.(string); !ok {
			return c.Op + " needs a text value"
		}
	case "is_any_of":
		list, ok := c.Value.([]any)
		if !ok || len(list) == 0 {
			return "is_any_of needs a list"
		}
		for _, v := range list {
			if _, ok := v.(string); !ok {
				return "is_any_of needs a list"
			}
		}
	}
	return ""
}
