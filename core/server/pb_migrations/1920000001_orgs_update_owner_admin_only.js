/// <reference path="../pb_data/types.d.ts" />
// SECURITY: restrict org rename / re-slug to owner + admin.
//
// 1870000000 set orgs.updateRule to admit ANY non-guest member — a plain
// `member` could PATCH the org's name/slug/logo. The slug drives every
// `/a/<slug>/…` URL, so a member could break every deep link (and every other
// member's bookmarks/OAuth redirects) by re-slugging the org.
//
// Roles are exactly ['owner','admin','member','guest'], so "owner or admin" is
// expressed as "non-guest AND non-member". The two `?!=` clauses share the same
// `user_org_via_org` relation-path prefix as the `user ?= @request.auth.id`
// pin, so PocketBase applies all three to the SAME joined user_org row — the
// caller's own membership — exactly as the guest exclusion in 1870000000 does
// (verified there against the real rule engine). This matches the client gate
// `canManageOrg = role === 'owner' || role === 'admin'` in use-current-role.ts.
migrate(
    app => {
        const orgsAdminWriteRule =
            '@request.auth.id != "" && ' +
            'user_org_via_org.user ?= @request.auth.id && ' +
            'user_org_via_org.role ?!= "guest" && ' +
            'user_org_via_org.role ?!= "member"'

        const orgs = app.findCollectionByNameOrId('orgs')
        orgs.updateRule = orgsAdminWriteRule
        app.save(orgs)
    },
    app => {
        // Restore the EXACT prior rule set by 1870000000 (any non-guest member).
        const orgsPrior =
            '@request.auth.id != "" && (' +
            'user_org_via_org.user ?= @request.auth.id && user_org_via_org.role ?!= "guest")'

        const orgs = app.findCollectionByNameOrId('orgs')
        orgs.updateRule = orgsPrior
        app.save(orgs)
    }
)
