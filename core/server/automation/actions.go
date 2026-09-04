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

// MarkEngineWrite stamps provenance on a write a native action handler performs
// itself, then runs it. Record-ops get this for free inside ExecuteAction;
// a native handler holds its own app and calls Save directly, so without this
// its output looks like an ordinary user write to the record hook — a handler
// that writes to a collection its own trigger watches re-fires that trigger
// with depth 0 forever, never reaching maxChainDepth.
//
// Call it around the Save/Delete, passing the ActionRequest so the rule id and
// depth come from the firing that invoked the handler:
//
//	return automation.MarkEngineWrite(req, record.Id, func() error {
//	    return app.Save(record)
//	})
//
// The record id must be known before the write (native handlers set an explicit
// id on create). The stamp is removed again if the write fails, so a failed
// attempt doesn't suppress the next genuine user edit of that record.
func MarkEngineWrite(req ActionRequest, recordID string, write func() error) error {
	if req.Rule == nil || recordID == "" {
		return write()
	}
	markEngineWrite(recordID, req.Rule.Id, req.Depth)
	if err := write(); err != nil {
		takeEngineWrite(recordID)
		return err
	}
	return nil
}

func substituteParams(defs ActionDef, raw map[string]any, record *core.Record, trigger TriggerDef, relationKeys map[string]bool) map[string]string {
	out := map[string]string{}
	for _, p := range defs.Params {
		v, ok := raw[p.Key]
		if !ok {
			continue
		}
		s := normalize(v)
		// A relation param carries a picker-chosen record id. Substituting
		// {{templates}} in it would let trigger-record CONTENT choose which
		// record the action touches — ids pass verbatim.
		if relationKeys[p.Key] {
			out[p.Key] = s
			continue
		}
		out[p.Key] = SubstituteTemplates(s, record, trigger)
	}
	return out
}

// relationParamTarget classifies one param: is it relation-typed, and which
// collection does it pick from. A typed param declares both; a column param
// inherits the column's. An empty target with isRelation=true means the
// declaration cannot resolve in this deployment — callers must fail closed,
// mirroring resolveAction greying the action out in the catalog.
func relationParamTarget(app core.App, action ActionDef, p ParamDef) (target string, isRelation bool) {
	if p.Field == "" {
		if p.Type != "relation" {
			return "", false
		}
		if p.RelationTarget == "" {
			return "", true
		}
		if col, err := app.FindCachedCollectionByNameOrId(p.RelationTarget); err == nil {
			return col.Name, true
		}
		return "", true
	}
	col, err := app.FindCachedCollectionByNameOrId(action.Collection)
	if err != nil {
		return "", false
	}
	rel, ok := col.Fields.GetByName(p.Field).(*core.RelationField)
	if !ok {
		return "", false
	}
	if targetCol, err := app.FindCachedCollectionByNameOrId(rel.CollectionId); err == nil {
		return targetCol.Name, true
	}
	return "", true
}

// authorizeRelationParams gates every caller-supplied record id before an
// action of either kind runs. Two layers, both fail-closed:
//
//  1. The FLOOR, engine-owned and generic: the rule owner must pass the
//     target collection's own view rule for the referenced record
//     (CanAccessRecord as the owner). This is pure data — collection rules
//     travel in migrations — so it holds in every deployment, but it only
//     proves the owner may SEE the record. A rule whose disjuncts need
//     request context beyond auth (e.g. boards' share-link token header)
//     correctly evaluates those branches false: automation acts as the
//     owner, never as an anonymous link holder.
//  2. The package's registered RelationAuthorizer, which owns the
//     write-level question (membership, role, ownership). Declaring a
//     relation param without registering its authorizer is refused outright
//     — the rollout's real bugs were all which-record bugs, and this turns
//     that question from a review-time convention into a contract.
func authorizeRelationParams(app core.App, ref string, action ActionDef, req ActionRequest) error {
	var owner *core.Record
	for _, p := range action.Params {
		target, isRelation := relationParamTarget(app, action, p)
		if !isRelation {
			continue
		}
		id := req.Params[p.Key]
		if id == "" {
			continue
		}
		if target == "" {
			return fmt.Errorf("relation param %q has no resolvable target collection — refusing", p.Key)
		}
		rec, err := app.FindRecordById(target, id)
		if err != nil {
			return fmt.Errorf("relation param %q: no %s record %q", p.Key, target, id)
		}
		if owner == nil {
			owner, err = app.FindRecordById("users", req.OwnerID)
			if err != nil {
				return fmt.Errorf("relation param %q: rule owner lookup: %w", p.Key, err)
			}
		}
		info := &core.RequestInfo{Auth: owner}
		if ok, err := app.CanAccessRecord(rec, info, rec.Collection().ViewRule); err != nil || !ok {
			return fmt.Errorf("relation param %q: rule owner may not use %s record %q", p.Key, target, id)
		}
		authorize, registered := relationAuthorizer(ref, p.Key)
		if !registered {
			return fmt.Errorf(
				"action %q relation param %q has no registered authorizer — the owning package must RegisterRelationAuthorizer for it",
				ref, p.Key,
			)
		}
		if err := authorize(app, req, id); err != nil {
			return fmt.Errorf("relation param %q: %w", p.Key, err)
		}
	}
	return nil
}

