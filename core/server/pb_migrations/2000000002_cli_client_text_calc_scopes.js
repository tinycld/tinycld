/// <reference path="../pb_data/types.d.ts" />
// Add the text and calc scopes to the seeded tinycld-cli client.
//
// Same shape, and the same reason, as 2000000001 did for cards: the CLI client
// row is a hard ceiling (ValidateClientScopes rejects any scope it does not
// name), so a scope missing there fails the LOGIN outright, while one missing
// from the CLI's own cliScopes list fails later and quietly — the grant is
// issued without it and the command 403s.
//
// Appended rather than folded into 1985000001 (or 2000000001) because
// PocketBase never re-runs an applied migration: editing either file would fix
// only databases created after the edit and silently leave every existing one
// broken.
//
// Rewrites the whole scope string from the current catalog rather than
// appending to whatever is there, so a row that already has the scopes (a DB
// seeded after this ships) converges instead of accumulating duplicates.
const CLI_SCOPES =
    'profile mail:read mail:send drive:read drive:write ' +
    'contacts:read contacts:write calendar:read calendar:write ' +
    'cards:read cards:write text:read text:write calc:read calc:write'

const CLI_SCOPES_BEFORE =
    'profile mail:read mail:send drive:read drive:write ' +
    'contacts:read contacts:write calendar:read calendar:write ' +
    'cards:read cards:write'

migrate(
    app => {
        let cli
        try {
            cli = app.findFirstRecordByFilter('oauth_clients', 'client_id = {:id}', {
                id: 'tinycld-cli',
            })
        } catch {
            // No CLI client on this deployment (the seed was rolled back, or
            // the operator deleted it). Nothing to widen — a fresh seed will
            // carry the current scope list.
            return
        }
        cli.set('scopes', CLI_SCOPES)
        app.save(cli)
    },
    app => {
        try {
            const cli = app.findFirstRecordByFilter('oauth_clients', 'client_id = {:id}', {
                id: 'tinycld-cli',
            })
            cli.set('scopes', CLI_SCOPES_BEFORE)
            app.save(cli)
        } catch {
            // Already gone — nothing to undo.
        }
    }
)
