/// <reference path="../pb_data/types.d.ts" />
// OAuth 2.1 authorization server storage.
//
// oauth_clients is the registry of things allowed to ask for access: the
// first-party CLI, and later integrations such as Zapier. oauth_grants is one
// row per issued authorization — the row an access token's `jti` points at.
//
// Why a row at all, when the token is a signed JWT: PocketBase signs auth
// tokens with (record.tokenKey + collection secret), so the only built-in
// revocation is rotating tokenKey, which kills EVERY token for that user
// including their web session. Per-device revoke ("disconnect my laptop",
// "disconnect Zapier") therefore needs server-side state consulted on each
// request. That is this table.
//
// Every rule is null => superuser-only. PocketBase rules cannot constrain
// WHICH fields a write touches (the same reason users_guard.go exists in Go),
// so minting, approval, and revocation all go through the oauth package's
// handlers rather than the record API.
migrate(
    app => {
        const clients = new Collection({
            id: 'pbc_oauth_clients_01',
            name: 'oauth_clients',
            type: 'base',
            system: false,
            listRule: null,
            viewRule: null,
            createRule: null,
            updateRule: null,
            deleteRule: null,
            fields: [
                {
                    id: 'oc_client_id',
                    name: 'client_id',
                    type: 'text',
                    required: true,
                    min: 3,
                    max: 100,
                    pattern: '^[a-z0-9][a-z0-9-]*$',
                },
                { id: 'oc_name', name: 'name', type: 'text', required: true, min: 1, max: 200 },
                { id: 'oc_redirect_uris', name: 'redirect_uris', type: 'json' },
                { id: 'oc_scopes', name: 'scopes', type: 'text', max: 500 },
                {
                    id: 'oc_type',
                    name: 'type',
                    type: 'select',
                    required: true,
                    values: ['public', 'confidential'],
                    maxSelect: 1,
                },
                // Hash only. A public client (the CLI) has no secret at all —
                // PKCE is what protects it.
                { id: 'oc_secret_hash', name: 'client_secret_hash', type: 'text', max: 200 },
                { id: 'oc_first_party', name: 'is_first_party', type: 'bool' },
            ],
            indexes: [
                'CREATE UNIQUE INDEX idx_oauth_clients_client_id ON oauth_clients (client_id)',
            ],
        })
        app.save(clients)

        const users = app.findCollectionByNameOrId('users')

        const grants = new Collection({
            id: 'pbc_oauth_grants_01',
            name: 'oauth_grants',
            type: 'base',
            system: false,
            // A user may LIST and VIEW their own grants so the Connected apps
            // screen can render without a bespoke endpoint. Writes stay closed.
            listRule: '@request.auth.id != "" && user = @request.auth.id',
            viewRule: '@request.auth.id != "" && user = @request.auth.id',
            createRule: null,
            updateRule: null,
            deleteRule: null,
            fields: [
                // NOT required: the device flow (RFC 8628) creates a PENDING
                // grant BEFORE any user is known — the user is bound when they
                // approve in the browser. PocketBase's RelationField.Validate-
                // Value returns ErrRequired for an empty required relation, so
                // marking this required would make the device flow unable to
                // store its own pending row. The invariant is enforced in Go
                // instead: VerifyGrant only accepts status "active", and a
                // grant only reaches "active" through approval, which sets the
                // user.
                {
                    id: 'og_user',
                    name: 'user',
                    type: 'relation',
                    required: false,
                    collectionId: users.id,
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                {
                    id: 'og_client',
                    name: 'client',
                    type: 'relation',
                    required: true,
                    collectionId: 'pbc_oauth_clients_01',
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                // jti: the access token's grant id. Unique because it is the
                // lookup key on every authenticated request.
                { id: 'og_jti', name: 'jti', type: 'text', max: 100 },
                { id: 'og_scopes', name: 'scopes', type: 'text', max: 500 },
                { id: 'og_refresh_hash', name: 'refresh_token_hash', type: 'text', max: 200 },
                // Device Grant (RFC 8628) working state, cleared on approval.
                { id: 'og_device_code', name: 'device_code', type: 'text', max: 200 },
                { id: 'og_user_code', name: 'user_code', type: 'text', max: 20 },
                // Authorization Code + PKCE working state, cleared on exchange.
                { id: 'og_code_challenge', name: 'code_challenge', type: 'text', max: 200 },
                { id: 'og_auth_code_hash', name: 'auth_code_hash', type: 'text', max: 200 },
                { id: 'og_redirect_uri', name: 'redirect_uri', type: 'text', max: 2000 },
                {
                    id: 'og_status',
                    name: 'status',
                    type: 'select',
                    required: true,
                    values: ['pending', 'active', 'revoked'],
                    maxSelect: 1,
                },
                { id: 'og_expires_at', name: 'expires_at', type: 'date' },
                { id: 'og_last_used_at', name: 'last_used_at', type: 'date' },
                // Shown in Connected apps so a user can tell devices apart.
                { id: 'og_device_label', name: 'device_label', type: 'text', max: 200 },
            ],
            indexes: [
                'CREATE UNIQUE INDEX idx_oauth_grants_jti ON oauth_grants (jti)',
                'CREATE INDEX idx_oauth_grants_user ON oauth_grants (user)',
                'CREATE INDEX idx_oauth_grants_user_code ON oauth_grants (user_code)',
            ],
        })
        app.save(grants)
    },
    app => {
        // Drop grants first: it holds a cascading relation into clients.
        app.delete(app.findCollectionByNameOrId('oauth_grants'))
        app.delete(app.findCollectionByNameOrId('oauth_clients'))
    }
)
