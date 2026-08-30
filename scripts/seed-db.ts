#!/usr/bin/env -S pnpm exec tsx

/**
 * Database Seed Script
 *
 * Populates the PocketBase database with a target user, org, and package data.
 *
 * Usage:
 *   pnpm exec tsx scripts/seed-db.ts [options]
 *
 * Options:
 *   --mode <test|demo>     Preset (default: test)
 *                            test: user@tinycld.org + test-org (+ acme second org)
 *                            demo: demo@tinycld.org (is_demo=true) + demo org, single org
 *   --user-email <email>   Override user email
 *   --user-name <name>     Override user display name
 *   --user-pw <pw>         Override user password. In demo mode also honors
 *                          $REVIEW_DEMO_PASSWORD from .env, so the App Review
 *                          reviewer account has a stable password.
 *
 * Demo mode also honors $REVIEW_DEMO_EMAIL for the user's email field. Keep
 * it set to the demo singleton ("demo@tinycld.org") unless you also patch
 * demo_start.go's demoUserEmail constant; the demo-token flow looks the user
 * up by that email.
 *   --url <url>            PocketBase URL (default: http://127.0.0.1:7100)
 *   --admin-email <email>  Superuser email
 *   --admin-pw <pw>        Superuser password
 *   --help                 Show this help message
 */

import { deriveUsername } from '@tinycld/core/lib/derive-username'
import { loadEnv } from '@tinycld/core/lib/load-env'
import type { SeedContext, SeedUser } from '@tinycld/core/lib/packages/config-types'
import { deriveSeeds } from '@tinycld/core/lib/packages/derive-seeds'
import PocketBase from 'pocketbase'
import { tinycldSeeds } from '../tinycld.seeds'
import { printBox } from './print-box'

function log(...args: unknown[]) {
    process.stdout.write(`[seed] ${args.join(' ')}\n`)
}

function logError(...args: unknown[]) {
    process.stderr.write(`[seed] ${args.join(' ')}\n`)
}

loadEnv()

type SeedMode = 'test' | 'demo'

interface SeedConfig {
    url: string
    adminEmail: string
    adminPassword: string
    // True only when the caller (reset-dev-db.ts) randomly generated the admin
    // password and asked us to surface it. A password supplied via ADMIN_USER_PW
    // / --admin-pw (env/CI/flag) is a secret we must NOT echo — see
    // printLoginSummary. Defaults to false: seed-db never generates it itself.
    adminPasswordGenerated: boolean
    mode: SeedMode
    userEmail: string
    userUsername: string
    userName: string
    userPassword: string
    // True when the password came from an explicit source (--user-pw, or the
    // TEST_USER_PW / REVIEW_DEMO_PASSWORD env vars) rather than being left
    // unset. When false and we're creating a brand-new user, we mint a random
    // password instead of falling back to a shared literal — see seedForUser.
    userPasswordExplicit: boolean
    isDemo: boolean
}

// What seedForUser resolved for the app user, so the caller can print an
// accurate login summary. `password` is null when the user already existed and
// we left their credential untouched (we can't know it). `passwordGenerated` is
// true only when we minted a random password; an explicitly-supplied one
// (--user-pw / TEST_USER_PW / REVIEW_DEMO_PASSWORD) is a secret we don't echo.
interface SeedLoginResult {
    created: boolean
    userEmail: string
    password: string | null
    passwordGenerated: boolean
}

// The password the e2e login helper uses when TEST_USER_PW is unset. It MUST
// match TEST_USER_PASSWORD in tests/e2e/helpers.ts: playwright.config.ts never
// loads .env, so on a clean checkout without TEST_USER_PW the seed and the
// helper have to agree on a shared literal or every login() fails. An explicit
// TEST_USER_PW (e.g. from CI) still overrides this for both sides.
const TEST_USER_DEFAULT_PASSWORD = 'TestUser1234!'

const TEST_DEFAULTS = {
    userEmail: process.env.TEST_USER_LOGIN || 'user@tinycld.org',
    userUsername: process.env.TEST_USER_USERNAME || 'tester',
    userName: 'Test User',
}

