package forms

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"

	validation "github.com/pocketbase/ozzo-validation/v4"
)

// Wire mirror of core/lib/forms/definition.ts. Only what the server reads;
// unknown field properties pass through untouched.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type Field struct {
	Key   string `json:"key"`
	Kind  string `json:"kind"`
	Page  string `json:"page"`
	Label string `json:"label"`
	// Description is the help text the renderer draws with the field.
	Description string   `json:"description,omitempty"`
	Width       string   `json:"width,omitempty"`
	Required    bool     `json:"required"`
	Hidden      bool     `json:"hidden"`
	Options     []Option `json:"options,omitempty"`
	Multiple    bool     `json:"multiple,omitempty"`
	// Display picks a select's control: "dropdown" (the default), "radio"
	// or "checkbox". definitionSchema's select display.
	Display       string   `json:"display,omitempty"`
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
	// AddOnSubmit is presentation only (Enter in the cell adds a row); the
	// server decodes it so the host file round-trips, and never reads it.
	AddOnSubmit bool `json:"addOnSubmit,omitempty"`
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

// selectDisplays mirrors the select display enum in core/lib/forms/definition.ts.
var selectDisplays = []string{"dropdown", "radio", "checkbox"}

const keyRule = "Use 1 to 40 lowercase letters, digits or _, starting with a letter."

// problems collects every definition problem, keyed by its path, so one
// refused save lists all of them. The first problem at a path wins: it is
// the one the others usually follow from.
type problems validation.Errors

func (p problems) add(msg string, path ...any) {
	p.merge(defErr(msg, path...))
}

func (p problems) merge(errs validation.Errors) {
	for k, v := range errs {
		if _, seen := p[k]; !seen {
			p[k] = v
		}
	}
}

func (p problems) result() validation.Errors {
	if len(p) == 0 {
		return nil
	}
	return validation.Errors(p)
}

// fieldIndexOf reads n from a "definition.fields.<n>…" problem path.
func fieldIndexOf(path string) (int, bool) {
	rest, ok := strings.CutPrefix(path, "definition.fields.")
	if !ok {
		return 0, false
	}
	n, _, _ := strings.Cut(rest, ".")
	i, err := strconv.Atoi(n)
	return i, err == nil
}

func sortedBoolKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func listOrNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ValidateDefinition mirrors definitionSchema + validateDefinitionForHost in
// core/lib/forms. It returns every problem as a field error keyed by its
// path inside the definition (see defErr), so the builder shows each on the
// part of the form that is wrong and a client that writes the JSON itself
// (the CLI) can fix them all from one refusal. Each message names the bad
// value and what is allowed.
func ValidateDefinition(def *Definition, host *Host) validation.Errors {
	p := problems{}
	if def.Version != 1 {
		p.add(fmt.Sprintf("version %d is not supported. Use 1.", def.Version), "version")
	}
	if len(def.Pages) == 0 {
		p.add(`a form needs at least one page, for example {"key":"main","title":"Main"}`, "pages")
		return p.result()
	}
	pages := map[string]bool{}
	pageKeys := []string{}
	for i, pg := range def.Pages {
		if !keyRe.MatchString(pg.Key) {
			p.add(fmt.Sprintf("%q is not a valid page key. %s", pg.Key, keyRule), "pages", i, "key")
			continue
		}
		if pages[pg.Key] {
			p.add(fmt.Sprintf("page key %q is used twice. Each page key must be unique.", pg.Key), "pages", i, "key")
			continue
		}
		pages[pg.Key] = true
		pageKeys = append(pageKeys, pg.Key)
	}
	kinds := map[string]string{}
	// declared holds every field key the definition lists with a valid,
	// unique key, even when the field itself has a problem: a reference to
	// it is not a second problem, and a built-in in the list is not missing.
	declared := map[string]bool{}
	seenBuiltin := map[string]bool{}
	// requiredOnPage lists, per page, the built-ins placed on it that the host
	// requires or locks: none of them may be hidden.
	requiredOnPage := map[string][]string{}
	for i, f := range def.Fields {
		at := func(leaf string) []any { return []any{"fields", i, leaf} }
		if !keyRe.MatchString(f.Key) {
			p.add(fmt.Sprintf("%q is not a valid field key. %s", f.Key, keyRule), at("key")...)
			continue
		}
		if declared[f.Key] {
			p.add(fmt.Sprintf("field key %q is used twice. Each field key must be unique.", f.Key), at("key")...)
			continue
		}
		declared[f.Key] = true
		if f.Kind == "builtin" && host.Builtin(f.Key) != nil {
			seenBuiltin[f.Key] = true
		}
		if !pages[f.Page] {
			p.add(fmt.Sprintf("%q is not a page of this form. Pages: %s.", f.Page, listOrNone(pageKeys)), at("page")...)
			continue
		}
		if errs := fieldTextLengthErrors(f, i); errs != nil {
			p.merge(errs)
			continue
		}
		if f.Kind == "builtin" {
			b := host.Builtin(f.Key)
			if b == nil {
				p.add(fmt.Sprintf("%q is not a built-in field of %s. Built-in fields: %s.", f.Key, host.Label, listOrNone(host.BuiltinKeys())), at("key")...)
				continue
			}
			lock := hideLock(b)
			if lock != "" && f.Hidden {
				p.add(fmt.Sprintf("%s is %s by the record and cannot be hidden. Set hidden to false.", f.Key, lock), at("hidden")...)
				continue
			}
			if lock != "" {
				requiredOnPage[f.Page] = append(requiredOnPage[f.Page], f.Key)
			}
			kinds[f.Key] = b.Kind
			continue
		}
		if !slices.Contains(CustomKinds(), f.Kind) {
			// The kinds this host takes, so the next try is not refused again.
			allowed := host.Kinds
			if len(allowed) == 0 {
				allowed = CustomKinds()
			}
			p.add(fmt.Sprintf("%q is not a field kind. Kinds: %s; or builtin for a built-in field.", f.Kind, strings.Join(allowed, ", ")), at("kind")...)
			continue
		}
		if len(host.Kinds) > 0 && !slices.Contains(host.Kinds, f.Kind) {
			p.add(fmt.Sprintf("%s fields are not allowed in %s. Allowed: %s.", f.Kind, host.Label, strings.Join(host.Kinds, ", ")), at("kind")...)
			continue
		}
		if host.Builtin(f.Key) != nil {
			p.add(fmt.Sprintf("%s is a built-in field and cannot be a custom field. Use kind builtin, or another key.", f.Key), at("key")...)
			continue
		}
		if host.HasColumn(f.Key) {
			p.add(fmt.Sprintf("%s is a column of the record and cannot be a custom field. Use another key.", f.Key), at("key")...)
			continue
		}
		if f.Kind == "select" && f.Display != "" && !slices.Contains(selectDisplays, f.Display) {
			p.add(fmt.Sprintf("%q is not a select display. Use: %s.", f.Display, strings.Join(selectDisplays, ", ")), at("display")...)
			continue
		}
		if errs := duplicateOptionError(f.Options, "fields", i, "options"); errs != nil {
			p.merge(errs)
			continue
		}
		if f.Kind == "number" && f.Min != nil && f.Max != nil && *f.Min > *f.Max {
			p.add(fmt.Sprintf("minimum %g is above maximum %g.", *f.Min, *f.Max), at("min")...)
			continue
		}
		if f.Collaborative && host.Collab == nil {
			p.add(host.Label+" does not support collaborative fields. Set collaborative to false.", at("collaborative")...)
			continue
		}
		if errs := fieldComputeErrors(f, i); errs != nil {
			p.merge(errs)
			continue
		}
		kinds[f.Key] = f.Kind
	}
	var missing []string
	for _, b := range host.Builtins {
		if !seenBuiltin[b.Key] {
			missing = append(missing, b.Key)
		}
	}
	if len(missing) == 1 {
		p.add(fmt.Sprintf("missing built-in field %s. Every built-in field of %s must be in fields, with kind builtin.", missing[0], host.Label), "fields")
	} else if len(missing) > 1 {
		p.add(fmt.Sprintf("missing built-in fields %s. Every built-in field of %s must be in fields, with kind builtin.", strings.Join(missing, ", "), host.Label), "fields")
	}
	// A field that failed its own checks already has its problem, so a
	// cross-field problem at that field is not reported again. The paths keep
	// each field's index in the definition.
	for k, v := range computeCrossErrors(def, host) {
		if i, ok := fieldIndexOf(k); ok && i < len(def.Fields) {
			if _, valid := kinds[def.Fields[i].Key]; !valid {
				continue
			}
		}
		p.merge(validation.Errors{k: v})
	}
	// unusable reports a reference to a field that is declared but has its
	// own problem: that reference is skipped, not reported again.
	unusable := func(key string) bool {
		_, ok := kinds[key]
		return !ok && declared[key]
	}
	ruleKeys := map[string]bool{}
	for ri, r := range def.Rules {
		if !keyRe.MatchString(r.Key) {
			p.add(fmt.Sprintf("%q is not a valid rule key. %s", r.Key, keyRule), "rules", ri, "key")
			continue
		}
		if ruleKeys[r.Key] {
			p.add(fmt.Sprintf("rule key %q is used twice. Each rule key must be unique.", r.Key), "rules", ri, "key")
			continue
		}
		ruleKeys[r.Key] = true
		if len(r.When) == 0 {
			p.add("rule "+r.Key+" needs at least one condition in when.", "rules", ri, "when")
		}
		if len(r.Actions) == 0 {
			p.add("rule "+r.Key+" needs at least one action.", "rules", ri, "actions")
		}
		for ci, c := range r.When {
			if unusable(c.Field) {
				continue
			}
			kind, ok := kinds[c.Field]
			if !ok {
				p.add(fmt.Sprintf("%q is not a field of this form. Fields: %s.", c.Field, listOrNone(sortedKeys(kinds))), "rules", ri, "when", ci, "field")
				continue
			}
			if !opAllowed(kind, c.Op) {
				p.add(fmt.Sprintf("%s is not valid for a %s field. Use: %s.", c.Op, kind, listOrNone(operatorsByKind[kind])), "rules", ri, "when", ci, "op")
				continue
			}
			if msg := conditionValueError(c); msg != "" {
				p.add(msg, "rules", ri, "when", ci, "value")
			}
		}
		for ai, a := range r.Actions {
			if a.Field != "" && unusable(a.Field) {
				continue
			}
			p.merge(actionError(a, kinds, pages, requiredOnPage, host, "rules", ri, "actions", ai))
		}
	}
	for k, v := range def.Defaults {
		if unusable(k) {
			continue
		}
		if _, ok := kinds[k]; !ok {
			p.add(fmt.Sprintf("%q is not a field of this form. Fields: %s.", k, listOrNone(sortedKeys(kinds))), "defaults", k)
			continue
		}
		if !validExpression(v) {
			p.add(`the default of `+k+` is not a value or a supported expression. Use a literal, {"today":[]}, {"me":[]} or {"addDays":[<date or {"today":[]}>, <whole days>]}.`, "defaults", k)
		}
	}
	for i, k := range def.Submit.Required {
		if unusable(k) {
			continue
		}
		if _, ok := kinds[k]; !ok {
			p.add(fmt.Sprintf("%q is not a field of this form. Fields: %s.", k, listOrNone(sortedKeys(kinds))), "submit", "required", i)
		}
	}
	return p.result()
}

