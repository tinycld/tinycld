package userorg

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// Mode chooses what happens to the leaver's content inside the org.
type Mode string

const (
	// ModeReassign rewrites every reassignable FK from the leaver's
	// user_org to the successor's. The records stay; ownership transfers.
	ModeReassign Mode = "reassign"

	// ModeDeleteMyData deletes every record where a reassignable FK points
	// at the leaver. Used when the user wants their stuff gone, not handed
	// over. The user_org itself still deletes, and cascade-delete cleans up
	// per-user state.
	ModeDeleteMyData Mode = "delete_my_data"

	// ModeDeleteOrg deletes the entire org. Forced server-side when the
	// leaver is the sole member; rejected for multi-member orgs.
	ModeDeleteOrg Mode = "delete_org"
)

// Plan captures the user's per-org decision.
//
// SuccessorUserOrgID is meaningful only when Mode == ModeReassign:
//   - omit (or empty string) to let the server auto-pick the oldest owner
//     in the org, falling back to the oldest non-guest peer (who will be
//     promoted to owner inside the transaction).
//   - pass a specific user_org id to override the auto-pick. Must belong
//     to a peer of the same org or the call is rejected with ErrInvalidPlan.
//
// In any other mode (ModeDeleteMyData, ModeDeleteOrg) the field is ignored.
type Plan struct {
	Mode               Mode   `json:"mode"`
	SuccessorUserOrgID string `json:"successor_user_org_id,omitempty"`
}

const deletedEmailDomain = "@deleted.tinycld.org"

// ErrInvalidPlan is returned by LeaveOrg when the caller's plan is internally
// inconsistent (missing successor, successor not in the org, etc.).
var ErrInvalidPlan = errors.New("invalid leave-org plan")

// Result captures what the transaction actually did. anonymized=true means
// this was the user's last user_org and we anonymized the users record in the
// same transaction.
type Result struct {
	OrgDeleted        bool `json:"org_deleted"`
	UserAnonymized    bool `json:"user_anonymized"`
	RecordsReassigned int  `json:"records_reassigned"`
	RecordsDeleted    int  `json:"records_deleted"`
}

// LeaveOrg is the canonical leave-org transaction. It is called by:
//   - the self-leave UI (actorIsLeaver = true)
//   - the admin-remove-member UI (actorIsLeaver = false)
//   - the delete-account flow (actorIsLeaver = true, in a loop over orgs)
//
// The transaction is per-org. If a multi-org delete-account fails mid-loop,
// the orgs that already completed stay completed and the user can retry; the
// final anonymize only happens when the *last* user_org row goes away, so a
// partial failure leaves the user able to log in again and finish.
//
// Steps:
//  1. Resolve user_org → org and validate the plan against the org's member
//     list (sole-member orgs are forced to ModeDeleteOrg regardless of input).
//  2. If sole owner of a multi-member org and reassigning, promote the
//     successor to owner before rewriting refs.
//  3. Reassign (UPDATE in bulk, hook-free) or delete (per record, hooks
//     fire) the registered FKs.
//  4. Delete the user_org row. PB's cascade-delete handles per-user state.
//  5. For ModeDeleteOrg: delete the org record itself, bypassing the
//     orgs.deleteRule:null via UnsafeWithoutHooks. CascadeDelete on every
//     pbc_user_org_01 -> pbc_orgs_00001 relation tears down all per-org data.
//  6. If this was the user's last user_org AND the caller is the leaver,
//     anonymize the user record.
func LeaveOrg(app core.App, userOrgID string, plan Plan, actorIsLeaver bool) (*Result, error) {
	return LeaveOrgAs(app, userOrgID, plan, actorIsLeaver, "")
}

