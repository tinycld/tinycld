import { checkForUpdate, downloadAndStage } from '@tinycld/core/lib/app-updater/client'
import { sha256HexOfFile } from '@tinycld/core/lib/app-updater/hash'
import { PB_SERVER_ADDR } from '@tinycld/core/lib/config'
import { captureException } from '@tinycld/core/lib/errors'
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

async function checkAndApplyUpdate(): Promise<void> {
    if (__DEV__ || Platform.OS === 'web') return
    const platform = Platform.OS === 'ios' ? 'ios' : 'android'

    try {
        // PB_SERVER_ADDR throws if the app hasn't connected to a server yet, so
        // reading it here is the gate: no update check runs until connected. The
        // surrounding try/catch swallows that throw as a no-op.
        const manifest = await checkForUpdate({
            serverUrl: PB_SERVER_ADDR,
            platform,
            runtimeVersion: AppUpdater.getRuntimeVersion(),
            currentId: AppUpdater.getCurrentBundleId(),
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
            serverUrl: PB_SERVER_ADDR,
            platform,
            downloadFn: async (url, dest) => {
                // Assets keep the server's relative paths (e.g. assets/a), so a
                // dest can sit in a subdir that doesn't exist yet. downloadAsync
                // requires the parent dir to exist — create it first.
                const parentDir = dest.slice(0, dest.lastIndexOf('/') + 1)
                await FileSystem.makeDirectoryAsync(parentDir, { intermediates: true })
                const r = await FileSystem.downloadAsync(url, dest)
                return { uri: r.uri }
            },
            hashFn: sha256HexOfFile,
            stageBundleFn: (dir, id) => AppUpdater.stageBundle(dir, id),
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

export function useAppUpdates(): void {
    useEffect(() => {
        if (__DEV__ || Platform.OS === 'web') return

        const launchTimer = setTimeout(checkAndApplyUpdate, LAUNCH_DELAY_MS)

        const handleAppStateChange = (next: AppStateStatus) => {
            if (next === 'active') {
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
