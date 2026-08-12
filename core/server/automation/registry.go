// Package automation's registries (actionHandlers, ownerResolvers,
// triggerFilters below) are process-global vars, not per-app state. In
// multi-org mode one process hosts many tenant apps, each with its own
// Engine — all of them share these same maps. That's fine in practice: refs
// are package-qualified ("mail:send", "core:notify", …) and collide only on a
// 15-char random id, which random-id generation makes negligible. Do not
// assume a registered handler/resolver/filter is scoped to one tenant.
package automation

import (
	"sync"

	"github.com/pocketbase/pocketbase/core"
)

// ActionRequest is what a native action handler receives. Params are already
// template-substituted strings; Record is nil for synthetic triggers.
//
// Native handlers run outside the record-op/pkgaccess path (see
// checkPersonalAccess in actions.go, which only re-applies pkgaccess to
// record-op actions): a native handler must self-enforce any access control
// it needs, the engine does not gate it for you.
type ActionRequest struct {
	Rule    *core.Record
	OwnerID string
	Params  map[string]string
	Record  *core.Record
}

// ActionHandler is the function shape RegisterAction installs. See
// ActionRequest's doc for the access-control implication: nothing upstream of
// the handler enforces pkgaccess for native actions.
type ActionHandler func(app core.App, req ActionRequest) error

// OwnerResolver maps a trigger record to the user ids the event belongs to,
// for triggers whose collection has no direct user FK (e.g. mail's shared
// mailboxes). Empty result = the event has no personal scope.
type OwnerResolver func(app core.App, record *core.Record) []string

// TriggerFilter gates whether a record event counts as this trigger at all —
// for triggers whose collection carries rows the trigger's semantics exclude
// (e.g. mail's "message-received" must not fire on drafts or outbound sends).
// Applies to ALL rule scopes, unlike OwnerResolver which only scopes personal
// rules.
type TriggerFilter func(app core.App, record *core.Record) bool

var (
	registryMu     sync.RWMutex
	actionHandlers = map[string]ActionHandler{}
	ownerResolvers = map[string]OwnerResolver{}
	triggerFilters = map[string]TriggerFilter{}
)

// RegisterAction installs the Go handler for a native action ref. Packages
// call this from their Register(app), mirroring $-binding registration.
// pkgaccess is NOT applied to native actions (only to record-ops, via
// checkPersonalAccess) — the handler itself must enforce any access control
// it needs.
func RegisterAction(ref string, h ActionHandler) {
	registryMu.Lock()
	defer registryMu.Unlock()
	actionHandlers[ref] = h
}

func actionHandler(ref string) (ActionHandler, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	h, ok := actionHandlers[ref]
	return h, ok
}

// RegisterOwnerResolver overrides owner auto-detection for one trigger ref.
func RegisterOwnerResolver(triggerRef string, r OwnerResolver) {
	registryMu.Lock()
	defer registryMu.Unlock()
	ownerResolvers[triggerRef] = r
}

// RegisterTriggerFilter installs a gate for one trigger ref: a record event
// is only treated as that trigger when the filter returns true. Applies
// before owner resolution and to org-scoped rules alike.
func RegisterTriggerFilter(triggerRef string, f TriggerFilter) {
	registryMu.Lock()
	defer registryMu.Unlock()
	triggerFilters[triggerRef] = f
}

// triggerAllowed reports whether record counts as triggerRef. No registered
// filter = always allowed.
func triggerAllowed(app core.App, triggerRef string, record *core.Record) bool {
	registryMu.RLock()
	filter, ok := triggerFilters[triggerRef]
	registryMu.RUnlock()
	if !ok {
		return true
	}
	return filter(app, record)
}

func ResetRegistriesForTest() {
	registryMu.Lock()
	defer registryMu.Unlock()
	actionHandlers = map[string]ActionHandler{}
	ownerResolvers = map[string]OwnerResolver{}
	triggerFilters = map[string]TriggerFilter{}
}

var autoOwnerFields = []string{"user", "owner", "author"}

// ResolveOwners: registered resolver > declared ownerField > auto-detected
// user/owner/author relation → users. Empty = personal rules never match
// this event (locked Phase 2 decision — org rules still fire).
func ResolveOwners(app core.App, triggerRef string, trigger TriggerDef, record *core.Record) []string {
	if record == nil {
		return nil
	}
	registryMu.RLock()
	resolver, hasResolver := ownerResolvers[triggerRef]
	registryMu.RUnlock()
	if hasResolver {
		return resolver(app, record)
	}

	candidates := autoOwnerFields
	if trigger.OwnerField != "" {
		candidates = []string{trigger.OwnerField}
	}
	usersCol, err := app.FindCachedCollectionByNameOrId("users")
	if err != nil {
		return nil
	}
	for _, name := range candidates {
		field := record.Collection().Fields.GetByName(name)
		rel, ok := field.(*core.RelationField)
		if !ok || rel.CollectionId != usersCol.Id {
			continue
		}
		return stringValues(record, name)
	}
	return nil
}