// Limits of a field's label and description in definitionSchema's base.
const (
	maxLabelLength       = 120
	maxDescriptionLength = 500
)

// jsLength counts a string as JavaScript's length does (UTF-16 code units),
// so a limit here refuses exactly what zod's .max() refuses.
func jsLength(s string) int {
	return len(utf16.Encode([]rune(s)))
}

// jsTrim trims what String.prototype.trim trims (zod's .trim()): white space
// and the byte order mark.
func jsTrim(s string) string {
	const byteOrderMark = 0xFEFF
	return strings.TrimFunc(s, func(r rune) bool { return unicode.IsSpace(r) || r == byteOrderMark })
}

// fieldTextLengthErrors mirrors definitionSchema's label and description
// limits, so a definition the server stores is one the client can read.
func fieldTextLengthErrors(f Field, i int) validation.Errors {
	p := problems{}
	if n := jsLength(jsTrim(f.Label)); n > maxLabelLength {
		p.add(fmt.Sprintf("label has %d characters. Use at most %d.", n, maxLabelLength), "fields", i, "label")
	}
	if n := jsLength(f.Description); n > maxDescriptionLength {
		p.add(fmt.Sprintf("description has %d characters. Use at most %d.", n, maxDescriptionLength), "fields", i, "description")
	}
	return p.result()
}

