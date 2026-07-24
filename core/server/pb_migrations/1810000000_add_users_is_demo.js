/// <reference path="../pb_data/types.d.ts" />
// Add is_demo flag to users. When true, every outbound effect that would
// normally leave the box (mail send, invite emails, share emails, Expo push)
// is suppressed at the server-side chokepoint, and the rest of the local
// persistence path runs unchanged so the user still sees the message in
// Sent / Notifications etc. Used for App Review and prospect demos.
migrate(
    app => {
        const users = app.findCollectionByNameOrId('users')
        users.fields.addAt(
            users.fields.length,
            new Field({
                id: 'users_is_demo',
                name: 'is_demo',
                type: 'bool',
            })
        )

        // Loosen the updateRule from the previous self-only restriction so any
        // authenticated user can attempt an update of another user. The
        // RegisterUsersFieldGuard hook in coreserver narrows from there: it
        // enforces an allowlist of admin-editable fields (name, avatar,
        // is_demo) and verifies the caller is an admin/owner. Sensitive fields
        // (password, tokenKey, email, emailVisibility, verified) stay owner-only
        // because PB collection rules can't constrain *which* fields a write
        // touches.
        users.updateRule = '@request.auth.id != "" && (id = @request.auth.id || @request.auth.role != "guest")'

        app.save(users)
    },
    app => {
        const users = app.findCollectionByNameOrId('users')
        users.fields.removeById('users_is_demo')
        users.updateRule = '@request.auth.id != "" && id = @request.auth.id'
        app.save(users)
    }
)
