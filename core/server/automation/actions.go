// tinycld/core/server/automation/actions.go
package automation

import (
	"fmt"
	"sync"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/pkgaccess"
)

type engineWrite struct {
	RuleID string
	Depth  int
}

// engineWrites carries provenance from an engine-performed Save/Delete to the
// record hook it fires — the recentlyIndexed sentinel shape from mail. Entries
// are consumed by the dispatcher (Task 6); stale entries are harmless.
var engineWrites sync.Map

func markEngineWrite(recordID, ruleID string, depth int) {
	engineWrites.Store(recordID, engineWrite{RuleID: ruleID, Depth: depth})
}

func takeEngineWrite(recordID string) (engineWrite, bool) {
	v, ok := engineWrites.LoadAndDelete(recordID)
	if !ok {
		return engineWrite{}, false
	}
	return v.(engineWrite), true
}

func substituteParams(defs ActionDef, raw map[string]any, record *core.Record, trigger TriggerDef) map[string]string {
	out := map[string]string{}
	for _, p := range defs.Params {
		v, ok := raw[p.Key]
		if !ok {
			continue
		}
		s := normalize(v)
		out[p.Key] = SubstituteTemplates(s, record, trigger)
	}
	return out
}

func resolveSetValue(sv SetValue, params map[string]string, record *core.Record, ownerID string) (any, error) {
	switch {
	case sv.Param != "":
		return params[sv.Param], nil
	case sv.Context != "":
		switch sv.Context {
		case "record-id":
			if record == nil {
				return nil, fmt.Errorf("context record-id with no trigger record")
			}
			return record.Id, nil
		case "collection":
			if record == nil {
				return nil, fmt.Errorf("context collection with no trigger record")
			}
			return record.Collection().Name, nil
		case "owner":
			return ownerID, nil
		default:
			return nil, fmt.Errorf("unknown set context %q", sv.Context)
		}
	default:
		return sv.Literal, nil
	}
}

// checkPersonalAccess applies pkgaccess to personal-rule record-ops. The
// engine writes with superuser Save, which bypasses the REST guard entirely
// (pkgaccess binds request hooks only) — so the engine re-applies the check
// itself. Org rules run with system authority: admin-authored, documented in
// the spec. Core-owned collections (pkg "core") are not package-gated.
func checkPersonalAccess(app core.App, rule *core.Record, pkg string) error {
	if rule.GetString("scope") != "personal" || pkg == "core" {
		return nil
	}
	owner, err := app.FindRecordById("users", rule.GetString("owner"))
	if err != nil {
		return fmt.Errorf("rule owner lookup: %w", err)
	}
	return pkgaccess.WriteError(app, owner, pkg)
}

// ExecuteAction runs one rule action against one trigger event. depth is the
// chain depth to stamp onto any resulting engine write.
func ExecuteAction(app core.App, defs *Defs, ref string, rawParams map[string]any, rule *core.Record, trigger TriggerDef, record *core.Record, depth int) error {
	action, pkg, ok := defs.Action(ref)
	if !ok {
		return fmt.Errorf("unknown action %q (package uninstalled?)", ref)
	}
	ownerID := rule.GetString("owner")
	params := substituteParams(action, rawParams, record, trigger)

	if action.Kind == "native" {
		h, ok := actionHandler(ref)
		if !ok {
			return fmt.Errorf("native action %q has no registered handler (package Go not linked?)", ref)
		}
		return h(app, ActionRequest{Rule: rule, OwnerID: ownerID, Params: params, Record: record})
	}

	if err := checkPersonalAccess(app, rule, pkg); err != nil {
		return err
	}

	switch action.Op.Type {
	case "create":
		col, err := app.FindCollectionByNameOrId(action.Collection)
		if err != nil {
			return fmt.Errorf("action collection %q: %w", action.Collection, err)
		}
		made := core.NewRecord(col)
		made.Set("id", core.GenerateDefaultRandomId())
		for field, sv := range action.Op.Set {
			v, err := resolveSetValue(sv, params, record, ownerID)
			if err != nil {
				return err
			}
			made.Set(field, v)
		}
		marked := defsHaveTrigger(defs, action.Collection, "create")
		if marked {
			markEngineWrite(made.Id, rule.Id, depth)
		}
		if err := app.Save(made); err != nil {
			if marked {
				takeEngineWrite(made.Id)
			}
			return err
		}
		return nil
	case "update", "delete":
		if action.Op.Target != "trigger-record" {
			return fmt.Errorf("record-op %q: unsupported target %q", ref, action.Op.Target)
		}
		if record == nil {
			return fmt.Errorf("record-op %q targets the trigger record but the trigger has none", ref)
		}
		if record.Collection().Name != action.Collection {
			return fmt.Errorf("record-op %q: trigger record is %q, action declares %q", ref, record.Collection().Name, action.Collection)
		}
		if action.Op.Type == "delete" {
			marked := defsHaveTrigger(defs, action.Collection, "delete")
			if marked {
				markEngineWrite(record.Id, rule.Id, depth)
			}
			if err := app.Delete(record); err != nil {
				if marked {
					takeEngineWrite(record.Id)
				}
				return err
			}
			return nil
		}
		for field, sv := range action.Op.Set {
			v, err := resolveSetValue(sv, params, record, ownerID)
			if err != nil {
				return err
			}
			record.Set(field, v)
		}
		marked := defsHaveTrigger(defs, action.Collection, "update")
		if marked {
			markEngineWrite(record.Id, rule.Id, depth)
		}
		if err := app.Save(record); err != nil {
			if marked {
				takeEngineWrite(record.Id)
			}
			return err
		}
		return nil
	default:
		return fmt.Errorf("record-op %q: unknown op type %q", ref, action.Op.Type)
	}
}

// defsHaveTrigger reports whether any package declares a (collection, op)
// trigger — i.e. whether some hook is actually bound for this write. Marking
// a sentinel for a write nothing will ever consume (e.g. core:apply-label →
// label_assignments has no bound hook) leaks the sync.Map entry forever.
func defsHaveTrigger(defs *Defs, collection, op string) bool {
	return len(defs.TriggersFor(collection, op)) > 0
}
