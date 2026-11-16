/// <reference path="../pb_data/types.d.ts" />
// A report download and a sent report email each write one audit row
// (reports.writeExportAudit). The action select must list both values, or the
// save fails and the export is refused.
const REPORT_ACTIONS = ['report.export', 'report.email']

migrate(
    app => {
        const collection = app.findCollectionByNameOrId('audit_logs')
        const field = collection.fields.getById('al_action')
        field.values = [...field.values.filter(v => !REPORT_ACTIONS.includes(v)), ...REPORT_ACTIONS]
        app.save(collection)
    },
    app => {
        const collection = app.findCollectionByNameOrId('audit_logs')
        const field = collection.fields.getById('al_action')
        field.values = field.values.filter(v => !REPORT_ACTIONS.includes(v))
        app.save(collection)
    }
)
