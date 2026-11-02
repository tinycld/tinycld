/// <reference path="../pb_data/types.d.ts" />
// Which record a run was for, so a run can be found from its record (a web
// form submission's delivery status) and the rule run again for it (retry).
migrate(
    app => {
        const runs = app.findCollectionByNameOrId('rule_runs')
        runs.fields.add(new TextField({ id: 'rr_record_id', name: 'record_id', max: 64 }))
        runs.fields.add(new TextField({ id: 'rr_collection', name: 'collection', max: 100 }))
        runs.indexes.push('CREATE INDEX `idx_rule_runs_record` ON `rule_runs` (`record_id`)')
        app.save(runs)
    },
    app => {
        const runs = app.findCollectionByNameOrId('rule_runs')
        runs.indexes = runs.indexes.filter(i => !i.includes('idx_rule_runs_record'))
        runs.fields.removeById('rr_record_id')
        runs.fields.removeById('rr_collection')
        app.save(runs)
    }
)
