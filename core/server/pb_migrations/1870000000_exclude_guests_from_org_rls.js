/// <reference path="../pb_data/types.d.ts" />
// SECURITY: exclude the 'guest' role from org-membership access rules.
//
// A guest is an anonymous share-link visitor provisioned a real users record +
// a user_org row with role='guest' in the OWNER's org. ~7 core collection
// rules granted access to ANY org member regardless of role, so a guest
// membership row would leak the member roster + emails (users, user_org),
// the audit trail (audit_logs), org settings (settings), labels, and the
// per-org package toggles (org_pkg_enabled) — and let a guest mutate them.
//
// Each rule below requires the CALLER's OWN membership row to be non-guest.
// The role pin (`role ?!= "guest"`) shares the exact same relation-path prefix
// as the user pin (`user ?= @request.auth.id`), so PocketBase applies both
// conditions to the SAME joined user_org row (verified against the real rule
// engine in coreserver/guest_rls_test.go — totalItems assertions for a seeded
// guest vs. member). It does NOT mean "some non-guest row exists in the org."
//
// Narrow guest-allow: a guest legitimately needs to read the org row they're a
// guest in (for the editor to show the org name) and their own user_org row.
// Those allowances are folded into the orgs list/view and user_org list/view
// rules. Everything else (roster, emails, audit, settings, labels, pkg
// toggles, org UPDATE) is denied to guests.
//
// The down-migration restores the EXACT prior rule strings set by
// 1795000000 / 1800000001 / 1800000002.
migrate(
    app => {
        // --- users: list/view (was 1800000002) ---
        const usersRule =
            '@request.auth.id != "" && ' +
            'user_org_via_user.org.user_org_via_org.user ?= @request.auth.id && ' +
            'user_org_via_user.org.user_org_via_org.role ?!= "guest"'
        const users = app.findCollectionByNameOrId('users')
        users.listRule = usersRule
        users.viewRule = usersRule
        app.save(users)

        // --- user_org: list/view (was 1800000001) ---
        // Non-guest members see the full roster; a guest sees ONLY their own row.
        const userOrgRule =
            '@request.auth.id != "" && (' +
            '(org.user_org_via_org.user ?= @request.auth.id && org.user_org_via_org.role ?!= "guest")' +
            ' || user = @request.auth.id)'
        const userOrg = app.findCollectionByNameOrId('user_org')
        userOrg.listRule = userOrgRule
        userOrg.viewRule = userOrgRule
        app.save(userOrg)

        // --- orgs: list/view + update (was 1795000000) ---
        // Read: a guest may VIEW the org(s) they hold a membership in (narrow
        // allow). Update: guests excluded entirely.
        const orgsReadRule =
            '@request.auth.id != "" && (' +
            '(user_org_via_org.user ?= @request.auth.id && user_org_via_org.role ?!= "guest")' +
            ' || user_org_via_org.user ?= @request.auth.id)'
        const orgsWriteRule =
            '@request.auth.id != "" && (' +
            'user_org_via_org.user ?= @request.auth.id && user_org_via_org.role ?!= "guest")'
        const orgs = app.findCollectionByNameOrId('orgs')
        orgs.listRule = orgsReadRule
        orgs.viewRule = orgsReadRule
        orgs.updateRule = orgsWriteRule
        app.save(orgs)

        // --- org-scoped collections: caller must be a non-guest member ---
        const orgScopedRule =
            'org.user_org_via_org.user ?= @request.auth.id && ' +
            'org.user_org_via_org.role ?!= "guest"'

        const labels = app.findCollectionByNameOrId('labels')
        labels.listRule = orgScopedRule
        labels.viewRule = orgScopedRule
        labels.createRule = orgScopedRule
        labels.updateRule = orgScopedRule
        labels.deleteRule = orgScopedRule
        app.save(labels)

        const settings = app.findCollectionByNameOrId('settings')
        settings.listRule = orgScopedRule
        settings.viewRule = orgScopedRule
        settings.createRule = orgScopedRule
        settings.updateRule = orgScopedRule
        app.save(settings)

        const orgPkgEnabled = app.findCollectionByNameOrId('org_pkg_enabled')
        orgPkgEnabled.listRule = orgScopedRule
        orgPkgEnabled.viewRule = orgScopedRule
        orgPkgEnabled.createRule = orgScopedRule
        orgPkgEnabled.updateRule = orgScopedRule
        orgPkgEnabled.deleteRule = orgScopedRule
        app.save(orgPkgEnabled)

        // --- audit_logs: list/view ---
        const auditRule =
            '@request.auth.id != "" && (' +
            'org.user_org_via_org.user ?= @request.auth.id && ' +
            'org.user_org_via_org.role ?!= "guest")'
        const auditLogs = app.findCollectionByNameOrId('audit_logs')
        auditLogs.listRule = auditRule
        auditLogs.viewRule = auditRule
        app.save(auditLogs)
    },
    app => {
        // Restore EXACT prior rule strings.

        // users (1800000002)
        const usersPrior =
            '@request.auth.id != "" && user_org_via_user.org.user_org_via_org.user ?= @request.auth.id'
        const users = app.findCollectionByNameOrId('users')
        users.listRule = usersPrior
        users.viewRule = usersPrior
        app.save(users)

        // user_org (1800000001)
        const userOrgPrior =
            '@request.auth.id != "" && org.user_org_via_org.user ?= @request.auth.id'
        const userOrg = app.findCollectionByNameOrId('user_org')
        userOrg.listRule = userOrgPrior
        userOrg.viewRule = userOrgPrior
        app.save(userOrg)

        // orgs (1795000000)
        const orgSelfMemberRule = 'user_org_via_org.user ?= @request.auth.id'
        const orgsPrior = `@request.auth.id != "" && ${orgSelfMemberRule}`
        const orgs = app.findCollectionByNameOrId('orgs')
        orgs.listRule = orgsPrior
        orgs.viewRule = orgsPrior
        orgs.updateRule = orgsPrior
        app.save(orgs)

        // org-scoped (1795000000)
        const orgMemberRule = 'org.user_org_via_org.user ?= @request.auth.id'

        const labels = app.findCollectionByNameOrId('labels')
        labels.listRule = orgMemberRule
        labels.viewRule = orgMemberRule
        labels.createRule = orgMemberRule
        labels.updateRule = orgMemberRule
        labels.deleteRule = orgMemberRule
        app.save(labels)

        const settings = app.findCollectionByNameOrId('settings')
        settings.listRule = orgMemberRule
        settings.viewRule = orgMemberRule
        settings.createRule = orgMemberRule
        settings.updateRule = orgMemberRule
        app.save(settings)

        const orgPkgEnabled = app.findCollectionByNameOrId('org_pkg_enabled')
        orgPkgEnabled.listRule = orgMemberRule
        orgPkgEnabled.viewRule = orgMemberRule
        orgPkgEnabled.createRule = orgMemberRule
        orgPkgEnabled.updateRule = orgMemberRule
        orgPkgEnabled.deleteRule = orgMemberRule
        app.save(orgPkgEnabled)

        // audit_logs (1795000000)
        const auditPrior = `@request.auth.id != "" && ${orgMemberRule}`
        const auditLogs = app.findCollectionByNameOrId('audit_logs')
        auditLogs.listRule = auditPrior
        auditLogs.viewRule = auditPrior
        app.save(auditLogs)
    }
)
