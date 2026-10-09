/// <reference path="../pb_data/types.d.ts" />
// Carries live install/upgrade progress on the durable row instead of the SSE
// stream, which dies at the server restart that ends every successful apply.
// `steps` holds the main-step history only (headline milestones, e.g. "Build
// started", "Migrating database") — not pkgbuild's detail log lines, which
// stay in `log` and are written once, at finalize. `current_step` /
// `current_message` mirror the latest entry so a client can show "now doing
// X" without re-deriving it from the array on every row.
migrate(
    app => {
        const collection = app.findCollectionByNameOrId('pkg_install_log')
        collection.fields.add(
            new Field({
                id: 'pil_steps',
                name: 'steps',
                type: 'json',
                maxSize: 2000000,
            })
        )
        collection.fields.add(
            new Field({
                id: 'pil_current_step',
                name: 'current_step',
                type: 'text',
                max: 200,
            })
        )
        collection.fields.add(
            new Field({
                id: 'pil_current_message',
                name: 'current_message',
                type: 'text',
                max: 500,
            })
        )
        app.save(collection)
    },
    app => {
        const collection = app.findCollectionByNameOrId('pkg_install_log')
        collection.fields.removeById('pil_current_message')
        collection.fields.removeById('pil_current_step')
        collection.fields.removeById('pil_steps')
        app.save(collection)
    }
)
