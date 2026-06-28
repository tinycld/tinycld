/// <reference path="../pb_data/types.d.ts" />
// Grant super-admins LIST access to `users`.
//
// 1910000007_pkg_admin_super_admin_rules granted super-admins view/update/manage
// on `users` but NOT list — so a super-admin could fetch a specific user by id,
// but a LIST query (filtered or not) silently returned only rows the org-scoped
// rule already allowed (their own). The /admin Organizations tab resolves each
// org's owner with a LIST/join over the local `users` store, so cross-org owners
// came back unreadable and rendered as "No owner assigned" even though the
// user_org owner row exists. Listing users is consistent with the super-admin's
// existing cross-org reach (it already lists orgs + user_org).
//
// This appends the same super_admins clause to users.listRule that viewRule
// already carries. Idempotent: skip if the clause is already present.
const SA = '@collection.super_admins.user ?= @request.auth.id'

function ruleStr(rule) {
    if (rule === null || rule === undefined) return null
    const s = String(rule)
    return s === '' || s === 'null' ? null : s
}

migrate(
    app => {
        const users = app.findCollectionByNameOrId('users')
        const current = ruleStr(users.listRule)
        // null (superuser-only) → just the clause; otherwise OR it in. Skip when
        // the clause is already present so a re-run is a no-op.
        if (current === null) {
            users.listRule = SA
        } else if (!current.includes(SA)) {
            users.listRule = `(${current}) || ${SA}`
        }
        app.save(users)
    },
    app => {
        const users = app.findCollectionByNameOrId('users')
        const s = ruleStr(users.listRule)
        if (s === null) return
        if (s === SA) {
            users.listRule = null
        } else {
            const suffix = ` || ${SA}`
            if (s.endsWith(suffix)) {
                const inner = s.slice(0, -suffix.length)
                users.listRule =
                    inner.startsWith('(') && inner.endsWith(')') ? inner.slice(1, -1) : inner
            }
        }
        app.save(users)
    }
)
