/// <reference path="../pb_data/types.d.ts" />
migrate(
    app => {
        const collection = app.findCollectionByNameOrId('pkg_install_log')
        const field = collection.fields.getById('pil_action')
        field.values = ['install', 'uninstall', 'enable', 'disable', 'revert']
        app.save(collection)
    },
    app => {
        const collection = app.findCollectionByNameOrId('pkg_install_log')
        const field = collection.fields.getById('pil_action')
        field.values = ['install', 'uninstall', 'enable', 'disable']
        app.save(collection)
    }
)