// duplicateOptionError mirrors the schema's check on a select's options and a
// set_options action's options: each value appears once.
func duplicateOptionError(options []Option, path ...any) validation.Errors {
	seen := map[string]bool{}
	for oi, o := range options {
		if seen[o.Value] {
			return defErr("duplicate option value "+o.Value+". Each option value must be unique.", append(append([]any{}, path...), oi, "value")...)
		}
		seen[o.Value] = true
	}
	return nil
}

// hideLock is why a built-in may never be hidden ("required" or "locked"), or
// "" when it may; hideLock in core/lib/forms/definition.ts is its mirror.
func hideLock(b *Builtin) string {
	switch {
	case b == nil:
		return ""
	case b.Required:
		return "required"
	case b.Locked:
		return "locked"
	}
	return ""
}

func actionError(a Action, kinds map[string]string, pages map[string]bool, requiredOnPage map[string][]string, host *Host, path ...any) validation.Errors {
	at := func(leaf string) []any { return append(append([]any{}, path...), leaf) }
	switch a.Action {
	case "show", "hide":
		if a.Page != "" {
			if !pages[a.Page] {
				return defErr("unknown page "+a.Page+". Pages: "+listOrNone(sortedBoolKeys(pages))+".", at("page")...)
			}
			if held := requiredOnPage[a.Page]; a.Action == "hide" && len(held) > 0 {
				return defErr("page "+a.Page+" holds "+hideLock(host.Builtin(held[0]))+" field "+held[0]+", so it cannot be hidden. Hide its other fields one by one instead.", at("page")...)
			}
			return nil
		}
		if _, ok := kinds[a.Field]; !ok {
			return defErr("unknown field "+a.Field+". Fields: "+listOrNone(sortedKeys(kinds))+".", at("field")...)
		}
		if lock := hideLock(host.Builtin(a.Field)); a.Action == "hide" && lock != "" {
			return defErr("cannot hide "+lock+" field "+a.Field+". The record needs it, so it must stay visible.", at("field")...)
		}
	case "require", "set_options", "set_value":
		if _, ok := kinds[a.Field]; !ok {
			return defErr("unknown field "+a.Field+". Fields: "+listOrNone(sortedKeys(kinds))+".", at("field")...)
		}
		if a.Action == "set_options" && len(a.Options) == 0 {
			return defErr("set_options needs at least one option", at("options")...)
		}
		if errs := duplicateOptionError(a.Options, at("options")...); errs != nil {
			return errs
		}
		if a.Action == "set_value" && !validExpression(a.Value) {
			return defErr(`the value of `+a.Field+` is not a value or a supported expression. Use a literal, {"today":[]}, {"me":[]} or {"addDays":[<date or {"today":[]}>, <whole days>]}.`, at("value")...)
		}
	default:
		return defErr(fmt.Sprintf("%q is not a form rule action. Actions: show, hide, require, set_options, set_value.", a.Action), at("action")...)
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
