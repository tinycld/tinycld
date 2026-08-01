/// <reference path="../pb_data/types.d.ts" />
// SECURITY: exclude the 'guest' role from member-scoped access rules.
//
// A guest is an anonymous share-link visitor provisioned a real users record
// with role='guest'. The member-scoped collection rules grant access to any
// authenticated user, so a guest would otherwise leak the member roster +
// emails (users), the audit trail (audit_logs), settings, labels, and the
// package toggles (org_pkg_enabled) — and be able to mutate them.
//
// Single-org deployment: the role now lives on the users auth record, so the
// caller's role is `@request.auth.role`. Each rule below requires the caller to
// be a non-guest authenticated user. A guest may still read their OWN users
// record (needed for auth-refresh) via the `|| id = @request.auth.id` carve-out.
migrate(
    app => {
        const nonGuest = '@request.auth.id != "" && @request.auth.role != "guest"'

        // --- users: non-guest members see everyone; a guest sees only themselves ---
        const usersRule = `(${nonGuest}) || id = @request.auth.id`
        const users = app.findCollectionByNameOrId('users')
        users.listRule = usersRule
        users.viewRule = usersRule
        app.save(users)

        // --- member-scoped collections: caller must be a non-guest member ---
        const labels = app.findCollectionByNameOrId('labels')
        labels.listRule = nonGuest
        labels.viewRule = nonGuest
        labels.createRule = nonGuest
        labels.updateRule = nonGuest
        labels.deleteRule = nonGuest
        app.save(labels)

        const settings = app.findCollectionByNameOrId('settings')
        settings.listRule = nonGuest
        settings.viewRule = nonGuest
        settings.createRule = nonGuest
        settings.updateRule = nonGuest
        app.save(settings)

        const orgPkgEnabled = app.findCollectionByNameOrId('org_pkg_enabled')
        orgPkgEnabled.listRule = nonGuest
        orgPkgEnabled.viewRule = nonGuest
        orgPkgEnabled.createRule = nonGuest
        orgPkgEnabled.updateRule = nonGuest
        orgPkgEnabled.deleteRule = nonGuest
        app.save(orgPkgEnabled)

        const auditLogs = app.findCollectionByNameOrId('audit_logs')
        auditLogs.listRule = nonGuest
        auditLogs.viewRule = nonGuest
        app.save(auditLogs)
    },
    app => {
        // Restore the permissive (any-authed) rules from the create migrations.
        const authed = '@request.auth.id != ""'

        const users = app.findCollectionByNameOrId('users')
        users.listRule = authed
        users.viewRule = authed
        app.save(users)

        for (const name of ['labels', 'settings', 'org_pkg_enabled']) {
            const col = app.findCollectionByNameOrId(name)
            col.listRule = authed
            col.viewRule = authed
            col.createRule = authed
            col.updateRule = authed
            if (name !== 'settings') col.deleteRule = authed
            app.save(col)
        }

        const auditLogs = app.findCollectionByNameOrId('audit_logs')
        auditLogs.listRule = authed
        auditLogs.viewRule = authed
        app.save(auditLogs)
    }
)
