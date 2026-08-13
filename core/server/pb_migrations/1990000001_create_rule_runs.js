/// <reference path="../pb_data/types.d.ts" />
migrate(
    app => {
        const runs = new Collection({
            id: 'pbc_rule_runs_01',
            name: 'rule_runs',
            type: 'base',
            system: false,
            // Readable by whoever can read the rule; written only by the
            // engine (superuser DAO) — no client create/update/delete.
            listRule:
                'rule.owner = @request.auth.id || (rule.scope = "org" && (@request.auth.role = "admin" || @request.auth.role = "owner"))',
            viewRule:
                'rule.owner = @request.auth.id || (rule.scope = "org" && (@request.auth.role = "admin" || @request.auth.role = "owner"))',
            createRule: null,
            updateRule: null,
            deleteRule: null,
            fields: [
                {
                    id: 'rr_rule',
                    name: 'rule',
                    type: 'relation',
                    required: true,
                    collectionId: 'pbc_rules_01',
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                {
                    id: 'rr_fired_at',
                    name: 'fired_at',
                    type: 'date',
                    required: true,
                },
                {
                    id: 'rr_matched',
                    name: 'matched',
                    type: 'bool',
                },
                {
                    id: 'rr_trigger_summary',
                    name: 'trigger_summary',
                    type: 'json',
                },
                {
                    id: 'rr_results',
                    name: 'results',
                    type: 'json',
                },
                {
                    id: 'rr_error',
                    name: 'error',
                    type: 'text',
                    max: 2000,
                },
                {
                    id: 'rr_duration_ms',
                    name: 'duration_ms',
                    type: 'number',
                },
                {
                    id: 'rr_created',
                    name: 'created',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: false,
                },
                {
                    id: 'rr_updated',
                    name: 'updated',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: true,
                },
            ],
            indexes: [
                'CREATE INDEX `idx_rule_runs_rule` ON `rule_runs` (`rule`, `fired_at`)',
            ],
        })
        app.save(runs)
    },
    app => {
        try {
            const c = app.findCollectionByNameOrId('rule_runs')
            app.delete(c)
        } catch (e) {
            // may not exist
        }
    }
)
