package reports

import (
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// shareCap bounds the shares one report holds: direct shares and group
// grants. The rows a grant derives, one per member, are bounded by the
// group, which an admin manages. Two shares added at the same moment can
// pass the count together, so the cap can be passed by the number of
// concurrent requests, never without bound.
var shareCap = 200

// A report_shares row is one of three kinds, as groups.GrantTable defines
// them: direct (user set), a group grant (group set), or derived (both set,
// written by the group expander for each member of a granted group).
func isDerivedShare(r *core.Record) bool {
	return r.GetString("user") != "" && r.GetString("group") != ""
}

func onShareCreate(e *core.RecordRequestEvent) error {
	if e.HasSuperuserAuth() {
		return e.Next()
	}
	user, group := e.Record.GetString("user"), e.Record.GetString("group")
	if (user == "") == (group == "") {
		const msg = "A share names one user or one group."
		return e.BadRequestError(msg, fieldError("user", "one_target", msg))
	}
	// The create rule passed, so the report exists and the caller manages it.
	report, err := e.App.FindRecordById(reportsCollection, e.Record.GetString("report"))
	if err != nil {
		return err
	}
	if user == report.GetString("owner") {
		const msg = "The owner already has this report."
		return e.BadRequestError(msg, fieldError("user", "owner", msg))
	}
	n, err := e.App.CountRecords(sharesCollection, dbx.NewExp(
		"[[report]] = {:report} AND ([[user]] = '' OR [[group]] = '')", dbx.Params{"report": report.Id}))
	if err != nil {
		return err
	}
	if n >= int64(shareCap) {
		msg := fmt.Sprintf("A report can be shared at most %d times.", shareCap)
		return e.BadRequestError(msg, fieldError("report", "too_many_shares", msg))
	}
	return e.Next()
}

func onShareUpdate(e *core.RecordRequestEvent) error {
	if e.HasSuperuserAuth() {
		return e.Next()
	}
	original := e.Record.Original()
	if isDerivedShare(original) {
		return refuseDerived(e)
	}
	for _, f := range []string{"report", "user", "group"} {
		if e.Record.GetString(f) != original.GetString(f) {
			const msg = "A share changes only its role. Remove it and add another."
			return e.BadRequestError(msg, fieldError(f, "read_only", msg))
		}
	}
	return e.Next()
}

func onShareDelete(e *core.RecordRequestEvent) error {
	if !e.HasSuperuserAuth() && isDerivedShare(e.Record) {
		return refuseDerived(e)
	}
	return e.Next()
}

// refuseDerived: a member's row follows its group's grant. Editing or
// removing it alone would be undone at the next boot reconcile.
func refuseDerived(e *core.RecordRequestEvent) error {
	const msg = "This access comes from a group. Change the group's share instead."
	return e.BadRequestError(msg, fieldError("group", "follows_grant", msg))
}
