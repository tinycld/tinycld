/// <reference path="../pb_data/types.d.ts" />
// super_admins: a denormalized junction granting a regular app user cross-org
// admin powers (the /admin console — packages, orgs, versions, builds).
// Membership in this table == super admin. It is deliberately NOT a flag on
// `users` so the grant is auditable (created_by) and easy to revoke.
//
// RLS: writes are superuser-only (null rules) so a regular user can never mint
// themselves admin from the client. Reads are self-only — a caller sees ONLY
// their own row (`user ?= @request.auth.id`), exactly what useIsSuperAdmin()
// needs to decide whether to show the Admin rail entry. The grant/revoke UI
// lists the full roster through the superuser/admin-guarded server endpoint,
// which runs in the app's Go context and bypasses these record rules — so the
// read rule never needs to expose other users' rows.
migrate(
    app => {
        const superAdmins = new Collection({
            id: 'pbc_super_admins',
            name: 'super_admins',
            type: 'base',
            system: false,
            listRule: '@request.auth.id != "" && user ?= @request.auth.id',
            viewRule: '@request.auth.id != "" && user ?= @request.auth.id',
            createRule: null,
            updateRule: null,
            deleteRule: null,
            fields: [
                {
                    id: 'sa_user',
                    name: 'user',
                    type: 'relation',
                    required: true,
                    collectionId: '_pb_users_auth_',
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                {
                    id: 'sa_created_by',
                    name: 'created_by',
                    type: 'relation',
                    collectionId: '_pb_users_auth_',
                    cascadeDelete: false,
                    maxSelect: 1,
                },
                {
                    id: 'sa_created',
                    name: 'created',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: false,
                },
                {
                    id: 'sa_updated',
                    name: 'updated',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: true,
                },
            ],
            indexes: [
                'CREATE UNIQUE INDEX `idx_super_admins_user` ON `super_admins` (`user`)',
            ],
        })
        app.save(superAdmins)
    },
    app => {
        try {
            const c = app.findCollectionByNameOrId('super_admins')
            app.delete(c)
        } catch (e) {
            // may not exist
        }
    }
)
