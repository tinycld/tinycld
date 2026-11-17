/// <reference path="../pb_data/types.d.ts" />
// org_pkg_access is read by user and package in every package's access rule,
// once per rule evaluation (a report statement, each realtime access check).
// With no index each read scans the table.
// Not unique: nothing has kept one row per (user, pkg). A new migration
// because 1700000000_create_core_collections.js is released.
migrate(
    app => {
        const col = app.findCollectionByNameOrId('org_pkg_access')
        col.indexes = [
            ...col.indexes,
            'CREATE INDEX `idx_org_pkg_access_user_pkg` ON `org_pkg_access` (`user`, `pkg`)',
        ]
        app.save(col)
    },
    app => {
        const col = app.findCollectionByNameOrId('org_pkg_access')
        col.indexes = col.indexes.filter(i => !i.includes('idx_org_pkg_access_user_pkg'))
        app.save(col)
    }
)
