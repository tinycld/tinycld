// tinycld/core/server/automation/catalog.go
//
// Materializes the engine's in-memory Defs into the automation_catalog
// collection: one row per trigger/action ref, with the resolved field/param
// type info that the client would otherwise have to re-derive from raw
// schema. buildCatalog is a pure derivation (unit-tested directly);
// syncCatalog reconciles it into rows the same shape as the cron-reconcile in
// schedule.go — upsert changed/new, delete stale.
package automation

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

type catalogField struct {
	Key            string   `json:"key"`
	Label          string   `json:"label"`
	Type           string   `json:"type"` // text|number|boolean|date|select|relation
	Options        []string `json:"options,omitempty"`
	RelationTarget string   `json:"relationTarget,omitempty"` // collection NAME
	DisplayField   string   `json:"displayField,omitempty"`   // for relation targets
}

type catalogTrigger struct {
	Ref        string         `json:"ref"`
	Pkg        string         `json:"pkg"`
	Label      string         `json:"label"`
	Synthetic  string         `json:"synthetic,omitempty"`
	Collection string         `json:"collection,omitempty"`
	Fields     []catalogField `json:"fields,omitempty"` // resolved exposed set, declaration order then alphabetical for open triggers
}

type catalogParam struct {
	Key      string       `json:"key"`
	Label    string       `json:"label"`
	Field    catalogField `json:"field"`    // resolved type info (novel params synthesize from ParamDef.Type)
	Template bool         `json:"template"` // true for text params when the trigger has fields (UI shows the placeholder menu)
}

type catalogAction struct {
	Ref        string         `json:"ref"`
	Pkg        string         `json:"pkg"`
	Label      string         `json:"label"`
	Kind       string         `json:"kind"`
	Collection string         `json:"collection,omitempty"`
	OpType     string         `json:"opType,omitempty"`   // create|update|delete (record-ops)
	OpTarget   string         `json:"opTarget,omitempty"` // trigger-record when applicable
	Params     []catalogParam `json:"params,omitempty"`
	Available  bool           `json:"available"` // native: handler registered; record-op: always true
}

type catalogResponse struct {
	Triggers []catalogTrigger `json:"triggers"`
	Actions  []catalogAction  `json:"actions"`
}

// humanizeFieldKeyGo mirrors core/lib/automation/helpers.ts's
// humanizeFieldKey: underscores become spaces, first letter capitalized. Kept
// in exact parity so a trigger's Go-derived label matches what the TS side
// would produce for the same key (catalog_test.go's TestCatalogResolution
// asserts has_attachments -> "Has attachments").
func humanizeFieldKeyGo(key string) string {
	spaced := strings.ReplaceAll(key, "_", " ")
	if spaced == "" {
		return spaced
	}
	return strings.ToUpper(spaced[:1]) + spaced[1:]
}

// resolvableColumns returns the same column set exposedFields would compute
// for an OPEN trigger (no declared allowlist) on this collection, further
// filtered to types the catalog/condition system has operators for. Factored
// out so the catalog's "what would an open trigger expose" resolution and
// template.go's security filters (system/hidden excluded, autodate
// carve-out) stay single-sourced — this literally calls exposedFields with an
// empty TriggerDef rather than re-implementing the skip logic.
func resolvableColumns(col *core.Collection) []core.Field {
	// exposedFields keys off record+trigger, not collection+fields directly;
	// build a zero-value record so its skip logic (system/hidden/autodate)
	// runs against this collection's schema without needing a real row.
	dummy := core.NewRecord(col)
	exposed := exposedFields(dummy, TriggerDef{})
	out := make([]core.Field, 0, len(exposed))
	for _, field := range col.Fields {
		if !exposed[field.GetName()] {
			continue
		}
		if resolveFieldType(field) == "" {
			continue // json/file/password/etc — no operators
		}
		out = append(out, field)
	}
	return out
}

// resolveFieldType maps a PB field to the catalog's coarse type vocabulary.
// Empty string means "not resolvable" (json/file/password/geoPoint/...) — the
// caller skips these, matching the "no operators" rule in OPERATORS_BY_TYPE.
func resolveFieldType(field core.Field) string {
	switch field.(type) {
	case *core.TextField, *core.EmailField, *core.URLField, *core.EditorField:
		return "text"
	case *core.NumberField:
		return "number"
	case *core.BoolField:
		return "boolean"
	case *core.DateField, *core.AutodateField:
		return "date"
	case *core.SelectField:
		return "select"
	case *core.RelationField:
		return "relation"
	default:
		return ""
	}
}

