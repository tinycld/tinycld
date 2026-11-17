import {
    type Browser,
    type BrowserContext,
    test as base,
    type Page,
    type WorkerInfo,
} from '@playwright/test'
import {
    acceptInvite,
    appShell,
    type InvitedUser,
    newInvitedUser,
    openMembers,
    sendInvite,
} from './helpers.ts'
import { settleRealtimeReleases, trackRealtimeClients } from './realtime-release.ts'

// Cheap throwaway members for specs whose rows must be apart from every other
// spec's (a report reads all the rows its reader may see, so a shared user sees
// the rows of every test before it).
//
// createInvitedUser pays for an owner sign-in, the walk to Settings → Members and
// the invitee's accept for EACH call, and a spec that seeds its rows after that
// pays for the seed in each test too. Screen changes, not assertions, are most of
// a spec's time: each one is a wait of most of a second. Two things here remove
// them:
//
//   - `ownerMembersPage` keeps ONE owner page on the Members screen for the
//     whole worker, so an invite is a dialog, not a sign-in.
//   - `shareMember` makes one member for a whole spec file and runs the file's
//     seed once. Each test then opens its own browser context already signed in
//     as that member (`p`), so nothing a test leaves in the browser reaches the
//     next one. Use it when the tests of a file read the same rows and add none
//     that another test would count.
//
// A test that changes the rows other tests read, or that needs rows of its own,
// takes `freshMember` instead: a new member for that test only.
//
// The `.ts` extensions are required: packages reach this file through
// @tinycld/core/e2e-helpers (see core/e2e-helpers.ts).

export interface MemberSession {
    user: InvitedUser
    /** The member's signed-in browser storage, for a new context. */
    storageState: Awaited<ReturnType<Awaited<ReturnType<Browser['newContext']>>['storageState']>>
}

export interface FreshMember {
    page: Page
    user: InvitedUser
}

// Playwright gives `use` options (baseURL, reduced motion) to contexts a TEST
// makes. A worker fixture and a beforeAll hook are set up outside a test, so
// they pass the options themselves.
function contextOptions(info: Pick<WorkerInfo, 'project'>) {
    const { baseURL, reducedMotion, viewport } = info.project.use
    return { baseURL, reducedMotion, viewport }
}

async function inviteMember(
    ownerMembersPage: Page,
    browser: Browser,
    info: Pick<WorkerInfo, 'project'>,
    label: string,
    name?: string
) {
    const user = newInvitedUser(label)
    const token = await sendInvite(ownerMembersPage, user)
    const context = await browser.newContext(contextOptions(info))
    trackRealtimeClients(context)
    const page = await context.newPage()
    await acceptInvite(page, token, user, name)
    return { user, context, page }
}

// Each context here is signed in, so its close leaves realtime clients that
// the fork keeps detached until they are released (see realtime-release.ts).
async function closeTracked(context: BrowserContext) {
    await context.close()
    await settleRealtimeReleases()
}

// The shared member of each spec file that called shareMember, by file path.
const shared = new Map<string, MemberSession>()

export const test = base.extend<{ p: Page; freshMember: FreshMember }, { ownerMembersPage: Page }>({
    ownerMembersPage: [
        async ({ browser }, use, workerInfo) => {
            const context = await browser.newContext(contextOptions(workerInfo))
            trackRealtimeClients(context)
            const page = await context.newPage()
            await openMembers(page)
            await use(page)
            await closeTracked(context)
        },
        { scope: 'worker' },
    ],

    /** A page signed in as the file's shared member (see shareMember). */
    p: async ({ browser }, use, testInfo) => {
        const session = shared.get(testInfo.file)
        if (!session) {
            throw new Error('the `p` fixture needs shareMember(...) at the top of the spec file')
        }
        const context = await browser.newContext({
            ...contextOptions(testInfo),
            storageState: session.storageState,
        })
        trackRealtimeClients(context)
        const page = await context.newPage()
        await page.goto('/')
        await appShell(page).waitFor({ state: 'visible', timeout: 15_000 })
        await use(page)
        await closeTracked(context)
    },

    /** A new member for this test only, signed in on its own page. */
    freshMember: async ({ ownerMembersPage, browser }, use, testInfo) => {
        const { user, context, page } = await inviteMember(
            ownerMembersPage,
            browser,
            testInfo,
            'fresh'
        )
        await use({ page, user })
        await closeTracked(context)
    },
})

/**
 * Makes one member for the spec file that calls it (at the top level of the file),
 * and runs `seed` once as that member. Each test of the file then gets `p`: its own
 * page, signed in as the member.
 *
 * A failed test restarts the worker, so the tests after it get a new member and a
 * new seed: they never read what the failed test left.
 */
export function shareMember(
    label: string,
    seed?: (page: Page) => Promise<void>,
    options: { name?: string } = {}
) {
    test.beforeAll(async ({ ownerMembersPage, browser }, testInfo) => {
        const { user, context, page } = await inviteMember(
            ownerMembersPage,
            browser,
            testInfo,
            label,
            options.name
        )
        if (seed) await seed(page)
        shared.set(testInfo.file, { user, storageState: await context.storageState() })
        await closeTracked(context)
    })
}

export { expect } from '@playwright/test'
