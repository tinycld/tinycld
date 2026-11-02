/// <reference path="../pb_data/types.d.ts" />
// Web form submissions and their files. Admins list, view and delete; no one
// creates or updates through the REST API, because only the public submit
// endpoint (core/server/webforms/submit.go) may write, after its spam and
// size checks.
const SIGNED_IN = '@request.auth.id != "" && @request.auth.disabled != true'
const ADMIN = `${SIGNED_IN} && (@request.auth.role = "owner" || @request.auth.role = "admin")`

migrate(
    app => {
        const forms = app.findCollectionByNameOrId('forms')
        const pubs = app.findCollectionByNameOrId('form_publications')
        const users = app.findCollectionByNameOrId('users')
        const subs = new Collection({
            id: 'pbc_form_subs_01',
            name: 'form_submissions',
            type: 'base',
            system: false,
            listRule: ADMIN,
            viewRule: ADMIN,
            createRule: null,
            updateRule: null,
            deleteRule: ADMIN,
            fields: [
                // No cascade: the forms hooks refuse to delete a form that records use.
                {
                    id: 'fsub_form',
                    name: 'form',
                    type: 'relation',
                    required: true,
                    collectionId: forms.id,
                    cascadeDelete: false,
                    maxSelect: 1,
                },
                {
                    id: 'fsub_publication',
                    name: 'publication',
                    type: 'relation',
                    required: true,
                    collectionId: pubs.id,
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                { id: 'fsub_values', name: 'values', type: 'json', maxSize: 1000000 },
                { id: 'fsub_client_id', name: 'client_id', type: 'text', required: true, max: 64 },
                { id: 'fsub_ip_hash', name: 'ip_hash', type: 'text', max: 64 },
                { id: 'fsub_user_agent', name: 'user_agent', type: 'text', max: 300 },
                { id: 'fsub_created', name: 'created', type: 'autodate', onCreate: true, onUpdate: false },
            ],
            indexes: [
                'CREATE UNIQUE INDEX `idx_form_submissions_client` ON `form_submissions` (`publication`, `client_id`)',
                'CREATE INDEX `idx_form_submissions_form_created` ON `form_submissions` (`form`, `created`)',
            ],
        })
        app.save(subs)

        const files = new Collection({
            id: 'pbc_form_subfiles_01',
            name: 'form_submission_files',
            type: 'base',
            system: false,
            listRule: ADMIN,
            viewRule: ADMIN,
            createRule: null,
            updateRule: null,
            deleteRule: ADMIN,
            fields: [
                {
                    id: 'fsf_submission',
                    name: 'submission',
                    type: 'relation',
                    required: true,
                    collectionId: subs.id,
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                { id: 'fsf_field', name: 'field', type: 'text', required: true, max: 40 },
                {
                    id: 'fsf_file',
                    name: 'file',
                    type: 'file',
                    required: true,
                    maxSelect: 1,
                    maxSize: 104857600,
                    protected: true,
                },
                { id: 'fsf_size', name: 'size', type: 'number', onlyInt: true },
                { id: 'fsf_name', name: 'name', type: 'text', max: 255 },
                // The files contract names an uploader column; a public upload leaves it empty.
                {
                    id: 'fsf_uploaded_by',
                    name: 'uploaded_by',
                    type: 'relation',
                    collectionId: users.id,
                    cascadeDelete: false,
                    maxSelect: 1,
                },
                { id: 'fsf_created', name: 'created', type: 'autodate', onCreate: true, onUpdate: false },
            ],
            indexes: [
                'CREATE INDEX `idx_form_submission_files_sub` ON `form_submission_files` (`submission`)',
            ],
        })
        app.save(files)
    },
    app => {
        for (const name of ['form_submission_files', 'form_submissions']) {
            try {
                app.delete(app.findCollectionByNameOrId(name))
            } catch (e) {
                // may not exist
            }
        }
    }
)
