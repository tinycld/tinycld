package fts

import (
	"github.com/grafana/sobek"
	"github.com/pocketbase/pocketbase"
)

// JSVMBinder installs the `$fts` namespace onto a JS VM. It is exported so
// coreserver can register it with its OnInit binder registry
// (coreserver.RegisterJSVMBinder) WITHOUT this package importing coreserver —
// avoiding an import cycle, since coreserver imports fts to call Register.
//
// The data-plane path (index sync on record events) is pure core Go and does NOT
// use this binding — RegisterSync handles it. `$fts` exists only for the rare
// package that must query the index imperatively from its TS hooks. Keeping this
// surface tiny is deliberate: it is part of the enumerable set of capabilities
// untrusted tenant TS can reach.
//
// configsBySlug is captured so a TS call names its own slug (a package can only
// search the index it declared).
func JSVMBinder(configs []Config) func(vm *sobek.Runtime, app *pocketbase.PocketBase) error {
	bySlug := make(map[string]Config, len(configs))
	for _, c := range configs {
		bySlug[c.Slug] = c
	}

	return func(vm *sobek.Runtime, app *pocketbase.PocketBase) error {
		obj := vm.NewObject()

		// $fts.search(slug, userId, { q, limit, offset, includeDeleted })
		search := func(slug, userID string, opts map[string]any) (map[string]any, error) {
			cfg, ok := bySlug[slug]
			if !ok {
				return map[string]any{"items": []any{}, "total": 0}, nil
			}
			so := SearchOpts{Limit: 25}
			if v, ok := opts["q"].(string); ok {
				so.Query = v
			}
			if v, ok := opts["limit"].(int64); ok && v > 0 {
				so.Limit = int(v)
			}
			if v, ok := opts["offset"].(int64); ok && v >= 0 {
				so.Offset = int(v)
			}
			if v, ok := opts["includeDeleted"].(bool); ok {
				so.IncludeDeleted = v
			}
			results, total, err := Search(app, cfg, userID, so)
			if err != nil {
				return nil, err
			}
			items := make([]any, len(results))
			for i, r := range results {
				m := map[string]any{"id": r.ID}
				for k, v := range r.Columns {
					m[k] = v
				}
				items[i] = m
			}
			return map[string]any{"items": items, "total": total}, nil
		}

		if err := obj.Set("search", search); err != nil {
			return err
		}
		return vm.Set("$fts", obj)
	}
}