// authorizeTriggerRecordWrite gates a record-op that mutates or deletes the
// trigger record itself. It is the write-side mirror of the relation-param
// floor in authorizeRelationParams, and exists for the same reason: the op
// runs as superuser, so nothing below it consults the collection's own rules.
//
// checkPersonalAccess is not a substitute — it asks whether the owner may
// write to the PACKAGE, never whether they may write THIS record. A trigger
// fires for a whole audience (an org rule, a shared mailbox, a team board), so
// without this a rule owner who can merely see a record could move or delete
// it through automation while the same edit through the UI is refused.
//
// The floor is the collection's own updateRule/deleteRule evaluated as the
// owner, so it travels in migrations and holds in every deployment. A nil rule
// means superuser-only in PocketBase, so it is refused rather than waved
// through. Packages needing a stricter check register a TriggerRecordAuthorizer.
func authorizeTriggerRecordWrite(app core.App, ref string, op string, record *core.Record, req ActionRequest) error {
	owner, err := app.FindRecordById("users", req.OwnerID)
	if err != nil {
		return fmt.Errorf("trigger-record %s: rule owner lookup: %w", op, err)
	}

	collectionRule := record.Collection().UpdateRule
	if op == "delete" {
		collectionRule = record.Collection().DeleteRule
	}
	if collectionRule == nil {
		return fmt.Errorf(
			"trigger-record %s: %s is superuser-only (no %s rule) — refusing to act as the rule owner",
			op, record.Collection().Name, op,
		)
	}
	info := &core.RequestInfo{Auth: owner}
	if ok, err := app.CanAccessRecord(record, info, collectionRule); err != nil || !ok {
		return fmt.Errorf("trigger-record %s: rule owner may not %s %s record %q",
			op, op, record.Collection().Name, record.Id)
	}

	if authorize, registered := triggerRecordAuthorizer(ref); registered {
		if err := authorize(app, req, record); err != nil {
			return fmt.Errorf("trigger-record %s: %w", op, err)
		}
	}
	return nil
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
	relationKeys := map[string]bool{}
	for _, p := range action.Params {
		if _, isRelation := relationParamTarget(app, action, p); isRelation {
			relationKeys[p.Key] = true
		}
	}
	params := substituteParams(action, rawParams, record, trigger, relationKeys)
	req := ActionRequest{Rule: rule, OwnerID: ownerID, Params: params, Record: record, Depth: depth}

	// Both kinds pass the relation-id gates: a record-op's superuser Save
	// bypasses collection rules just as thoroughly as a native handler's
	// superuser app does.
	if err := authorizeRelationParams(app, ref, action, req); err != nil {
		return err
	}

	if action.Kind == "native" {
		h, ok := actionHandler(ref)
		if !ok {
			return fmt.Errorf("native action %q has no registered handler (package Go not linked?)", ref)
		}
		return h(app, req)
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
		if err := authorizeTriggerRecordWrite(app, ref, action.Op.Type, record, req); err != nil {
			return err
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
