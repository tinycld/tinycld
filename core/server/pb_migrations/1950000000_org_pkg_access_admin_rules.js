/// <reference path="../pb_data/types.d.ts" />
// Let org admins manage per-user package access.
//
// org_pkg_access shipped with createRule/updateRule/deleteRule = null, i.e.
// superuser-only, and list/view scoped to `user = @request.auth.id` (read your
// own row). Nothing ever relaxed them — 1870000000's guest-RLS sweep skipped
// this collection.
//
// But the UI that manages these rows, PackageAccessPanel, renders inside
// Settings > Members, which is gated on useCurrentRole().isAdmin. So an org
// admin opening a member's drawer got:
//
//   - an always-empty list, because the read rules only ever match the
//     caller's own row, never the member they are editing; and
//   - a silent 403 on every toggle, because create/update/delete admit nobody
//     but a superuser. The panel's mutations have no onError, so the switches
//     simply sprang back.
//
// That also made guests unreachable: use-accessible-packages grants a guest
// packages ONLY via an org_pkg_access row, so with no writable path a guest
// could never be given access to anything.
//
// Fix: admins and owners get full CRUD; a member keeps reading their own row
// (that read is what use-pkg-access depends on).
const ADMIN = '@request.auth.role = "owner" || @request.auth.role = "admin"'
const SELF = 'user = @request.auth.id'

const MANAGE = `@request.auth.id != "" && (${ADMIN})`
// A member must still read their own row, or their own package access stops
// resolving; admins additionally read everyone's.
const READ = `@request.auth.id != "" && (${SELF} || (${ADMIN}))`

migrate(
    app => {
        const col = app.findCollectionByNameOrId('org_pkg_access')
        col.listRule = READ
        col.viewRule = READ
        col.createRule = MANAGE
        col.updateRule = MANAGE
        col.deleteRule = MANAGE
        app.save(col)
    },
    app => {
        const col = app.findCollectionByNameOrId('org_pkg_access')
        col.listRule = SELF
        col.viewRule = SELF
        col.createRule = null
        col.updateRule = null
        col.deleteRule = null
        app.save(col)
    }
)