// A SECOND ordinary member, seeded ready-to-use so a spec that needs "another
// person on the board" can just sign them in.
//
// Before this existed the only way to get a second account was
// createInvitedUser(), which drives the whole invite arc through the UI: owner
// → Settings → Members → Invite → extract token → NEW browser context →
// /accept-invite → set password → wait for the shell. Measured, that is ~4.4s
// on a fast dev machine, and it strands the owner's page on Settings so every
// caller then pays another ~1.4s login() to get back. ~5.8s of setup before a
// single assertion — which is why seven cards specs carried test.slow(), and
// why all of them timed out on CI once the slower, contended runner multiplied
// that baseline. The marker was the symptom; this fixture is the cause.
//
// Seeding it is not a rules bypass: provisioning fixtures is exactly the seed
// layer's job (the fixture owner and each package's demo data arrive the same
// way). The invite FLOW keeps its own dedicated coverage in
// tests/e2e/invite-flow.spec.ts, and specs that genuinely need to mint a
// throwaway account (change-password, password-reset, admin-role-access) still
// call createInvitedUser.
//
// role is 'member', deliberately NOT 'owner': a second owner would silently
// weaken every last-owner and role-gate assertion that relies on the fixture
// owner being the only one.
const COLLABORATOR_DEFAULTS = {
    email: process.env.TEST_COLLABORATOR_LOGIN || 'collaborator@tinycld.org',
    username: process.env.TEST_COLLABORATOR_USERNAME || 'collaborator',
    name: 'Collaborator Tester',
}

// Demo mode's equivalent of the collaborator. The demo tour shows off shared
// boards, shared calendars and shared files, all of which need a second person
// to point at. Before this existed the seeds resolved that person by querying
// `users` for `id != demo`, which meant they wrote mailboxes, memberships and
// shares owned by whoever else happened to exist — rows the nightly reset
// (scoped to the demo user) could never reclaim.
//
// Seeding a dedicated companion instead makes the demo's data set exactly two
// users wide, so reset-demo.ts can wipe both and return the workspace to a
// known state. is_demo is set so the account is recognizably part of the demo
// singleton and never mistaken for a real signup.
export const DEMO_COMPANION_DEFAULTS = {
    email: 'demo-teammate@tinycld.org',
    username: 'demo-teammate',
    name: 'Sam Rivera',
}

// These mirror the singleton constants in
// core/server/coreserver/demo_start.go. Keep in sync.
// REVIEW_DEMO_EMAIL overrides userEmail at runtime; if you set it to
// something other than demo@tinycld.org you must also patch demo_start.go's
// demoUserEmail constant or the demo-token flow won't find the user.
const DEMO_DEFAULTS = {
    userEmail: process.env.REVIEW_DEMO_EMAIL || 'demo@tinycld.org',
    userUsername: 'demo',
    userName: 'Demo Tour',
}

