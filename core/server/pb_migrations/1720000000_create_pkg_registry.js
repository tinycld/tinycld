/// <reference path="../pb_data/types.d.ts" />
migrate(
    app => {
        // pkg_registry — global package catalog (superuser-managed). Its
        // `status` field is the single source of truth for whether a package
        // is active on this deployment.
        const pkgRegistry = new Collection({
            id: 'pbc_pkg_reg_01',
            name: 'pkg_registry',
            type: 'base',
            system: false,
            listRule: '@request.auth.id != ""',
            viewRule: '@request.auth.id != ""',
            createRule: null,
            updateRule: null,
            deleteRule: null,
            fields: [
                {
                    id: 'pr_name',
                    name: 'name',
                    type: 'text',
                    required: true,
                    min: 1,
                    max: 200,
                },
                {
                    id: 'pr_slug',
                    name: 'slug',
                    type: 'text',
                    required: true,
                    min: 1,
                    max: 100,
                    pattern: '^[a-z0-9][a-z0-9-]*$',
                },
                {
                    id: 'pr_npm_package',
                    name: 'npm_package',
                    type: 'text',
                    max: 500,
                },
                {
                    id: 'pr_version',
                    name: 'version',
                    type: 'text',
                    max: 50,
                },
                {
                    id: 'pr_status',
                    name: 'status',
                    type: 'select',
                    required: true,
                    values: ['bundled', 'available', 'installed', 'disabled'],
                    maxSelect: 1,
                },
                {
                    id: 'pr_manifest_json',
                    name: 'manifest_json',
                    type: 'json',
                },
                {
                    id: 'pr_has_server',
                    name: 'has_server',
                    type: 'bool',
                },
                {
                    id: 'pr_icon',
                    name: 'icon',
                    type: 'text',
                    max: 100,
                },
                {
                    id: 'pr_description',
                    name: 'description',
                    type: 'text',
                    max: 1000,
                },
                {
                    id: 'pr_nav_order',
                    name: 'nav_order',
                    type: 'number',
                },
                {
                    id: 'pr_created',
                    name: 'created',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: false,
                },
                {
                    id: 'pr_updated',
                    name: 'updated',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: true,
                },
            ],
            indexes: [
                'CREATE UNIQUE INDEX `idx_pkg_registry_slug` ON `pkg_registry` (`slug`)',
            ],
        })
        app.save(pkgRegistry)
    },
    app => {
        try {
            const c = app.findCollectionByNameOrId('pkg_registry')
            app.delete(c)
        } catch (e) {
            // may not exist
        }
    }
)
