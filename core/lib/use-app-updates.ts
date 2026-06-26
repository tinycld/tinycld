import {
    checkForUpdate,
    downloadAndStage,
    isUpdateTransportAllowed,
    reportBadBundle,
} from '@tinycld/core/lib/app-updater/client'
import { sha256HexOfFile } from '@tinycld/core/lib/app-updater/hash'
import { captureException } from '@tinycld/core/lib/errors'
import { getResolvedAddress } from '@tinycld/core/lib/server-address'
import { useToastStore } from '@tinycld/core/lib/stores/toast-store'
import AppUpdater from 'app-updater'
import * as FileSystem from 'expo-file-system/legacy'
import { useEffect } from 'react'
import { AppState, type AppStateStatus, Platform } from 'react-native'

declare const __DEV__: boolean

const LAUNCH_DELAY_MS = 3000
// Time the "Update ready" toast is on screen before the native reload swaps the
// JS bundle. Anything under ~1s and the toast renders for a few frames before
// the JS context is killed, which is worse than no toast at all. 1.5s is long
// enough to register the message and short enough not to feel sluggish.
const TOAST_VISIBLE_BEFORE_RELOAD_MS = 1500

// Coalesces overlapping runs. checkAndApplyUpdate fires from the launch timer
// AND on every AppState→'active' transition; rapid background/foreground churn
// would otherwise start concurrent downloads into the SAME per-id staging dir
// (torn writes) and stack reload()s + toasts. While a run is in flight, later
// calls return the same promise instead of starting another.
let inFlight: Promise<void> | null = null

function checkAndApplyUpdate(): Promise<void> {
    if (inFlight) return inFlight
    inFlight = runUpdateCheck().finally(() => {
        inFlight = null
    })
    return inFlight
}