function parseArgs(): SeedConfig {
    const args = process.argv.slice(2)
    let mode: SeedMode = 'test'
    const overrides: Partial<{
        userEmail: string
        userUsername: string
        userName: string
        userPassword: string
    }> = {}

    let url = 'http://127.0.0.1:7100'
    let adminEmail = process.env.ADMIN_USER_LOGIN || 'admin@tinycld.org'
    // No hardcoded fallback — this script authenticates as an EXISTING superuser,
    // so a baked-in default would (a) ship a known admin password and (b) only
    // ever work against an admin created with that same default. Require it via
    // --admin-pw or ADMIN_USER_PW. reset-dev-db.ts passes the (possibly random)
    // password it created the superuser with; CI sets ADMIN_USER_PW.
    let adminPassword = process.env.ADMIN_USER_PW || ''
    // reset-dev-db.ts appends --admin-pw-generated (after its --admin-pw) when it
    // randomly generated the admin password, so the login summary may safely echo
    // it. A password supplied via ADMIN_USER_PW / --admin-pw (env/CI/flag) is a
    // secret and stays unset here (default: not generated).
    let adminPasswordGenerated = false

    if (args.includes('--help')) process.exit(0)

    for (let i = 0; i < args.length; i++) {
        const arg = args[i]
        switch (arg) {
            case '--mode': {
                const next = args[++i]
                if (next !== 'test' && next !== 'demo') {
                    logError(`Invalid --mode: ${next} (expected: test|demo)`)
                    process.exit(1)
                }
                mode = next
                break
            }
            case '--user-email':
                overrides.userEmail = args[++i]
                break
            case '--user-username':
                overrides.userUsername = args[++i]
                break
            case '--user-name':
                overrides.userName = args[++i]
                break
            case '--user-pw':
                overrides.userPassword = args[++i]
                break
            case '--url':
                url = args[++i]
                break
            case '--admin-email':
                adminEmail = args[++i]
                break
            case '--admin-pw':
                adminPassword = args[++i]
                // An explicitly passed password is caller-supplied, not generated.
                adminPasswordGenerated = false
                break
            case '--admin-pw-generated':
                adminPasswordGenerated = true
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

    const defaults = mode === 'demo' ? DEMO_DEFAULTS : TEST_DEFAULTS

    // An explicit password comes from --user-pw, or from the mode's env var
    // (TEST_USER_PW in test mode — set by CI — or REVIEW_DEMO_PASSWORD in demo
    // mode for App Review). In test mode, fall back to the shared default the
    // e2e login helper reads (TEST_USER_DEFAULT_PASSWORD) so a clean checkout
    // without TEST_USER_PW still logs in — the helper and seed MUST agree on
    // that literal. Demo mode has no shared login helper, so when
    // REVIEW_DEMO_PASSWORD is unset userPassword stays '' and seedForUser mints
    // a random one on create rather than reusing a known literal.
    const envPassword =
        mode === 'test'
            ? (process.env.TEST_USER_PW ?? TEST_USER_DEFAULT_PASSWORD)
            : (process.env.REVIEW_DEMO_PASSWORD ?? '')
    const userPassword = overrides.userPassword ?? envPassword

    return {
        url,
        adminEmail,
        adminPassword,
        adminPasswordGenerated,
        mode,
        userEmail: overrides.userEmail ?? defaults.userEmail,
        userUsername: overrides.userUsername ?? defaults.userUsername,
        userName: overrides.userName ?? defaults.userName,
        userPassword,
        userPasswordExplicit: userPassword !== '',
        isDemo: mode === 'demo',
    }
}

// A random password that satisfies PocketBase's auth rules (>=8 chars, mixed
// case + digit + symbol). Used when seeding a brand-new local user with no
// explicit password configured, so each fresh checkout gets a unique
// credential that's printed once rather than a shared literal.
function generatePassword(): string {
    const rand = () => Math.random().toString(36).slice(2, 10)
    return `Dev${rand()}${rand()}!`
}

// PocketBase's `getFirstListItem` rejects with a 404 ClientResponseError when
// no record matches. We need to distinguish that from genuine errors (auth
// failures, server-side guards, network issues) so we don't accidentally
// fall through to a `create` that masks the real cause.
function isNotFoundError(err: unknown): boolean {
    if (err && typeof err === 'object' && 'status' in err) {
        return (err as { status: number }).status === 404
    }
    return false
}

/**
 * Find-or-create the target user (single-org: the `role` enum lives on the user
 * record; the orgs/user_org collections are gone) plus the companion that owns
 * the other half of the shared fixtures, then run all linked package seeds
 * against both.
 *
 * Exported so reset-demo.ts can re-seed without shelling out.
 */
export async function seedForUser(pb: PocketBase, config: SeedConfig): Promise<SeedLoginResult> {
    // Find first; only branch into create when the lookup specifically returns
    // nothing. Catching around the update too would mask a real failure (e.g.
    // a server-side guard rejecting the write) and silently fall through to a
    // duplicate-create attempt, which then fails with a confusing
    // "username must be unique" error instead of the underlying cause.
    let existingUser: { id: string } | null = null
    try {
        existingUser = await pb
            .collection('users')
            .getFirstListItem(`username = "${config.userUsername}"`)
    } catch (err) {
        if (!isNotFoundError(err)) throw err
    }

    let user: { id: string; role?: string }
    const login: SeedLoginResult = {
        created: !existingUser,
        userEmail: config.userEmail,
        password: null,
        passwordGenerated: false,
    }
    if (existingUser) {
        user = existingUser
        log('Found existing user:', config.userUsername)
        // A pre-existing non-demo user keeps whatever password it already has;
        // we don't know (and won't reset) it. If a password was passed
        // explicitly the caller knows it, so report that; otherwise null.
        if (config.userPasswordExplicit) login.password = config.userPassword
        if (config.isDemo) {
            // The singleton demo account is shared across all anonymous
            // visitors. Any field a previous visitor edited (name via direct
            // update, email/password via the confirmation endpoints) would
            // otherwise persist forever. Force-reset every visitor-mutable
            // field on each reset so the next session starts clean.
            //
            // Password: when REVIEW_DEMO_PASSWORD is set (or --user-pw was
            // passed) we set that stable password so App Review can sign in
            // directly. Otherwise reroll to a random value — the normal
            // /api/demo/start token flow doesn't need a known password.
            const newPassword = config.userPasswordExplicit
                ? config.userPassword
                : generatePassword()
            await pb.collection('users').update(user.id, {
                is_demo: true,
                email: config.userEmail,
                name: config.userName,
                password: newPassword,
                passwordConfirm: newPassword,
            })
            login.password = newPassword
            login.passwordGenerated = !config.userPasswordExplicit
        }
    } else {
        log('Creating user:', config.userUsername)
        // Auth collections require *some* password. When one was provided
        // explicitly (--user-pw, or the TEST_USER_PW / REVIEW_DEMO_PASSWORD env
        // vars — CI sets TEST_USER_PW so e2e logins keep working) we use it.
        // Otherwise mint a random one and surface it in the login summary so a
        // fresh local checkout has a unique, discoverable credential.
        const password = config.userPasswordExplicit ? config.userPassword : generatePassword()
        login.password = password
        login.passwordGenerated = !config.userPasswordExplicit
        user = await pb.collection('users').create({
            username: config.userUsername,
            email: config.userEmail,
            password,
            passwordConfirm: password,
            name: config.userName,
            emailVisibility: true,
            verified: true,
            // `role` is required (1940000000_backfill_and_require_users_role), so
            // it must be set on the create itself — the `owner` stamp below is an
            // update and runs too late to satisfy validation.
            role: 'owner',
            ...(config.isDemo ? { is_demo: true } : {}),
        })
    }

    // Single-org: the `role` enum moved onto the users record (the orgs/user_org
    // collections are gone). Stamp the seeded user as owner so role-gated UI and
    // RLS (@request.auth.role) resolve.
    if (user.role !== 'owner') {
        log('Setting user role to "owner"')
        user = await pb.collection('users').update(user.id, { role: 'owner' })
    }

    // Created here rather than in main() so BOTH entry points get a companion —
    // reset-demo.ts calls seedForUser directly, and a demo re-seeded without one
    // would fall back to resolving teammates from real accounts, reintroducing
    // exactly the unreclaimable rows this refactor removes.
    const companion = await ensureCompanionUser(pb, config, companionPassword(config))

    const seedContext: SeedContext = {
        user: {
            id: user.id,
            username: config.userUsername,
            email: config.userEmail,
            name: config.userName,
        },
        companion,
    }
    const orderedSeeds = deriveSeeds(tinycldSeeds)
    log(`Running ${orderedSeeds.length} package seed(s)...`)
    for (const { slug, seed: seedFn } of orderedSeeds) {
        log(`  → ${slug}`)
        await seedFn(pb, seedContext)
        log(`  ✓ ${slug} done`)
    }

    return login
}

// Make the admin a REAL app user, not just a PocketBase _superusers record.
// `reset-dev-db.ts` creates the admin via `superuser upsert`, which only writes
// _superusers — so without this the admin can sign into the /admin console (PB
// superuser path) but has no `users` row, no app-shell login, and no membership.
// This mirrors what the first-boot wizard now does (handleSetupInit →
// createOwnerOperator): create a `users` record for the admin email with
// role=owner, so admin@ is a usable app user with the same authority a real
// operator gets. Idempotent: skips creation when the row exists.
async function ensureAdminAppUser(pb: PocketBase, config: SeedConfig): Promise<void> {
    let existing: { id: string } | null = null
    try {
        existing = await pb.collection('users').getFirstListItem(`email = "${config.adminEmail}"`)
    } catch (err) {
        if (!isNotFoundError(err)) throw err
    }

    let adminUser: { id: string }
    if (existing) {
        adminUser = existing
        log('Found existing admin app user:', config.adminEmail)
    } else {
        log('Creating admin app user:', config.adminEmail)
        adminUser = await pb.collection('users').create({
            username: deriveUsername(config.adminEmail),
            email: config.adminEmail,
            password: config.adminPassword,
            passwordConfirm: config.adminPassword,
            name: config.adminEmail.split('@')[0],
            emailVisibility: true,
            verified: true,
            // `role` is required (1940000000_backfill_and_require_users_role).
            // owner, matching createOwnerOperator in setup_bootstrap.go: the
            // person who provisioned the deployment is its owner, and a seeded
            // dev DB must behave like one a real operator set up through the
            // first-boot wizard. Seeding `admin` here left the admin login
            // unable to reach owner-only surfaces (Settings > Packages, Build
            // History) on an otherwise-fresh deployment. Specs needing an
            // admin-vs-owner contrast mint their own admin through the invite
            // flow (admin-role-access.spec.ts) rather than relying on this
            // fixture.
            role: 'owner',
        })
    }

    // An existing fixture may predate the role change (seeded as `member` when
    // authority came from a separate grant, or as `admin` before this account
    // was aligned with the setup wizard's owner operator). Bring it up to date
    // so a re-seeded dev DB behaves like a fresh one.
    if (existing) {
        const current = await pb.collection('users').getOne(adminUser.id)
        if (current.role !== 'owner') {
            log('Setting admin app user role to "owner"')
            await pb.collection('users').update(adminUser.id, { role: 'owner' })
        }
    }
}

/**
 * The companion's password, resolved the same way the fixture user's is.
 *
 * TEST_COLLABORATOR_PW wins so CI can set it; otherwise the collaborator shares
 * the fixture user's password. Falling back to the SAME literal the e2e helper
 * hardcodes is what makes a clean checkout work at all — playwright.config.ts
 * never loads .env, so seed and helper can only meet at a shared constant.
 *
 * The demo companion is never signed into (it exists only to own the other half
 * of the shared fixtures), so it gets a fresh random password each reset rather
 * than a predictable literal on a public deployment.
 */
function companionPassword(config: SeedConfig): string {
    if (config.isDemo) return generatePassword()
    return (
        process.env.TEST_COLLABORATOR_PW ||
        (config.userPasswordExplicit ? config.userPassword : TEST_USER_DEFAULT_PASSWORD)
    )
}

/**
 * Find-or-create the seeded companion — the collaborator in test mode, the demo
 * teammate in demo mode (see COLLABORATOR_DEFAULTS / DEMO_COMPANION_DEFAULTS).
 *
 * Runs BEFORE seedForUser and its record is passed through as
 * SeedContext.companion, so package seeds that need a second person (cards'
 * teammate-owned board, calendar's shared calendars, drive's share targets)
 * resolve to THIS user rather than querying for any other account. That is what
 * bounds the seeded data to two users and makes the demo reset able to reclaim
 * all of it.
 *
 * Idempotent in the same shape as ensureAdminAppUser: an existing row keeps its
 * id but has its password reset to the known fixture value, because a
 * re-seeded DB must be signable-into with the credential the e2e helper
 * hardcodes — a row left with an unknown password would fail every
 * signInAsCollaborator() with no clue why.
 *
 * role is 'member', deliberately NOT 'owner': a second owner would silently
 * weaken every last-owner and role-gate assertion that relies on the fixture
 * owner being the only one.
 */
async function ensureCompanionUser(
    pb: PocketBase,
    config: SeedConfig,
    password: string
): Promise<SeedUser> {
    const defaults = config.isDemo ? DEMO_COMPANION_DEFAULTS : COLLABORATOR_DEFAULTS
    const fields = {
        email: defaults.email,
        name: defaults.name,
        password,
        passwordConfirm: password,
        role: 'member',
        // The demo companion carries is_demo so it is recognizable as part of
        // the demo singleton rather than a real signup, and so reset-demo.ts can
        // find it the same way it finds the demo user.
        ...(config.isDemo ? { is_demo: true } : {}),
    }

    let existing: { id: string } | null = null
    try {
        existing = await pb
            .collection('users')
            .getFirstListItem(`username = "${defaults.username}"`)
    } catch (err) {
        if (!isNotFoundError(err)) throw err
    }

    if (existing) {
        log('Found existing companion user:', defaults.username)
        await pb.collection('users').update(existing.id, fields)
        return { id: existing.id, ...defaults }
    }

    log('Creating companion user:', defaults.username)
    const created = await pb.collection('users').create({
        username: defaults.username,
        ...fields,
        // The share dialog searches people by email; a companion whose email
        // is hidden cannot be found and added through the real UI.
        emailVisibility: true,
        verified: true,
    })
    return { id: created.id, ...defaults }
}

export async function authSuperuser(config: {
    url: string
    adminEmail: string
    adminPassword: string
}): Promise<PocketBase> {
    log('Connecting to PocketBase at', config.url)
    const pb = new PocketBase(config.url)
    log('Authenticating as superuser...')
    await pb.collection('_superusers').authWithPassword(config.adminEmail, config.adminPassword)
    return pb
}

// The URL a developer actually opens in the browser. The PB connection URL
// often binds 127.0.0.1; localhost reads better and is what the docs use. When
// a caller (reset-dev-db.ts) fronts PB with a proxy on a different port, it
// passes the real browse URL via $TINYCLD_BROWSE_URL so the printed links point
// where the user browses rather than at PB's internal port.
function browseUrl(pbUrl: string): string {
    const override = process.env.TINYCLD_BROWSE_URL
    if (override) return override.replace(/\/$/, '')
    return pbUrl.replace('127.0.0.1', 'localhost').replace(/\/$/, '')
}

// A supplied secret (from env/CI/flag) is never echoed — only a password we
// generated ourselves. This keeps the summary useful (it still confirms a
// password was set) without printing a credential the caller already knows.
const SUPPLIED_NOTICE = '(supplied — not shown; use the value you provided)'
const UNCHANGED_NOTICE = '(unchanged — use the password from when this user was created)'

function printLoginSummary(config: SeedConfig, login: SeedLoginResult): void {
    const url = browseUrl(config.url)
    const appPassword =
        login.password === null
            ? UNCHANGED_NOTICE
            : login.passwordGenerated
              ? login.password
              : SUPPLIED_NOTICE
    const adminPassword = config.adminPasswordGenerated ? config.adminPassword : SUPPLIED_NOTICE

    printBox([
        'Seed complete — log in with:',
        '',
        'App  (sign in to TinyCld)',
        `  ${url}`,
        `  user:     ${login.userEmail}`,
        `  password: ${appPassword}`,
        '',
        'Admin  (super-admin app user — signs into the app AND /admin to manage',
        'packages; also a PocketBase superuser for the /_/ dashboard)',
        `  ${url}/admin`,
        `  user:     ${config.adminEmail}`,
        `  password: ${adminPassword}`,
    ])
}

// Give the deployment a branding name, matching what the setup wizard (or the
// multi-org router's display_name materialization) would set in a real
// deployment. /api/org-info serves it and DocumentTitle renders it as the org
// segment — the document-title e2e asserts this exact value.
async function seedAppName(pb: PocketBase): Promise<void> {
    await pb.settings.update({ meta: { appName: 'Test Organization' } })
    log('Set settings meta.appName = "Test Organization"')
}

async function main() {
    const config = parseArgs()
    log(`Mode: ${config.mode} (user=${config.userEmail})`)
    const pb = await authSuperuser(config)
    await seedAppName(pb)
    await ensureAdminAppUser(pb, config)
    // The companion is created inside seedForUser (both entry points need it),
    // so there is nothing to provision here.
    const login = await seedForUser(pb, config)
    log('Seeding complete!')
    printLoginSummary(config, login)
    process.exit(0)
}

if (process.argv[1]?.endsWith('seed-db.ts')) {
    main().catch(err => {
        logError('Failed:', err)
        if (err?.response) {
            logError('Response:', JSON.stringify(err.response, null, 2))
        }
        process.exit(1)
    })
}
