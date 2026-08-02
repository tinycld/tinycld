/// <reference path="../pb_data/types.d.ts" />
// Core collections required by all package migrations.
// Must run before any package migration (timestamps start at 1712000000).
migrate(
    app => {
        // 1. role on users
        // Single-org deployment: the process IS one org, so membership no longer
        // needs a junction. The role a user holds is a plain field on their auth
        // record; access rules gate on @request.auth.role. Optional here only
        // because this migration predates the backfill; 1940000000 makes it
        // required once every row has a value.
        const users = app.findCollectionByNameOrId('users')
        users.fields.add(
            new Field({
                id: 'users_role',
                name: 'role',
                type: 'select',
                required: false,
                values: ['owner', 'admin', 'member', 'guest'],
                maxSelect: 1,
            })
        )
        app.save(users)

        // 3. labels
        const labels = new Collection({
            id: 'pbc_labels_00001',
            name: 'labels',
            type: 'base',
            system: false,
            listRule: '@request.auth.id != ""',
            viewRule: '@request.auth.id != ""',
            createRule: '@request.auth.id != ""',
            updateRule: '@request.auth.id != ""',
            deleteRule: '@request.auth.id != ""',
            fields: [
                {
                    id: 'labels_name',
                    name: 'name',
                    type: 'text',
                    required: true,
                    min: 1,
                    max: 100,
                },
                {
                    id: 'labels_color',
                    name: 'color',
                    type: 'text',
                    required: true,
                    max: 20,
                },
                {
                    id: 'labels_created',
                    name: 'created',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: false,
                },
                {
                    id: 'labels_updated',
                    name: 'updated',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: true,
                },
                {
                    id: 'labels_user',
                    name: 'user',
                    type: 'relation',
                    collectionId: '_pb_users_auth_',
                    maxSelect: 1,
                },
            ],
            indexes: [],
        })
        app.save(labels)

        // 4. label_assignments
        const labelAssignments = new Collection({
            id: 'pbc_label_asgn_01',
            name: 'label_assignments',
            type: 'base',
            system: false,
            listRule: 'user = @request.auth.id',
            viewRule: 'user = @request.auth.id',
            createRule: 'user = @request.auth.id',
            updateRule: 'user = @request.auth.id',
            deleteRule: 'user = @request.auth.id',
            fields: [
                {
                    id: 'la_label',
                    name: 'label',
                    type: 'relation',
                    required: true,
                    collectionId: 'pbc_labels_00001',
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                {
                    id: 'la_record_id',
                    name: 'record_id',
                    type: 'text',
                    required: true,
                },
                {
                    id: 'la_collection',
                    name: 'collection',
                    type: 'text',
                    required: true,
                },
                {
                    id: 'la_user',
                    name: 'user',
                    type: 'relation',
                    required: true,
                    collectionId: '_pb_users_auth_',
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                {
                    id: 'la_created',
                    name: 'created',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: false,
                },
                {
                    id: 'la_updated',
                    name: 'updated',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: true,
                },
            ],
            indexes: [
                'CREATE INDEX `idx_la_record` ON `label_assignments` (`record_id`, `collection`)',
                'CREATE INDEX `idx_la_label` ON `label_assignments` (`label`)',
            ],
        })
        app.save(labelAssignments)

        // 5. org_pkg_access
        const orgPkgAccess = new Collection({
            id: 'pbc_org_addon_01',
            name: 'org_pkg_access',
            type: 'base',
            system: false,
            listRule: 'user = @request.auth.id',
            viewRule: 'user = @request.auth.id',
            createRule: null,
            updateRule: null,
            deleteRule: null,
            fields: [
                {
                    id: 'oaa_user',
                    name: 'user',
                    type: 'relation',
                    required: true,
                    collectionId: '_pb_users_auth_',
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                {
                    id: 'oaa_addon',
                    name: 'pkg',
                    type: 'text',
                    required: true,
                    max: 100,
                },
                {
                    id: 'oaa_access',
                    name: 'access',
                    type: 'select',
                    required: true,
                    values: ['full', 'readonly', 'none'],
                    maxSelect: 1,
                },
                {
                    id: 'oaa_created',
                    name: 'created',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: false,
                },
                {
                    id: 'oaa_updated',
                    name: 'updated',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: true,
                },
            ],
        })
        app.save(orgPkgAccess)

        // 6. push_subscriptions
        const pushSubs = new Collection({
            id: 'pbc_push_subs_01',
            name: 'push_subscriptions',
            type: 'base',
            system: false,
            listRule: 'user = @request.auth.id',
            viewRule: 'user = @request.auth.id',
            createRule: '@request.auth.id != ""',
            updateRule: 'user = @request.auth.id',
            deleteRule: 'user = @request.auth.id',
            fields: [
                {
                    id: 'ps_user',
                    name: 'user',
                    type: 'relation',
                    required: true,
                    collectionId: '_pb_users_auth_',
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                {
                    id: 'ps_endpoint',
                    name: 'endpoint',
                    type: 'url',
                    required: true,
                },
                {
                    id: 'ps_keys',
                    name: 'keys',
                    type: 'json',
                    required: true,
                },
                {
                    id: 'ps_user_agent',
                    name: 'user_agent',
                    type: 'text',
                    max: 500,
                },
                {
                    id: 'ps_created',
                    name: 'created',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: false,
                },
                {
                    id: 'ps_updated',
                    name: 'updated',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: true,
                },
            ],
            indexes: [
                'CREATE UNIQUE INDEX `idx_push_endpoint` ON `push_subscriptions` (`endpoint`)',
            ],
        })
        app.save(pushSubs)

        // 7. settings
        const settings = new Collection({
            id: 'pbc_settings_01',
            name: 'settings',
            type: 'base',
            system: false,
            listRule: '@request.auth.id != ""',
            viewRule: '@request.auth.id != ""',
            createRule: '@request.auth.id != ""',
            updateRule: '@request.auth.id != ""',
            deleteRule: null,
            fields: [
                {
                    id: 'settings_app',
                    name: 'app',
                    type: 'text',
                    required: true,
                    max: 100,
                },
                {
                    id: 'settings_key',
                    name: 'key',
                    type: 'text',
                    required: true,
                    max: 200,
                },
                {
                    id: 'settings_value',
                    name: 'value',
                    type: 'json',
                },
                {
                    id: 'settings_created',
                    name: 'created',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: false,
                },
                {
                    id: 'settings_updated',
                    name: 'updated',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: true,
                },
            ],
            indexes: [
                'CREATE UNIQUE INDEX `idx_settings_unique` ON `settings` (`app`, `key`)',
            ],
        })
        app.save(settings)

        // 8. user_preferences
        const userPrefs = new Collection({
            id: 'pbc_user_pref_01',
            name: 'user_preferences',
            type: 'base',
            system: false,
            listRule: 'user = @request.auth.id',
            viewRule: 'user = @request.auth.id',
            createRule: '@request.auth.id != ""',
            updateRule: 'user = @request.auth.id',
            deleteRule: 'user = @request.auth.id',
            fields: [
                {
                    id: 'up_app',
                    name: 'app',
                    type: 'text',
                    required: true,
                    max: 100,
                },
                {
                    id: 'up_key',
                    name: 'key',
                    type: 'text',
                    required: true,
                    max: 200,
                },
                {
                    id: 'up_value',
                    name: 'value',
                    type: 'json',
                },
                {
                    id: 'up_user',
                    name: 'user',
                    type: 'relation',
                    required: true,
                    collectionId: '_pb_users_auth_',
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                {
                    id: 'up_created',
                    name: 'created',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: false,
                },
                {
                    id: 'up_updated',
                    name: 'updated',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: true,
                },
            ],
            indexes: [
                'CREATE UNIQUE INDEX `idx_user_pref_unique` ON `user_preferences` (`user`, `app`, `key`)',
            ],
        })
        app.save(userPrefs)
    },
    app => {
        const names = [
            'user_preferences', 'settings', 'push_subscriptions',
            'org_pkg_access', 'label_assignments', 'labels',
        ]
        for (const name of names) {
            try {
                const c = app.findCollectionByNameOrId(name)
                app.delete(c)
            } catch (e) {
                // may not exist
            }
        }
        try {
            const users = app.findCollectionByNameOrId('users')
            users.fields.removeById('users_role')
            app.save(users)
        } catch (e) {
            // role field may not exist
        }
    }
)
