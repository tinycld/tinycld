package reports

import (
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/groups"
	"tinycld.org/core/offboard"
)

// The saved-report collections (core migration 2090000008_create_reports.js).
const (
	reportsCollection = "reports"
	sharesCollection  = "report_shares"
)

// RegisterSharing binds saved reports and their sharing: report_shares as a
// group grant table, the save and publish guards on reports, the share
// guards on report_shares, shipped reports for the first admin who appears,
// and the offboard handler. Call it before groups.Register, which binds the
// expansion of every grant table registered by then.
//
// The collections' own rules (the migration) decide who may read and write
// a row; these hooks guard what a permitted write may change. A superuser
// request passes them unchanged: a superuser can change any row anyway.
func RegisterSharing(app core.App) {
	groups.RegisterGrantTable(groups.GrantTable{Collection: sharesCollection, ResourceField: "report"})
	app.OnRecordCreateRequest(reportsCollection).BindFunc(onReportCreate)
	app.OnRecordUpdateRequest(reportsCollection).BindFunc(onReportUpdate)
	app.OnRecordDeleteRequest(reportsCollection).BindFunc(onReportDelete)
	app.OnRecordCreateRequest(sharesCollection).BindFunc(onShareCreate)
	app.OnRecordUpdateRequest(sharesCollection).BindFunc(onShareUpdate)
	app.OnRecordDeleteRequest(sharesCollection).BindFunc(onShareDelete)
	app.OnRecordAfterCreateSuccess("users").BindFunc(shipForAdmin)
	app.OnRecordAfterUpdateSuccess("users").BindFunc(shipForAdmin)
	offboard.RegisterHandler("reports", offboardReports)
}
