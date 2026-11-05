package forms

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// defaultDefinition mirrors defaultDefinitionFor in core/lib/forms/host.ts:
// every builtin in declared order, on one full-width page named after the
// host, or, when the host declares a layout, on the layout's pages with the
// page and width it gives each one (the first page and full width otherwise)
// and the layout's rules. Every slice and map is non-nil, because the client
// schema rejects a JSON null where it expects a list or an object
// (core/lib/forms/fixtures/seeded-definition.json, layout-definition.json).
func defaultDefinition(host *Host) *Definition {
	def := &Definition{
		Version:  1,
		Pages:    []Page{{Key: "main", Title: host.Label}},
		Fields:   []Field{},
		Rules:    []Rule{},
		Defaults: map[string]any{},
		Submit:   Submit{Required: []string{}},
	}
	layout := host.Layout
	if layout != nil && len(layout.Pages) > 0 {
		def.Pages = layout.Pages
	}
	if layout != nil && layout.Rules != nil {
		def.Rules = layout.Rules
	}
	first := def.Pages[0].Key
	for _, b := range host.Builtins {
		page, width, description := first, "full", ""
		if layout != nil {
			placed := layout.Fields[b.Key]
			if placed.Page != "" {
				page = placed.Page
			}
			if placed.Width != "" {
				width = placed.Width
			}
			description = placed.Description
		}
		def.Fields = append(def.Fields, builtinRef(b, page, width, description))
	}
	return def
}

// builtinRef is a form field that places built-in b. A host's help text for a
// built-in (its layout description) reaches every form seeded from the host,
// and a stored form that gains the built-in later.
func builtinRef(b Builtin, page, width, description string) Field {
	return Field{
		Key: b.Key, Kind: "builtin", Page: page, Label: b.Label, Description: description,
		Width: width, Required: b.Required,
	}
}

// layoutDescription is the help text a host's layout gives built-in key, or
// "" when the host has no layout or the layout gives it none.
func layoutDescription(host *Host, key string) string {
	if host.Layout == nil {
		return ""
	}
	return host.Layout.Fields[key].Description
}

func slugOf(label string) string {
	s := strings.ToLower(strings.TrimSpace(label))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" || out[0] < 'a' || out[0] > 'z' {
		out = "form_" + out
	}
	return out
}

