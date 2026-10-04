/// <reference path="../pb_data/types.d.ts" />
// The write-ahead journal is gone. Its rows were raw Yjs updates against a
// document incarnation the server rebuilt on every room open, so a replay
// never applied and the rows only accumulated. The broker now parks the
// document in memory and stores its full state in realtime_doc_checkpoints
// at lifecycle events instead (see 2070000000).
//
// Rows left in the collection belong to incarnations that no longer exist,
// so nothing is migrated out of it.
migrate(
    app => {
        try {
            const c = app.findCollectionByNameOrId('realtime_doc_updates')
            app.delete(c)
        } catch (e) {
            // already gone
        }
    },
    app => {
        const collection = new Collection({
            id: 'pbc_realtime_doc_updates_01',
            name: 'realtime_doc_updates',
            type: 'base',
            system: false,
            listRule: null,
            viewRule: null,
            createRule: null,
            updateRule: null,
            deleteRule: null,
            fields: [
                { id: 'rdu_room_kind', name: 'room_kind', type: 'text', required: true, max: 64 },
                { id: 'rdu_room_id', name: 'room_id', type: 'text', required: true, max: 64 },
                { id: 'rdu_seq', name: 'seq', type: 'number', required: true, min: 1, onlyInt: true },
                { id: 'rdu_update', name: 'update', type: 'text', required: true, max: 358400 },
                { id: 'rdu_created', name: 'created', type: 'autodate', onCreate: true, onUpdate: false },
            ],
            indexes: [
                'CREATE UNIQUE INDEX `idx_realtime_doc_updates_room_seq` ON `realtime_doc_updates` (`room_kind`, `room_id`, `seq`)',
                'CREATE INDEX `idx_realtime_doc_updates_room` ON `realtime_doc_updates` (`room_kind`, `room_id`)',
            ],
        })
        app.save(collection)
    }
)
