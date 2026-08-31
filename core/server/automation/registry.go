// Package automation's registries (actionHandlers, ownerResolvers,
// triggerFilters, relationAuthorizers below) are process-global vars, not
// per-app state. Every deployment shape runs one org per OS process — a
// hosting router spawns a per-org build artifact as its own process
// (hosting/README.md) — so these maps only ever serve one org's Engine;
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
	// Depth is how many engine writes deep this firing already is. A handler
	// that writes to a trigger-bound collection must pass the request to
	// MarkEngineWrite so its output carries this provenance forward and the
	// chain terminates at maxChainDepth.
	Depth int
}

// ActionHandler is the function shape RegisterAction installs. See
// ActionRequest's doc for the access-control implication: nothing upstream of
// the handler enforces pkgaccess for native actions.
//
// A handler that writes to a collection any installed trigger watches MUST
// perform that write through MarkEngineWrite. The engine cannot stamp
// provenance for you here — it hands the handler an app and never sees the
// Save — so an unstamped write reads as a user edit and re-fires the trigger
// that invoked the handler, with no depth to terminate on.
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

// TriggerRecordAuthorizer answers the may-this-rule-write-THIS-record question
// for a record-op whose target is the trigger record. Unlike a relation param,
// this one is optional: the engine's floor (the rule owner passes the
// collection's own update/delete rule) is meaningful on its own here, because
// the record was chosen by the trigger rather than supplied by the rule author.
// Register one when a collection's rules are looser than its write semantics —
// e.g. a board a member may view and a card they may not move. Return nil to
// allow; an error fails the action and lands in run history.
type TriggerRecordAuthorizer func(app core.App, req ActionRequest, record *core.Record) error

var (
	registryMu     sync.RWMutex
	actionHandlers = map[string]ActionHandler{}
	ownerResolvers = map[string]OwnerResolver{}
	triggerFilters = map[string]TriggerFilter{}
	// actionRef -> paramKey -> authorizer
	relationAuthorizers = map[string]map[string]RelationAuthorizer{}
	// actionRef -> authorizer
	triggerRecordAuthorizers = map[string]TriggerRecordAuthorizer{}
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

// RegisterTriggerRecordAuthorizer installs the package's write-level check for
// a record-op that targets the trigger record. Optional — see
// TriggerRecordAuthorizer for when the engine's floor alone is not enough.
func RegisterTriggerRecordAuthorizer(actionRef string, a TriggerRecordAuthorizer) {
	registryMu.Lock()
	defer registryMu.Unlock()
	triggerRecordAuthorizers[actionRef] = a
}

func triggerRecordAuthorizer(actionRef string) (TriggerRecordAuthorizer, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	a, ok := triggerRecordAuthorizers[actionRef]
	return a, ok
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
	triggerRecordAuthorizers = map[string]TriggerRecordAuthorizer{}
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
