package offboard

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/logging"
)

var log = logging.ForPackage("offboard")

// Mode chooses what happens to the offboarded user's owned content.
type Mode string

const (
	// ModeReassign rewrites every reassignable FK from the offboarded user to
	// the successor. The records stay; ownership transfers.
	ModeReassign Mode = "reassign"

	// ModeDeleteMyData deletes every record where a reassignable FK points at
	// the offboarded user. Used when the user wants their stuff gone, not
	// handed over.
	ModeDeleteMyData Mode = "delete_my_data"
)

// Plan captures the user's offboarding decision.
//
// SuccessorUserID is meaningful only when Mode == ModeReassign: it must be the
// id of another (non-deleted) users record that inherits the offboarded user's
// content. In ModeDeleteMyData the field is ignored.
//
// Single-org: there is exactly one org (the process). "Leaving the org" is no
// longer a distinct action — an account is either kept or deleted. Offboarding
// therefore has no auto-successor pick, no owner promotion, and no org
// deletion: those were all cross-org / multi-member concepts.
type Plan struct {
	Mode            Mode   `json:"mode"`
	SuccessorUserID string `json:"successor_user_id,omitempty"`
}

const deletedEmailDomain = "@deleted.tinycld.org"

// ErrInvalidPlan is returned by OffboardUser when the caller's plan is
// internally inconsistent (reassign without a successor, successor missing, etc.).
var ErrInvalidPlan = errors.New("invalid offboard plan")

// Result captures what the offboarding transaction actually did.
type Result struct {
	UserAnonymized    bool `json:"user_anonymized"`
	RecordsReassigned int  `json:"records_reassigned"`
	RecordsDeleted    int  `json:"records_deleted"`
}

// OffboardUser is the canonical account-offboarding transaction. It reassigns
// or deletes every record the user owns (per the registered reassignable FKs,
// which now hold users ids) and then anonymizes the users record. It is the
// single-org replacement for the former per-org leave-org loop.
//
// It is called by the delete-account flow (coreserver). Pass actorUserID = ""
// to suppress the summary audit row (system / migration use); otherwise the row
// records who triggered the action.
//
// Steps:
//  1. Validate the plan (reassign requires an existing successor users record).
//  2. Reassign (bulk UPDATE, hook-free) or delete (per record, hooks fire) the
//     registered reassignable FKs from the offboarded user.
//  3. Anonymize the users record.
func OffboardUser(app core.App, userID string, plan Plan, actorUserID string) (*Result, error) {
	if userID == "" {
		return nil, fmt.Errorf("%w: empty user id", ErrInvalidPlan)
	}

	result := &Result{}

	err := app.RunInTransaction(func(txApp core.App) error {
		if _, err := txApp.FindRecordById("users", userID); err != nil {
			return fmt.Errorf("load user %s: %w", userID, err)
		}

		switch plan.Mode {
		case ModeReassign:
			if plan.SuccessorUserID == "" {
				return fmt.Errorf("%w: reassign requires a successor user id", ErrInvalidPlan)
			}
			if plan.SuccessorUserID == userID {
				return fmt.Errorf("%w: successor cannot be the offboarded user", ErrInvalidPlan)
			}
			if _, err := txApp.FindRecordById("users", plan.SuccessorUserID); err != nil {
				return fmt.Errorf("%w: successor %s not found", ErrInvalidPlan, plan.SuccessorUserID)
			}
			n, err := reassignRefs(txApp, userID, plan.SuccessorUserID)
			if err != nil {
				return err
			}
			result.RecordsReassigned = n
		case ModeDeleteMyData:
			n, err := deleteOwnedRefs(txApp, userID)
			if err != nil {
				return err
			}
			result.RecordsDeleted = n
		default:
			return fmt.Errorf("%w: unknown mode %q", ErrInvalidPlan, plan.Mode)
		}

		if err := anonymizeUser(txApp, userID); err != nil {
			return fmt.Errorf("anonymize user: %w", err)
		}
		result.UserAnonymized = true
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Audit row is written outside the transaction so a downed audit_logs
	// collection can never fail an otherwise-successful offboard. Skipped when
	// the caller didn't pass an actor (system / migration use).
	if actorUserID != "" {
		writeOffboardAudit(app, userID, actorUserID, plan.Mode, result)
	}

	return result, nil
}

// reassignRefs walks the global registry and rewrites every FK pointing at the
// offboarded user to point at the successor. We do this as one raw UPDATE per
// (collection, field) pair — record-hooks aren't fired.
//
// Hook bypass is deliberate but has consequences worth knowing:
//
//   - audit_logs: per-row update hooks don't fire, so the audit table doesn't
//     get one row per reassigned record. That's intentional (a million
//     synthetic "edits" would drown out signal) — instead, OffboardUser writes
//     a single summary audit row at the end, so a reviewer still sees that the
//     action happened.
//   - drive_items.last_modified_by / FTS indexers: these key off content
//     changes (file body, name), not on created_by. A pure created_by rewrite
//     doesn't change indexed content, so skipping the hook is safe.
//   - calendar/realtime notify: calendar has no after-update hooks on
//     calendar_events; no notification is suppressed.
//
// If you add a reassignable ref to a collection whose update hook does
// meaningful per-row work (re-indexing, denormalized counters, etc.), either
// fire that work manually here or stop using the registry for that collection.
//
// Returns the total number of rows rewritten across all registered relations.
func reassignRefs(app core.App, leaverID, successorID string) (int, error) {
	refs := RegisteredReassignable()
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Collection != refs[j].Collection {
			return refs[i].Collection < refs[j].Collection
		}
		return refs[i].Field < refs[j].Field
	})
	total := 0
	for _, ref := range refs {
		if !collectionExists(app, ref.Collection) {
			// Lean-shell installs may not have every package's collections.
			// Quietly skip — registry entries from absent packages are inert.
			continue
		}
		q := app.DB().NewQuery(fmt.Sprintf(
			"UPDATE %s SET %s = {:successor} WHERE %s = {:leaver}",
			ref.Collection, ref.Field, ref.Field,
		)).Bind(dbx.Params{"successor": successorID, "leaver": leaverID})
		res, err := q.Execute()
		if err != nil {
			return total, fmt.Errorf("update %s.%s: %w", ref.Collection, ref.Field, err)
		}
		n, _ := res.RowsAffected()
		total += int(n)
	}
	return total, nil
}

