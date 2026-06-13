import { spawn } from 'node:child_process'
import { readFileSync } from 'node:fs'
import path from 'node:path'
import { classifyBundleId, embeddedIdForVersion } from './identity'
import { fetchAppUpdateCurrentIds, pollForBundleId, superuserToken } from './logs-poller'
import { precheckNewerBundle } from './server-bundle'

const SERVER_URL = process.env.OTA_E2E_SERVER_URL ?? 'http://localhost:7200'
const SUPERUSER_EMAIL = process.env.OTA_E2E_SUPERUSER_EMAIL
const SUPERUSER_PASSWORD = process.env.OTA_E2E_SUPERUSER_PASSWORD
const SIM_UDID = process.env.IPHONE_SIMULATOR_UDID
// When set, skip the Release build/boot + seed and only run precheck + poll. The
// shell driver (run-ota-dry-run.sh) sets this because it already built, booted,
// and pointed the app at the server itself — this script is then just the
// assertion step. Run standalone (unset) it builds the sim itself.
const SKIP_BUILD = process.env.OTA_E2E_SKIP_BUILD === '1'
// `|| <default>` covers unset, empty, non-numeric (NaN), and 0 in one expression —
// a bare Number(...) of a non-numeric override would yield NaN and fire setTimeout
// almost immediately, producing a confusing instant "timeout".
const RELOAD_TIMEOUT_MS = Number(process.env.OTA_E2E_TIMEOUT_MS) || 180_000
const POLL_INTERVAL_MS = Number(process.env.OTA_E2E_POLL_INTERVAL_MS) || 3_000

const APP_DIR = path.resolve(import.meta.dirname, '..', '..')

// Set once the build child is spawned so fail() can reap it — otherwise a reload
// timeout calls process.exit while a long xcodebuild keeps running orphaned.
let buildChild: import('node:child_process').ChildProcess | null = null

// Read expo.version from app.json without a JSON module import (which the
// project tsconfig may reject) — a plain read keeps the script tsconfig-agnostic.
function readAppVersion(): string {
    const raw = readFileSync(path.join(APP_DIR, 'app.json'), 'utf8')
    const parsed = JSON.parse(raw) as { expo: { version: string } }
    return parsed.expo.version
}

function fail(msg: string): never {
    console.error(`\n[ota-e2e] FAIL: ${msg}\n`)
    buildChild?.kill()
    process.exit(1)
}

async function main() {
    const appVersion = readAppVersion()
    const embeddedId = embeddedIdForVersion(appVersion)
    console.log(`[ota-e2e] app version ${appVersion} → embedded id ${embeddedId}`)

    // Guard creds before any network so an offline misconfigured run fails fast
    // and deterministically, never spawning a build.
    if (!SUPERUSER_EMAIL || !SUPERUSER_PASSWORD) {
        fail(
            'OTA_E2E_SUPERUSER_EMAIL and OTA_E2E_SUPERUSER_PASSWORD must be set — they ' +
                'authenticate the PB superuser used to read /api/logs. See scripts/ota-e2e/README.md.'
        )
    }

    console.log('[ota-e2e] prechecking server for a newer ios bundle…')
    const newId = await precheckNewerBundle({
        serverUrl: SERVER_URL,
        runtimeVersion: appVersion,
        embeddedId,
    })
    if (classifyBundleId(newId) !== 'server') {
        fail(`Precheck returned an unexpected bundle id shape: ${newId}`)
    }
    console.log(`[ota-e2e] server offers new bundle ${newId}`)

    let token: string
    try {
        token = await superuserToken(SERVER_URL, SUPERUSER_EMAIL, SUPERUSER_PASSWORD)
    } catch (err) {
        return fail(
            `superuser auth failed (server down or bad OTA_E2E_SUPERUSER_* creds): ${(err as Error).message}`
        )
    }

    // Start polling BEFORE booting so the app's first check isn't missed. A fresh
    // PB token outlives a <=180s run, so no refresh logic is needed.
    const reloaded = pollForBundleId({
        fetchCurrentIds: () => fetchAppUpdateCurrentIds(SERVER_URL, token),
        target: newId,
        timeoutMs: RELOAD_TIMEOUT_MS,
        intervalMs: POLL_INTERVAL_MS,
        onPoll: ids => {
            if (ids.length) console.log(`[ota-e2e]   _logs currentIds: ${ids.join(', ')}`)
        },
    })

    if (SKIP_BUILD) {
        console.log('[ota-e2e] OTA_E2E_SKIP_BUILD=1 — app already built/booted/connected; polling…')
    } else {
        if (!SIM_UDID) fail('IPHONE_SIMULATOR_UDID is not set (in .env or the environment).')
        console.log(
            `[ota-e2e] building + booting Release on ${SIM_UDID} (app must already be connected to ${SERVER_URL} — see README)…`
        )
        buildChild = spawn('scripts/ios-simulator.sh', ['--prod'], {
            cwd: APP_DIR,
            stdio: 'inherit',
            env: { ...process.env },
        })
        const child = buildChild
        // spawn emits 'error' (not an exit code) when the script is missing or not
        // executable — without this listener Node would throw it unhandled and crash
        // past our friendly fail().
        child.on('error', err => fail(`could not spawn ios-simulator.sh: ${(err as Error).message}`))
        child.on('exit', code => {
            if (code !== 0) fail(`ios-simulator.sh --prod exited ${code} (build/boot failed).`)
            console.log('[ota-e2e] app installed + launched; waiting for OTA reload…')
        })
    }

    try {
        const observed = await reloaded
        console.log(`\n[ota-e2e] PASS: app reloaded into ${observed} (was ${embeddedId}).\n`)
        process.exit(0)
    } catch (err) {
        fail((err as Error).message)
    }
}

main().catch(err => fail((err as Error).message))
