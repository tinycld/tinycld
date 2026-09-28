/// <reference path="../pb_data/types.d.ts" />
// The audit log screen now pages the newest entries first — `ORDER BY created
// DESC LIMIT n` with no actor or resource predicate in the default view. The
// collection's existing indexes are `(actor, created DESC)` and
// `(resource_type, resource_id)`, neither of which a leading-column-free sort
// can use, so that default page was a full scan plus a sort of the whole table
// on a deployment with any history.
//
// A new file rather than an edit to 1780000000_create_audit_logs.js: that
// migration has shipped, and PocketBase never re-runs an applied one, so an
// in-place edit would silently never reach an existing database.
migrate(
    app => {
        const collection = app.findCollectionByNameOrId('audit_logs')
        collection.indexes = [
            ...collection.indexes,
            'CREATE INDEX `idx_audit_logs_created` ON `audit_logs` (`created` DESC)',
        ]
        app.save(collection)
    },
    app => {
        const collection = app.findCollectionByNameOrId('audit_logs')
        collection.indexes = collection.indexes.filter(i => !i.includes('idx_audit_logs_created'))
        app.save(collection)
    }
)
