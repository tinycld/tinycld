/// <reference path="../pb_data/types.d.ts" />
// org_branding: the deployment's uploaded logo.
//
// A collection of its own rather than a row in system_settings, because that
// collection is key/value TEXT with admin-only read — and the logo has to be
// world-readable: login and other pre-auth screens render it before any token
// exists. Public read here is what lets /api/org-info hand out a URL that
// works unauthenticated.
//
// Single row, id 'branding'. Writes are owner/admin only, matching how the
// rest of deployment configuration is gated.
migrate(
    app => {
        // `disabled != true` matches 1910000010_create_system_settings,
        // 1960000000_audit_logs_admin_only and 1970000000_admin_console_role_rules:
        // without it, an admin account that has just been suspended keeps write
        // access for as long as its already-issued JWT lives.
        const ADMIN =
            '@request.auth.id != "" && @request.auth.disabled != true && ' +
            '(@request.auth.role = "owner" || @request.auth.role = "admin")'

        const col = new Collection({
            id: 'pbc_org_branding',
            name: 'org_branding',
            type: 'base',
            system: false,
            listRule: '',
            viewRule: '',
            createRule: ADMIN,
            updateRule: ADMIN,
            deleteRule: ADMIN,
            fields: [
                {
                    id: 'ob_logo',
                    name: 'logo',
                    type: 'file',
                    maxSelect: 1,
                    maxSize: 2097152,
                    mimeTypes: ['image/png', 'image/jpeg', 'image/webp'],
                },
                {
                    id: 'ob_logo_crop',
                    name: 'logo_crop',
                    type: 'text',
                    max: 200,
                },
                {
                    id: 'ob_created',
                    name: 'created',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: false,
                },
                {
                    id: 'ob_updated',
                    name: 'updated',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: true,
                },
            ],
        })
        app.save(col)
    },
    app => {
        app.delete(app.findCollectionByNameOrId('org_branding'))
    }
)
