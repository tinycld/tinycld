package webdav

import "github.com/pocketbase/pocketbase/core"

const sourceHooksStoreKey = "webdav.sourceHooks."

// RegisterSourceHooks records feature Go hooks for the Source with the given
// slug, for compositions where the Source list is materialized from config —
// a tenant — while the feature's Go links separately. A Go closure cannot
// travel through the materialized JSON, so a tenant's sources arrive with
// zero Hooks and, before this seam, a tenant-served overwrite silently
// skipped version archiving (HANDOFF §6 / R7).
//
// Call it before the sources mount: the tenant composition runs
// RegisterExtras (feature Go) ahead of webdav.Register, so a feature's
// RegisterTenant is the natural site. NewFileSystem adopts registered hooks
// onto the matching source; hooks set explicitly on a Source win — the
// registry only fills the gap materialization leaves.
func RegisterSourceHooks(app core.App, slug string, hooks Hooks) {
	app.Store().Set(sourceHooksStoreKey+slug, hooks)
}

// RegisteredSourceHooks reports the hooks recorded for a slug, if any.
// Exported so a feature's composition tests can assert its RegisterTenant
// actually wires the seam.
func RegisteredSourceHooks(app core.App, slug string) (Hooks, bool) {
	hooks, ok := app.Store().Get(sourceHooksStoreKey + slug).(Hooks)
	return hooks, ok
}

// adoptRegisteredHooks fills a source's zero hook slots from the registry.
func adoptRegisteredHooks(app core.App, src Source) Source {
	registered, ok := RegisteredSourceHooks(app, src.Slug)
	if !ok {
		return src
	}
	if src.Hooks.BeforeOverwrite == nil {
		src.Hooks.BeforeOverwrite = registered.BeforeOverwrite
	}
	return src
}
