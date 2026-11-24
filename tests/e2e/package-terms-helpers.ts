import * as fs from 'node:fs'
import * as path from 'node:path'
import type { Locator, Page } from '@playwright/test'
import { appHref } from '@tinycld/core/lib/org-routes'
import PocketBase from 'pocketbase'
import { login } from './helpers'

// The package-terms specs drive the termsstub package
// (tests/scripts/scaffold-terms-stub.ts): the one package of the e2e build that
// declares terms and conditions. Keep these in step with that script.
export const TERMS_STUB = {
    slug: 'termsstub',
    name: 'Terms Stub',
    version: '1',
    sentence: 'You agree to feed the stub once a day.',
    landing: 'termsstub-landing',
    sidebar: 'termsstub-sidebar',
    settingsLink: 'settings-org-termsstub-general',
    settingsPanel: 'termsstub-settings-panel',
    helpTopic: 'Terms stub topic',
} as const

export const PHONE_VIEWPORT = { width: 390, height: 844 }

const PRIMARY_PORT = Number(process.env.E2E_PORT ?? 7200)
const SECOND_PORT = Number(process.env.E2E_PORT_2 ?? 7201)
/** The second e2e deployment: its own database, the same seeded users. */
export const SECOND_ORIGIN = `http://localhost:${SECOND_PORT}`

const ADMIN_LOGIN = process.env.ADMIN_USER_LOGIN || 'admin@tinycld.org'
const ADMIN_PASSWORD = process.env.ADMIN_USER_PW || 'AdminPass1234!'

// Same rule as skipWithoutFormsStub (helpers.ts): skip in a dev workspace
// without the stub, fail loudly on CI, where the scaffold step is mandatory.
// Pass it as a callback.
export function skipWithoutTermsStub(): boolean {
    const stubDir = path.resolve(import.meta.dirname, '..', '..', '..', TERMS_STUB.slug)
    if (fs.existsSync(stubDir)) return false
    if (process.env.CI) {
        throw new Error(
            'termsstub is not scaffolded but CI is set — the terms stub scaffold step must run before e2e.'
        )
    }
    return true
}

/** A PocketBase client signed in as the superuser of one e2e deployment. */
export async function termsSuperuser(server: 'primary' | 'second'): Promise<PocketBase> {
    const port = server === 'primary' ? PRIMARY_PORT : SECOND_PORT
    const pb = new PocketBase(`http://127.0.0.1:${port}`)
    pb.autoCancellation(false)
    await pb.collection('_superusers').authWithPassword(ADMIN_LOGIN, ADMIN_PASSWORD)
    return pb
}

/**
 * Puts the stub's terms in a known state: not accepted, and either undecided
 * or declined. A superuser write (see the header of
 * package-terms-owner.spec.ts for why): the app has no way to take back an
 * acceptance or a decline.
 */
export async function resetStubTerms(pb: PocketBase, state: 'undecided' | 'declined') {
    const filter = pb.filter('pkg = {:slug}', { slug: TERMS_STUB.slug })
    const decisions = await pb.collection('pkg_terms_decisions').getFullList({ filter })
    for (const decision of decisions) {
        await pb.collection('pkg_terms_decisions').delete(decision.id)
    }
    const row = await pb
        .collection('pkg_registry')
        .getFirstListItem(pb.filter('slug = {:slug}', { slug: TERMS_STUB.slug }))
    await pb.collection('pkg_registry').update(row.id, { terms_accepted_version: '' })
    if (state === 'undecided') return
    await pb.collection('pkg_terms_decisions').create({
        pkg: TERMS_STUB.slug,
        terms_version: TERMS_STUB.version,
        decision: 'declined',
    })
}

/**
 * Signs the owner (the shared test user) in on the second deployment. Give
 * `landed` for a route outside the app shell (a setup step): it is what shows
 * that the sign-in is done.
 */
export async function loginOnSecond(page: Page, route = '', landed?: Locator) {
    const startAt = `${SECOND_ORIGIN}${route === '' ? '/' : appHref(route)}`
    await login(page, { startAt, landed })
}

const FIXED_NAV = ['home', 'help', 'settings', 'sidebar-toggle', 'more']

/**
 * The package entries of the rail or the tab bar: every `nav-*` item but the
 * fixed ones. One of them visible shows that the user's package list has
 * loaded, so "no entry for the stub" is not read from an empty list.
 */
export function packageNavEntries(page: Page): Locator {
    const notFixed = FIXED_NAV.map(name => `:not([data-testid="nav-${name}"])`).join('')
    return page.locator(`[data-testid^="nav-"]${notFixed}`)
}
