// scripts/ota-e2e/run-ota-crash-rollback.ts
// Crash-rollback variant of run-ota-e2e.ts. After the Release sim boots on its
// embedded bundle and an OTA bundle is staged on the server, ONE of two things
// must happen, and we assert WHICH:
//   - HEALTHY: the app reloads into build-<ts>-ios and stays up (happy path).
//   - ROLLBACK: the app crash-loops the new bundle and reverts to embedded; the
//     server records a pkg_bad_bundle row whose last_error carries the captured
//     reason (native rollback reason / regex detail). An EMPTY last_error is a
//     FAILURE — it is exactly the gap the manual repro hit.
// Env knobs mirror run-ota-e2e.ts; OTA_E2E_EXPECT=healthy|rollback selects which
// outcome is the pass (default: rollback, the case we are chasing).

import { readFileSync } from 'node:fs'
import path from 'node:path'
import { collectionExists, fetchBadBundles, pollForBadBundle } from './bad-bundle-poller'
import { classifyBundleId, embeddedIdForVersion } from './identity'
import { fetchAppUpdateCurrentIds, pollForBundleId, superuserToken } from './logs-poller'
import { precheckNewerBundle } from './server-bundle'
import { assertUpdateIsLive } from './update-is-live'

const SERVER_URL = process.env.OTA_E2E_SERVER_URL ?? 'http://localhost:7200'
const SUPERUSER_EMAIL = process.env.OTA_E2E_SUPERUSER_EMAIL
const SUPERUSER_PASSWORD = process.env.OTA_E2E_SUPERUSER_PASSWORD
const EXPECT = (process.env.OTA_E2E_EXPECT ?? 'rollback') as 'healthy' | 'rollback'
const TIMEOUT_MS = Number(process.env.OTA_E2E_TIMEOUT_MS) || 240_000
const POLL_INTERVAL_MS = Number(process.env.OTA_E2E_POLL_INTERVAL_MS) || 3_000
const SIM_UDID = process.env.IPHONE_SIMULATOR_UDID
const APP_DIR = path.resolve(import.meta.dirname, '..', '..')

function fail(msg: string): never {
    console.error(`\n[ota-rollback] FAIL: ${msg}\n`)
    process.exit(1)
}

// Read the app version from package.json — the SAME source app.config.ts uses for
// expo.version / expo.runtimeVersion (app.json has no version key). Reading
// expo.version from app.json yields undefined and the precheck 204s on a
// runtimeVersion mismatch. Mirrors tests/install/todo-install.spec.ts.
function readAppVersion(): string {
    const raw = readFileSync(path.join(APP_DIR, 'package.json'), 'utf8')
    return (JSON.parse(raw) as { version: string }).version
}

// After a HEALTHY install, verify the install actually created the package's
// schema — the "booking tables sometimes not created" bug surfaces as a healthy
// install with MISSING collections. Tolerate a brief post-restart window by
// retrying each collection a few times before failing.
async function assertBookingTables(serverUrl: string, token: string): Promise<void> {
    const required = ['booking_pages', 'booking_slot_types', 'booking_availability', 'bookings']
    for (const name of required) {
        let exists: boolean | null = null
        for (let i = 0; i < 10; i++) {
            exists = await collectionExists(serverUrl, token, name)
            if (exists === true) break
            await new Promise(r => setTimeout(r, 2_000))
        }
        if (exists !== true) {
            fail(
                `booking table '${name}' was NOT created by the calendar-slots install (exists=${exists}) ` +
                    `— the table-creation bug reproduced.`
            )
        }
    }
    console.log(`[ota-rollback] booking tables present: ${required.join(', ')}`)
}

