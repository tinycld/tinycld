import fs from 'node:fs'
import path from 'node:path'
import { type Page, test } from '@playwright/test'
import { login, loginAs, TEST_COLLABORATOR_EMAIL, TEST_COLLABORATOR_PASSWORD } from './helpers'

// Screenshot grid for the helix skill's visual gate: the same routes captured
// at the same widths and color schemes before (the mockup reference) and after
// (the built screen), so the comparison is like-for-like. Driven by a JSON
// manifest rather than a per-feature spec so nothing has to be scaffolded and
// deleted per run. Inert unless HELIX_CAPTURE points at a manifest — an
// ordinary suite skips it.
//
//     HELIX_CAPTURE=/abs/path/capture.json pnpm exec playwright test -g helix
//
// Manifest: { outDir, widths?, schemes?, shots: [{ name, url, user?, ready?,
// widths?, schemes?, fullPage? }] }. outDir is resolved relative to the
// manifest file. `ready` is a testid to wait for before the first capture.
// `user: 'collaborator'` captures as the seeded collaborator — the way to
// photograph an empty state without writing to the database.
type Scheme = 'light' | 'dark'

interface Shot {
    name: string
    url: string
    user?: 'default' | 'collaborator'
    ready?: string
    widths?: number[]
    schemes?: Scheme[]
    fullPage?: boolean
}

interface Manifest {
    outDir: string
    widths?: number[]
    schemes?: Scheme[]
    shots: Shot[]
}

const DEFAULT_WIDTHS = [390, 1280]
const DEFAULT_SCHEMES: Scheme[] = ['light', 'dark']

function loadManifest(): { manifest: Manifest; outDir: string } | null {
    const manifestPath = process.env.HELIX_CAPTURE
    if (!manifestPath) return null
    const abs = path.resolve(manifestPath)
    const manifest = JSON.parse(fs.readFileSync(abs, 'utf8')) as Manifest
    return { manifest, outDir: path.resolve(path.dirname(abs), manifest.outDir) }
}

// Two frames is enough for a viewport or color-scheme change to reflow; a
// fixed sleep would be a guess and a network-idle wait would hang on the
// realtime subscription.
async function settle(page: Page) {
    await page.evaluate(
        () =>
            new Promise<void>(resolve => {
                requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
            })
    )
}

async function open(page: Page, shot: Shot) {
    if (shot.user === 'collaborator') {
        await loginAs(page, TEST_COLLABORATOR_EMAIL, TEST_COLLABORATOR_PASSWORD)
        // A fresh full load, not in-app navigation: the collaborator session
        // was just established and nothing is in flight to cancel.
        await page.goto(shot.url)
    } else {
        await login(page, { startAt: shot.url })
    }
    if (shot.ready) await page.getByTestId(shot.ready).waitFor({ state: 'visible' })
}

const loaded = loadManifest()

test.describe('helix capture', () => {
    test.skip(!loaded, 'set HELIX_CAPTURE=<manifest.json> to capture')

    const shots = loaded?.manifest.shots ?? [{ name: 'none', url: '/' }]
    for (const shot of shots) {
        test(`helix capture ${shot.name}`, async ({ page }) => {
            if (!loaded) return
            const { manifest, outDir } = loaded
            fs.mkdirSync(outDir, { recursive: true })
            await open(page, shot)

            const widths = shot.widths ?? manifest.widths ?? DEFAULT_WIDTHS
            const schemes = shot.schemes ?? manifest.schemes ?? DEFAULT_SCHEMES
            const written: string[] = []
            for (const width of widths) {
                for (const scheme of schemes) {
                    await page.setViewportSize({ width, height: width < 600 ? 844 : 800 })
                    await page.emulateMedia({ colorScheme: scheme })
                    await settle(page)
                    const file = `${shot.name}-${width}-${scheme}.png`
                    await page.screenshot({
                        path: path.join(outDir, file),
                        fullPage: shot.fullPage ?? false,
                    })
                    written.push(file)
                }
            }
            console.log(`[helix-capture] ${shot.name}: ${written.join(', ')}`)
        })
    }
})
