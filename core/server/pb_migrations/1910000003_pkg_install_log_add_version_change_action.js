/// <reference path="../pb_data/types.d.ts" />
// Adds the 'version_change' action so per-package version updates/downgrades are
// recorded in the install log alongside install/uninstall/revert.
migrate(
    app => {
        const collection = app.findCollectionByNameOrId('pkg_install_log')
        const field = collection.fields.getById('pil_action')
        field.values = ['install', 'uninstall', 'enable', 'disable', 'revert', 'version_change']
        app.save(collection)
    },
    app => {
        const collection = app.findCollectionByNameOrId('pkg_install_log')
        const field = collection.fields.getById('pil_action')
        field.values = ['install', 'uninstall', 'enable', 'disable', 'revert']
        app.save(collection)
    }
)
