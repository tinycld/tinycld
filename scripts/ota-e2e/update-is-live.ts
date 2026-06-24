// Shared "update is live" assertion used by both ota-e2e runners. After a healthy
// flip, prove the NEW bundle's JS actually executed + mounted (boot-log) and is
// present ON SCREEN (a11y sentinel) — not just that the native currentId flipped.
// The boot-log check needs only simctl (always present). The a11y check needs idb:
// absent → skip-with-log (never a false fail); present → a missing/mismatched
// sentinel FAILS. The caller passes its own fail(msg): never and a log(msg) so the
// proof logic lives in one place while each runner keeps its own output prefix.
import { idbAvailable, queryA11ySentinel } from './a11y-sentinel'
import { scrapeBootBundleId } from './boot-log-scraper'

export async function assertUpdateIsLive(
    udid: string,
    newId: string,
    fail: (msg: string) => never,
    log: (msg: string) => void
): Promise<void> {
    // Boot-log: retry to allow first render to lag the currentId flip.
    let bootId: string | null = null
    for (let i = 0; i < 15; i++) {
        bootId = await scrapeBootBundleId(udid, 180).catch(() => null)
        if (bootId === newId) break
        await new Promise(r => setTimeout(r, 2_000))
    }
    if (bootId !== newId) {
        fail(
            `boot-log proof missing: expected the app to log 'app-boot: rendered ... id=${newId}' ` +
                `after the update, but saw id=${JSON.stringify(bootId)}. The new bundle's JS did not ` +
                `execute/mount even though the native currentId flipped.`
        )
    }
    log(`boot-log proof: new bundle JS executed + mounted (id=${newId})`)

    // A11y sentinel: skip-with-log if idb is unavailable; otherwise require it.
    if (!(await idbAvailable())) {
        log('idb not found — skipping on-screen sentinel check (boot-log proof stands)')
        return
    }
    let sentinelId: string | null = null
    for (let i = 0; i < 15; i++) {
        sentinelId = await queryA11ySentinel(udid)
        if (sentinelId === newId) break
        await new Promise(r => setTimeout(r, 2_000))
    }
    if (sentinelId !== newId) {
        fail(
            `on-screen sentinel proof missing: idb is present but the a11y element ` +
                `'ota-bundle-sentinel' did not carry id=${newId} (saw ${JSON.stringify(sentinelId)}). ` +
                `The update did not render visibly.`
        )
    }
    log(`on-screen sentinel proof: update visible (id=${newId})`)
}
