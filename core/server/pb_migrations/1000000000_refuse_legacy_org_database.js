/// <reference path="../pb_data/types.d.ts" />
// Refuse to run against a database created by a pre-hosting build.
//
// The hosting transition dropped the org-scoped schema (`orgs`, `user_org`)
// by editing already-shipped migrations in place, and no data-conversion
// migration exists anywhere. That is deliberate: every deployment is
// provisioned fresh (see CLAUDE.md "Migrations may be edited in place"), so
// there is no in-place upgrade path. But PocketBase never re-runs an applied
// migration, so a LEGACY database upgraded to this build would keep `user_org`
// ids in its `user`/`owner`/`created_by` columns while the rewritten rules
// (`user ?= @request.auth.id`) silently never match — every user locked out of
// their own data, with no error anywhere. This migration turns that silent
// lockout into a loud boot failure that says what happened.
//
// The timestamp deliberately sorts BEFORE every other migration: on a legacy
// database this is the first pending migration to run, so nothing else gets a
// chance to half-apply against the old schema first. On a fresh database
// neither collection exists and the guard is a no-op.
// TestFreshProvisionGuard_SortsFirst (coreserver) pins the ordering.
migrate(
    app => {
        for (const name of ['user_org', 'orgs']) {
            let legacy = false
            try {
                app.findCollectionByNameOrId(name)
                legacy = true
            } catch {
                // Not found — the required state: this database was
                // provisioned fresh on the post-hosting schema.
            }
            if (legacy) {
                throw new Error(
                    `refusing to migrate: found legacy collection '${name}'. ` +
                        'This database was created by a pre-hosting build, and there ' +
                        'is no in-place upgrade path from the org-scoped schema: the ' +
                        'shipped migrations were rewritten in place, already-applied ' +
                        'ones will never re-run, and the current access rules would ' +
                        'silently never match the legacy rows. Provision a fresh ' +
                        'deployment and import the data instead.'
                )
            }
        }
    },
    () => {
        // Nothing to revert — the up migration only inspects.
    }
)
