/// <reference path="../pb_data/types.d.ts" />
// Grant super-admin app users (rows in super_admins) access to the collections
// the admin console manages, alongside PB superusers.
//
// Background: the in-shell Admin panel (app/a/[orgSlug]/admin) is gated by
// useSuperAdminStatus — it admits super-admin app users, NOT just PB superusers.
// The collections it manages shipped with rules scoped to org membership (or
// null = superuser-only), so a super-admin operating the panel got 403s the
// moment they touched another org's data or a superuser-only collection.
//
// Fix: OR a super_admins clause into the relevant rules. In PocketBase a null
// rule is superuser-only and a non-null rule is evaluated for regular users while
// superusers always bypass it, so `<existing> || @collection.super_admins.user
// ?= @request.auth.id` authorizes exactly {PB superusers} ∪ {existing grantees} ∪
// {super-admins}. The @collection.* lookup queries the table directly, bypassing
// super_admins' own self-only read rule.
//
// We do this PER COLLECTION (only the few tables the console needs) rather than a
// global superuser-elevation middleware — a super-admin is granted exactly the
// access the panel requires, nothing more. Impersonation is the one console
// action with no rule equivalent (it mints a token for another user) and stays a
// server endpoint (org_admin.go).
const SA = '@collection.super_admins.user ?= @request.auth.id'

// PocketBase's JS migration binding hands rule fields back as a *string wrapper
// object (typeof === 'object'), not a JS primitive — and an empty/null rule is a
// truthy object that stringifies to "". Normalize to a primitive first so the
// null/""/non-empty branches below are reliable. A null rule comes through as the
// string "null" or an object stringifying to ""; we treat any blank as "no rule".
function ruleStr(rule) {
    if (rule === null || rule === undefined) return null
    const s = String(rule)
    return s === '' || s === 'null' ? null : s
}

// orWith appends the super_admins clause to an existing rule.
//   - null/blank rule (superuser-only)  → just the clause (grants super-admins)
//   - non-empty rule                    → OR-extended with the clause
// Note: a genuinely public ("") rule never reaches here — those collections (e.g.
// users.createRule) already let everyone through, so they're not in GRANTS.
function orWith(rule) {
    const s = ruleStr(rule)
    return s ? `(${s}) || ${SA}` : SA
}

// stripSA reverses orWith: returns the original rule (or null) given the combined
// value, so the down-migration restores the exact prior rule.
function stripSA(rule) {
    const s = ruleStr(rule)
    if (s === null || s === SA) return null
    const suffix = ` || ${SA}`
    if (s.endsWith(suffix)) {
        const inner = s.slice(0, -suffix.length)
        return inner.startsWith('(') && inner.endsWith(')') ? inner.slice(1, -1) : inner
    }
    return s
}

// Per-collection rule fields to grant. Each entry lists the operations the
// console performs on that collection.
const GRANTS = [
    // package console
    { name: 'pkg_registry', ops: ['create', 'update', 'delete'] },
    { name: 'pkg_build', ops: ['list', 'view', 'create', 'update', 'delete'] },
    // organizations console — list/edit/create orgs and their owner user + link
    { name: 'orgs', ops: ['list', 'view', 'create', 'update'] },
    { name: 'user_org', ops: ['list', 'view', 'create'] },
    // users.createRule is already public (""), so super-admins can already create
    // owner accounts. They need view/update to see + edit owners, and `manage`
    // so they can set the managed auth fields (verified/email/password) when
    // creating a pre-verified org owner or resetting an owner's credentials.
    { name: 'users', ops: ['view', 'update', 'manage'] },
    // mail_domains only exists when the mail package is installed
    { name: 'mail_domains', ops: ['create'], optional: true },
]

const FIELD = {
    list: 'listRule',
    view: 'viewRule',
    create: 'createRule',
    update: 'updateRule',
    delete: 'deleteRule',
    // Auth collections only. Gates setting managed fields (verified, email,
    // password) on a record via the API.
    manage: 'manageRule',
}

function applyGrants(app, transform) {
    for (const grant of GRANTS) {
        let col
        try {
            col = app.findCollectionByNameOrId(grant.name)
        } catch (e) {
            if (grant.optional) continue
            throw e
        }
        for (const op of grant.ops) {
            const field = FIELD[op]
            col[field] = transform(col[field])
        }
        app.save(col)
    }
}

migrate(
    app => applyGrants(app, orWith),
    app => applyGrants(app, stripSA)
)
