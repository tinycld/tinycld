// Package automation's registries (actionHandlers, ownerResolvers,
// triggerFilters, relationAuthorizers below) are process-global vars, not
// per-app state. Every deployment shape runs one org per OS process — a
// multi-org router spawns a per-org build artifact as its own process
// (multi-org/README.md) — so these maps only ever serve one org's Engine;
// they are package globals for registration ergonomics, not for sharing.
package automation

import (
	"sync"

	"github.com/pocketbase/pocketbase/core"
)

// ActionRequest is what a native action handler receives. Params are already
// template-substituted strings (relation params pass verbatim — ids are
// picker-chosen, never templated); Record is nil for synthetic triggers.
//
// Native handlers run outside the record-op/pkgaccess path (see
// checkPersonalAccess in actions.go, which only re-applies pkgaccess to
// record-op actions): a native handler must self-enforce any access control
// it needs, the engine does not gate it for you. The one exception is
// relation params, which the engine does gate — a view-rule floor plus the
// action's registered RelationAuthorizer — because their values are
// caller-supplied record ids; everything else (recipients, folders named by
// text, the records a handler looks up itself) remains the handler's job.
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

// RelationAuthorizer answers the which-record question for one relation param:
// may this rule use the record `recordID` names? The engine refuses to run an
// action whose relation param has no registered authorizer, so declaring the
// param forces the package to answer — in code, not in a comment. The engine
// has already applied its own floor (the rule owner passes the target
// collection's view rule) before calling this; the authorizer adds the
// package's write-level semantics (membership, roles, ownership). Return nil
// to allow; a returned error fails the action and lands in run history.
type RelationAuthorizer func(app core.App, req ActionRequest, recordID string) error

var (
	registryMu     sync.RWMutex
	actionHandlers = map[string]ActionHandler{}
	ownerResolvers = map[string]OwnerResolver{}
	triggerFilters = map[string]TriggerFilter{}
	// actionRef -> paramKey -> authorizer
	relationAuthorizers = map[string]map[string]RelationAuthorizer{}
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

// RegisterRelationAuthorizer installs the which-record check for one relation
// param of one action. Packages call this next to RegisterAction (or, for a
// record-op's relation param, next to their other registrations) — an action
// with an unauthorized relation param is unavailable in the catalog and
// refused at execution, so this is not optional hardening but part of
// declaring the param.
func RegisterRelationAuthorizer(actionRef, paramKey string, a RelationAuthorizer) {
	registryMu.Lock()
	defer registryMu.Unlock()
	m, ok := relationAuthorizers[actionRef]
	if !ok {
		m = map[string]RelationAuthorizer{}
		relationAuthorizers[actionRef] = m
	}
	m[paramKey] = a
}

func relationAuthorizer(actionRef, paramKey string) (RelationAuthorizer, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	a, ok := relationAuthorizers[actionRef][paramKey]
	return a, ok
}

func hasRelationAuthorizer(actionRef, paramKey string) bool {
	_, ok := relationAuthorizer(actionRef, paramKey)
	return ok
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
	relationAuthorizers = map[string]map[string]RelationAuthorizer{}
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
