/// <reference path="../pb_data/types.d.ts" />
// Saved reports and their shares (docs/superpowers/specs/2026-10-30-report-engine-design.md,
// "3. Saved reports and sharing").
//
// A report belongs to its owner. A share gives one user, or every member of a
// group, the report's design as viewer or editor; each reader still runs the
// report with their own rights. report_shares is a group grant table
// (core/server/groups): a grant row has `group` set and `user` empty, and the
// expander writes one derived row (both set) per member. So every rule that
// reads report_shares.user starts with the login guard: an anonymous
// request's NULL @request.auth.id matches a grant row's empty user.
//
// The server owns source, published_by, broken, shipped_key and
// shipped_revision, and only an admin may change published: hooks in
// core/server/reports/saved.go enforce that, not these rules.
const SIGNED_IN = '@request.auth.id != "" && @request.auth.disabled != true'
const ADMIN = '(@request.auth.role = "owner" || @request.auth.role = "admin")'
const NOT_GUEST = '@request.auth.role != "guest"'
const EDITOR = 'report_shares_via_report.user ?= @request.auth.id && report_shares_via_report.role ?= "editor"'
const READ = `${SIGNED_IN} && (owner = @request.auth.id || report_shares_via_report.user ?= @request.auth.id || (published = true && ${NOT_GUEST}))`
const SHARE_MANAGER =
    '(report.owner = @request.auth.id || ' +
    '(report.report_shares_via_report.user ?= @request.auth.id && report.report_shares_via_report.role ?= "editor"))'

migrate(
    app => {
        const reports = new Collection({
            id: 'pbc_reports_01',
            name: 'reports',
            type: 'base',
            system: false,
            listRule: null,
            viewRule: null,
            createRule: null,
            updateRule: null,
            deleteRule: null,
            fields: [
                {
                    id: 'reports_owner',
                    name: 'owner',
                    type: 'relation',
                    required: true,
                    collectionId: '_pb_users_auth_',
                    cascadeDelete: false,
                    maxSelect: 1,
                },
                { id: 'reports_title', name: 'title', type: 'text', required: true, min: 1, max: 200 },
                { id: 'reports_description', name: 'description', type: 'text', required: false, max: 2000 },
                { id: 'reports_spec', name: 'spec', type: 'json', required: true, maxSize: 1048576 },
                { id: 'reports_source', name: 'source', type: 'text', required: true, max: 120 },
                { id: 'reports_published', name: 'published', type: 'bool' },
                {
                    id: 'reports_published_by',
                    name: 'published_by',
                    type: 'relation',
                    required: false,
                    collectionId: '_pb_users_auth_',
                    cascadeDelete: false,
                    maxSelect: 1,
                },
                { id: 'reports_broken', name: 'broken', type: 'json', required: false, maxSize: 65536 },
                { id: 'reports_shipped_key', name: 'shipped_key', type: 'text', required: false, max: 120 },
                { id: 'reports_shipped_revision', name: 'shipped_revision', type: 'number', required: false, onlyInt: true, min: 0 },
                { id: 'reports_created', name: 'created', type: 'autodate', onCreate: true, onUpdate: false },
                { id: 'reports_updated', name: 'updated', type: 'autodate', onCreate: true, onUpdate: true },
            ],
            indexes: [
                'CREATE INDEX `idx_reports_owner` ON `reports` (`owner`)',
                'CREATE INDEX `idx_reports_source` ON `reports` (`source`)',
                'CREATE INDEX `idx_reports_published` ON `reports` (`published`)',
                "CREATE UNIQUE INDEX `idx_reports_shipped_key` ON `reports` (`shipped_key`) WHERE `shipped_key` != ''",
            ],
        })
        app.save(reports)

        const shares = new Collection({
            id: 'pbc_report_shares_01',
            name: 'report_shares',
            type: 'base',
            system: false,
            listRule: null,
            viewRule: null,
            createRule: null,
            updateRule: null,
            deleteRule: null,
            fields: [
                {
                    id: 'report_shares_report',
                    name: 'report',
                    type: 'relation',
                    required: true,
                    collectionId: 'pbc_reports_01',
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                {
                    id: 'report_shares_user',
                    name: 'user',
                    type: 'relation',
                    required: false,
                    collectionId: '_pb_users_auth_',
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                {
                    id: 'report_shares_group',
                    name: 'group',
                    type: 'relation',
                    required: false,
                    collectionId: 'pbc_groups_01',
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                { id: 'report_shares_role', name: 'role', type: 'select', required: true, maxSelect: 1, values: ['viewer', 'editor'] },
                { id: 'report_shares_created', name: 'created', type: 'autodate', onCreate: true, onUpdate: false },
                { id: 'report_shares_updated', name: 'updated', type: 'autodate', onCreate: true, onUpdate: true },
            ],
            indexes: [
                // groups.GrantTable requires (resource, user, group) unique.
                'CREATE UNIQUE INDEX `idx_report_shares_unique` ON `report_shares` (`report`, `user`, `group`)',
                'CREATE INDEX `idx_report_shares_user` ON `report_shares` (`user`)',
                'CREATE INDEX `idx_report_shares_group` ON `report_shares` (`group`)',
            ],
        })
        app.save(shares)

        // The rules read the back-relation report_shares_via_report, so they
        // are set once both collections exist.
        const shared = app.findCollectionByNameOrId('report_shares')
        shared.listRule = `${SIGNED_IN} && (user = @request.auth.id || ${SHARE_MANAGER})`
        shared.viewRule = shared.listRule
        shared.createRule = `${SIGNED_IN} && ${SHARE_MANAGER}`
        shared.updateRule = shared.createRule
        shared.deleteRule = shared.createRule
        app.save(shared)

        const saved = app.findCollectionByNameOrId('reports')
        saved.listRule = READ
        saved.viewRule = READ
        saved.createRule = `${SIGNED_IN} && owner = @request.auth.id`
        saved.updateRule = `${SIGNED_IN} && (owner = @request.auth.id || (${EDITOR}))`
        saved.deleteRule = `${SIGNED_IN} && (owner = @request.auth.id || (published = true && ${ADMIN}))`
        app.save(saved)
    },
    app => {
        for (const name of ['report_shares', 'reports']) {
            try {
                app.delete(app.findCollectionByNameOrId(name))
            } catch (e) {
                // may not exist
            }
        }
    }
)
