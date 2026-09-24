package groups

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// isGrant reports a client-written group grant: group set, user empty.
func isGrant(r *core.Record) bool {
	return r.GetString("user") == "" && r.GetString("group") != ""
}

// isDerived reports a server-owned expansion of a grant: both set.
func isDerived(r *core.Record) bool {
	return r.GetString("user") != "" && r.GetString("group") != ""
}

// copyGrantFields makes dst a clone of grant apart from identity and the user
// slot. Every other field (role, created_by, a per-member color, …) copies
// through, which is why the registry needs no per-package field list.
func copyGrantFields(dst, grant *core.Record) {
	for _, f := range grant.Collection().Fields {
		name := f.GetName()
		if name == "id" || name == "user" || f.Type() == core.FieldTypeAutodate {
			continue
		}
		dst.Set(name, grant.Get(name))
	}
}

func memberUserIDs(app core.App, groupID string) ([]string, error) {
	rows, err := app.FindRecordsByFilter("group_members", "group = {:g}", "", 0, 0, map[string]any{"g": groupID})
	if err != nil {
		return nil, fmt.Errorf("groups: list members of %s: %w", groupID, err)
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.GetString("user"))
	}
	return ids, nil
}

func findDerived(app core.App, t GrantTable, grant *core.Record, userID string) (*core.Record, error) {
	rec, err := app.FindFirstRecordByFilter(
		t.Collection,
		fmt.Sprintf("group = {:g} && user = {:u} && %s = {:r}", t.ResourceField),
		map[string]any{"g": grant.GetString("group"), "u": userID, "r": grant.GetString(t.ResourceField)},
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("groups: find derived %s row for user %s: %w", t.Collection, userID, err)
	}
	return rec, nil
}

// upsertDerived ensures one derived row for (grant, user) that mirrors the
// grant's fields.
func upsertDerived(app core.App, t GrantTable, grant *core.Record, userID string) error {
	existing, err := findDerived(app, t, grant, userID)
	if err != nil {
		return err
	}
	if existing == nil {
		existing = core.NewRecord(grant.Collection())
	}
	copyGrantFields(existing, grant)
	existing.Set("user", userID)
	if err := app.Save(existing); err != nil {
		return fmt.Errorf("groups: derive %s row for user %s: %w", t.Collection, userID, err)
	}
	return nil
}

// expandGrant inserts a derived row for every current member of the grant's
// group.
func expandGrant(app core.App, t GrantTable, grant *core.Record) error {
	ids, err := memberUserIDs(app, grant.GetString("group"))
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := upsertDerived(app, t, grant, id); err != nil {
			return err
		}
	}
	return nil
}

// syncDerived re-copies the grant's fields onto its derived rows after the
// grant changed (typically a role change).
func syncDerived(app core.App, t GrantTable, grant *core.Record) error {
	return expandGrant(app, t, grant)
}

func derivedForGrant(app core.App, t GrantTable, grant *core.Record) ([]*core.Record, error) {
	return app.FindRecordsByFilter(
		t.Collection,
		fmt.Sprintf(`group = {:g} && user != "" && %s = {:r}`, t.ResourceField),
		"", 0, 0,
		map[string]any{"g": grant.GetString("group"), "r": grant.GetString(t.ResourceField)},
	)
}

// removeDerivedForGrant deletes every derived row of one grant. Rows already
// gone (a cascade from a group delete) are not an error.
func removeDerivedForGrant(app core.App, t GrantTable, grant *core.Record) error {
	rows, err := derivedForGrant(app, t, grant)
	if err != nil {
		return fmt.Errorf("groups: list derived rows of %s: %w", t.Collection, err)
	}
	for _, r := range rows {
		if err := app.Delete(r); err != nil {
			return fmt.Errorf("groups: delete derived %s row: %w", t.Collection, err)
		}
	}
	return nil
}

// deriveForMember inserts, in every registered table, a derived row for each
// grant of the group the user just joined.
func deriveForMember(app core.App, groupID, userID string) error {
	for _, t := range RegisteredGrantTables() {
		grants, err := app.FindRecordsByFilter(t.Collection, `group = {:g} && user = ""`, "", 0, 0, map[string]any{"g": groupID})
		if err != nil {
			return fmt.Errorf("groups: list grants in %s: %w", t.Collection, err)
		}
		for _, grant := range grants {
			if err := upsertDerived(app, t, grant, userID); err != nil {
				return err
			}
		}
	}
	return nil
}

// removeDerivedForMember deletes, in every registered table, the derived rows
// of (group, user). A row the user holds through another group stays.
func removeDerivedForMember(app core.App, groupID, userID string) error {
	for _, t := range RegisteredGrantTables() {
		rows, err := app.FindRecordsByFilter(t.Collection, "group = {:g} && user = {:u}", "", 0, 0, map[string]any{"g": groupID, "u": userID})
		if err != nil {
			return fmt.Errorf("groups: list derived rows in %s: %w", t.Collection, err)
		}
		for _, r := range rows {
			if err := app.Delete(r); err != nil {
				return fmt.Errorf("groups: delete derived %s row: %w", t.Collection, err)
			}
		}
	}
	return nil
}

// RemoveUserMemberships deletes every group_members row of a user. Each delete
// runs the membership hooks, so derived rows go with it. Called when a user
// becomes a guest and when a user is offboarded.
//
// Tolerates a missing group_members collection: offboard runs against any
// core-backed app, including narrow test fixtures that never applied the
// groups migration, and a boot-order composition where offboard runs before
// groups has registered its collections.
func RemoveUserMemberships(app core.App, userID string) error {
	if _, err := app.FindCollectionByNameOrId("group_members"); err != nil {
		return nil
	}
	rows, err := app.FindRecordsByFilter("group_members", "user = {:u}", "", 0, 0, map[string]any{"u": userID})
	if err != nil {
		return fmt.Errorf("groups: list memberships of %s: %w", userID, err)
	}
	for _, r := range rows {
		if err := app.Delete(r); err != nil {
			return fmt.Errorf("groups: delete membership %s: %w", r.Id, err)
		}
	}
	return nil
}
