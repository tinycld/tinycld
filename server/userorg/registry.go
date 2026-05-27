// Package userorg owns the "leave an org" workflow.
//
// The user-facing action is per-org: a user can be a member of many orgs, and
// leaving one is independent from leaving another. Account anonymization is a
// downstream consequence of leaving the *last* org, not a separate user action.
//
// The package exposes:
//
//   - A RegisterReassignable hook each feature package calls during Register()
//     to declare which of its (collection, field) pairs point at user_org as
//     authorship/ownership (e.g. calendar_events.created_by). The leave-org
//     transaction walks the registry to either rewrite those fields to the
//     successor user_org or delete the owning records, depending on the plan.
//   - LeaveOrg(app, userOrgID, plan, actorIsLeaver) — the canonical
//     transaction. Called by the self-leave UI, the admin-remove-member UI,
//     and the multi-step delete-account flow.
//   - HTTP handlers for /api/account/leave-org and its preview counterpart.
//
// The reassignable registry is process-global (one app instance per process)
// and is meant to be populated at startup before OnServe fires. Tests can
// reset it with ResetReassignableForTesting.
package userorg

import "sync"

// ReassignableRef declares a single FK from <Collection>.<Field> to user_org.
// At leave-org time, every row where Field == leaverUserOrgID is either
// rewritten (mode=reassign) or deleted (mode=delete_my_data).
//
// We deliberately keep this struct dumb (no behavior, no validation): the
// transaction code does all the work. Packages declare their refs at startup
// and never look at them again.
type ReassignableRef struct {
	// Collection is the PocketBase collection name, e.g. "calendar_events".
	Collection string
	// Field is the column name pointing at user_org, e.g. "created_by".
	Field string
}

var (
	registryMu sync.RWMutex
	registry   []ReassignableRef
)

// RegisterReassignable adds a reassignable reference to the global registry.
// Idempotent: registering the same (Collection, Field) twice is a no-op (so
// re-registering on app reload during dev doesn't double-list anything).
func RegisterReassignable(ref ReassignableRef) {
	if ref.Collection == "" || ref.Field == "" {
		return
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	for _, existing := range registry {
		if existing.Collection == ref.Collection && existing.Field == ref.Field {
			return
		}
	}
	registry = append(registry, ref)
}

// RegisteredReassignable returns a snapshot of the registry. Used by the
// leave-org transaction and the preview endpoint.
func RegisteredReassignable() []ReassignableRef {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]ReassignableRef, len(registry))
	copy(out, registry)
	return out
}

// ResetReassignableForTesting clears the registry. Tests register their own
// minimal set of refs and need a clean slate per case.
func ResetReassignableForTesting() {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = nil
}
