// Package offboard owns account offboarding (formerly the "leave an org"
// workflow, and formerly named userorg for the user_org junction that no
// longer exists).
//
// Single-org: the process IS one org, so "leaving the org" is no longer a
// distinct action — an account is either kept or deleted. What survives is
// account offboarding: when a user's account is deleted, reassign or delete the
// records they own, then anonymize the users record.
//
// The package exposes:
//
//   - A RegisterReassignable hook each feature package calls during Register()
//     to declare which of its (collection, field) pairs point at a user as
//     authorship/ownership (e.g. calendar_events.created_by). Those FKs now
//     hold users ids directly (the former user_org junction is gone). The
//     offboarding transaction walks the registry to either rewrite those fields
//     to the successor user or delete the owning records, depending on the plan.
//   - OffboardUser(app, userID, plan, actorUserID) — the canonical transaction.
//     Called by the delete-account flow (coreserver).
//   - AnonymizeUser(app, userID) — anonymize a users record directly.
//
// The reassignable registry is process-global (one app instance per process)
// and is meant to be populated at startup. Tests can reset it with
// ResetReassignableForTesting.
package offboard

import (
	"sync"

	"github.com/pocketbase/pocketbase"
)

// Register is retained as a no-op wiring point so callers (coreserver) don't
// need to change shape. The former leave-org HTTP endpoints were pure cross-org
// membership management and no longer exist; account deletion goes through
// coreserver's /api/account/delete, which drives OffboardUser.
func Register(_ *pocketbase.PocketBase) {}

// ReassignableRef declares a single FK from <Collection>.<Field> to a user.
// At offboard time, every row where Field == the offboarded user's id is either
// rewritten (mode=reassign) or deleted (mode=delete_my_data).
//
// We deliberately keep this struct dumb (no behavior, no validation): the
// transaction code does all the work. Packages declare their refs at startup
// and never look at them again.
type ReassignableRef struct {
	// Collection is the PocketBase collection name, e.g. "calendar_events".
	Collection string
	// Field is the column name pointing at a user, e.g. "created_by".
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