async function runUpdateCheck(): Promise<void> {
    if (__DEV__ || Platform.OS === 'web') return
    const platform = Platform.OS === 'ios' ? 'ios' : 'android'

    try {
        // Gate on the resolved address WITHOUT throwing. This check fires from the
        // launch timer AND from handleAppStateChange (every foreground), either of
        // which can run before the _layout.tsx server-address gate has resolved —
        // e.g. on a cold boot, or in a build where EXPO_PUBLIC_ENV didn't seed an
        // address. Reading String(PB_SERVER_ADDR) there THROWS ("accessed before
        // resolved"); the catch below then reports it to Sentry on every such
        // foreground — noise, not a bug. Use the non-throwing getResolvedAddress()
        // and bail as a clean no-op until connected, exactly like reportRevertedBundle.
        const serverUrl = getResolvedAddress()
        if (!serverUrl) return

        // Refuse to fetch/verify/stage a bundle over an untrusted transport. The
        // per-file SHA-256 check is worthless against a MITM on plaintext http://
        // (the attacker rewrites the bytes AND the manifest hash that arrives over
        // the same channel). We gate on the scheme of the address the CLIENT
        // actually connects to — not anything the server reports — because the
        // server may sit behind a TLS-terminating proxy and only the client knows
        // its real connection scheme. See isUpdateTransportAllowed for the policy.
        if (!isUpdateTransportAllowed(serverUrl)) {
            captureException(
                'app-updater.insecure-transport',
                new Error('OTA update blocked: insecure transport')
            )
            return
        }

        const manifest = await checkForUpdate({
            serverUrl,
            platform,
            runtimeVersion: AppUpdater.getRuntimeVersion(),
            currentId: AppUpdater.getCurrentBundleId(),
            currentHash: AppUpdater.getCurrentBundleHash(),
            fetchFn: fetch,
        })
        if (!manifest) return

        // Stage under documentDirectory, NOT cacheDirectory: once promoted, this
        // dir IS the running bundle, and the OS can purge the cache dir under
        // storage pressure — which would make the native loader miss the .hbc and
        // trigger a spurious rollback. documentDirectory is not auto-evicted.
        const tmpDir = `${FileSystem.documentDirectory}app-update/${manifest.id}/`
        await FileSystem.makeDirectoryAsync(tmpDir, { intermediates: true })

        await downloadAndStage(manifest, {
            serverUrl,
            platform,
            downloadFn: async (url, dest) => {
                // Assets keep the server's relative paths (e.g. assets/a), so a
                // dest can sit in a subdir that doesn't exist yet. downloadAsync
                // requires the parent dir to exist — create it first.
                const parentDir = dest.slice(0, dest.lastIndexOf('/') + 1)
                await FileSystem.makeDirectoryAsync(parentDir, { intermediates: true })
                const r = await FileSystem.downloadAsync(url, dest)
                // downloadAsync resolves (doesn't throw) on a non-2xx response and
                // writes the error body to dest. Without this check that junk gets
                // hashed and surfaces as a misleading "hash mismatch"; fail with
                // the real cause instead.
                if (r.status < 200 || r.status >= 300) {
                    throw new Error(`download failed: ${r.status} for ${url}`)
                }
                return { uri: r.uri }
            },
            hashFn: sha256HexOfFile,
            // The download/hash layer is URI-aware (file:// is fine), but the
            // native module does raw filesystem path ops on `dir`
            // (URL(fileURLWithPath:) / java.io.File), which treat a file://
            // prefix as a literal path component and never find the staged .hbc
            // — silently rolling back to embedded on every update. Hand native a
            // plain path; keep the file:// tmpDir for the download layer.
            stageBundleFn: (dir, id, hash) =>
                AppUpdater.stageBundle(dir.replace(/^file:\/\//, ''), id, hash),
            tmpDir,
        })

        // Surface a toast first so the imminent reload has context — without it
        // the screen blinks and the user has no idea why. Delay the reload by
        // TOAST_VISIBLE_BEFORE_RELOAD_MS so the toast is on screen long enough to
        // read.
        useToastStore.getState().addToast({
            title: 'Update ready',
            body: 'Restarting to apply the latest version…',
            variant: 'info',
            duration: TOAST_VISIBLE_BEFORE_RELOAD_MS + 500,
        })
        await new Promise(resolve => setTimeout(resolve, TOAST_VISIBLE_BEFORE_RELOAD_MS))
        await AppUpdater.reload()
    } catch (error) {
        captureException('use-app-updates.check', error)
    }
}

// reportRevertedBundle runs on boot (and again on each foreground): if the native
// side rolled a bundle back (this device crash-looped on it and recovered), tell
// the server so it stops advertising that bundle to the rest of the fleet.
// Best-effort — the local rollback already protected THIS device; this is
// fleet-wide signal, so any failure (offline, server down) is swallowed. Runs in
// the RECOVERED (good) bundle, the only place a network call can reliably complete.
// Exported for unit testing (the gate-ordering invariant below is the regression
// guard for a crash where it consumed the marker then threw pre-gate).
export async function reportRevertedBundle(): Promise<void> {
    if (__DEV__ || Platform.OS === 'web') return

    // Gate on the resolved address WITHOUT consuming the reverted marker. Reading
    // it via getResolvedAddress() (not String(PB_SERVER_ADDR)) avoids the throw
    // that proxy raises pre-gate — this effect fires on the layout's first commit,
    // which can beat the server-address gate. Bailing BEFORE takeRevertedBundle()
    // (read-once) leaves the marker intact so the next foreground retries; the old
    // order consumed it then threw, both losing the report AND spamming Sentry with
    // a non-bug.
    const serverUrl = getResolvedAddress()
    if (!serverUrl || !isUpdateTransportAllowed(serverUrl)) return

    const reverted = AppUpdater.takeRevertedBundle()
    if (!reverted) return
    try {
        // Prefer the detail the fatal handler persisted (the offending regex
        // pattern + Hermes error) so the report-bad row names WHY the bundle
        // crashed — not just that it did. Older binaries return no `error`; fall
        // back to the generic reason then.
        const detail =
            'error' in reverted && reverted.error
                ? reverted.error
                : 'client rolled back: bundle failed to reach healthy'
        await reportBadBundle({
            serverUrl,
            platform: Platform.OS === 'ios' ? 'ios' : 'android',
            id: reverted.id,
            hash: reverted.hash,
            error: detail,
            fetchFn: fetch,
        })
    } catch (error) {
        captureException('use-app-updates.report-bad', error, { id: reverted.id })
    }
}

export function useAppUpdates(): void {
    useEffect(() => {
        if (__DEV__ || Platform.OS === 'web') return

        // Report a bundle this device just rolled back, before checking for new
        // ones — so the server can stop serving it to others right away. No-ops
        // (keeping the marker) until the server-address gate resolves, so it's also
        // re-attempted on each foreground below until it gets through.
        void reportRevertedBundle()

        const launchTimer = setTimeout(checkAndApplyUpdate, LAUNCH_DELAY_MS)

        const handleAppStateChange = (next: AppStateStatus) => {
            if (next === 'active') {
                void reportRevertedBundle()
                checkAndApplyUpdate()
            }
        }
        const subscription = AppState.addEventListener('change', handleAppStateChange)

        return () => {
            clearTimeout(launchTimer)
            subscription.remove()
        }
    }, [])
}
