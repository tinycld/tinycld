#!/usr/bin/env -S pnpm exec tsx
/**
 * Demo Reset Script
 *
 * Wipes the data owned by the demo user and its companion, then re-seeds both.
 * Designed to run nightly (see coreserver/demo_reset.go) so the demo workspace
 * is always pristine for the next unauthenticated visitor that hits
 * /api/demo/start.
 *
 * The demo *users* (`demo@tinycld.org` and `demo-teammate@tinycld.org`, both
 * is_demo=true) are preserved across resets — only the records they own are
 * wiped, then every linked package's seed() re-creates them.
 *
 * TWO users, not one: the demo shows off shared boards, shared calendars and
 * shared files, which need a second person to point at. The seeds used to
 * resolve that person by querying `users` for `id != demo`, so on a deployment
 * with real accounts they wrote mailboxes, memberships and shares owned by
 * strangers — rows a demo-scoped reset could never reclaim, accumulating on
 * every nightly run. Seeding a dedicated companion makes the demo data set
 * exactly two users wide, so this script's blast radius equals what the seeds
 * actually created. See SeedContext.companion in core/lib/packages/config-types.ts.
 *
 * Single-org: there is no `orgs` row to delete and no cascade to ride. Data
 * is reached through each collection's ownership FK, which now points at
 * `users` directly. This is why the wipe enumerates collections explicitly.
 *
 * Usage:
 *   pnpm exec tsx scripts/reset-demo.ts [options]
 *
 * Options:
 *   --url <url>            PocketBase URL (default: http://127.0.0.1:7100)
 *   --admin-email <email>  Superuser email
 *   --admin-pw <pw>        Superuser password
 *   --help                 Show this help message
 */

import { loadEnv } from '@tinycld/core/lib/load-env'
import type PocketBase from 'pocketbase'
import { authSuperuser, DEMO_COMPANION_DEFAULTS, seedForUser } from './seed-db'

function log(...args: unknown[]) {
    process.stdout.write(`[reset-demo] ${args.join(' ')}\n`)
}

function logError(...args: unknown[]) {
    process.stderr.write(`[reset-demo] ${args.join(' ')}\n`)
}

loadEnv()

// Mirror demo_start.go constants exactly. REVIEW_DEMO_EMAIL overrides the
// email so App Review can sign in directly; if it differs from
// demo@tinycld.org, demo_start.go's demoUserEmail constant must be patched
// too or the demo-token flow won't find the user.
const DEMO_USER_EMAIL = process.env.REVIEW_DEMO_EMAIL || 'demo@tinycld.org'
const DEMO_USER_USERNAME = 'demo'
const DEMO_USER_NAME = 'Demo Tour'

// The companion that owns the other half of the shared fixtures. Imported from
// seed-db (which creates it) rather than re-declared here, so the identity this
// script wipes cannot drift from the one that gets seeded — a mismatch would
// silently orphan the companion's data forever.
const DEMO_COMPANION_USERNAME = DEMO_COMPANION_DEFAULTS.username
const DEMO_COMPANION_NAME = DEMO_COMPANION_DEFAULTS.name

// Collections holding demo-owned data, paired with the FK naming the owner.
// Order matters: children before parents, so a cascade or a required-relation
// guard never blocks a delete.
//
// A collection absent from this deployment (its package isn't linked) is
// skipped by name — deliberately, via hasCollection. Errors are NOT swallowed:
// this script previously wrapped its lookup in a bare catch, which turned the
// removal of the `orgs` collection into a silent no-op that reported success
// while wiping nothing, nightly. Any other failure must be loud.
const OWNED_COLLECTIONS: Array<{ collection: string; ownerField: string }> = [
    // label_assignments is polymorphic: `record_id` is a plain text id with no
    // FK, so NOTHING cascades into it. Mail creates one per labeled thread
    // (mail/seed.ts), and before this entry existed they survived every reset
    // and accumulated nightly, forever. Must run first — once the thread state
    // it points at is gone, the row is unreachable garbage.
    { collection: 'label_assignments', ownerField: 'user' },
    // Comments and mentions reference drive_items + users; clear them first.
    { collection: 'comment_mentions', ownerField: 'mentioned_user' },
    { collection: 'text_comments', ownerField: 'author' },
    { collection: 'calc_comments', ownerField: 'author' },
    { collection: 'drive_share_links', ownerField: 'created_by' },
    { collection: 'drive_item_versions', ownerField: 'created_by' },
    // created_by, NOT user: the seed sets `user` to the share RECIPIENT and
    // `created_by` to the sharer (drive/seed.ts). Keying on `user` missed every
    // share the demo user granted — they were reclaimed only when the recipient
    // happened to be wiped too.
    { collection: 'drive_shares', ownerField: 'created_by' },
    { collection: 'drive_item_state', ownerField: 'user' },
    { collection: 'drive_items', ownerField: 'created_by' },
    { collection: 'contacts', ownerField: 'owner' },
    // Only labels with an owner. Mail seeds its labels with NO `user` — they are
    // deliberately shared, visible to everyone (mail/seed.ts) — so this filter
    // skips them and the shared set survives the reset. That is the intended
    // outcome; the seed find-or-creates them by name, so a surviving row is
    // reused rather than duplicated. Contacts' labels DO set `user` and are
    // reclaimed here.
    { collection: 'labels', ownerField: 'user' },
    { collection: 'calendar_events', ownerField: 'created_by' },
    { collection: 'notifications', ownerField: 'user' },
]

