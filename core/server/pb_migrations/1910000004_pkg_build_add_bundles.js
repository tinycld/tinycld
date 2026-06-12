/// <reference path="../pb_data/types.d.ts" />
// Stores per-platform native OTA bundle metadata (platform, bundle_id, bundle_hash,
// bundle_file, runtime_version, assets[]) for the /api/app/update endpoint.
migrate(
    app => {
        const collection = app.findCollectionByNameOrId('pkg_build')
        collection.fields.add(
            new Field({
                id: 'pb_bundles',
                name: 'bundles',
                type: 'json',
                maxSize: 1000000,
            })
        )
        app.save(collection)
    },
    app => {
        const collection = app.findCollectionByNameOrId('pkg_build')
        const field = collection.fields.getById('pb_bundles')
        if (field) {
            collection.fields.removeById('pb_bundles')
            app.save(collection)
        }
    }
)
