/// <reference path="../pb_data/types.d.ts" />
migrate(
    app => {
        const pkgBuild = new Collection({
            id: 'pbc_pkg_build_01',
            name: 'pkg_build',
            type: 'base',
            system: false,
            listRule: null,
            viewRule: null,
            createRule: null,
            updateRule: null,
            deleteRule: null,
            fields: [
                {
                    id: 'pb_build_id',
                    name: 'build_id',
                    type: 'text',
                    required: true,
                    max: 100,
                },
                {
                    id: 'pb_pkg_slug',
                    name: 'pkg_slug',
                    type: 'text',
                    required: true,
                    max: 100,
                },
                {
                    id: 'pb_npm_package',
                    name: 'npm_package',
                    type: 'text',
                    max: 500,
                },
                {
                    id: 'pb_version',
                    name: 'version',
                    type: 'text',
                    max: 50,
                },
                {
                    id: 'pb_action',
                    name: 'action',
                    type: 'select',
                    required: true,
                    values: ['install', 'revert'],
                    maxSelect: 1,
                },
                {
                    id: 'pb_binary_archived',
                    name: 'binary_archived',
                    type: 'bool',
                },
                {
                    id: 'pb_release_id',
                    name: 'release_id',
                    type: 'text',
                    max: 100,
                },
                {
                    id: 'pb_migrations_applied',
                    name: 'migrations_applied',
                    type: 'number',
                },
                {
                    id: 'pb_migration_files',
                    name: 'migration_files',
                    type: 'json',
                    maxSize: 100000,
                },
                {
                    id: 'pb_reverted_from',
                    name: 'reverted_from',
                    type: 'text',
                    max: 100,
                },
                {
                    id: 'pb_status',
                    name: 'status',
                    type: 'select',
                    required: true,
                    values: ['available', 'current', 'superseded'],
                    maxSelect: 1,
                },
                {
                    id: 'pb_notes',
                    name: 'notes',
                    type: 'text',
                    max: 1000,
                },
                {
                    id: 'pb_created',
                    name: 'created',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: false,
                },
                {
                    id: 'pb_updated',
                    name: 'updated',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: true,
                },
            ],
            indexes: [
                'CREATE UNIQUE INDEX `idx_pkg_build_build_id` ON `pkg_build` (`build_id`)',
                'CREATE INDEX `idx_pkg_build_status` ON `pkg_build` (`status`)',
            ],
        })
        app.save(pkgBuild)
    },
    app => {
        try {
            const c = app.findCollectionByNameOrId('pkg_build')
            app.delete(c)
        } catch (e) {
            // may not exist
        }
    }
)
