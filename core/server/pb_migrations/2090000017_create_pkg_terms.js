/// <reference path="../pb_data/types.d.ts" />
//
// Package terms and conditions (manifest `terms`).
//
// pkg_registry.terms_version is the version the installed package declares,
// written at boot from bundled-packages.json. terms_accepted_version is the
// version the owner accepted, written only by the server when the owner
// creates an `accepted` row in pkg_terms_decisions (core/server/pkgterms).
// A package whose two values differ is refused to everybody.
//
// pkg_terms holds the text, one row per package, written only at boot. Any
// signed-in user may read it: a member reads the terms the owner accepted.
//
// pkg_terms_decisions is the record of each decision: package, terms version,
// user, time. Only the owner creates a row; nobody changes or removes one.
// A decision on terms that are already accepted is refused, so each acceptance
// row is one change from pending to accepted. A version can be accepted again
// after the package left it and came back to it.
//
// A new migration because 1720000000_create_pkg_registry.js is released.
migrate(
    app => {
        const SIGNED_IN = '@request.auth.id != "" && @request.auth.disabled != true'
        const ADMIN = `${SIGNED_IN} && (@request.auth.role = "owner" || @request.auth.role = "admin")`
        const OWNER = `${SIGNED_IN} && @request.auth.role = "owner"`

        const registry = app.findCollectionByNameOrId('pkg_registry')
        registry.fields.add(new TextField({ id: 'pr_terms_version', name: 'terms_version', max: 50 }))
        registry.fields.add(
            new TextField({ id: 'pr_terms_accepted', name: 'terms_accepted_version', max: 50 })
        )
        app.save(registry)

        app.save(
            new Collection({
                id: 'pbc_pkg_terms_01',
                name: 'pkg_terms',
                type: 'base',
                system: false,
                listRule: SIGNED_IN,
                viewRule: SIGNED_IN,
                createRule: null,
                updateRule: null,
                deleteRule: null,
                fields: [
                    { id: 'pt_pkg', name: 'pkg', type: 'text', required: true, max: 100 },
                    { id: 'pt_version', name: 'version', type: 'text', required: true, max: 50 },
                    // Keep max in step with TERMS_MAX_BYTES in scripts/gen-terms.ts.
                    { id: 'pt_body', name: 'body', type: 'text', required: true, max: 200000 },
                    { id: 'pt_created', name: 'created', type: 'autodate', onCreate: true, onUpdate: false },
                    { id: 'pt_updated', name: 'updated', type: 'autodate', onCreate: true, onUpdate: true },
                ],
                indexes: ['CREATE UNIQUE INDEX `idx_pkg_terms_pkg` ON `pkg_terms` (`pkg`)'],
            })
        )

        app.save(
            new Collection({
                id: 'pbc_pkg_terms_dec_01',
                name: 'pkg_terms_decisions',
                type: 'base',
                system: false,
                listRule: ADMIN,
                viewRule: ADMIN,
                createRule: OWNER,
                updateRule: null,
                deleteRule: null,
                fields: [
                    { id: 'ptd_pkg', name: 'pkg', type: 'text', required: true, max: 100 },
                    { id: 'ptd_version', name: 'terms_version', type: 'text', required: true, max: 50 },
                    {
                        id: 'ptd_decision',
                        name: 'decision',
                        type: 'select',
                        required: true,
                        maxSelect: 1,
                        values: ['accepted', 'declined'],
                    },
                    // Not cascade: the record of an acceptance outlives the account.
                    {
                        id: 'ptd_user',
                        name: 'user',
                        type: 'relation',
                        required: false,
                        collectionId: '_pb_users_auth_',
                        cascadeDelete: false,
                        maxSelect: 1,
                    },
                    { id: 'ptd_created', name: 'created', type: 'autodate', onCreate: true, onUpdate: false },
                ],
                indexes: [
                    'CREATE INDEX `idx_pkg_terms_decisions_pkg` ON `pkg_terms_decisions` (`pkg`, `terms_version`, `decision`)',
                ],
            })
        )
    },
    app => {
        for (const name of ['pkg_terms_decisions', 'pkg_terms']) {
            try {
                app.delete(app.findCollectionByNameOrId(name))
            } catch (e) {
                // may not exist
            }
        }
        const registry = app.findCollectionByNameOrId('pkg_registry')
        registry.fields.removeById('pr_terms_version')
        registry.fields.removeById('pr_terms_accepted')
        app.save(registry)
    }
)
