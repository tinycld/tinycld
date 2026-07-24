/// <reference path="../pb_data/types.d.ts" />
// The Members settings page expands relations to `users` to show each member's
// name and email, but the default `users` auth-collection view rule is
// self-only (`id = @request.auth.id`), so expand returns an empty user for
// everyone except the current user — rows render blank.
//
// Single-org deployment: every user in this DB is a member of the one org, so
// loosen users.listRule / users.viewRule to any authenticated user. PocketBase
// never exposes `password` / `tokenKey` via the record API, so this only reveals
// the same fields already shown in the members table (name, email, verified,
// avatar).
migrate(
    app => {
        const authedRule = '@request.auth.id != ""'

        const col = app.findCollectionByNameOrId('users')
        col.listRule = authedRule
        col.viewRule = authedRule
        app.save(col)
    },
    app => {
        const selfOnlyRule = 'id = @request.auth.id'

        const col = app.findCollectionByNameOrId('users')
        col.listRule = selfOnlyRule
        col.viewRule = selfOnlyRule
        app.save(col)
    }
)