// displayFieldCandidates is the DisplayField heuristic order: first existing
// field among these on the target collection; else "id".
var displayFieldCandidates = []string{"name", "title", "label", "subject", "display_name", "email", "username"}

func displayFieldFor(col *core.Collection) string {
	for _, cand := range displayFieldCandidates {
		if col.Fields.GetByName(cand) != nil {
			return cand
		}
	}
	return "id"
}

// resolveCatalogField converts one PB field into the catalog's wire shape.
// label is the declared override if any, else the humanized key.
func resolveCatalogField(app core.App, field core.Field, labelOverride string) catalogField {
	out := catalogField{
		Key:  field.GetName(),
		Type: resolveFieldType(field),
	}
	if labelOverride != "" {
		out.Label = labelOverride
	} else {
		out.Label = humanizeFieldKeyGo(field.GetName())
	}
	switch f := field.(type) {
	case *core.SelectField:
		out.Options = append([]string{}, f.Values...)
	case *core.RelationField:
		if target, err := app.FindCachedCollectionByNameOrId(f.CollectionId); err == nil {
			out.RelationTarget = target.Name
			out.DisplayField = displayFieldFor(target)
		}
	}
	return out
}

// resolveTriggerFields resolves a trigger's exposed field set: the declared
// allowlist in declaration order (skipping fields that don't resolve to a
// usable type, don't exist, or aren't exposable per exposedFields' hidden/
// system rules), or — for open triggers — every resolvable column sorted
// alphabetically. Both branches run through exposedFields (the same set
// SubstituteTemplates enforces at runtime) so a trigger def that names a
// hidden/system field in its declared allowlist can't leak it into the
// catalog — declaring a field doesn't override the collection's own
// hidden/system flags, it only narrows an already-exposable set.
func resolveTriggerFields(app core.App, col *core.Collection, trigger TriggerDef) []catalogField {
	dummy := core.NewRecord(col)
	exposed := exposedFields(dummy, trigger)
	if len(trigger.Fields) > 0 {
		out := make([]catalogField, 0, len(trigger.Fields))
		for _, fr := range trigger.Fields {
			if !exposed[fr.Key] {
				continue
			}
			field := col.Fields.GetByName(fr.Key)
			if field == nil {
				continue
			}
			if resolveFieldType(field) == "" {
				continue
			}
			out = append(out, resolveCatalogField(app, field, fr.Label))
		}
		return out
	}
	cols := resolvableColumns(col)
	out := make([]catalogField, 0, len(cols))
	for _, field := range cols {
		out = append(out, resolveCatalogField(app, field, ""))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// resolveTrigger builds one catalogTrigger. Synthetic triggers (no backing
// collection) resolve with no Fields.
func resolveTrigger(app core.App, pkg string, t TriggerDef) catalogTrigger {
	out := catalogTrigger{
		Ref:       pkg + ":" + t.ID,
		Pkg:       pkg,
		Label:     t.Label,
		Synthetic: t.Synthetic,
	}
	if t.Synthetic != "" {
		return out
	}
	out.Collection = t.Collection
	col, err := app.FindCachedCollectionByNameOrId(t.Collection)
	if err != nil {
		return out // package data absent — leave Fields empty rather than error
	}
	out.Fields = resolveTriggerFields(app, col, t)
	return out
}

// resolveParam resolves one action param: a column-referencing param takes
// its type from the named field on the action's collection; a novel param
// (no Field, has Type) synthesizes a catalogField from its own declared
// type/options. Template is true for every text-typed param, full stop — the
// server has no authoritative answer to "does the trigger this rule ends up
// using have fields to insert" (that's a build-time UI-only fact: which
// trigger the rule author picked), so it can't gate this flag by trigger. The
// UI decides whether to actually show the {{placeholder}} MENU based on the
// selected trigger's resolved fields; this flag only says the param's TYPE
// supports templating at all.
func resolveParam(app core.App, col *core.Collection, p ParamDef) catalogParam {
	out := catalogParam{Key: p.Key}
	if p.Label != "" {
		out.Label = p.Label
	} else {
		out.Label = humanizeFieldKeyGo(p.Key)
	}
	if p.Field != "" && col != nil {
		if field := col.Fields.GetByName(p.Field); field != nil {
			out.Field = resolveCatalogField(app, field, "")
		}
	} else {
		out.Field = catalogField{Key: p.Key, Label: out.Label, Type: p.Type, Options: p.Options}
	}
	out.Template = out.Field.Type == "text"
	return out
}

// resolveAction builds one catalogAction. For record-ops, Available is
// always true unless the action's collection is absent from this deployment
// (package data absent), in which case it stays false so the UI can show
// "needs X" rather than silently omitting the action.
func resolveAction(app core.App, pkg string, a ActionDef) catalogAction {
	out := catalogAction{
		Ref:   pkg + ":" + a.ID,
		Pkg:   pkg,
		Label: a.Label,
		Kind:  a.Kind,
	}
	// A native action's params reference no collection — resolve them as
	// novel/typed params. A record-op's params may reference the action's
	// own collection's fields.
	var col *core.Collection
	if a.Kind == "native" {
		_, out.Available = actionHandler(out.Ref)
	} else {
		out.Collection = a.Collection
		out.OpType = a.Op.Type
		out.OpTarget = a.Op.Target
		var err error
		col, err = app.FindCachedCollectionByNameOrId(a.Collection)
		out.Available = err == nil
	}
	out.Params = make([]catalogParam, 0, len(a.Params))
	for _, p := range a.Params {
		out.Params = append(out.Params, resolveParam(app, col, p))
	}
	return out
}

// buildCatalog is the pure derivation: given the engine's app (for schema
// lookups) and defs, resolve every trigger/action into its catalog wire
// shape. No side effects, no DB writes — syncCatalog is the reconcile layer
// on top.
func (e *Engine) buildCatalog(app core.App) catalogResponse {
	var res catalogResponse
	for _, p := range e.defs.Packages {
		for _, t := range p.Triggers {
			res.Triggers = append(res.Triggers, resolveTrigger(app, p.Slug, t))
		}
		for _, a := range p.Actions {
			res.Actions = append(res.Actions, resolveAction(app, p.Slug, a))
		}
	}
	return res
}

type catalogRow struct {
	Kind      string
	Pkg       string
	Label     string
	Available bool
	Def       any
}

// syncCatalog reconciles the automation_catalog collection to match the
// engine's current defs: upsert changed/new rows keyed by ref, delete rows
// whose ref no longer appears (package uninstalled, action removed). Plain
// superuser Save/Delete with NO markEngineWrite — the provenance sentinel
// exists so a rule-triggered write doesn't re-fire its own trigger, and no
// defs can declare a trigger on automation_catalog (it's not a
// package-declarable collection), so there's nothing for the sentinel to
// protect here.
func (e *Engine) syncCatalog() {
	col, err := e.app.FindCollectionByNameOrId("automation_catalog")
	if err != nil {
		e.app.Logger().Error("automation: automation_catalog collection missing", "err", err)
		return
	}
	res := e.buildCatalog(e.app)

	desired := map[string]catalogRow{}
	for _, t := range res.Triggers {
		desired[t.Ref] = catalogRow{Kind: "trigger", Pkg: t.Pkg, Label: t.Label, Available: true, Def: t}
	}
	for _, a := range res.Actions {
		desired[a.Ref] = catalogRow{Kind: "action", Pkg: a.Pkg, Label: a.Label, Available: a.Available, Def: a}
	}

	existing, err := e.app.FindRecordsByFilter("automation_catalog", "", "", 0, 0)
	if err != nil {
		e.app.Logger().Error("automation: load catalog rows", "err", err)
		return
	}
	byRef := map[string]*core.Record{}
	for _, r := range existing {
		byRef[r.GetString("ref")] = r
	}

	for ref, row := range desired {
		defJSON, err := json.Marshal(row.Def)
		if err != nil {
			e.app.Logger().Error("automation: marshal catalog definition", "ref", ref, "err", err)
			continue
		}
		rec, ok := byRef[ref]
		if !ok {
			rec = core.NewRecord(col)
			rec.Set("ref", ref)
		}
		rec.Set("kind", row.Kind)
		rec.Set("pkg", row.Pkg)
		rec.Set("label", row.Label)
		rec.Set("definition", json.RawMessage(defJSON))
		rec.Set("available", row.Available)
		if err := e.app.Save(rec); err != nil {
			e.app.Logger().Error("automation: save catalog row", "ref", ref, "err", err)
		}
	}

	for ref, rec := range byRef {
		if _, ok := desired[ref]; ok {
			continue
		}
		if err := e.app.Delete(rec); err != nil {
			e.app.Logger().Error("automation: delete stale catalog row", "ref", ref, "err", err)
		}
	}
}
