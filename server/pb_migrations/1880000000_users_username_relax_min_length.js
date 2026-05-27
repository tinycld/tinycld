/// <reference path="../pb_data/types.d.ts" />
// Relax the username field constraint from a 3-char minimum to a 1-char
// minimum so single-character email local-parts (e.g. "a@x.com") can round
// trip into a username. The character set and starts-with-alphanumeric rule
// are unchanged; only the length floor moves from 3 to 1.
//
// Keep this in sync with coreserver/usernames.go::IsValidUsername (regex
// `^[a-z0-9][a-z0-9_-]{0,31}$`), the parity tests in
// users_username_migration_test.go, and the TS validator
// (core/lib/derive-username.ts + MembersDrawer's invite schema).

migrate(
    app => {
        const users = app.findCollectionByNameOrId('users')
        const usernameField = users.fields.getByName('username')
        if (!usernameField) return
        usernameField.pattern = '^[a-z0-9][a-z0-9_-]{0,31}$'
        usernameField.min = 1
        app.save(users)
    },
    app => {
        const users = app.findCollectionByNameOrId('users')
        const usernameField = users.fields.getByName('username')
        if (!usernameField) return
        usernameField.pattern = '^[a-z0-9][a-z0-9_-]{2,31}$'
        usernameField.min = 3
        app.save(users)
    }
)
