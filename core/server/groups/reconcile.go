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
			log.Warn("reconcile: table skipped", "collection", t.Collection, "err", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func reconcileTable(app core.App, t GrantTable) error {
	grants, err := app.FindRecordsByFilter(t.Collection, `user = "" && group != ""`, "", 0, 0, nil)
	if err != nil {
		return fmt.Errorf("list grants: %w", err)
	}
	// expected[group][resource][user] = true
	expected := map[string]map[string]map[string]bool{}
	for _, grant := range grants {
		groupID, resource := grant.GetString("group"), grant.GetString(t.ResourceField)
		members, err := memberUserIDs(app, groupID)
		if err != nil {
			return err
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
		}
	}

	derived, err := app.FindRecordsByFilter(t.Collection, `user != "" && group != ""`, "", 0, 0, nil)
	if err != nil {
		return fmt.Errorf("list derived rows: %w", err)
	}
	for _, row := range derived {
		groupID, resource, userID := row.GetString("group"), row.GetString(t.ResourceField), row.GetString("user")
		if expected[groupID][resource][userID] {
			continue
		}
		log.Warn("reconcile: removing stale derived row", "collection", t.Collection, "row", row.Id)
		if err := app.Delete(row); err != nil {
			return fmt.Errorf("delete stale row %s: %w", row.Id, err)
		}
	}
	return nil
}