// deleteOwnedRefs deletes every record where a reassignable FK points at the
// offboarded user. We do this record-by-record (not bulk DELETE) so hooks fire
// — downstream packages need cascade-via-hook behavior (mail FTS sync, calendar
// notify, etc.) to clean up properly.
func deleteOwnedRefs(app core.App, leaverID string) (int, error) {
	total := 0
	for _, ref := range RegisteredReassignable() {
		if !collectionExists(app, ref.Collection) {
			continue
		}
		records, err := app.FindRecordsByFilter(
			ref.Collection,
			fmt.Sprintf("%s = {:leaver}", ref.Field),
			"", 0, 0,
			map[string]any{"leaver": leaverID},
		)
		if err != nil {
			return total, fmt.Errorf("load %s.%s rows: %w", ref.Collection, ref.Field, err)
		}
		for _, r := range records {
			if err := app.Delete(r); err != nil {
				return total, fmt.Errorf("delete %s/%s: %w", ref.Collection, r.Id, err)
			}
			total++
		}
	}
	return total, nil
}

// writeOffboardAudit logs a single summary audit row capturing the offboarding
// action. Called outside the main transaction so an audit-write failure can't
// roll back the actual offboard. Best-effort: a missing or broken audit_logs
// collection is logged at warn level and ignored.
func writeOffboardAudit(app core.App, leaverUserID, actorUserID string, mode Mode, result *Result) {
	collection, err := app.FindCollectionByNameOrId("audit_logs")
	if err != nil {
		return // installs without the audit collection are tolerated
	}
	verb := "account_offboard"
	switch mode {
	case ModeDeleteMyData:
		verb += ".delete_data"
	case ModeReassign:
		verb += ".reassign"
	}
	r := core.NewRecord(collection)
	r.Set("action", verb)
	r.Set("resource_type", "users")
	r.Set("resource_id", leaverUserID)
	r.Set("actor", actorUserID)
	r.Set("metadata", map[string]any{
		"records_reassigned": result.RecordsReassigned,
		"records_deleted":    result.RecordsDeleted,
		"user_anonymized":    result.UserAnonymized,
	})
	if err := app.Save(r); err != nil {
		log.Error("audit write failed", "userID", leaverUserID, "err", err)
	}
}

// AnonymizeUser overwrites PII on the users record and invalidates the session
// (sentinel email, random password, refreshed token key). Exported so the
// account-delete orchestrator can call it directly when the user owns no
// reassignable records and a full OffboardUser pass isn't needed.
func AnonymizeUser(app core.App, userID string) error {
	return anonymizeUser(app, userID)
}

func anonymizeUser(app core.App, userID string) error {
	user, err := app.FindRecordById("users", userID)
	if err != nil {
		return err
	}
	sentinelEmail := fmt.Sprintf("deleted-%s%s", userID, deletedEmailDomain)
	randomPwd, err := randomHex(32)
	if err != nil {
		return err
	}
	user.SetEmail(sentinelEmail)
	user.Set("name", "Deleted user")
	user.Set("avatar", "")
	user.SetVerified(false)
	user.SetPassword(randomPwd)
	user.RefreshTokenKey()
	return app.Save(user)
}

func collectionExists(app core.App, name string) bool {
	_, err := app.FindCollectionByNameOrId(name)
	return err == nil
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
