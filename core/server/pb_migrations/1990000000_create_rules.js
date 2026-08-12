/// <reference path="../pb_data/types.d.ts" />
migrate(
    app => {
        const rules = new Collection({
            id: 'pbc_rules_01',
            name: 'rules',
            type: 'base',
            system: false,
            // Personal rules: owner only. Org rules: readable by every
            // non-guest authenticated user (so people can see why org
            // automation touches their data) but writable only by
            // admins/owners. Guests get neither read nor create (matches the
            // house posture: 1870000000_exclude_guests_from_org_rls.js,
            // 1960000000_audit_logs_admin_only.js).
            listRule:
                'owner = @request.auth.id || (scope = "org" && @request.auth.id != "" && @request.auth.role != "guest")',
            viewRule:
                'owner = @request.auth.id || (scope = "org" && @request.auth.id != "" && @request.auth.role != "guest")',
            createRule:
                '@request.auth.id != "" && @request.auth.role != "guest" && owner = @request.auth.id && (scope = "personal" || @request.auth.role = "admin" || @request.auth.role = "owner")',
            // Body-locked: a personal-rule owner may PATCH other fields but
            // must not smuggle `scope`/`owner` changes into the same request
            // (the stored-value check alone can't see the body) — otherwise a
            // personal-rule owner could PATCH { scope: 'org' } or
            // { owner: <other> } and escalate. Admins/owners are trusted on
            // the org branch, so no body locks there.
            updateRule:
                '(scope = "personal" && owner = @request.auth.id && (@request.body.scope:isset = false || @request.body.scope = "personal") && (@request.body.owner:isset = false || @request.body.owner = @request.auth.id)) || (scope = "org" && (@request.auth.role = "admin" || @request.auth.role = "owner"))',
            deleteRule:
                '(scope = "personal" && owner = @request.auth.id) || (scope = "org" && (@request.auth.role = "admin" || @request.auth.role = "owner"))',
            fields: [
                {
                    id: 'rules_name',
                    name: 'name',
                    type: 'text',
                    required: true,
                    max: 200,
                },
                {
                    id: 'rules_scope',
                    name: 'scope',
                    type: 'select',
                    required: true,
                    maxSelect: 1,
                    values: ['personal', 'org'],
                },
                {
                    id: 'rules_owner',
                    name: 'owner',
                    type: 'relation',
                    required: true,
                    collectionId: '_pb_users_auth_',
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                {
                    id: 'rules_trigger',
                    name: 'trigger',
                    type: 'text',
                    required: true,
                    max: 200,
                },
                {
                    id: 'rules_trigger_config',
                    name: 'trigger_config',
                    type: 'json',
                },
                {
                    id: 'rules_conditions',
                    name: 'conditions',
                    type: 'json',
                },
                {
                    id: 'rules_actions',
                    name: 'actions',
                    type: 'json',
                },
                {
                    id: 'rules_enabled',
                    name: 'enabled',
                    type: 'bool',
                },
                {
                    id: 'rules_order',
                    name: 'order',
                    type: 'number',
                },
                {
                    id: 'rules_stop_processing',
                    name: 'stop_processing',
                    type: 'bool',
                },
                {
                    id: 'rules_created',
                    name: 'created',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: false,
                },
                {
                    id: 'rules_updated',
                    name: 'updated',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: true,
                },
            ],
            indexes: [
                'CREATE INDEX `idx_rules_owner` ON `rules` (`owner`, `enabled`)',
                'CREATE INDEX `idx_rules_trigger` ON `rules` (`trigger`, `enabled`)',
            ],
        })
        app.save(rules)
    },
    app => {
        try {
            const c = app.findCollectionByNameOrId('rules')
            app.delete(c)
        } catch (e) {
            // may not exist
        }
    }
)
