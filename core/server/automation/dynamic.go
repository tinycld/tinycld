package automation

import (
	"encoding/json"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/list"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/spf13/cast"
)

// DynamicPrefix namespaces values a trigger exposes beyond its record's
// columns, such as one answer of a web form: rules read them as
// {{fields.<key>}} and as condition fields "fields.<key>".
const DynamicPrefix = "fields."

// DynamicField is one such value for one rule and record. Type is a catalog
// field type (text, number, boolean, date, select).
type DynamicField struct {
	Key     string
	Label   string
	Type    string
	Value   any
	Options []string
}

// FieldsResolver returns a rule's dynamic fields for a record, from the
// rule's trigger_config. It must not error: a missing value is absent.
type FieldsResolver func(app core.App, config map[string]any, record *core.Record) []DynamicField

// RuleFilter reports whether a rule, by its trigger_config, wants a record —
// a web form rule wants only its own form's submissions. Unlike a
// TriggerFilter, it sees the rule; a rule it refuses writes no run row.
type RuleFilter func(app core.App, config map[string]any, record *core.Record) bool

func RegisterFieldsResolver(triggerRef string, r FieldsResolver) {
	registryMu.Lock()
	defer registryMu.Unlock()
	fieldsResolvers[triggerRef] = r
}

func RegisterRuleFilter(triggerRef string, f RuleFilter) {
	registryMu.Lock()
	defer registryMu.Unlock()
	ruleFilters[triggerRef] = f
}

// RuleScope narrows the records a dry run reads to the ones a rule can match,
// from its trigger_config, as a PocketBase filter: a web form rule reads its
// own form's submissions, not the newest of every form. Param names start
// with "scope" so they cannot collide with the dry run's own.
type RuleScope func(config map[string]any) (filter string, params map[string]any)

func RegisterRuleScope(triggerRef string, s RuleScope) {
	registryMu.Lock()
	defer registryMu.Unlock()
	ruleScopes[triggerRef] = s
}

func ruleScope(triggerRef string, config map[string]any) (string, map[string]any) {
	registryMu.RLock()
	s := ruleScopes[triggerRef]
	registryMu.RUnlock()
	if s == nil {
		return "", nil
	}
	return s(config)
}

func ruleConfig(rule *core.Record) map[string]any {
	out := map[string]any{}
	b, err := json.Marshal(rule.Get("trigger_config"))
	if err != nil {
		return out
	}
	_ = json.Unmarshal(b, &out)
	return out
}

func ruleAllowed(app core.App, triggerRef string, config map[string]any, record *core.Record) bool {
	registryMu.RLock()
	f := ruleFilters[triggerRef]
	registryMu.RUnlock()
	return f == nil || record == nil || f(app, config, record)
}

func dynamicFields(app core.App, triggerRef string, config map[string]any, record *core.Record) []DynamicField {
	registryMu.RLock()
	r := fieldsResolvers[triggerRef]
	registryMu.RUnlock()
	if r == nil || record == nil {
		return nil
	}
	return r(app, config, record)
}

func dynamicValues(fields []DynamicField) map[string]any {
	if len(fields) == 0 {
		return nil
	}
	out := make(map[string]any, len(fields))
	for _, f := range fields {
		out[DynamicPrefix+f.Key] = f.Value
	}
	return out
}

// valueSource is what a condition reads a field from: the trigger record, or
// the record overlaid with a run's dynamic values.
type valueSource interface {
	Get(key string) any
	GetString(key string) string
	GetFloat(key string) float64
	GetBool(key string) bool
	GetDateTime(key string) types.DateTime
	GetStringSlice(key string) []string
}

type overlaySource struct {
	*core.Record
	dyn map[string]any
}

func (o overlaySource) Get(key string) any {
	if v, ok := o.dyn[key]; ok {
		return v
	}
	return o.Record.Get(key)
}

func (o overlaySource) GetString(key string) string {
	if v, ok := o.dyn[key]; ok {
		return cast.ToString(v)
	}
	return o.Record.GetString(key)
}

func (o overlaySource) GetFloat(key string) float64 {
	if v, ok := o.dyn[key]; ok {
		return cast.ToFloat64(v)
	}
	return o.Record.GetFloat(key)
}

func (o overlaySource) GetBool(key string) bool {
	if v, ok := o.dyn[key]; ok {
		return cast.ToBool(v)
	}
	return o.Record.GetBool(key)
}

func (o overlaySource) GetDateTime(key string) types.DateTime {
	if v, ok := o.dyn[key]; ok {
		d, _ := types.ParseDateTime(v)
		return d
	}
	return o.Record.GetDateTime(key)
}

func (o overlaySource) GetStringSlice(key string) []string {
	if v, ok := o.dyn[key]; ok {
		return list.ToUniqueStringSlice(v)
	}
	return o.Record.GetStringSlice(key)
}