// Mail hangs off a membership junction rather than a per-record owner FK:
// mail_mailbox_members.user → mailbox, and mail_threads.mailbox /
// mail_messages.thread / mail_thread_state / mail_folder_counts all declare
// cascadeDelete on that chain. So deleting the mailboxes the demo user
// belongs to clears every thread and message under them — a flat
// ownerField entry cannot express this (mail_messages has no user column at
// all, and mail_mailboxes has no user column either).
const MAIL_MEMBERSHIP = { junction: 'mail_mailbox_members', mailboxes: 'mail_mailboxes' }

// Boards and calendar hang off a root record that has no usable owner FK, so a
// flat OWNED_COLLECTIONS entry cannot express them — and until now neither
// package was wiped by this script AT ALL. Their demo data survived every reset.
//
// - calendar_calendars has NO owner column whatsoever; ownership lives only in
//   the calendar_members junction. (This script used to list it with
//   ownerField: 'created_by', a field that does not exist — a silent no-op.)
// - boards_projects HAS created_by, but the seed deliberately points it at the
//   teammate for the "Team retrospective" board, so filtering on created_by
//   alone leaves a whole board behind.
//
// Both roots cascade cleanly: deleting the project/calendar row removes every
// list, card, checklist item, comment, attachment, share link, member and event
// beneath it (verified against each package's create migration). So resolving
// the root ids through membership and deleting those is sufficient and complete.
const MEMBERSHIP_ROOTS: Array<{ junction: string; rootField: string; roots: string }> = [
    { junction: 'boards_project_members', rootField: 'project', roots: 'boards_projects' },
    { junction: 'calendar_members', rootField: 'calendar', roots: 'calendar_calendars' },
]

function parseArgs() {
    const args = process.argv.slice(2)
    if (args.includes('--help')) process.exit(0)

    let url = 'http://127.0.0.1:7100'
    let adminEmail = process.env.ADMIN_USER_LOGIN || 'admin@tinycld.org'
    // No hardcoded fallback — this authenticates as an existing superuser, so a
    // baked-in default would ship a known admin password and only work against
    // an admin created with it. Require --admin-pw or ADMIN_USER_PW.
    let adminPassword = process.env.ADMIN_USER_PW || ''

    for (let i = 0; i < args.length; i++) {
        const arg = args[i]
        switch (arg) {
            case '--url':
                url = args[++i]
                break
            case '--admin-email':
                adminEmail = args[++i]
                break
            case '--admin-pw':
                adminPassword = args[++i]
                break
            default:
                if (arg.startsWith('-')) {
                    logError(`Unknown flag: ${arg}`)
                    process.exit(1)
                }
        }
    }

    if (!adminPassword) {
        logError('Superuser password required: pass --admin-pw <pw> or set ADMIN_USER_PW.')
        process.exit(1)
    }

    return { url, adminEmail, adminPassword }
}

// isNotFound distinguishes "this deployment doesn't ship that collection" from
// a real failure. Only a 404 is tolerated, and only when probing existence.
function isNotFound(err: unknown): boolean {
    return (err as { status?: number })?.status === 404
}

// hasCollection probes for a collection, tolerating only a 404. Any other
// error (auth, network, a server-side guard) propagates — a reset that cannot
// see the schema must not claim to have wiped it.
async function hasCollection(pb: PocketBase, name: string): Promise<boolean> {
    try {
        await pb.collections.getOne(name)
        return true
    } catch (err) {
        if (isNotFound(err)) return false
        throw err
    }
}

async function findUserByUsername(
    pb: PocketBase,
    username: string
): Promise<{ id: string } | null> {
    try {
        return await pb.collection('users').getFirstListItem(`username = "${username}"`)
    } catch (err) {
        if (isNotFound(err)) return null
        throw err
    }
}

