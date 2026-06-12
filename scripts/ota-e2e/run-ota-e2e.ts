import { spawn } from 'node:child_process'
import { createReadStream, existsSync, readFileSync } from 'node:fs'
import path from 'node:path'
import { classifyBundleId, embeddedIdForVersion } from './identity'
import { waitForCurrentId } from './log-watcher'
import { precheckNewerBundle } from './server-bundle'

const SERVER_URL = process.env.OTA_E2E_SERVER_URL ?? 'http://localhost:7200'
const SERVER_LOG = process.env.OTA_E2E_SERVER_LOG
const SIM_UDID = process.env.IPHONE_SIMULATOR_UDID
// `|| 180_000` covers unset, empty, non-numeric (NaN), and 0 in one expression —
// a bare Number(...) of a non-numeric override would yield NaN and fire setTimeout
// almost immediately, producing a confusing instant "timeout".
const RELOAD_TIMEOUT_MS = Number(process.env.OTA_E2E_TIMEOUT_MS) || 180_000

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

    if (!SERVER_LOG || !existsSync(SERVER_LOG)) {
        fail(
            `OTA_E2E_SERVER_LOG must point at the file capturing the server's stdout ` +
                `(got ${SERVER_LOG ?? '<unset>'}). See scripts/ota-e2e/README.md.`
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

    // Start watching BEFORE booting so the app's first check isn't missed.
    const logStream = createReadStream(SERVER_LOG, { encoding: 'utf8' })
    const reloaded = waitForCurrentId(logStream, {
        predicate: id => id === newId,
        timeoutMs: RELOAD_TIMEOUT_MS,
        onSeen: id => console.log(`[ota-e2e]   client reported currentId=${id}`),
    })

    if (!SIM_UDID) fail('IPHONE_SIMULATOR_UDID is not set (in .env or the environment).')
    console.log(`[ota-e2e] building + booting Release on ${SIM_UDID}, pointed at ${SERVER_URL}…`)
    buildChild = spawn('scripts/ios-simulator.sh', ['--prod'], {
        cwd: APP_DIR,
        stdio: 'inherit',
        env: { ...process.env, EXPO_PUBLIC_PB_SERVER_ADDR: SERVER_URL },
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

    try {
        const observed = await reloaded
        console.log(`\n[ota-e2e] PASS: app reloaded into ${observed} (was ${embeddedId}).\n`)
        process.exit(0)
    } catch (err) {
        fail((err as Error).message)
    }
}

main().catch(err => fail((err as Error).message))
