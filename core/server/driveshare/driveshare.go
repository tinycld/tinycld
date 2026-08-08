// Package driveshare is core's single implementation of drive-item
// authorization: the predicate "may this user read / write / delete this
// drive_item". It exists because the answer must be identical in four
// places that cannot import each other — drive's WebDAV and custom
// endpoints, text's and calc's realtime room admission, and their render
// endpoints — and because those packages are separate Go modules whose
// only shared dependency is core.
//
// It is the Go-side mirror of the PocketBase collection rules on
// drive_items, as they stand after drive migration 1782100000:
//
//	view:   disabled != true && (created_by ?= auth.id || has-share)
//	update: disabled != true && (created_by ?= auth.id ||
//	                             (has-share && role ?= "editor" || role ?= "owner"))
//	delete: disabled != true && created_by ?= auth.id
//
// Two clauses there are easy to lose and both have been lost before:
//
//   - the `disabled != true` guard (1782000000), without which a suspended
//     account keeps reaching everything shared with it; and
//   - naming the roles that may WRITE rather than excluding viewer. The old
//     `role ?!= "viewer"` idiom admitted every role that was not viewer, so
//     `commentor` silently gained update the day it was added to the schema.
//
// Every function here bypasses PocketBase's rule engine — the SDK's
// FindRecordById / FindRecordsByFilter do — so each Go path that serves
// bytes or admits a socket must call one of these explicitly. When the
// migration's rules change, this file changes with it.
//
// Single-org: a drive_item has no owning organization and drive_shares
// points straight at a users row, so the authenticated user id IS the
// access key. The pre-migration predicate joined through a user_org
// junction; that junction and drive_items.org are both gone.
//
// Fail closed: every error path — DB failure, missing item, unknown role
// — denies. No branch in this package grants access on an error.
package driveshare

import (
	"fmt"
	"os"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/useraccount"
)

// Collection names of the drive-owned tables this package reads.
// Duplicated here rather than imported from drive because drive is a
// separate module; the names are the stable contract between them.
const (
	sharesCollection     = "drive_shares"
	driveItemsCollection = "drive_items"
	usersCollection      = "users"
)

// ErrNoAccess is returned when the user may not perform the requested
// operation on the item — no share row, insufficient role, the item does
// not exist, or the lookup failed. The causes are deliberately not
// distinguished: telling an unauthorized caller "that item exists but you
// lack the role" leaks the item's existence.
//
// It wraps os.ErrPermission so callers that branch on
// errors.Is(err, os.ErrPermission) — drive's WebDAV layer does — keep
// working unchanged.
var ErrNoAccess = fmt.Errorf("driveshare: no access to drive item: %w", os.ErrPermission)

// Role is a drive_shares.role value. Owner and editor may write; commentor
// and viewer may read only.
type Role string

const (
	RoleOwner  Role = "owner"
	RoleEditor Role = "editor"
	// RoleCommentor reads and comments; it never edits. Ranked between viewer
	// and editor: strictly more than read-only, strictly less than write.
	//
	// It was added to the drive_shares schema by migration 1781100000 but
	// never taught to this package, so rank() answered 0 — an unknown role —
	// and a signed-in commentor was denied even READ on every path gated here:
	// download tokens, text/calc render, realtime admission. That left them
	// with strictly less access than an anonymous visitor on the same share
	// link, while the collection rules were separately over-granting them
	// UPDATE. Both halves are corrected; see drive migration 1782100000.
	RoleCommentor Role = "commentor"
	RoleViewer    Role = "viewer"
	// RoleNone is the zero Role: no share row and not the creator.
	RoleNone Role = ""
)

// CanWrite reports whether the role grants edit access. Unknown roles — a
// future role this build has not been taught about — are read-only by
// default; fail closed.
func (r Role) CanWrite() bool {
	return r == RoleOwner || r == RoleEditor
}

// CanRead reports whether the role grants any access at all.
func (r Role) CanRead() bool {
	return rank(r) > 0
}

// rank orders roles so ResolveRole can pick the highest when several share
// rows exist for one (user, item) pair. Unknown roles rank 0.
func rank(r Role) int {
	switch r {
	case RoleOwner:
		return 4
	case RoleEditor:
		return 3
	case RoleCommentor:
		return 2
	case RoleViewer:
		return 1
	default:
		return 0
	}
}

