/// <reference path="../pb_data/types.d.ts" />
// org_provisioning: a core-owned "intent" signal emitted when an admin creates an
// org and supplies optional feature inputs that core itself doesn't own.
//
// Today its only payload is mail_domain: the admin org-create form collects a
// mail domain, but core must NOT write mail's mail_domains collection (that would
// couple core to the mail package and break the lean-shell guarantee). Instead
// core inserts an org_provisioning row, and the mail package's Go server binds
// OnRecordAfterCreateSuccess("org_provisioning") to create its own mail_domains
// row — mirroring how mail already reacts to user_org creation. When mail isn't
// installed the row simply sits unconsumed.
//
// createRule grants super-admins (PB superusers bypass non-null rules anyway), so
// the admin console can emit the signal through the pbtsdb store like every other
// org-create write. Reads/updates/deletes stay superuser-only — it's a one-shot
// internal signal, not user-facing data.
migrate(
    app => {
        const col = new Collection({
            id: 'pbc_org_provision_1',
            name: 'org_provisioning',
            type: 'base',
            system: false,
            listRule: null,
            viewRule: null,
            createRule: '@collection.super_admins.user ?= @request.auth.id',
            updateRule: null,
            deleteRule: null,
            fields: [
                {
                    id: 'op_org',
                    name: 'org',
                    type: 'relation',
                    required: true,
                    collectionId: 'pbc_orgs_00001',
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                {
                    id: 'op_mail_domain',
                    name: 'mail_domain',
                    type: 'text',
                    max: 253,
                },
                {
                    id: 'op_created',
                    name: 'created',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: false,
                },
                {
                    id: 'op_updated',
                    name: 'updated',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: true,
                },
            ],
        })
        app.save(col)
    },
    app => {
        try {
            const c = app.findCollectionByNameOrId('org_provisioning')
            app.delete(c)
        } catch (e) {
            // may not exist
        }
    }
)
