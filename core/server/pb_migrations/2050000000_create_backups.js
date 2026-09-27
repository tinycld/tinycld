/// <reference path="../pb_data/types.d.ts" />
// The backup ledger. Every backup or restore run inserts its row before doing
// any work and always ends in a terminal status, so "last backed up" and the
// history list never need to guess. Server-only writes: clients read.
const ADMIN =
    '@request.auth.id != "" && @request.auth.disabled != true && ' +
    '(@request.auth.role = "owner" || @request.auth.role = "admin")'

migrate(
    app => {
        const col = new Collection({
            id: 'pbc_backups',
            name: 'backups',
            type: 'base',
            system: false,
            listRule: ADMIN,
            viewRule: ADMIN,
            createRule: null,
            updateRule: null,
            deleteRule: null,
            fields: [
                { id: 'bk_kind', name: 'kind', type: 'select', required: true, maxSelect: 1,
                  values: ['manual', 'scheduled', 'pre_restore', 'restore'] },
                { id: 'bk_status', name: 'status', type: 'select', required: true, maxSelect: 1,
                  values: ['running', 'waiting_for_source', 'succeeded', 'failed', 'interrupted'] },
                { id: 'bk_initiated_by', name: 'initiated_by', type: 'relation', required: false,
                  collectionId: '_pb_users_auth_', cascadeDelete: false, maxSelect: 1 },
                { id: 'bk_started', name: 'started', type: 'date', required: true },
                { id: 'bk_finished', name: 'finished', type: 'date' },
                { id: 'bk_bytes', name: 'bytes', type: 'number', min: 0 },
                { id: 'bk_sha256', name: 'sha256', type: 'text', max: 64 },
                { id: 'bk_manifest', name: 'manifest', type: 'json', maxSize: 200000 },
                { id: 'bk_target_host', name: 'target_host', type: 'text', max: 253 },
                { id: 'bk_error', name: 'error', type: 'text', max: 2000 },
                { id: 'bk_metadata', name: 'metadata', type: 'json', maxSize: 20000 },
                { id: 'bk_created', name: 'created', type: 'autodate', onCreate: true, onUpdate: false },
                { id: 'bk_updated', name: 'updated', type: 'autodate', onCreate: true, onUpdate: true },
            ],
            indexes: ['CREATE INDEX `idx_backups_started` ON `backups` (`started`)'],
        })
        app.save(col)
    },
    app => {
        try {
            app.delete(app.findCollectionByNameOrId('backups'))
        } catch (e) {
            // may not exist
        }
    }
)
