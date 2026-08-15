/// <reference path="../pb_data/types.d.ts" />
// Register the tinycld CLI as a first-party OAuth client.
//
// It is a PUBLIC client: an installed binary cannot keep a secret, so there is
// none to steal. PKCE (S256) is what binds an authorization exchange to the
// process that started it, and the Device Grant it actually uses never
// redirects at all.
//
// Seeded rather than hand-registered so every deployment — self-hosted or a
// multi-org tenant — can authenticate a CLI the moment it boots.
migrate(
    app => {
        const clients = app.findCollectionByNameOrId('oauth_clients')
        const cli = new Record(clients)
        cli.set('client_id', 'tinycld-cli')
        cli.set('name', 'TinyCld CLI')
        cli.set('type', 'public')
        cli.set('is_first_party', true)
        // The CLI uses the device grant, which has no redirect. The loopback
        // entry is there for a future `--browser` authorization-code login.
        cli.set('redirect_uris', ['http://127.0.0.1/callback'])
        // Keep in step with oauth.AllScopes and the CLI's own cliScopes. A
        // package whose scopes are missing here cannot be used from the CLI at
        // all: this row is the ceiling ValidateClientScopes enforces.
        cli.set(
            'scopes',
            'profile mail:read mail:send drive:read drive:write ' +
                'contacts:read contacts:write calendar:read calendar:write ' +
                'cards:read cards:write'
        )
        app.save(cli)
    },
    app => {
        try {
            const existing = app.findFirstRecordByFilter(
                'oauth_clients',
                'client_id = {:id}',
                { id: 'tinycld-cli' }
            )
            app.delete(existing)
        } catch {
            // Already gone — nothing to undo.
        }
    }
)
