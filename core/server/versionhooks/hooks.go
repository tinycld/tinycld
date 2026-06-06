// Package versionhooks owns the per-itemType extension point drive uses
// to surface package-specific behavior around version snapshot/restore.
//
// Why this lives in core (and not drive): drive can't host this registry
// without forcing any package that wants to participate (text, calc, …)
// to require drive as a Go module dependency. The lean-shell guarantee
// — that a workspace assembled with only text but no drive must still
// build — forbids text→drive imports. Core already sits at the bottom
// of every package's dependency graph, so the hook registry lives here.
//
// Wiring:
//   - drive calls versionhooks.For(item.type) in handleSnapshotVersion /
//     handleRestoreVersion and invokes the matching callback.
//   - text (or any other package) calls versionhooks.Register("text",
//     versionhooks.Hook{OnSnapshot: ..., OnRestore: ...}) in its
//     Register() at process bootstrap.
//
// Hooks are best-effort. Drive logs failures but does not roll back the
// version row — the docx blob round-trip still works either way; the
// hook contributes package-specific metadata on top.
package versionhooks

import (
	"sync"

	"github.com/pocketbase/pocketbase/core"
)

// Hook bundles a snapshot + restore callback for a given drive_items.type.
type Hook struct {
	// OnSnapshot fires after the drive_item_versions row has been
	// created and the file blob copied. The package can read the live
	// state (e.g. an in-memory Y.Doc) and write metadata back to the
	// version row.
	OnSnapshot func(app core.App, item *core.Record, version *core.Record) error

	// OnRestore fires after the drive_item file has been replaced from
	// the version's blob. The package can read the version's
	// package-specific fields (e.g. yjs_state) and apply them to the
	// live state.
	OnRestore func(app core.App, item *core.Record, version *core.Record) error
}

var (
	mu    sync.RWMutex
	hooks = map[string]Hook{}
)

// Register records a hook for the given drive_items.type. Idempotent —
// re-registering replaces. Called from a package's Register() during
// process bootstrap.
func Register(itemType string, hook Hook) {
	mu.Lock()
	defer mu.Unlock()
	hooks[itemType] = hook
}

// For returns the registered hook for itemType, or a zero-value Hook
// (both fields nil) if none. Drive checks the returned fields for nil
// before invoking.
func For(itemType string) Hook {
	mu.RLock()
	defer mu.RUnlock()
	return hooks[itemType]
}

// ResetForTest clears the registry. Exposed only for tests — production
// callers never invoke this.
func ResetForTest() {
	mu.Lock()
	defer mu.Unlock()
	hooks = map[string]Hook{}
}
