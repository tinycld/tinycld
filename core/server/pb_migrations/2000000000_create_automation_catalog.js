/// <reference path="../pb_data/types.d.ts" />
migrate(
    app => {
        const catalog = new Collection({
            id: 'pbc_automation_catalog_01',
            name: 'automation_catalog',
            type: 'base',
            system: false,
            // Readable by any authenticated non-guest user (matches the rules
            // collection's read posture — every member browsing the rule
            // builder needs to see what triggers/actions are available), but
            // written only by the engine's boot-time sync (superuser DAO).
            listRule: '@request.auth.id != "" && @request.auth.role != "guest"',
            viewRule: '@request.auth.id != "" && @request.auth.role != "guest"',
            createRule: null,
            updateRule: null,
            deleteRule: null,
            fields: [
                {
                    id: 'ac_ref',
                    name: 'ref',
                    type: 'text',
                    required: true,
                    max: 200,
                },
                {
                    id: 'ac_kind',
                    name: 'kind',
                    type: 'select',
                    required: true,
                    maxSelect: 1,
                    values: ['trigger', 'action'],
                },
                {
                    id: 'ac_pkg',
                    name: 'pkg',
                    type: 'text',
                    max: 100,
                },
                {
                    id: 'ac_label',
                    name: 'label',
                    type: 'text',
                    max: 200,
                },
                {
                    id: 'ac_definition',
                    name: 'definition',
                    type: 'json',
                },
                {
                    id: 'ac_available',
                    name: 'available',
                    type: 'bool',
                },
                {
                    id: 'ac_created',
                    name: 'created',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: false,
                },
                {
                    id: 'ac_updated',
                    name: 'updated',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: true,
                },
            ],
            indexes: [
                'CREATE UNIQUE INDEX `idx_automation_catalog_ref` ON `automation_catalog` (`ref`)',
            ],
        })
        app.save(catalog)
    },
    app => {
        try {
            const c = app.findCollectionByNameOrId('automation_catalog')
            app.delete(c)
        } catch (e) {
            // may not exist
        }
    }
)
