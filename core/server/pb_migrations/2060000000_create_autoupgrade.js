/// <reference path="../pb_data/types.d.ts" />
// Automatic package upgrades. autoupgrade_state holds what the scheduler must
// remember across restarts: at most one conflict `pause`, and one `blocked` row
// per version set that was rolled back. Rows are written by the server only;
// the owner may set `cleared` to let a blocked set be tried again.
//
// pkg_install_log gains `trigger` and `changes` so a boot after a rollback can
// tell an automatic upgrade from a manual one and know which set it tried —
// the rollback restores the pre-upgrade DB, and the log row is the only record
// that survives it (it is written before the backup is taken).
//
// The setting is on by default: the row is seeded here, so every reader can
// treat a missing row as an error rather than guess a default.
const OWNER =
    '@request.auth.id != "" && @request.auth.disabled != true && @request.auth.role = "owner"'

migrate(
    app => {
        const state = new Collection({
            id: 'pbc_autoupgrade_state',
            name: 'autoupgrade_state',
            type: 'base',
            system: false,
            listRule: OWNER,
            viewRule: OWNER,
            createRule: null,
            updateRule: OWNER,
            deleteRule: null,
            fields: [
                { id: 'aus_kind', name: 'kind', type: 'select', required: true, maxSelect: 1, values: ['pause', 'blocked'] },
                { id: 'aus_fingerprint', name: 'fingerprint', type: 'text', required: true, max: 64 },
                { id: 'aus_target', name: 'target', type: 'json', maxSize: 20000 },
                { id: 'aus_reason', name: 'reason', type: 'text', max: 5000 },
                { id: 'aus_first_seen', name: 'first_seen', type: 'date' },
                { id: 'aus_last_notified', name: 'last_notified', type: 'date' },
                { id: 'aus_cleared', name: 'cleared', type: 'bool' },
                {
                    id: 'aus_install_log',
                    name: 'install_log',
                    type: 'relation',
                    required: false,
                    collectionId: 'pbc_pkg_ilog_01',
                    cascadeDelete: false,
                    maxSelect: 1,
                },
                { id: 'aus_created', name: 'created', type: 'autodate', onCreate: true, onUpdate: false },
                { id: 'aus_updated', name: 'updated', type: 'autodate', onCreate: true, onUpdate: true },
            ],
            indexes: [
                'CREATE INDEX `idx_autoupgrade_state_kind` ON `autoupgrade_state` (`kind`)',
            ],
        })
        app.save(state)

        const log = app.findCollectionByNameOrId('pkg_install_log')
        log.fields.add(
            new Field({ id: 'pil_trigger', name: 'trigger', type: 'select', maxSelect: 1, values: ['manual', 'auto'] })
        )
        log.fields.add(new Field({ id: 'pil_changes', name: 'changes', type: 'json', maxSize: 20000 }))
        app.save(log)

        const settings = app.findCollectionByNameOrId('system_settings')
        const row = new Record(settings)
        row.set('key', 'autoupgrade.enabled')
        row.set('value', 'true')
        row.set('is_secret', false)
        app.save(row)
    },
    app => {
        const settings = app.findCollectionByNameOrId('system_settings')
        const rows = app.findRecordsByFilter(settings, "key ~ 'autoupgrade.%'", '', 0, 0)
        for (const r of rows) {
            app.delete(r)
        }
        const log = app.findCollectionByNameOrId('pkg_install_log')
        log.fields.removeById('pil_trigger')
        log.fields.removeById('pil_changes')
        app.save(log)
        app.delete(app.findCollectionByNameOrId('autoupgrade_state'))
    }
)
