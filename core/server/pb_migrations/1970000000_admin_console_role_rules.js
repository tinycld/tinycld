/// <reference path="../pb_data/types.d.ts" />
// Grant the /admin console's collections by ORG ROLE, replacing the deleted
// super_admins junction.
//
// Background: platform authority used to live in a separate `super_admins`
// table, and 1910000005/07/11 created it and OR-ed a
// `@collection.super_admins.user ?= @request.auth.id` clause into every
// collection the console touches. That made "super admin" orthogonal to
// `users.role` — being an owner did not grant the console, and a super admin
// might hold no org role at all. Those three migrations are gone; role is now
// the single privilege axis.
//
// The split this restores:
//   - ADMIN (owner or admin) reaches the console and its read surfaces.
//   - OWNER alone may WRITE pkg_registry/pkg_build — installing, uninstalling
//     or version-changing a package rebuilds the artifact the whole
//     deployment runs, so it is not an ordinary admin action. Mirrors the
//     Go-side requireOwner/requireAdmin split in coreserver/pkg_install.go;
//     the endpoints are the real enforcement, these rules are the RLS half.
//
// The `disabled != true` clause matches 1960000000_audit_logs_admin_only: a
// suspended admin's still-live JWT must not keep reaching the console before
// its token expires.
const ADMIN =
    '@request.auth.id != "" && @request.auth.disabled != true && ' +
    '(@request.auth.role = "owner" || @request.auth.role = "admin")'
const OWNER =
    '@request.auth.id != "" && @request.auth.disabled != true && ' +
    '@request.auth.role = "owner"'

// PocketBase's JS migration binding hands rule fields back as a *string wrapper
// object (typeof === 'object'), not a JS primitive — and an empty/null rule is a
// truthy object that stringifies to "". Normalize before inspecting.
function ruleStr(rule) {
    if (rule === null || rule === undefined) return null
    const s = String(rule)
    return s === '' || s === 'null' ? null : s
}

// users.listRule/viewRule already carry the non-guest + self clause from
// 1870000000. The console additionally needs admins to read every user (the
// Members and Organizations screens list them) and to write the managed auth
// fields (`verified`, `email`, `password`) when creating a pre-verified user —
// that last one is `manageRule`, which is auth-collection-only.
//
// OR the admin clause in rather than replacing: a member must keep reading the
// roster, and every user must keep reading themselves.
function orAdmin(rule) {
    const s = ruleStr(rule)
    if (s === null) return ADMIN
    return s.includes(ADMIN) ? s : `(${s}) || ${ADMIN}`
}

function stripAdmin(rule) {
    const s = ruleStr(rule)
    if (s === null || s === ADMIN) return null
    const suffix = ` || ${ADMIN}`
    if (s.endsWith(suffix)) {
        const inner = s.slice(0, -suffix.length)
        return inner.startsWith('(') && inner.endsWith(')') ? inner.slice(1, -1) : inner
    }
    return s
}

migrate(
    app => {
        // pkg_registry shipped list/view = any authed user (the app reads the
        // installed set to build its nav) and null writes (superuser-only).
        // Keep the reads, hand writes to the owner.
        const registry = app.findCollectionByNameOrId('pkg_registry')
        registry.createRule = OWNER
        registry.updateRule = OWNER
        registry.deleteRule = OWNER
        app.save(registry)

        // pkg_build shipped fully null (superuser-only). Build history is a
        // console read for any admin; mutating it (deleting a build) is the
        // owner's, same as the package operations that produce it.
        const build = app.findCollectionByNameOrId('pkg_build')
        build.listRule = ADMIN
        build.viewRule = ADMIN
        build.createRule = OWNER
        build.updateRule = OWNER
        build.deleteRule = OWNER
        app.save(build)

        const users = app.findCollectionByNameOrId('users')
        users.listRule = orAdmin(users.listRule)
        users.viewRule = orAdmin(users.viewRule)
        users.updateRule = orAdmin(users.updateRule)
        users.manageRule = orAdmin(users.manageRule)
        app.save(users)
    },
    app => {
        const registry = app.findCollectionByNameOrId('pkg_registry')
        registry.createRule = null
        registry.updateRule = null
        registry.deleteRule = null
        app.save(registry)

        const build = app.findCollectionByNameOrId('pkg_build')
        build.listRule = null
        build.viewRule = null
        build.createRule = null
        build.updateRule = null
        build.deleteRule = null
        app.save(build)

        const users = app.findCollectionByNameOrId('users')
        users.listRule = stripAdmin(users.listRule)
        users.viewRule = stripAdmin(users.viewRule)
        users.updateRule = stripAdmin(users.updateRule)
        users.manageRule = stripAdmin(users.manageRule)
        app.save(users)
    }
)
