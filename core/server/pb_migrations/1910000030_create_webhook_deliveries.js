/// <reference path="../pb_data/types.d.ts" />
//
// webhook_deliveries — the replay ledger for inbound webhooks.
//
// A provider retries a failed delivery with the SAME delivery id, which is
// exactly what makes the id a dedupe key: a retry must be accepted as a
// duplicate (200, no work) rather than processed twice. Without this, a
// receiver that times out AFTER writing its rows does its work again on the
// retry.
//
// SERVER-WRITTEN, CLIENT-INVISIBLE. Every rule is nil: no client of any kind
// reads or writes this. A superuser bypasses nil rules, which is the only
// access path, and the receiver runs as one.
//
// Rows are pruned by age, not kept forever — the dedupe window only has to
// outlive a provider's retry schedule (typically hours, not days).
migrate(
    app => {
        const col = new Collection({
            id: 'pbc_webhook_deliveries',
            name: 'webhook_deliveries',
            type: 'base',
            system: false,
            listRule: null,
            viewRule: null,
            createRule: null,
            updateRule: null,
            deleteRule: null,
            fields: [
                {
                    id: 'wd_source',
                    name: 'source',
                    type: 'text',
                    required: true,
                    max: 50,
                },
                {
                    id: 'wd_delivery_id',
                    name: 'delivery_id',
                    type: 'text',
                    required: true,
                    max: 200,
                },
                {
                    id: 'wd_received',
                    name: 'received',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: false,
                },
            ],
            indexes: [
                'CREATE UNIQUE INDEX idx_webhook_deliveries_unique ' +
                    'ON webhook_deliveries (source, delivery_id)',
                'CREATE INDEX idx_webhook_deliveries_received ' +
                    'ON webhook_deliveries (received)',
            ],
        })
        app.save(col)
    },
    app => {
        app.delete(app.findCollectionByNameOrId('webhook_deliveries'))
    }
)