// ResolveRole returns the highest-privilege role userID holds on the given
// drive_item, or RoleNone with ErrNoAccess if they hold none.
//
// The item's creator is treated as RoleOwner without needing a
// drive_shares row, matching the `created_by ?= @request.auth.id` disjunct
// in every one of the collection's rules. A newly created item may have no
// share row yet — drive's owner-share hook can be bypassed by direct SDK
// writes, and historically was — so the creator-only path is not
// hypothetical.
func ResolveRole(app core.App, userID, itemID string) (Role, error) {
	if userID == "" || itemID == "" {
		return RoleNone, ErrNoAccess
	}
	item, err := app.FindRecordById(driveItemsCollection, itemID)
	if err != nil {
		// Either the item doesn't exist or the DB call failed; either way
		// the caller gets no access to it.
		return RoleNone, ErrNoAccess
	}
	return ResolveRoleForItem(app, userID, item)
}

// ResolveRoleForItem is ResolveRole for a drive_items record the caller has
// already loaded, saving the FindRecordById round trip on paths that need
// the record anyway (drive's WebDAV walk loads every item before checking
// it).
//
// The record must be a drive_items row; passing anything else resolves
// created_by from the wrong collection.
func ResolveRoleForItem(app core.App, userID string, item *core.Record) (Role, error) {
	if userID == "" || item == nil {
		return RoleNone, ErrNoAccess
	}
	// A disabled account reaches nothing — not even its own files. Checked
	// before the creator branch so suspension outranks ownership.
	//
	// This is the Go half of the guard. PocketBase's REST API evaluates the
	// drive_items collection rules instead of this code, so those carry a
	// matching `@request.auth.disabled != true` clause (drive migration
	// 1782000000). Both halves are required; neither covers the other.
	if useraccount.IsSuspended(app, userID) {
		return RoleNone, ErrNoAccess
	}
	// The `created_by ?= @request.auth.id` disjunct. Short-circuits before
	// the share query — a creator needs no row.
	if item.GetString("created_by") == userID {
		return RoleOwner, nil
	}
	// No limit: we need every matching row to pick the highest role. The
	// UNIQUE (item, user) index caps this at one in practice; the loop
	// defends rows that predate the index.
	rows, err := app.FindRecordsByFilter(
		sharesCollection,
		"item = {:item} && user = {:user}",
		"", 0, 0,
		map[string]any{"item": item.Id, "user": userID},
	)
	if err != nil || len(rows) == 0 {
		return RoleNone, ErrNoAccess
	}
	best := RoleNone
	for _, r := range rows {
		if role := Role(r.GetString("role")); rank(role) > rank(best) {
			best = role
		}
	}
	if best == RoleNone {
		// Rows existed but every role was unrecognized — deny rather than
		// admit on a role this build doesn't understand.
		return RoleNone, ErrNoAccess
	}
	return best, nil
}

// CheckRead returns nil iff the user may view the item: its creator, or the
// holder of any drive_shares row. Mirrors the drive_items view rule.
//
// This is also the realtime room-admission predicate — a viewer may open
// the room, and write enforcement happens separately via CanWrite.
func CheckRead(app core.App, userID, itemID string) error {
	_, err := ResolveRole(app, userID, itemID)
	return err
}

// CheckReadItem is CheckRead for an already-loaded record.
func CheckReadItem(app core.App, userID string, item *core.Record) error {
	_, err := ResolveRoleForItem(app, userID, item)
	return err
}

// CheckWrite returns nil iff the user may modify the item: its creator, or
// an owner/editor share holder. Mirrors the drive_items update rule.
func CheckWrite(app core.App, userID, itemID string) error {
	role, err := ResolveRole(app, userID, itemID)
	if err != nil {
		return err
	}
	if !role.CanWrite() {
		return ErrNoAccess
	}
	return nil
}

// CanWrite is CheckWrite as a bool, for the read-only-connection decision
// on the realtime path where the reason for denial is never used. Any error
// is false — fail closed.
func CanWrite(app core.App, userID, itemID string) bool {
	return CheckWrite(app, userID, itemID) == nil
}

// CheckDelete returns nil iff the user may delete the item or manage its
// sharing: its creator, or an owner-role share holder. Mirrors the
// drive_items delete rule, widened by the owner role so a share explicitly
// granted "owner" is honored — drive's share endpoints depend on that.
func CheckDelete(app core.App, userID, itemID string) error {
	role, err := ResolveRole(app, userID, itemID)
	if err != nil {
		return err
	}
	if role != RoleOwner {
		return ErrNoAccess
	}
	return nil
}

// IsOwner is CheckDelete as a bool.
func IsOwner(app core.App, userID, itemID string) bool {
	return CheckDelete(app, userID, itemID) == nil
}

