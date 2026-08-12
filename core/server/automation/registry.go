package automation

import (
	"sync"

	"github.com/pocketbase/pocketbase/core"
)

// ActionRequest is what a native action handler receives. Params are already
// template-substituted strings; Record is nil for synthetic triggers.
type ActionRequest struct {
	Rule    *core.Record
	OwnerID string
	Params  map[string]string
	Record  *core.Record
}

type ActionHandler func(app core.App, req ActionRequest) error

// OwnerResolver maps a trigger record to the user ids the event belongs to,
// for triggers whose collection has no direct user FK (e.g. mail's shared
// mailboxes). Empty result = the event has no personal scope.
type OwnerResolver func(app core.App, record *core.Record) []string

var (
	registryMu     sync.RWMutex
	actionHandlers = map[string]ActionHandler{}
	ownerResolvers = map[string]OwnerResolver{}
)

// RegisterAction installs the Go handler for a native action ref. Packages
// call this from their Register(app), mirroring $-binding registration.
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

func ResetRegistriesForTest() {
	registryMu.Lock()
	defer registryMu.Unlock()
	actionHandlers = map[string]ActionHandler{}
	ownerResolvers = map[string]OwnerResolver{}
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
