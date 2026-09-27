/// <reference path="../pb_data/types.d.ts" />
// Add the backups scope to the seeded tinycld-cli client. The client row is a
// hard ceiling (ValidateClientScopes rejects any scope it does not name), so
// without this `tinycld auth login` fails once the server advertises backups.
// Appended, not folded into the seed: PocketBase never re-runs an applied
// migration. Rewrites the whole string so an already-widened row converges.
const CLI_SCOPES_BEFORE =
    'profile mail:read mail:send drive:read drive:write ' +
    'contacts:read contacts:write calendar:read calendar:write ' +
    'boards:read boards:write text:read text:write calc:read calc:write'

const CLI_SCOPES = CLI_SCOPES_BEFORE + ' backups'

migrate(
    app => {
        let cli
        try {
            cli = app.findFirstRecordByFilter('oauth_clients', 'client_id = {:id}', { id: 'tinycld-cli' })
        } catch {
            return
        }
        cli.set('scopes', CLI_SCOPES)
        app.save(cli)
    },
    app => {
        try {
            const cli = app.findFirstRecordByFilter('oauth_clients', 'client_id = {:id}', { id: 'tinycld-cli' })
            cli.set('scopes', CLI_SCOPES_BEFORE)
            app.save(cli)
        } catch {
            // already gone
        }
    }
)
