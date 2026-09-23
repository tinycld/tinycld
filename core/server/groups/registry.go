// Package groups owns admin-managed user groups and the expansion of group
// grants into per-user membership rows.
//
// A package shares a resource with a group by writing a GRANT row into its own
// membership table: `group` set, `user` empty. This package expands every grant
// into one DERIVED row per member (`group` and `user` both set), keeps those
// rows in sync as membership and grants change, and repairs them at boot. The
// package's existing rules, which test `user`, then work unchanged.
//
// Packages declare their membership tables with RegisterGrantTable from their
// Register(app), the same way they declare offboard.RegisterReassignable. Core
// never names a package; tests use a fictional zoo_keepers table.
package groups

import (
	"errors"
	"sync"
)

// GrantTable declares a package membership table that carries optional `user`
// and `group` relation fields. ResourceField names the column that points at
// the shared resource (e.g. "project"), which is how a grant's derived rows
// are told apart from another grant's on the same group.
//
// The table's migration MUST add a unique index on (ResourceField, user,
// group) — e.g. `zoo_keepers` uses `zoo, user, group`. Without it, concurrent
// or repeated expansion can insert more than one derived row for the same
// (resource, group, user), since the expander's find-then-write is not itself
// atomic against another writer.
type GrantTable struct {
	Collection    string
	ResourceField string
}

// MembershipEvent describes one user joining or leaving one group. It is
// delivered after the membership row commits.
type MembershipEvent struct {
	UserID  string
	GroupID string
	Joined  bool
}

var (
	mu        sync.RWMutex
	tables    []GrantTable
	listeners []func(MembershipEvent) error
	errStop   = errors.New("groups: listener refused")
)

// RegisterGrantTable adds a membership table to the registry. Idempotent, and
// a blank collection or resource field is ignored, so a half-filled struct
// cannot register a table the expander then fails on.
func RegisterGrantTable(t GrantTable) {
	if t.Collection == "" || t.ResourceField == "" {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	for _, existing := range tables {
		if existing.Collection == t.Collection {
			return
		}
	}
	tables = append(tables, t)
}

// RegisteredGrantTables returns a snapshot of the registry.
func RegisteredGrantTables() []GrantTable {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]GrantTable, len(tables))
	copy(out, tables)
	return out
}

// OnMembershipChange registers a listener for join/leave events. Listeners run
// in registration order; the first error stops the chain and is logged by the
// caller. Mail uses this to provision or disable a mailbox.
func OnMembershipChange(fn func(MembershipEvent) error) {
	mu.Lock()
	defer mu.Unlock()
	listeners = append(listeners, fn)
}

func notifyMembership(e MembershipEvent) error {
	mu.RLock()
	fns := make([]func(MembershipEvent) error, len(listeners))
	copy(fns, listeners)
	mu.RUnlock()
	for _, fn := range fns {
		if err := fn(e); err != nil {
			return err
		}
	}
	return nil
}

// ResetForTesting clears tables and listeners. Tests register their own.
func ResetForTesting() {
	mu.Lock()
	defer mu.Unlock()
	tables = nil
	listeners = nil
}