// wipeOwnedData deletes every record the demo user owns, across the
// collections this deployment actually ships. Returns the number deleted so
// the caller can log a real figure rather than an unverifiable "complete".
async function wipeOwnedData(pb: PocketBase, userId: string): Promise<number> {
    let deleted = 0
    for (const { collection, ownerField } of OWNED_COLLECTIONS) {
        if (!(await hasCollection(pb, collection))) continue

        // getFullList over a filtered set, then delete by id. PocketBase has no
        // bulk-delete-by-filter, and paging while mutating skips records.
        const rows = await pb
            .collection(collection)
            .getFullList({ filter: `${ownerField} = "${userId}"`, fields: 'id' })
        if (rows.length === 0) continue

        log(`Wiping ${rows.length} ${collection} record(s)`)
        for (const row of rows) {
            // A cascade from an earlier collection may already have removed
            // this row; that 404 is expected. Anything else is a real failure.
            try {
                await pb.collection(collection).delete(row.id)
                deleted++
            } catch (err) {
                if (!isNotFound(err)) throw err
            }
        }
    }

    deleted += await wipeMembershipRoots(pb, userId)
    deleted += await wipeMailboxes(pb, userId)
    return deleted
}

// wipeMembershipRoots deletes the boards and calendars the user belongs
// to, letting each package's cascade clear everything beneath. See
// MEMBERSHIP_ROOTS for why these can't be expressed as a flat owner filter.
//
// Membership (not created_by) is the key deliberately: it catches the
// teammate-owned board and the calendars the user is only an editor/viewer of,
// which an ownership filter misses. That is safe HERE — and only here —
// because the demo workspace's users are seed-created and every board or
// calendar they belong to is seed data by construction. This script must never
// be pointed at a deployment with real accounts.
async function wipeMembershipRoots(pb: PocketBase, userId: string): Promise<number> {
    let deleted = 0
    for (const { junction, rootField, roots } of MEMBERSHIP_ROOTS) {
        if (!(await hasCollection(pb, junction))) continue
        if (!(await hasCollection(pb, roots))) continue

        const memberships = await pb
            .collection(junction)
            .getFullList({ filter: `user = "${userId}"`, fields: `id,${rootField}` })
        const rootIds = [...new Set(memberships.map(m => m[rootField] as string).filter(Boolean))]
        if (rootIds.length === 0) continue

        log(`Wiping ${rootIds.length} ${roots} record(s) (contents cascade)`)
        for (const id of rootIds) {
            try {
                await pb.collection(roots).delete(id)
                deleted++
            } catch (err) {
                if (!isNotFound(err)) throw err
            }
        }
    }
    return deleted
}

// restorePersonalCalendar re-creates the calendar that calendar's lifecycle
// hook provisions on user-create (calendar/server/lifecycle.go).
//
// That hook fires ONLY on create. The demo users are preserved across resets by
// design, so the hook never re-fires — and wipeMembershipRoots deletes the
// personal calendar along with the seeded ones, since the user owns it. Without
// this the demo user ends up with no default calendar, and every subsequent
// reset leaves them worse off.
//
// Mirrors the hook's shape exactly: calendar + owner membership, name from the
// user's display name. Idempotent — an existing owned calendar is left alone.
async function restorePersonalCalendar(
    pb: PocketBase,
    userId: string,
    userName: string
): Promise<void> {
    if (!(await hasCollection(pb, 'calendar_calendars'))) return
    if (!(await hasCollection(pb, 'calendar_members'))) return

    const owned = await pb
        .collection('calendar_members')
        .getFullList({ filter: `user = "${userId}" && role = "owner"`, fields: 'id' })
    if (owned.length > 0) return

    const calendar = await pb.collection('calendar_calendars').create({
        name: userName,
        description: '',
        color: 'blue',
    })
    await pb.collection('calendar_members').create({
        calendar: calendar.id,
        user: userId,
        role: 'owner',
    })
    log(`Restored personal calendar for ${userId}`)
}

// wipeMailboxes clears the demo user's mail by deleting the mailboxes they
// are a member of; threads, messages and per-thread state cascade from there.
async function wipeMailboxes(pb: PocketBase, userId: string): Promise<number> {
    if (!(await hasCollection(pb, MAIL_MEMBERSHIP.junction))) return 0
    if (!(await hasCollection(pb, MAIL_MEMBERSHIP.mailboxes))) return 0

    const memberships = await pb
        .collection(MAIL_MEMBERSHIP.junction)
        .getFullList({ filter: `user = "${userId}"`, fields: 'id,mailbox' })
    const mailboxIds = [...new Set(memberships.map(m => m.mailbox as string).filter(Boolean))]
    if (mailboxIds.length === 0) return 0

    log(`Wiping ${mailboxIds.length} mailbox(es) (threads + messages cascade)`)
    let deleted = 0
    for (const id of mailboxIds) {
        try {
            await pb.collection(MAIL_MEMBERSHIP.mailboxes).delete(id)
            deleted++
        } catch (err) {
            if (!isNotFound(err)) throw err
        }
    }
    return deleted
}

