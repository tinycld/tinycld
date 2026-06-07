/// <reference path="../pb_data/types.d.ts" />
// Records, per build, the migration files OWNED by that build's package (the
// install delta intersected with the generator's migration→package map). A
// per-package version revert reads this to revert exactly one package's
// migrations by name, independent of the global `migration_files` count used by
// whole-image build revert.
migrate(
    app => {
        const collection = app.findCollectionByNameOrId('pkg_build')
        collection.fields.add(
            new Field({
                id: 'pb_pkg_migration_files',
                name: 'pkg_migration_files',
                type: 'json',
                maxSize: 100000,
            })
        )
        app.save(collection)
    },
    app => {
        const collection = app.findCollectionByNameOrId('pkg_build')
        const field = collection.fields.getById('pb_pkg_migration_files')
        if (field) {
            collection.fields.removeById('pb_pkg_migration_files')
            app.save(collection)
        }
    }
)
