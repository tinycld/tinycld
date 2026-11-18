/// <reference path="../pb_data/types.d.ts" />
//
// Set when a user is deleted (offboarded). The row stays so their work keeps
// its author; lists and pickers hide rows where this is set.
migrate(
    app => {
        const users = app.findCollectionByNameOrId('users')
        users.fields.add(
            new Field({
                id: 'users_deleted',
                name: 'deleted',
                type: 'date',
            })
        )
        app.save(users)
    },
    app => {
        const users = app.findCollectionByNameOrId('users')
        users.fields.removeById('users_deleted')
        app.save(users)
    }
)
