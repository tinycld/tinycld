package automation

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/core"
)

const codeInvalidAction = "validation_invalid_action"

// BindRuleValidation refuses a rule whose actions name an action no
// installed package declares, a param the action does not declare, or leave
// out a required param. Every problem is reported at once, keyed by its path
// in the rule. Before this, a bad action showed up only as a failed run.
//
// An update is checked only when it changes the actions, so a rule whose
// package was uninstalled can still be turned off or renamed.
func BindRuleValidation(app core.App, defs *Defs) {
	app.OnRecordCreate("rules").BindFunc(func(e *core.RecordEvent) error {
		if errs := ValidateRuleActions(e.App, defs, e.Record.Get("actions")); errs != nil {
			return errs
		}
		return e.Next()
	})
	app.OnRecordUpdate("rules").BindFunc(func(e *core.RecordEvent) error {
		stored, err := e.App.FindRecordById("rules", e.Record.Id)
		if err != nil {
			if errs := ValidateRuleActions(e.App, defs, e.Record.Get("actions")); errs != nil {
				return errs
			}
			return e.Next()
		}
		if normalizedJSON(stored.Get("actions")) != normalizedJSON(e.Record.Get("actions")) {
			previous, _ := decodeActions(stored.Get("actions"))
			if errs := validateActions(e.App, defs, e.Record.Get("actions"), previous); errs != nil {
				return errs
			}
		}
		return e.Next()
	})
}

func normalizedJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return string(b)
	}
	b, _ = json.Marshal(out)
	return string(b)
}

// ValidateRuleActions checks a rule's actions against the installed
// definitions. raw is the record's `actions` value.
func ValidateRuleActions(app core.App, defs *Defs, raw any) validation.Errors {
	return validateActions(app, defs, raw, nil)
}

// keptParam reports whether the stored rule already held this param on the
// same action. A param a package update dropped is kept by the app's editor,
// which cannot show it, so it must not block an edit.
func keptParam(previous []storedAction, i int, ref, key string) bool {
	if i >= len(previous) || previous[i].Ref != ref {
		return false
	}
	_, ok := previous[i].Params[key]
	return ok
}

func validateActions(app core.App, defs *Defs, raw any, previous []storedAction) validation.Errors {
	actions, err := decodeActions(raw)
	if err != nil {
		return validation.Errors{"actions": validation.NewError(codeInvalidAction,
			`actions must be a list of {"ref": "<package>:<action>", "params": {"<key>": <value>}}.`)}
	}
	if len(actions) == 0 {
		return validation.Errors{"actions": validation.NewError(codeInvalidAction, "a rule needs at least one action.")}
	}
	errs := validation.Errors{}
	for i, a := range actions {
		at := fmt.Sprintf("actions.%d", i)
		def, _, ok := defs.Action(a.Ref)
		if !ok {
			errs[at+".ref"] = validation.NewError(codeInvalidAction, fmt.Sprintf(
				"no installed action %q. Installed actions: %s.", a.Ref, strings.Join(defs.ActionRefs(), ", ")))
			continue
		}
		declared := map[string]ParamDef{}
		keys := make([]string, 0, len(def.Params))
		for _, p := range def.Params {
			declared[p.Key] = p
			keys = append(keys, p.Key)
		}
		given := make([]string, 0, len(a.Params))
		for k := range a.Params {
			given = append(given, k)
		}
		sort.Strings(given)
		for _, k := range given {
			if _, ok := declared[k]; !ok && !keptParam(previous, i, a.Ref, k) {
				errs[at+".params."+k] = validation.NewError(codeInvalidAction, fmt.Sprintf(
					"%q is not a param of %s. Params: %s.", k, a.Ref, paramList(keys)))
			}
		}
		for _, p := range def.Params {
			_, isRelation := relationParamTarget(app, def, p)
			if !p.Required && !isRelation {
				continue
			}
			if v, ok := a.Params[p.Key]; !ok || v == nil || strings.TrimSpace(fmt.Sprint(v)) == "" {
				errs[at+".params."+p.Key] = validation.NewError(codeInvalidAction, fmt.Sprintf(
					"%s needs a value. %s", p.Key, paramHint(p, isRelation)))
			}
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func paramHint(p ParamDef, isRelation bool) string {
	switch {
	case isRelation && p.RelationTarget != "":
		return "It is the id of a " + p.RelationTarget + " record."
	case isRelation:
		return "It is a record id."
	case len(p.Options) > 0:
		return "One of: " + strings.Join(p.Options, ", ") + "."
	default:
		return "It is " + p.Type + "."
	}
}

func paramList(keys []string) string {
	if len(keys) == 0 {
		return "none"
	}
	return strings.Join(keys, ", ")
}
