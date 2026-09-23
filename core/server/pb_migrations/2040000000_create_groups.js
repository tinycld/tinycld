/// <reference path="../pb_data/types.d.ts" />
//
// Admin-managed user groups. A package shares a resource with a group by
// writing a grant row (group set, user empty) into its own membership table;
// core/server/groups expands it into one derived row per member. These two
// collections are the only thing the rest of the product needs to know.
//
// Ids are fixed and referenced by package migrations (`collectionId:
// 'pbc_groups_01'`), so they never change.
migrate(
    app => {
        const enabled = '@request.auth.disabled != true'
        const authed = '@request.auth.id != ""'
        const notGuest = '@request.auth.role != "guest"'
        const reader = `${authed} && ${enabled} && ${notGuest}`
        const admin =
            `${authed} && ${enabled} && ` +
            '(@request.auth.role = "admin" || @request.auth.role = "owner")'

        const groups = new Collection({
            id: 'pbc_groups_01',
            name: 'groups',
            type: 'base',
            system: false,
            listRule: reader,
            viewRule: reader,
            createRule: admin,
            updateRule: admin,
            deleteRule: admin,
            fields: [
                { id: 'groups_name', name: 'name', type: 'text', required: true, min: 1, max: 100 },
                { id: 'groups_description', name: 'description', type: 'text', required: false, max: 500 },
                { id: 'groups_created', name: 'created', type: 'autodate', onCreate: true, onUpdate: false },
                { id: 'groups_updated', name: 'updated', type: 'autodate', onCreate: true, onUpdate: true },
            ],
            indexes: ['CREATE UNIQUE INDEX `idx_groups_name` ON `groups` (`name`)'],
        })
        app.save(groups)

        // No update rule: a membership is added or removed, never edited.
        // Guests and disabled users are refused at the rule, so the picker's
        // exclusion is a convenience and the rule is the backstop.
        const members = new Collection({
            id: 'pbc_group_members_01',
            name: 'group_members',
            type: 'base',
            system: false,
            listRule: reader,
            viewRule: reader,
            createRule: `${admin} && user.role != "guest" && user.disabled != true`,
            updateRule: null,
            deleteRule: admin,
            fields: [
                {
                    id: 'group_members_group',
                    name: 'group',
                    type: 'relation',
                    required: true,
                    collectionId: 'pbc_groups_01',
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                {
                    id: 'group_members_user',
                    name: 'user',
                    type: 'relation',
                    required: true,
                    collectionId: '_pb_users_auth_',
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                { id: 'group_members_created', name: 'created', type: 'autodate', onCreate: true, onUpdate: false },
                { id: 'group_members_updated', name: 'updated', type: 'autodate', onCreate: true, onUpdate: true },
            ],
            indexes: [
                'CREATE UNIQUE INDEX `idx_group_members_unique` ON `group_members` (`group`, `user`)',
                'CREATE INDEX `idx_group_members_user` ON `group_members` (`user`)',
            ],
        })
        app.save(members)
    },
    app => {
        for (const name of ['group_members', 'groups']) {
            try {
                app.delete(app.findCollectionByNameOrId(name))
            } catch (e) {
                // may not exist
            }
        }
    }
)
