/// <reference path="../pb_data/types.d.ts" />
// Make REST creates on `users` superuser-only.
//
// `users` is PocketBase's built-in auth collection and kept its default
// createRule of "" (public). No earlier migration or hook changed it, and the
// `role` field accepts any value from the request body. So an anonymous
// `POST /api/collections/users/records {"role":"owner"}` minted a working
// owner account, which then passed every role-based rule and Go guard.
//
// Every legitimate sign-up path saves from Go, where API rules do not apply:
// invite accept, setup bootstrap, the create-owner CLI, demo start, guest OTP
// (core guestauth and drive share OTP). Seed scripts and e2e fixtures create
// users with a superuser token. No client creates a user over REST.
//
// This is a new migration and not an edit of an old one, because the rule
// comes from PocketBase's init migration and `users` ships in released core.
migrate(
    app => {
        const users = app.findCollectionByNameOrId('users')
        users.createRule = null
        app.save(users)
    },
    app => {
        const users = app.findCollectionByNameOrId('users')
        users.createRule = ''
        app.save(users)
    }
)
