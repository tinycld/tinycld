/// <reference path="../pb_data/types.d.ts" />
// Backups and restores are audited as events, not as collection writes, so
// they need their own action values beside created/updated/deleted.
migrate(
    app => {
        const collection = app.findCollectionByNameOrId('audit_logs')
        const field = collection.fields.getById('al_action')
        field.values = [
            'created', 'updated', 'deleted',
            'backup.created', 'backup.failed',
            'restore.started', 'restore.succeeded', 'restore.failed',
        ]
        app.save(collection)
    },
    app => {
        const collection = app.findCollectionByNameOrId('audit_logs')
        const field = collection.fields.getById('al_action')
        field.values = ['created', 'updated', 'deleted']
        app.save(collection)
    }
)
