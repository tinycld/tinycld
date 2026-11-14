/// <reference path="../pb_data/types.d.ts" />
// AI classification: per-app taxonomies, one result row per (record, user),
// and a monthly spend ledger. Results and the ledger are written only by the
// server (classifier package), never through the REST API.
const SIGNED_IN = '@request.auth.id != "" && @request.auth.disabled != true'
const MEMBER = `${SIGNED_IN} && @request.auth.role != "guest"`
const ADMIN = `${SIGNED_IN} && (@request.auth.role = "owner" || @request.auth.role = "admin")`

migrate(
    app => {
        const users = app.findCollectionByNameOrId('users')
        const labels = app.findCollectionByNameOrId('labels')

        const taxonomy = new Collection({
            id: 'pbc_classifier_labels',
            name: 'classifier_labels',
            type: 'base',
            system: false,
            listRule: `${MEMBER} && (user = "" || user = @request.auth.id)`,
            viewRule: `${MEMBER} && (user = "" || user = @request.auth.id)`,
            createRule: `(${ADMIN} && @request.body.user = "") || (${MEMBER} && @request.body.user = @request.auth.id)`,
            updateRule: `((${ADMIN} && user = "") || (${MEMBER} && user = @request.auth.id)) && (@request.body.user:isset = false || @request.body.user = user)`,
            deleteRule: `(${ADMIN} && user = "") || (${MEMBER} && user = @request.auth.id)`,
            fields: [
                { id: 'cll_app', name: 'app', type: 'text', required: true, max: 64 },
                // The taxonomy owns the option name the model sees. Taking it from
                // the core label would let any member rename it for everyone.
                { id: 'cll_name', name: 'name', type: 'text', required: true, max: 60 },
                {
                    id: 'cll_label',
                    name: 'label',
                    type: 'relation',
                    required: true,
                    collectionId: labels.id,
                    // Not cascade: any member can delete a core label, which would
                    // silently remove an org option. A referenced label is undeletable.
                    cascadeDelete: false,
                    maxSelect: 1,
                },
                { id: 'cll_assign_when', name: 'assign_when', type: 'text', required: true, max: 2000 },
                { id: 'cll_auto_action', name: 'auto_action', type: 'bool' },
                { id: 'cll_order', name: 'order', type: 'number' },
                {
                    id: 'cll_user',
                    name: 'user',
                    type: 'relation',
                    collectionId: users.id,
                    cascadeDelete: true,
                    maxSelect: 1,
                },
            ],
            indexes: [
                'CREATE INDEX `idx_classifier_labels_scope` ON `classifier_labels` (`app`, `user`, `order`)',
                'CREATE UNIQUE INDEX `idx_classifier_labels_label` ON `classifier_labels` (`app`, `user`, `label`)',
                'CREATE UNIQUE INDEX `idx_classifier_labels_name` ON `classifier_labels` (`app`, `user`, `name`)',
            ],
        })
        app.save(taxonomy)

        const results = new Collection({
            id: 'pbc_classifications',
            name: 'classifications',
            type: 'base',
            system: false,
            listRule: `${SIGNED_IN} && user = @request.auth.id`,
            viewRule: `${SIGNED_IN} && user = @request.auth.id`,
            createRule: null,
            updateRule: null,
            deleteRule: null,
            fields: [
                { id: 'cls_app', name: 'app', type: 'text', required: true, max: 64 },
                { id: 'cls_collection', name: 'collection', type: 'text', required: true, max: 100 },
                { id: 'cls_record_id', name: 'record_id', type: 'text', required: true, max: 64 },
                {
                    id: 'cls_user',
                    name: 'user',
                    type: 'relation',
                    required: true,
                    collectionId: users.id,
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                {
                    id: 'cls_classifier_label',
                    name: 'classifier_label',
                    type: 'relation',
                    collectionId: taxonomy.id,
                    cascadeDelete: false,
                    maxSelect: 1,
                },
                { id: 'cls_label_name', name: 'label_name', type: 'text', max: 200 },
                { id: 'cls_confidence', name: 'confidence', type: 'number' },
                { id: 'cls_probabilities', name: 'probabilities', type: 'json', maxSize: 20000 },
                { id: 'cls_stage', name: 'stage', type: 'select', maxSelect: 1, values: ['metadata', 'full'] },
                { id: 'cls_model', name: 'model', type: 'text', max: 200 },
                { id: 'cls_cost_usd', name: 'cost_usd', type: 'number' },
                { id: 'cls_preview', name: 'preview', type: 'bool' },
                { id: 'cls_action', name: 'action', type: 'text', max: 64 },
                { id: 'cls_error', name: 'error', type: 'text', max: 64 },
                { id: 'cls_created', name: 'created', type: 'autodate', onCreate: true, onUpdate: false },
            ],
            indexes: [
                'CREATE UNIQUE INDEX `idx_classifications_record_user` ON `classifications` (`collection`, `record_id`, `user`)',
                'CREATE INDEX `idx_classifications_user_created` ON `classifications` (`user`, `app`, `created`)',
            ],
        })
        app.save(results)

        const usage = new Collection({
            id: 'pbc_classifier_usage',
            name: 'classifier_usage',
            type: 'base',
            system: false,
            listRule: ADMIN,
            viewRule: ADMIN,
            createRule: null,
            updateRule: null,
            deleteRule: null,
            fields: [
                { id: 'clu_month', name: 'month', type: 'text', required: true, max: 7 },
                { id: 'clu_cost_usd', name: 'cost_usd', type: 'number' },
                { id: 'clu_calls', name: 'calls', type: 'number' },
            ],
            indexes: ['CREATE UNIQUE INDEX `idx_classifier_usage_month` ON `classifier_usage` (`month`)'],
        })
        app.save(usage)
    },
    app => {
        for (const name of ['classifier_usage', 'classifications', 'classifier_labels']) {
            app.delete(app.findCollectionByNameOrId(name))
        }
    }
)
