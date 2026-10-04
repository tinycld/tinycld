/// <reference path="../pb_data/types.d.ts" />
// realtime_doc_checkpoints holds one row per collaborative-document room:
// the full Yjs state the broker last wrote, the epoch that names that
// document incarnation, and the fingerprint of the derived source (docx,
// xlsx, card rows) the state corresponds to.
//
// A room reopened from its row is the SAME Yjs document, with the same item
// identities, so a client that still holds it merges as a no-op and resends
// only what the server lacks. A room rebuilt from the derived file is a new
// incarnation, and clients must discard theirs; that is what the journal in
// realtime_doc_updates could never prevent, because its rows referenced the
// incarnation that was gone.
//
// Written only at lifecycle events (janitor eviction of a parked document,
// read-only enter, drain, terminate), never per edit.
//
// Schema notes:
//   - (room_kind, room_id) is unique; Save is an upsert.
//   - state is base64 of the encoded document; the cap is 4/3 of the
//     broker's MaxCheckpointBytes.
//   - admin-only by design: the broker reads and writes through the Go SDK.
migrate(
    app => {
        const collection = new Collection({
            id: 'pbc_realtime_doc_ckpt_01',
            name: 'realtime_doc_checkpoints',
            type: 'base',
            system: false,
            listRule: null,
            viewRule: null,
            createRule: null,
            updateRule: null,
            deleteRule: null,
            fields: [
                {
                    id: 'rdc_room_kind',
                    name: 'room_kind',
                    type: 'text',
                    required: true,
                    max: 64,
                },
                {
                    id: 'rdc_room_id',
                    name: 'room_id',
                    type: 'text',
                    required: true,
                    max: 64,
                },
                {
                    id: 'rdc_epoch',
                    name: 'epoch',
                    type: 'number',
                    required: true,
                    min: 1,
                    onlyInt: true,
                },
                {
                    id: 'rdc_fingerprint',
                    name: 'fingerprint',
                    type: 'text',
                    max: 512,
                },
                {
                    id: 'rdc_state',
                    name: 'state',
                    type: 'text',
                    required: true,
                    max: 40000000,
                },
                {
                    id: 'rdc_created',
                    name: 'created',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: false,
                },
                {
                    id: 'rdc_updated',
                    name: 'updated',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: true,
                },
            ],
            indexes: [
                'CREATE UNIQUE INDEX `idx_realtime_doc_checkpoints_room` ON `realtime_doc_checkpoints` (`room_kind`, `room_id`)',
            ],
        })
        app.save(collection)
    },
    app => {
        try {
            const c = app.findCollectionByNameOrId('realtime_doc_checkpoints')
            app.delete(c)
        } catch (e) {
            // may not exist
        }
    }
)
