/// <reference path="../pb_data/types.d.ts" />
// Avatar customization: how a user's circle renders when they don't want the
// default hashed-color initials.
//
// `avatar` (the file) already existed and was unused. These three carry the
// rest: how the photo is framed, and the two ways to customize without
// uploading anything. The crop is stored rather than baked into the image so
// re-framing never needs a re-upload, and one stored file serves every size.
//
// All three are self-editable only — see selfEditableUserFields in
// coreserver/users_guard.go, which must allow a field or writes 403.
migrate(
    app => {
        const users = app.findCollectionByNameOrId('users')

        users.fields.addAt(
            users.fields.length,
            new Field({
                id: 'users_avatar_crop',
                name: 'avatar_crop',
                type: 'text',
                max: 200,
            })
        )
        users.fields.addAt(
            users.fields.length,
            new Field({
                id: 'users_avatar_color',
                name: 'avatar_color',
                type: 'text',
                max: 20,
            })
        )
        users.fields.addAt(
            users.fields.length,
            new Field({
                id: 'users_avatar_emoji',
                name: 'avatar_emoji',
                type: 'text',
                max: 16,
            })
        )

        app.save(users)
    },
    app => {
        const users = app.findCollectionByNameOrId('users')
        users.fields.removeById('users_avatar_crop')
        users.fields.removeById('users_avatar_color')
        users.fields.removeById('users_avatar_emoji')
        app.save(users)
    }
)
