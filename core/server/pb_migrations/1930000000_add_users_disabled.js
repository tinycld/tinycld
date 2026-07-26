/// <reference path="../pb_data/types.d.ts" />
// Add a reversible `disabled` flag to users — the single-org replacement for
// "leave the organization" (meaningless when the deployment IS the org).
//
// A disabled account is suspended, not destroyed: the row, its authored
// content and every FK stay intact, and an owner/admin can re-enable it.
// Irreversible teardown remains userorg.AnonymizeUser via /api/account/delete.
//
// Enforcement lives in Go, deliberately:
//   - PocketBase collection rules cannot constrain WHICH fields a write
//     touches (see the note on users.updateRule in 1810000000_add_users_is_demo),
//     so `disabled` is kept out of selfEditableUserFields by the field guard in
//     coreserver/users_guard.go. Otherwise a disabled user could clear their
//     own flag through a plain record update.
//   - There is no PB `authRule`, so refusing a disabled user's sign-in is a
//     hook (coreserver/disabled_guard.go), not schema.
//
// users.listRule/viewRule are intentionally NOT narrowed here: an admin has to
// be able to see a disabled user in order to re-enable them. Callers that want
// to hide them filter client-side.
migrate(
    app => {
        const users = app.findCollectionByNameOrId('users')
        users.fields.addAt(
            users.fields.length,
            new Field({
                id: 'users_disabled',
                name: 'disabled',
                type: 'bool',
            })
        )
        app.save(users)
    },
    app => {
        const users = app.findCollectionByNameOrId('users')
        users.fields.removeById('users_disabled')
        app.save(users)
    }
)
