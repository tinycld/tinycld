/// <reference path="../pb_data/types.d.ts" />
// system_settings: deployment-wide configuration that applies to the ENTIRE
// system (not a single org). It holds the "use-your-own-service" values a
// third-party host must own — Sentry, web-push (VAPID), mail provider creds —
// so they're configured from the /admin Settings console instead of env vars.
//
// This is the SOLE source of truth at runtime: the server reads values through
// the in-memory SystemConfig (coreserver/system_config.go), which loads this
// collection — there is no os.Getenv fallback in the read path. Env vars are
// consulted ONCE, by the seed below, to carry an existing env-configured
// deployment across to the DB on the boot that runs this migration.
//
// Shape: a flat key/value store. `is_secret` marks values that must never be
// surfaced to clients or injected into the web HTML / JS bundle (tokens, the
// VAPID private key, IMAP password). `key` is namespaced by area
// (`sentry.dsn`, `vapid.private_key`, `mail.postmark_server_token`).
//
// Rules: all ops require a PB superuser OR a super-admin app user. A null rule
// is superuser-only and `@collection.super_admins.user ?= @request.auth.id`
// adds the super-admins (the same clause the admin console relies on
// everywhere — see 1910000007_pkg_admin_super_admin_rules). Secret rows are
// additionally never returned to clients by the server's config injection
// (see system_config.go / the web HTML injector).
const SA = '@collection.super_admins.user ?= @request.auth.id'

// key → legacy env var. Used ONLY by the one-time seed. Keep `is_secret` in
// sync with the secrets policy in docs/plans/system-settings.md.
const SEED = [
    { key: 'sentry.dsn', env: 'SENTRY_DSN', secret: false },
    { key: 'sentry.org', env: 'SENTRY_ORG', secret: false },
    { key: 'sentry.project', env: 'SENTRY_PROJECT', secret: false },
    { key: 'sentry.auth_token', env: 'SENTRY_AUTH_TOKEN', secret: true },
    { key: 'vapid.public_key', env: 'VAPID_PUBLIC_KEY', secret: false },
    { key: 'vapid.private_key', env: 'VAPID_PRIVATE_KEY', secret: true },
    { key: 'vapid.subject', env: 'VAPID_SUBJECT', secret: false },
]

migrate(
    app => {
        const col = new Collection({
            id: 'pbc_system_settings',
            name: 'system_settings',
            type: 'base',
            system: false,
            listRule: SA,
            viewRule: SA,
            createRule: SA,
            updateRule: SA,
            deleteRule: SA,
            fields: [
                {
                    id: 'ss_key',
                    name: 'key',
                    type: 'text',
                    required: true,
                    max: 100,
                },
                {
                    id: 'ss_value',
                    name: 'value',
                    type: 'text',
                    max: 5000,
                },
                {
                    id: 'ss_is_secret',
                    name: 'is_secret',
                    type: 'bool',
                },
                {
                    id: 'ss_updated_by',
                    name: 'updated_by',
                    type: 'relation',
                    required: false,
                    collectionId: '_pb_users_auth_',
                    cascadeDelete: false,
                    maxSelect: 1,
                },
                {
                    id: 'ss_created',
                    name: 'created',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: false,
                },
                {
                    id: 'ss_updated',
                    name: 'updated',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: true,
                },
            ],
            indexes: ['CREATE UNIQUE INDEX `idx_system_settings_key` ON `system_settings` (`key`)'],
        })
        app.save(col)

        // One-time seed from legacy env vars. Only writes a row when the env var
        // is non-empty AND no row for that key exists yet — so re-running on a
        // deployment that already configured a value via /admin is a no-op, and a
        // deployment with no env vars simply starts empty.
        //
        // $os is absent in a SANDBOXED jsvm — a hosted tenant booting a per-org
        // build runs this same migration there, and the router hands tenants a
        // scrubbed environment anyway, so "no env to seed from" is the correct
        // meaning of skipping (this seed is single-tenant-legacy by design).
        const seedEnv = typeof $os === 'undefined' ? [] : SEED
        for (const s of seedEnv) {
            // PocketBase's JSVM exposes env via $os.getenv (NOT Node's process.env,
            // which is undefined here). This is the only env read in the system —
            // the runtime path reads system_settings, never the environment.
            const val = $os.getenv(s.env)
            if (!val) continue
            try {
                app.findFirstRecordByFilter('system_settings', 'key = {:key}', { key: s.key })
                continue // already present
            } catch (e) {
                // not found → seed it
            }
            const rec = new Record(col)
            rec.set('key', s.key)
            rec.set('value', val)
            rec.set('is_secret', s.secret)
            app.save(rec)
        }
    },
    app => {
        try {
            const c = app.findCollectionByNameOrId('system_settings')
            app.delete(c)
        } catch (e) {
            // may not exist
        }
    }
)
