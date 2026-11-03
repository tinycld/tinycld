/// <reference path="../pb_data/types.d.ts" />
// One publication for each web form (docs/superpowers/specs/2026-11-02-web-forms-design.md).
// Only admins read or write it. The public endpoints read it as the server,
// and the server owns `token` (core/server/webforms/publications.go).
const SIGNED_IN = '@request.auth.id != "" && @request.auth.disabled != true'
const ADMIN = `${SIGNED_IN} && (@request.auth.role = "owner" || @request.auth.role = "admin")`

migrate(
    app => {
        const forms = app.findCollectionByNameOrId('forms')
        const col = new Collection({
            id: 'pbc_form_pubs_01',
            name: 'form_publications',
            type: 'base',
            system: false,
            listRule: ADMIN,
            viewRule: ADMIN,
            createRule: ADMIN,
            updateRule: ADMIN,
            deleteRule: ADMIN,
            fields: [
                {
                    id: 'fpub_form',
                    name: 'form',
                    type: 'relation',
                    required: true,
                    collectionId: forms.id,
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                { id: 'fpub_token', name: 'token', type: 'text', required: true, max: 64 },
                { id: 'fpub_enabled', name: 'enabled', type: 'bool' },
                { id: 'fpub_allowed_domains', name: 'allowed_domains', type: 'text', max: 2000 },
                { id: 'fpub_require_pow', name: 'require_pow', type: 'bool' },
                { id: 'fpub_confirmation', name: 'confirmation', type: 'text', max: 4000 },
                {
                    id: 'fpub_retention_days',
                    name: 'retention_days',
                    type: 'number',
                    required: true,
                    min: 1,
                    max: 3650,
                    onlyInt: true,
                },
                {
                    id: 'fpub_max_upload_mb',
                    name: 'max_upload_mb',
                    type: 'number',
                    required: true,
                    min: 1,
                    max: 100,
                    onlyInt: true,
                },
                { id: 'fpub_created', name: 'created', type: 'autodate', onCreate: true, onUpdate: false },
                { id: 'fpub_updated', name: 'updated', type: 'autodate', onCreate: true, onUpdate: true },
            ],
            indexes: [
                'CREATE UNIQUE INDEX `idx_form_publications_form` ON `form_publications` (`form`)',
                'CREATE UNIQUE INDEX `idx_form_publications_token` ON `form_publications` (`token`)',
            ],
        })
        app.save(col)
    },
    app => {
        try {
            app.delete(app.findCollectionByNameOrId('form_publications'))
        } catch (e) {
            // may not exist
        }
    }
)
