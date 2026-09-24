package groups

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// Reconcile diffs expected derived rows against actual rows for every
// registered table and repairs both directions. It runs once at boot and is
// the safety net for a crash between hook and commit. One broken table logs
// and does not block the others.
func Reconcile(app core.App) error {
	var firstErr error
	for _, t := range RegisteredGrantTables() {
		if err := reconcileTable(app, t); err != nil {
			log.Warn("reconcile: table finished with errors", "collection", t.Collection, "err", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// groupResource identifies one (group, resource) pair within a grant table —
// i.e. one grant's scope, since a table can hold several grants of the same
// group on different resources.
type groupResource struct {
	group    string
	resource string
}

// reconcileTable repairs one table's derived rows in two phases (insert
// missing, then delete stale). A single bad grant or row must not stop the
// rest of the phase from being repaired, but the function must still report
// that something failed — so each phase remembers its first error and keeps
// going, and the remembered errors (if any) are returned once both phases
// have run to completion.
func reconcileTable(app core.App, t GrantTable) error {
	grants, err := app.FindRecordsByFilter(t.Collection, `user = "" && group != ""`, "", 0, 0, nil)
	if err != nil {
		return fmt.Errorf("list grants: %w", err)
	}
	// expected[group][resource][user] = true
	expected := map[string]map[string]map[string]bool{}
	failed := map[groupResource]bool{}
	var firstErr error
	for _, grant := range grants {
		groupID, resource := grant.GetString("group"), grant.GetString(t.ResourceField)
		members, err := memberUserIDs(app, groupID)
		if err != nil {
			log.Warn("reconcile: listing members failed, leaving this grant's derived rows untouched", "collection", t.Collection, "grant", grant.Id, "group", groupID, "err", err)
			failed[groupResource{groupID, resource}] = true
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if expected[groupID] == nil {
			expected[groupID] = map[string]map[string]bool{}
		}
		if expected[groupID][resource] == nil {
			expected[groupID][resource] = map[string]bool{}
		}
		for _, userID := range members {
			expected[groupID][resource][userID] = true
		}
		// upsertDerived also repairs field drift on rows that exist.
		if err := expandGrant(app, t, grant); err != nil {
			log.Warn("reconcile: repaired grant expansion failed", "collection", t.Collection, "grant", grant.Id, "err", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	derived, err := app.FindRecordsByFilter(t.Collection, `user != "" && group != ""`, "", 0, 0, nil)
	if err != nil {
		if firstErr == nil {
			firstErr = fmt.Errorf("list derived rows: %w", err)
		}
		return firstErr
	}
	for _, row := range staleCandidates(expected, failed, derived, t.ResourceField) {
		log.Warn("reconcile: removing stale derived row", "collection", t.Collection, "row", row.Id)
		if err := app.Delete(row); err != nil {
			log.Warn("reconcile: failed to delete stale row", "collection", t.Collection, "row", row.Id, "err", err)
			if firstErr == nil {
				firstErr = fmt.Errorf("delete stale row %s: %w", row.Id, err)
			}
			continue
		}
	}
	return firstErr
}

// staleCandidates filters derived rows down to the ones reconcile should
// delete: rows whose (group, resource) pair was successfully evaluated (not
// in failed) and whose user is not in that pair's expected set. A pair whose
// membership lookup failed is skipped entirely, so a transient read error
// never causes every derived row of that grant to be treated as stale and
// wiped out.
func staleCandidates(expected map[string]map[string]map[string]bool, failed map[groupResource]bool, rows []*core.Record, resourceField string) []*core.Record {
	var stale []*core.Record
	for _, row := range rows {
		groupID, resource, userID := row.GetString("group"), row.GetString(resourceField), row.GetString("user")
		if failed[groupResource{groupID, resource}] {
			continue
		}
		if expected[groupID][resource][userID] {
			continue
		}
		stale = append(stale, row)
	}
	return stale
}