// SeedDefaults brings every form's built-in fields in line with what its
// host declares now, gives every installed host one default form, and points
// records with an empty form column at it. Idempotent; runs on serve. A
// failing host does not stop the others: their errors are joined.
func SeedDefaults(app core.App, defs *Defs) error {
	col, err := app.FindCollectionByNameOrId(formsCollection)
	if err != nil {
		return nil // forms migration not applied yet; nothing to seed
	}
	var errs []error
	for i := range defs.Hosts {
		host := &defs.Hosts[i]
		if _, err := app.FindCollectionByNameOrId(host.Collection); err != nil {
			log.Info("skipping form seed, host collection absent", "host", host.Key)
			continue
		}
		if err := seedHost(app, col, host); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func seedHost(app core.App, col *core.Collection, host *Host) error {
	reconcileErr := reconcileForms(app, host)
	id, err := ensureDefault(app, col, host)
	if err != nil {
		return errors.Join(reconcileErr, err)
	}
	_, err = app.DB().NewQuery(fmt.Sprintf("UPDATE {{%s}} SET [[%s]] = {:id} WHERE [[%s]] = '' OR [[%s]] IS NULL",
		host.Collection, host.FormColumn, host.FormColumn, host.FormColumn)).Bind(dbx.Params{"id": id}).Execute()
	if err != nil {
		return errors.Join(reconcileErr, fmt.Errorf("forms: backfill %s: %w", host.Collection, err))
	}
	return reconcileErr
}

func ensureDefault(app core.App, col *core.Collection, host *Host) (string, error) {
	id, err := DefaultFormID(app, host.Key)
	if err != nil || id != "" {
		return id, err
	}
	// Non-archived forms sort first: the default cannot be archived.
	existing, err := app.FindRecordsByFilter(formsCollection, "host = {:host}", "archived,order", 1, 0, dbx.Params{"host": host.Key})
	if err != nil {
		return "", fmt.Errorf("forms: list forms of %s: %w", host.Key, err)
	}
	if len(existing) > 0 {
		// Forms exist but none is default (a manual edit). Promote the first.
		promoted := existing[0]
		promoted.Set("is_default", true)
		promoted.Set("archived", false)
		if err := app.Save(promoted); err != nil {
			return "", fmt.Errorf("forms: promote default for %s: %w", host.Key, err)
		}
		return promoted.Id, nil
	}
	raw, err := json.Marshal(defaultDefinition(host))
	if err != nil {
		return "", err
	}
	rec := core.NewRecord(col)
	rec.Set("host", host.Key)
	rec.Set("name", host.Label)
	rec.Set("slug", slugOf(host.Label))
	rec.Set("definition", raw)
	rec.Set("is_default", true)
	rec.Set("order", 0)
	if err := app.Save(rec); err != nil {
		return "", fmt.Errorf("forms: seed default for %s: %w", host.Key, err)
	}
	return rec.Id, nil
}

// reconcileForms brings each stored form of a host in line with the
// built-ins the host declares now, after a package update added or removed
// one. The definition is edited as plain JSON so properties the server does
// not model (width, display, description, ...) survive.
func reconcileForms(app core.App, host *Host) error {
	forms, err := app.FindRecordsByFilter(formsCollection, "host = {:host}", "", 0, 0, dbx.Params{"host": host.Key})
	if err != nil {
		return fmt.Errorf("forms: list forms of %s: %w", host.Key, err)
	}
	var errs []error
	for _, rec := range forms {
		var def map[string]any
		if err := json.Unmarshal([]byte(rec.GetString("definition")), &def); err != nil {
			errs = append(errs, fmt.Errorf("forms: form %s definition does not parse: %w", rec.Id, err))
			continue
		}
		added, removed := reconcileDefinition(def, host)
		if len(added) == 0 && len(removed) == 0 {
			continue
		}
		raw, err := json.Marshal(def)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		rec.Set("definition", raw)
		if err := app.Save(rec); err != nil {
			errs = append(errs, fmt.Errorf("forms: reconcile form %s: %w", rec.Id, err))
			continue
		}
		log.Info("form built-ins reconciled with host", "host", host.Key, "form", rec.Id, "added", added, "removed", removed)
	}
	return errors.Join(errs...)
}

// reconcileDefinition edits a decoded definition in place: a built-in the
// host declares but the form lacks is appended to the first page, and a
// built-in the host no longer declares is removed with every rule, default
// and submit entry that names it. It returns the keys added and removed.
func reconcileDefinition(def map[string]any, host *Host) (added, removed []string) {
	fields, _ := def["fields"].([]any)
	kept := make([]any, 0, len(fields))
	present := map[string]bool{}
	gone := map[string]bool{}
	for _, raw := range fields {
		f, _ := raw.(map[string]any)
		key, _ := f["key"].(string)
		if f["kind"] == "builtin" && host.Builtin(key) == nil {
			gone[key] = true
			removed = append(removed, key)
			continue
		}
		present[key] = true
		kept = append(kept, raw)
	}
	page := firstPageKey(def)
	for _, b := range host.Builtins {
		if present[b.Key] {
			continue
		}
		ref := builtinRef(b, page, "full", layoutDescription(host, b.Key))
		field := map[string]any{
			"key": ref.Key, "kind": ref.Kind, "page": ref.Page, "label": ref.Label,
			"width": ref.Width, "required": ref.Required, "hidden": false,
		}
		if ref.Description != "" {
			field["description"] = ref.Description
		}
		kept = append(kept, field)
		added = append(added, b.Key)
	}
	def["fields"] = kept
	if len(gone) == 0 {
		return added, removed
	}
	def["rules"] = rulesWithout(def["rules"], gone)
	if defaults, ok := def["defaults"].(map[string]any); ok {
		for key := range gone {
			delete(defaults, key)
		}
	}
	if submit, ok := def["submit"].(map[string]any); ok {
		submit["required"] = keysWithout(submit["required"], gone)
	}
	return added, removed
}

func firstPageKey(def map[string]any) string {
	pages, _ := def["pages"].([]any)
	if len(pages) == 0 {
		return "main"
	}
	p, _ := pages[0].(map[string]any)
	key, _ := p["key"].(string)
	return key
}

// rulesWithout drops a rule whose condition reads a removed field, since
// dropping only the condition would change when the rule fires, and drops an
// action that targets one; a rule left with no action goes too.
func rulesWithout(raw any, gone map[string]bool) []any {
	rules, _ := raw.([]any)
	out := make([]any, 0, len(rules))
	for _, r := range rules {
		rule, _ := r.(map[string]any)
		if readsAny(rule["when"], gone) {
			continue
		}
		actions, _ := rule["actions"].([]any)
		keptActions := make([]any, 0, len(actions))
		for _, a := range actions {
			action, _ := a.(map[string]any)
			if key, _ := action["field"].(string); gone[key] {
				continue
			}
			keptActions = append(keptActions, a)
		}
		if len(keptActions) == 0 {
			continue
		}
		rule["actions"] = keptActions
		out = append(out, rule)
	}
	return out
}

func readsAny(raw any, gone map[string]bool) bool {
	conds, _ := raw.([]any)
	for _, c := range conds {
		cond, _ := c.(map[string]any)
		if key, _ := cond["field"].(string); gone[key] {
			return true
		}
	}
	return false
}

func keysWithout(raw any, gone map[string]bool) []any {
	keys, _ := raw.([]any)
	out := make([]any, 0, len(keys))
	for _, k := range keys {
		if s, _ := k.(string); !gone[s] {
			out = append(out, k)
		}
	}
	return out
}