// LeaveOrgAs is LeaveOrg with an explicit actor user ID so the summary
// audit row records who triggered the action. Pass "" to suppress the
// audit row entirely (useful for migrations / system jobs without a human
// actor). The boolean is still required because the endpoint may allow a
// superuser-driven leave on behalf of another user, where actorIsLeaver is
// false but the leaver's own account should still anonymize on their last
// membership.
func LeaveOrgAs(app core.App, userOrgID string, plan Plan, actorIsLeaver bool, actorUserID string) (*Result, error) {
	result := &Result{}
	var effectiveModeForAudit Mode
	var orgIDForAudit, leaverUserIDForAudit string

	err := app.RunInTransaction(func(txApp core.App) error {
		leaver, err := txApp.FindRecordById("user_org", userOrgID)
		if err != nil {
			return fmt.Errorf("load user_org %s: %w", userOrgID, err)
		}
		userID := leaver.GetString("user")
		orgID := leaver.GetString("org")
		if orgID == "" || userID == "" {
			return fmt.Errorf("user_org %s missing user/org", userOrgID)
		}
		orgIDForAudit = orgID
		leaverUserIDForAudit = userID

		peers, err := loadOrgPeers(txApp, orgID, userOrgID)
		if err != nil {
			return fmt.Errorf("load peers for org %s: %w", orgID, err)
		}
		soleMember := len(peers) == 0

		effectiveMode, successorID, err := resolveMode(plan, soleMember, peers)
		if err != nil {
			return err
		}
		effectiveModeForAudit = effectiveMode

		// If the leaver is the sole owner, the org needs an owner after they
		// leave. promotionTarget is the user_org id that gets promoted —
		// usually the reassign successor, but in delete_my_data mode (no
		// successor) we still need to elevate someone or the org ends up
		// ownerless. Returns "" when no promotion is needed.
		promotionTarget := pickPromotionTarget(leaver, peers, effectiveMode, successorID)
		if promotionTarget != "" {
			if err := promoteToOwner(txApp, promotionTarget); err != nil {
				return fmt.Errorf("promote successor to owner: %w", err)
			}
		}

		switch effectiveMode {
		case ModeDeleteOrg:
			if err := applyDeleteOrg(txApp, orgID); err != nil {
				return err
			}
			result.OrgDeleted = true
		case ModeReassign:
			n, err := applyReassign(txApp, leaver.Id, successorID)
			if err != nil {
				return err
			}
			result.RecordsReassigned = n
			if err := finalizeLeave(txApp, leaver, orgID, userID); err != nil {
				return err
			}
		case ModeDeleteMyData:
			n, err := applyDeleteMyData(txApp, userOrgID)
			if err != nil {
				return err
			}
			result.RecordsDeleted = n
			if err := finalizeLeave(txApp, leaver, orgID, userID); err != nil {
				return err
			}
		}

		if actorIsLeaver {
			remaining, err := txApp.FindRecordsByFilter(
				"user_org",
				"user = {:uid}",
				"",
				1, 0,
				map[string]any{"uid": userID},
			)
			if err != nil {
				return fmt.Errorf("count remaining memberships: %w", err)
			}
			if len(remaining) == 0 {
				if err := anonymizeUser(txApp, userID); err != nil {
					return fmt.Errorf("anonymize user: %w", err)
				}
				result.UserAnonymized = true
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Audit row is written outside the transaction so a downed audit_logs
	// collection can never fail an otherwise-successful leave. Skipped when
	// the caller didn't pass an actor (system / migration use).
	//
	// Note: ModeDeleteOrg cascade-deletes audit_logs along with the org, so
	// the row we write here for that mode would vanish immediately. Skip it.
	if actorUserID != "" && effectiveModeForAudit != ModeDeleteOrg {
		writeLeaveOrgAudit(app, orgIDForAudit, leaverUserIDForAudit, actorUserID, effectiveModeForAudit, result)
	}

	return result, nil
}

// applyDeleteOrg nukes the entire org. PocketBase's cascade-delete walks
// the reference graph from the org outward (orgs → calendar_calendars →
// calendar_events → user_org via its org FK, etc.), so we don't need to
// (and must not) delete the user_org row ourselves first — that would trip
// on a FK from any record that points at the user_org but isn't a direct
// child of orgs. Delete the org and let the cascade unwind.
//
// orgs.deleteRule is null at the API layer (superuser only), but we're
// calling app.Delete directly from Go which bypasses rules — rules only
// gate HTTP/PB routes. Hooks (including the cascade logic in
// onRecordDeleteExecute) must still run, so we deliberately do NOT use
// UnsafeWithoutHooks here.
//
// Every reassignable-bearing collection MUST have a cascade path rooted at
// orgs for this to succeed mid-transaction. Verified chains as of 2026-05-27:
//   - calendar_events.calendar → calendar_calendars(cascade) → orgs(cascade) ✓
//   - drive_items.org → orgs(cascade) ✓
//   - drive_shares.item → drive_items(cascade) → orgs(cascade) ✓
//   - drive_item_versions.item → drive_items(cascade) → orgs(cascade) ✓
//   - drive_share_links.item → drive_items(cascade) → orgs(cascade) ✓
//   - drive_preview_comments.drive_item → drive_items(cascade) → orgs(cascade) ✓
//   - calc_comments.drive_item → drive_items(cascade) → orgs(cascade) ✓
//   - text_comments.drive_item → drive_items(cascade) → orgs(cascade) ✓
//
// If you add a new reassignable ref to a collection without a cascade path
// to orgs, ModeDeleteOrg will fail with a referential-integrity error and
// you need to either (a) extend the schema to add the cascade, or
// (b) add a per-collection cleanup pass to applyDeleteOrg.
//
// TestDeleteOrgCascade_MultiLevelChain exercises a three-level chain
// (events → folders → orgs) and is the regression guard for the above.
func applyDeleteOrg(app core.App, orgID string) error {
	org, err := app.FindRecordById("orgs", orgID)
	if err != nil {
		return fmt.Errorf("load org %s: %w", orgID, err)
	}
	if err := app.Delete(org); err != nil {
		return fmt.Errorf("delete org %s: %w", orgID, err)
	}
	return nil
}

// applyReassign rewrites every registered FK from the leaver to the
// successor. The caller is responsible for any sole-owner promotion
// (handled at the LeaveOrg top level via pickPromotionTarget).
func applyReassign(app core.App, leaverUserOrgID, successorID string) (int, error) {
	n, err := reassignRefs(app, leaverUserOrgID, successorID)
	if err != nil {
		return 0, fmt.Errorf("reassign refs: %w", err)
	}
	return n, nil
}

// applyDeleteMyData removes every record where a reassignable FK points at
// the leaver, so the user_org row's cascade-delete path is clear.
func applyDeleteMyData(app core.App, leaverUserOrgID string) (int, error) {
	n, err := deleteOwnedRefs(app, leaverUserOrgID)
	if err != nil {
		return 0, fmt.Errorf("delete owned refs: %w", err)
	}
	return n, nil
}

// finalizeLeave runs the common tail after applyReassign / applyDeleteMyData:
// drop the user_org row and clean up the redundant orgs.users multi-relation.
// Skipped in delete_org mode — the cascade handles both.
func finalizeLeave(app core.App, leaver *core.Record, orgID, userID string) error {
	if err := app.Delete(leaver); err != nil {
		return fmt.Errorf("delete user_org %s: %w", leaver.Id, err)
	}
	if err := removeUserFromOrgsRelation(app, orgID, userID); err != nil {
		return fmt.Errorf("strip orgs.users: %w", err)
	}
	return nil
}

// orgPeer is a remaining org member after subtracting the leaver. Used for
// auto-pick (oldest owner) and successor validation.
type orgPeer struct {
	UserOrgID string
	UserID    string
	Role      string
	Created   string
}

func loadOrgPeers(app core.App, orgID, leaverUserOrgID string) ([]orgPeer, error) {
	records, err := app.FindRecordsByFilter(
		"user_org",
		"org = {:orgID} && id != {:leaver}",
		"+created",
		0, 0,
		map[string]any{"orgID": orgID, "leaver": leaverUserOrgID},
	)
	if err != nil {
		return nil, err
	}
	out := make([]orgPeer, 0, len(records))
	for _, r := range records {
		out = append(out, orgPeer{
			UserOrgID: r.Id,
			UserID:    r.GetString("user"),
			Role:      r.GetString("role"),
			Created:   r.GetString("created"),
		})
	}
	return out, nil
}

// resolveMode validates the caller's plan against reality and computes the
// effective mode + successor. The sole-member case overrides the client's
// chosen mode (we can't reassign when there's no one to reassign to, and
// delete_my_data on a sole-member org would leave the org orphaned without
// content — same end state as delete_org but messier).
func resolveMode(plan Plan, soleMember bool, peers []orgPeer) (Mode, string, error) {
	if soleMember {
		return ModeDeleteOrg, "", nil
	}
	switch plan.Mode {
	case ModeDeleteOrg:
		return "", "", fmt.Errorf("%w: delete_org is only allowed when the leaver is the sole member", ErrInvalidPlan)
	case ModeDeleteMyData:
		return ModeDeleteMyData, "", nil
	case ModeReassign:
		successorID := plan.SuccessorUserOrgID
		if successorID == "" {
			// Auto-pick: prefer the oldest owner (peers is already +created
			// sorted). Fall back to the oldest non-guest peer; they'll be
			// promoted to owner inside the transaction via pickPromotionTarget.
			//
			// Guests are deliberately excluded from auto-pick — they're
			// second-class per the org-RLS model and should never be silently
			// promoted into an owner role. A caller who really wants to pick
			// a guest must do so explicitly via plan.SuccessorUserOrgID.
			for _, p := range peers {
				if p.Role == "owner" {
					successorID = p.UserOrgID
					break
				}
			}
			if successorID == "" {
				for _, p := range peers {
					if p.Role != "guest" {
						successorID = p.UserOrgID
						break
					}
				}
			}
			if successorID == "" {
				return "", "", fmt.Errorf("%w: no eligible non-guest successor in this org", ErrInvalidPlan)
			}
		}
		// Validate successor is actually in the peer set.
		found := false
		for _, p := range peers {
			if p.UserOrgID == successorID {
				found = true
				break
			}
		}
		if !found {
			return "", "", fmt.Errorf("%w: successor %s is not a peer in this org", ErrInvalidPlan, successorID)
		}
		return ModeReassign, successorID, nil
	default:
		return "", "", fmt.Errorf("%w: unknown mode %q", ErrInvalidPlan, plan.Mode)
	}
}

// pickPromotionTarget returns the user_org id that should be promoted to
// owner inside the leave-org transaction, or "" when no promotion is needed.
//
// Promotion is needed when:
//   - the leaver currently has role=owner, AND
//   - after they leave, the org would have zero owners.
//
// In ModeReassign, the chosen successor is the natural promotion target
// (records go to them; making them owner keeps authority co-located with
// content). In ModeDeleteMyData, no successor is involved, but we still
// need to elevate *someone* — pick the oldest non-guest peer. ModeDeleteOrg
// nukes the org so promotion is moot.
func pickPromotionTarget(leaver *core.Record, peers []orgPeer, mode Mode, successorID string) string {
	if leaver.GetString("role") != "owner" {
		return ""
	}
	if mode == ModeDeleteOrg {
		return ""
	}
	for _, p := range peers {
		if p.Role == "owner" {
			return "" // an owner remains; no promotion needed
		}
	}
	if mode == ModeReassign && successorID != "" {
		return successorID
	}
	// ModeDeleteMyData (or reassign with no successor for some odd reason):
	// fall back to the oldest non-guest peer.
	for _, p := range peers {
		if p.Role != "guest" {
			return p.UserOrgID
		}
	}
	return ""
}

// promoteToOwner sets role=owner on the given user_org row. Used to elevate
// a successor / oldest peer when the sole-owner leaves.
func promoteToOwner(app core.App, userOrgID string) error {
	record, err := app.FindRecordById("user_org", userOrgID)
	if err != nil {
		return err
	}
	record.Set("role", "owner")
	return app.Save(record)
}

// reassignRefs walks the global registry and rewrites every FK pointing at
// the leaver to point at the successor. We do this as one raw UPDATE per
// (collection, field) pair — record-hooks aren't fired.
//
// Hook bypass is deliberate but has consequences worth knowing:
//
//   - audit_logs: per-row update hooks don't fire, so the audit table doesn't
//     get one row per reassigned record. That's intentional (a million
//     synthetic "edits" would drown out signal) — instead, LeaveOrg writes a
//     single summary audit row via writeReassignAuditRow at the end of the
//     transaction, so org owners reviewing the log still see that the action
//     happened.
//   - drive_items.last_modified_by / FTS indexers: these key off content
//     changes (file body, name), not on created_by. A pure created_by
//     rewrite doesn't change indexed content, so skipping the hook is safe.
//   - calendar/realtime notify: calendar has no after-update hooks on
//     calendar_events; no notification is suppressed.
//
// If you add a reassignable ref to a collection whose update hook does
// meaningful per-row work (re-indexing, denormalized counters, etc.),
// either fire that work manually here or stop using the registry for that
// collection.
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
// leaver. We do this record-by-record (not bulk DELETE) so hooks fire —
// downstream packages need cascade-via-hook behavior (mail FTS sync, calendar
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

// removeUserFromOrgsRelation strips the user from orgs.users (a redundant
// multi-relation kept for some legacy access rules). Optional — the field may
// not be present in every install.
func removeUserFromOrgsRelation(app core.App, orgID, userID string) error {
	orgsCollection, err := app.FindCollectionByNameOrId("orgs")
	if err != nil {
		return nil // collection missing → nothing to do
	}
	if orgsCollection.Fields.GetByName("users") == nil {
		return nil
	}
	org, err := app.FindRecordById("orgs", orgID)
	if err != nil {
		return err
	}
	users := org.GetStringSlice("users")
	kept := make([]string, 0, len(users))
	for _, u := range users {
		if u != userID {
			kept = append(kept, u)
		}
	}
	if len(kept) == len(users) {
		return nil // user wasn't in the list
	}
	org.Set("users", kept)
	return app.Save(org)
}

// writeLeaveOrgAudit logs a single summary audit row capturing the
// leave-org action. Called outside the main transaction so an audit-write
// failure can't roll back the actual leave. Best-effort: a missing or
// broken audit_logs collection is logged at warn level and ignored.
//
// The action verb encodes mode + actor so log readers can tell self-leave
// from admin-removal at a glance ("leave_org.self", "leave_org.admin", etc.).
func writeLeaveOrgAudit(app core.App, orgID, leaverUserID, actorUserID string, mode Mode, result *Result) {
	collection, err := app.FindCollectionByNameOrId("audit_logs")
	if err != nil {
		return // installs without the audit collection are tolerated
	}
	verb := "leave_org.self"
	if leaverUserID != actorUserID {
		verb = "leave_org.admin"
	}
	switch mode {
	case ModeDeleteOrg:
		verb = "delete_org"
	case ModeDeleteMyData:
		verb += ".delete_data"
	case ModeReassign:
		verb += ".reassign"
	}
	r := core.NewRecord(collection)
	r.Set("org", orgID)
	r.Set("action", verb)
	r.Set("resource_type", "user_org")
	r.Set("resource_id", leaverUserID)
	r.Set("actor", actorUserID)
	r.Set("metadata", map[string]any{
		"records_reassigned": result.RecordsReassigned,
		"records_deleted":    result.RecordsDeleted,
		"org_deleted":        result.OrgDeleted,
		"user_anonymized":    result.UserAnonymized,
	})
	if err := app.Save(r); err != nil {
		app.Logger().Warn("leave-org: audit write failed", "org", orgID, "error", err)
	}
}

// AnonymizeUser overwrites PII on the users record and invalidates the
// session. Mirrors the original account_delete.go behavior (sentinel email,
// random password, refreshed token key). Exported so the account-delete
// orchestrator can call it directly in the zero-org edge case (a user with
// no memberships at all skips the LeaveOrg loop entirely).
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
