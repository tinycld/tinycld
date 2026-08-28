/// <reference path="../pb_data/types.d.ts" />
// Rewrite stored `notifications.url` values onto the /a app-route prefix.
//
// App routes moved under a constant `/a` segment. `notifications.url` is a
// stored path that NotificationDrawer pushes at the router VERBATIM, so every
// row written before the move points at a route that no longer resolves —
// tapping a bell item lands on +not-found. Unlike the emailed-link case (which
// arrives from outside and is handled by legacyAppRedirect in
// core/server/coreserver/static.go), these values are ours and finite, so they
// are fixed once here rather than normalized on every read.
//
// WHY AN ALLOWLIST AND NOT A BLANKET PREFIX. The column is free-form: any
// package can write it, and today it holds at least three kinds of value that
// MUST NOT be rewritten —
//
//   - `/api/...`   e.g. "/api/cli/download/<platform>" (core), the drive
//                  download/export token links. These are server endpoints,
//                  not app routes; prefixing them 404s the download.
//   - `/p/...`     public share/document links written by drive and the
//                  document packages. They live outside the app tree.
//   - `/`          the app root, which is already correct — it is the auth
//                  landing and redirects onward.
//
// So the rewrite is keyed on the FIRST PATH SEGMENT matching a known app
// route. The list below is the union of what the notify call sites actually
// write (grep `URL:` under */server): package slugs, plus the app-owned
// settings/help areas.
//
// Automation rules can also set an arbitrary `url` (see automation/runs.go,
// `req.Params["url"]`). A value outside the allowlist is left alone — better a
// stale link than a corrupted one.
//
// IDEMPOTENT: rows already under /a are excluded by the LIKE guard, so a
// re-run (or a row written by a NEW build between this migration and the
// deploy completing) cannot become "/a/a/...".
//
// NOT REVERSIBLE IN FULL: the down migration strips the prefix back off, which
// restores the pre-move shape. It cannot distinguish a row this migration
// rewrote from one an /a-aware build wrote afterwards — acceptable, because
// the only reason to run it is rolling back to a build where /a doesn't exist
// and every one of those rows is wrong either way.
const APP_PREFIX = '/a'

// First path segments that are app routes. Package slugs come first (these are
// what notify actually writes today), then the app-owned areas.
const APP_SEGMENTS = [
    'calendar',
    'drive',
    'mail',
    'cards',
    'calc',
    'text',
    'contacts',
    'settings',
    'help',
]

// Matches "/<segment>" exactly, or "/<segment>" followed by a separator, so
// "/calendar" and "/calendar/abc123?x=1" both match while a hypothetical
// "/calendarical" does not. The trailing guard keeps the statement idempotent:
// a row already under /a is never rewritten twice.
function rowsToPrefix() {
    const clauses = APP_SEGMENTS.map(
        segment => `url = '/${segment}' OR url LIKE '/${segment}/%' OR url LIKE '/${segment}?%'`
    ).join(' OR ')
    return `(${clauses}) AND url NOT LIKE '${APP_PREFIX}/%'`
}

migrate(
    app => {
        try {
            app.findCollectionByNameOrId('notifications')
        } catch {
            // No notifications table in this workspace — nothing to rewrite.
            return
        }

        app.db()
            .newQuery(
                `UPDATE notifications SET url = '${APP_PREFIX}' || url WHERE ${rowsToPrefix()}`
            )
            .execute()
    },
    app => {
        try {
            app.findCollectionByNameOrId('notifications')
        } catch {
            return
        }

        // Strip the prefix back off: substr is 1-indexed in SQLite, so
        // length(APP_PREFIX) + 1 starts at the character after it.
        app.db()
            .newQuery(
                `UPDATE notifications SET url = substr(url, ${APP_PREFIX.length + 1})` +
                    ` WHERE url LIKE '${APP_PREFIX}/%'`
            )
            .execute()
    }
)
