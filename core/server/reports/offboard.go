package reports

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/offboard"
)

// offboardReports settles a leaver's saved reports inside the offboard
// transaction. It is a Handler, not a reassignable ref, because a flat FK
// rewrite cannot keep a published report while deleting the rest.
//
//   - ModeReassign: every report the leaver owns goes to the successor.
//   - ModeDeleteMyData: an unpublished report is deleted with its shares; a
//     published or shipped one goes to the heir (reportHeir).
//   - ModeKeep: an unpublished report stays with the anonymized account, as
//     other kept content does; a published or shipped one goes to the heir.
//
// A shipped report is never deleted: the next start would make it again,
// published. It keeps its published flag.
//
// A report that changes owner loses the new owner's own direct share on it,
// which ownership now covers. In every mode the leaver's direct shares on
// other people's reports are removed: nobody signs in as the anonymized
// account. Their group-derived rows are gone already, because OffboardUser
// removes the leaver's group memberships before the handlers run.
func offboardReports(txApp core.App, leaver *core.Record, plan offboard.Plan, actorUserID string) error {
	if _, err := txApp.FindCollectionByNameOrId(reportsCollection); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil // an app without core's report migration owns no reports
		}
		return fmt.Errorf("find the reports collection: %w", err)
	}
	owned, err := txApp.FindRecordsByFilter(reportsCollection, "owner = {:u}", "created,id", 0, 0, dbx.Params{"u": leaver.Id})
	if err != nil {
		return fmt.Errorf("list reports of %s: %w", leaver.Id, err)
	}
	heir := ""
	for _, r := range owned {
		move := plan.Mode == offboard.ModeReassign || r.GetBool("published") || r.GetString("shipped_key") != ""
		switch {
		case move:
			if heir == "" {
				if heir, err = reportHeir(txApp, leaver.Id, plan, actorUserID); err != nil {
					return err
				}
			}
			if err := moveReport(txApp, r, heir); err != nil {
				return err
			}
		case plan.Mode == offboard.ModeDeleteMyData:
			if err := txApp.Delete(r); err != nil {
				return fmt.Errorf("delete report %s: %w", r.Id, err)
			}
		}
	}
	return deleteDirectShares(txApp, dbx.Params{"u": leaver.Id}, `user = {:u} && group = ""`)
}

// moveReport gives a report to heir and removes heir's direct share on it.
func moveReport(txApp core.App, report *core.Record, heir string) error {
	report.Set("owner", heir)
	if err := txApp.Save(report); err != nil {
		return fmt.Errorf("reassign report %s: %w", report.Id, err)
	}
	return deleteDirectShares(txApp, dbx.Params{"r": report.Id, "u": heir}, `report = {:r} && user = {:u} && group = ""`)
}

func deleteDirectShares(txApp core.App, params dbx.Params, filter string) error {
	rows, err := txApp.FindRecordsByFilter(sharesCollection, filter, "", 0, 0, params)
	if err != nil {
		return fmt.Errorf("list report shares: %w", err)
	}
	for _, row := range rows {
		if err := txApp.Delete(row); err != nil {
			return fmt.Errorf("delete report share %s: %w", row.Id, err)
		}
	}
	return nil
}

// reportHeir is who takes the leaver's reports: the successor of a
// reassign plan; otherwise the actor, when an admin other than the leaver
// started the offboard; otherwise the first admin. A published or shipped
// report is never deleted, so with no heir the offboard is refused.
func reportHeir(txApp core.App, leaverID string, plan offboard.Plan, actorUserID string) (string, error) {
	if plan.Mode == offboard.ModeReassign {
		return plan.SuccessorUserID, nil // OffboardUser checked it
	}
	if actorUserID != "" && actorUserID != leaverID {
		actor, err := txApp.FindRecordById("users", actorUserID)
		switch {
		case err == nil && isActiveAdmin(actor):
			return actor.Id, nil
		case err != nil && !errors.Is(err, sql.ErrNoRows):
			return "", fmt.Errorf("find the acting user %s: %w", actorUserID, err)
		}
	}
	admin, err := firstAdmin(txApp, leaverID)
	if err != nil {
		return "", err
	}
	if admin == nil {
		return "", fmt.Errorf("%w: this user owns published reports and no other admin can take them; make another user an admin first", offboard.ErrInvalidPlan)
	}
	return admin.Id, nil
}
