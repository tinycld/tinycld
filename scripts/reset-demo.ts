#!/usr/bin/env -S pnpm exec tsx
/**
 * Demo Reset Script
 *
 * Wipes all data owned by the singleton demo user and re-seeds it. Designed
 * to run nightly (see coreserver/demo_reset.go) so the demo workspace is
 * always pristine for the next unauthenticated visitor that hits
 * /api/demo/start.
 *
 * The demo *user* (`demo@tinycld.org`, is_demo=true) is preserved across
 * resets — only the records they own are wiped, then every linked package's
 * seed() re-creates them.
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
import { authSuperuser, seedForUser } from './seed-db'

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
    // Comments and mentions reference drive_items + users; clear them first.
    { collection: 'comment_mentions', ownerField: 'mentioned_user' },
    { collection: 'text_comments', ownerField: 'author' },
    { collection: 'calc_comments', ownerField: 'author' },
    { collection: 'drive_share_links', ownerField: 'created_by' },
    { collection: 'drive_item_versions', ownerField: 'created_by' },
    { collection: 'drive_shares', ownerField: 'user' },
    { collection: 'drive_items', ownerField: 'created_by' },
    { collection: 'contacts', ownerField: 'owner' },
    { collection: 'labels', ownerField: 'user' },
    { collection: 'calendar_events', ownerField: 'created_by' },
    { collection: 'calendar_calendars', ownerField: 'created_by' },
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

async function findDemoUser(pb: PocketBase): Promise<{ id: string } | null> {
    try {
        return await pb.collection('users').getFirstListItem(`username = "${DEMO_USER_USERNAME}"`)
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

    deleted += await wipeMailboxes(pb, userId)
    return deleted
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

async function main() {
    const config = parseArgs()
    const pb = await authSuperuser(config)

    const demoUser = await findDemoUser(pb)
    if (demoUser) {
        const deleted = await wipeOwnedData(pb, demoUser.id)
        log(`Wiped ${deleted} record(s) owned by demo user ${demoUser.id}`)
    } else {
        log('No existing demo user found — nothing to wipe, will create fresh')
    }

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
