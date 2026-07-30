/// <reference path="../pb_data/types.d.ts" />
// Restrict audit_logs reads to owners/admins.
//
// 1870000000 narrowed the trail from any-authed to non-guest, but the audit
// log records member emails, role changes, and offboards — administrative
// material. The UI already shows the screen only to admins
// (app/(app)/settings/audit-log.tsx gates on isAdmin); until this migration
// the RULE still let any member read the full trail over REST, so the screen
// hid what the API served. Rules are the authorization; the UI gate is
// convenience.
//
// The disabled clause closes the token-expiry window for this collection: a
// suspended admin's still-live JWT must not keep reading the audit trail.
migrate(
    app => {
        const adminOnly =
            '@request.auth.id != "" && @request.auth.disabled != true && ' +
            '(@request.auth.role = "owner" || @request.auth.role = "admin")'
        const auditLogs = app.findCollectionByNameOrId('audit_logs')
        auditLogs.listRule = adminOnly
        auditLogs.viewRule = adminOnly
        app.save(auditLogs)
    },
    app => {
        // Restore 1870000000's non-guest rules.
        const nonGuest = '@request.auth.id != "" && @request.auth.role != "guest"'
        const auditLogs = app.findCollectionByNameOrId('audit_logs')
        auditLogs.listRule = nonGuest
        auditLogs.viewRule = nonGuest
        app.save(auditLogs)
    }
)