async function main() {
    if (!SUPERUSER_EMAIL || !SUPERUSER_PASSWORD) {
        fail('OTA_E2E_SUPERUSER_EMAIL and OTA_E2E_SUPERUSER_PASSWORD must be set (see README).')
    }
    const appVersion = readAppVersion()
    const embeddedId = embeddedIdForVersion(appVersion)
    console.log(
        `[ota-rollback] app version ${appVersion} → embedded id ${embeddedId}; expecting ${EXPECT}`
    )

    const newId = await precheckNewerBundle({
        serverUrl: SERVER_URL,
        runtimeVersion: appVersion,
        embeddedId,
    })
    if (classifyBundleId(newId) !== 'server')
        fail(`Precheck returned unexpected bundle id: ${newId}`)
    console.log(`[ota-rollback] server offers new bundle ${newId}`)

    const token = await superuserToken(SERVER_URL, SUPERUSER_EMAIL, SUPERUSER_PASSWORD).catch(err =>
        fail(`superuser auth failed: ${(err as Error).message}`)
    )

    // Race the two terminal outcomes. Whichever resolves first decides the result;
    // we then check it against EXPECT. Note: a transient fetch error inside EITHER
    // poller also settles the race (the pollers reject, they don't swallow), so a
    // flaky server can surface as a FAIL even when the other outcome was imminent —
    // re-run rather than trusting a lone network-error FAIL.
    const healthy = pollForBundleId({
        fetchCurrentIds: () => fetchAppUpdateCurrentIds(SERVER_URL, token),
        target: newId,
        timeoutMs: TIMEOUT_MS,
        intervalMs: POLL_INTERVAL_MS,
    }).then(() => ({ kind: 'healthy' as const }))

    const rolledBack = pollForBadBundle({
        fetchRows: () => fetchBadBundles(SERVER_URL, token),
        target: newId,
        timeoutMs: TIMEOUT_MS,
        intervalMs: POLL_INTERVAL_MS,
        onPoll: rows => {
            const r = rows.find(x => x.bundleId === newId)
            if (r)
                console.log(
                    `[ota-rollback]   pkg_bad_bundle[${newId}]: reports=${r.reports} last_error=${JSON.stringify(r.lastError)}`
                )
        },
    }).then(row => ({ kind: 'rollback' as const, row }))

    const outcome = await Promise.race([healthy, rolledBack]).catch(err =>
        fail((err as Error).message)
    )

    if (EXPECT === 'healthy') {
        if (outcome.kind === 'healthy') {
            console.log(
                '[ota-rollback] app reloaded healthily into the new bundle; checking schema…'
            )
            await assertBookingTables(SERVER_URL, token)
            if (!SIM_UDID) {
                console.log(
                    '[ota-rollback] IPHONE_SIMULATOR_UDID unset — skipping update-is-live assertion'
                )
            } else {
                await assertUpdateIsLive(SIM_UDID, newId, fail, m =>
                    console.log(`[ota-rollback] ${m}`)
                )
            }
            console.log('\n[ota-rollback] PASS: healthy update with all booking tables present.\n')
            process.exit(0)
        }
        fail(`expected a healthy update but the bundle rolled back: ${JSON.stringify(outcome)}`)
    } else {
        if (outcome.kind === 'rollback') {
            // The crux: the rollback must carry a CAPTURED reason, not the generic string.
            // This literal mirrors the CLIENT-side fallback in core/lib/use-app-updates.ts
            // (reportRevertedBundle, used when the native rollback wrote no error detail) —
            // the server's recordBadBundle stores body.Error verbatim, so this string is
            // what the client sends. Keep the two in sync, or this guard silently stops
            // catching the no-reason case.
            const generic = 'client rolled back: bundle failed to reach healthy'
            if (outcome.row.lastError === generic) {
                fail(
                    `rollback happened but last_error is the GENERIC string — the device did not ` +
                        `capture a reason (recordBundleError / RegExp shim / native rollback reason missing ` +
                        `from the build, or error.json was not written). last_error=${JSON.stringify(outcome.row.lastError)}`
                )
            }
            console.log(
                `\n[ota-rollback] PASS: rolled back with captured reason: ${outcome.row.lastError}\n`
            )
            process.exit(0)
        }
        fail(
            'expected a crash-rollback but the bundle updated healthily (the crash did not reproduce).'
        )
    }
}

main().catch(err => fail((err as Error).message))
