/// <reference path="../pb_data/types.d.ts" />
// Records which build an install-log row produced, so a rollback found at
// boot can mark every row of the build it rolled back from. Without it the
// reconciler could only guess (the newest running row), and a row finalized
// "success" before the restart stayed "success" after its build failed.
migrate(
    app => {
        const collection = app.findCollectionByNameOrId('pkg_install_log')
        collection.fields.add(
            new Field({
                id: 'pil_build_id',
                name: 'build_id',
                type: 'text',
                max: 100,
            })
        )
        collection.indexes = [
            ...collection.indexes,
            'CREATE INDEX `idx_pkg_install_log_build_id` ON `pkg_install_log` (`build_id`)',
        ]
        app.save(collection)
    },
    app => {
        const collection = app.findCollectionByNameOrId('pkg_install_log')
        collection.indexes = collection.indexes.filter(
            i => !i.includes('idx_pkg_install_log_build_id')
        )
        collection.fields.removeById('pil_build_id')
        app.save(collection)
    }
)
