/// <reference path="../pb_data/types.d.ts" />
// Records OTA bundles that clients reported as crash-looping (POST
// /api/app/update/report-bad). resolveManifest skips any bundle whose
// (bundle_id) or (bundle_hash) appears here, so a bundle that bricks one device
// stops being advertised to the whole fleet — fleet-wide blast-radius control on
// top of each device's local crash-rollback.
migrate(
    app => {
        const c = new Collection({
            id: 'pbc_pkg_bad_bundle1',
            name: 'pkg_bad_bundle',
            type: 'base',
            system: false,
            // Public create (the app reports pre/post-auth, like /api/app/update);
            // the HTTP handler does the validation. No read/update/delete via API.
            listRule: null,
            viewRule: null,
            createRule: null,
            updateRule: null,
            deleteRule: null,
            fields: [
                {
                    id: 'pbb_bundle_id',
                    name: 'bundle_id',
                    type: 'text',
                    required: true,
                    max: 200,
                },
                {
                    id: 'pbb_bundle_hash',
                    name: 'bundle_hash',
                    type: 'text',
                    max: 128,
                },
                {
                    id: 'pbb_platform',
                    name: 'platform',
                    type: 'select',
                    required: true,
                    values: ['ios', 'android'],
                    maxSelect: 1,
                },
                {
                    id: 'pbb_reports',
                    name: 'reports',
                    type: 'number',
                    required: true,
                },
                {
                    id: 'pbb_last_error',
                    name: 'last_error',
                    type: 'text',
                    max: 2000,
                },
                {
                    id: 'pbb_created',
                    name: 'created',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: false,
                },
                {
                    id: 'pbb_updated',
                    name: 'updated',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: true,
                },
            ],
            indexes: [
                'CREATE UNIQUE INDEX `idx_pkg_bad_bundle_id` ON `pkg_bad_bundle` (`bundle_id`)',
            ],
        })
        app.save(c)
    },
    app => {
        try {
            app.delete(app.findCollectionByNameOrId('pkg_bad_bundle'))
        } catch (e) {
            // may not exist
        }
    }
)
