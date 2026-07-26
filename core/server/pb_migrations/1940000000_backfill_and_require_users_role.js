/// <reference path="../pb_data/types.d.ts" />
// Backfill `users.role` and make it required.
//
// 1700000000_create_core_collections added `role` as OPTIONAL, reasoning that
// "an operator/super-admin may have no role". In practice that produced rows
// with an empty role — createSuperAdminOperator (coreserver/setup_bootstrap.go)
// sets email/password/verified/name/username but never `role`, so the bootstrap
// operator lands roleless. Every consumer then has to defend against a value the
// type system says is one of four strings, and one didn't: the Members screen
// grouped by `groups[m.role]`, which is `undefined` for '' → the whole screen
// crashed to the error boundary.
//
// Backfill value is 'member', chosen to be BEHAVIOUR-PRESERVING rather than
// merely plausible. The rules that read @request.auth.role come in two shapes:
//
//   role != "guest"                    (drive items, mail infra, calendar,
//                                       org RLS, users.updateRule)
//   role = "admin" || role = "owner"   (mail domains, mailbox aliases)
//
// An empty role PASSES the first family and FAILS the second. 'member' does
// exactly the same — and no rule keys on "member" specifically — so backfilling
// grants and revokes nothing. 'guest' would have REVOKED access the operator has
// today (it's the one value the first family excludes); 'owner'/'admin' would
// have GRANTED privileged access. Only 'member' is a no-op.
//
// The operator's actual authority does not come from this field at all — it
// comes from its `super_admins` row, which is unaffected.
migrate(
    app => {
        // Backfill BEFORE requiring, or saving the collection would reject the
        // existing empty rows.
        app.db()
            .newQuery("UPDATE users SET role = 'member' WHERE role IS NULL OR role = ''")
            .execute()

        const users = app.findCollectionByNameOrId('users')
        const role = users.fields.getById('users_role')
        role.required = true
        app.save(users)
    },
    app => {
        const users = app.findCollectionByNameOrId('users')
        const role = users.fields.getById('users_role')
        role.required = false
        app.save(users)
        // The backfilled values are intentionally NOT reverted: which rows were
        // empty beforehand isn't recorded, so restoring '' would have to guess.
        // Leaving them as 'member' is the behaviour-preserving direction (see
        // above), and re-running the up migration is then a no-op.
    }
)