// wipeOrphanedRealtimeJournal clears realtime_doc_updates rows whose room is
// gone. The journal has no owner FK and nothing cascades into it (it outlives
// its room by design so a crashed room can replay), so the per-collection
// wipe above never touches it and rows for deleted rooms accumulate forever.
// Rooms are drive_items; keying on room existence rather than this run's
// deletions also drains rows leaked by resets that ran before this cleanup
// existed. Must run AFTER the drive_items wipe so those rooms read as gone.
async function wipeOrphanedRealtimeJournal(pb: PocketBase): Promise<number> {
    if (!(await hasCollection(pb, 'realtime_doc_updates'))) return 0

    const rows = await pb
        .collection('realtime_doc_updates')
        .getFullList<{ id: string; room_id: string }>({ fields: 'id,room_id' })
    if (rows.length === 0) return 0

    const liveRooms = new Set<string>()
    for (const roomId of new Set(rows.map(r => r.room_id).filter(Boolean))) {
        try {
            await pb.collection('drive_items').getOne(roomId, { fields: 'id' })
            liveRooms.add(roomId)
        } catch (err) {
            if (!isNotFound(err)) throw err
        }
    }

    let deleted = 0
    for (const row of rows) {
        if (row.room_id && liveRooms.has(row.room_id)) continue
        try {
            await pb.collection('realtime_doc_updates').delete(row.id)
            deleted++
        } catch (err) {
            if (!isNotFound(err)) throw err
        }
    }
    return deleted
}

async function main() {
    const config = parseArgs()
    const pb = await authSuperuser(config)

    // Wipe the companion BEFORE the demo user. Shared fixtures (boards,
    // calendars, mailboxes) are reachable from either member, and clearing the
    // companion first means the demo user's pass finds a smaller, already
    // partly-cascaded set rather than re-walking the same roots.
    for (const username of [DEMO_COMPANION_USERNAME, DEMO_USER_USERNAME]) {
        const target = await findUserByUsername(pb, username)
        if (!target) {
            log(`No existing "${username}" user found — nothing to wipe`)
            continue
        }
        const deleted = await wipeOwnedData(pb, target.id)
        log(`Wiped ${deleted} record(s) owned by "${username}" (${target.id})`)
        // Before the seeds run: calendar's seed guard counts events, not
        // calendars, so it will happily seed into a user with no calendar at
        // all — leaving them without the default one the lifecycle hook would
        // have given a freshly-created account.
        await restorePersonalCalendar(
            pb,
            target.id,
            username === DEMO_USER_USERNAME ? DEMO_USER_NAME : DEMO_COMPANION_NAME
        )
    }

    // After BOTH users' drive_items are gone, so every room deleted in either
    // pass reads as orphaned. Running it inside wipeOwnedData would leave the
    // companion's journal rows behind whenever their items outlived the pass.
    const journalRows = await wipeOrphanedRealtimeJournal(pb)
    if (journalRows > 0) log(`Wiped ${journalRows} orphaned realtime_doc_updates record(s)`)

    // seedForUser handles the find-or-create dance for the user and runs every
    // linked package's seed() against the demo workspace.
    await seedForUser(pb, {
        url: config.url,
        adminEmail: config.adminEmail,
        adminPassword: config.adminPassword,
        adminPasswordGenerated: false,
        mode: 'demo',
        userEmail: DEMO_USER_EMAIL,
        userUsername: DEMO_USER_USERNAME,
        userName: DEMO_USER_NAME,
        // Passing through REVIEW_DEMO_PASSWORD here keeps App Review's demo
        // creds stable across nightly resets. When unset, seedForUser falls
        // back to a random password (the normal /api/demo/start flow doesn't
        // need a known password).
        userPassword: process.env.REVIEW_DEMO_PASSWORD ?? '',
        userPasswordExplicit: (process.env.REVIEW_DEMO_PASSWORD ?? '') !== '',
        isDemo: true,
    })

    log('Demo reset complete')
    process.exit(0)
}

main().catch(err => {
    logError('Failed:', err)
    if (err?.response) {
        logError('Response:', JSON.stringify(err.response, null, 2))
    }
    process.exit(1)
})
