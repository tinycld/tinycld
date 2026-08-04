// diagnose-regexp and the two polyfills are NOT imported here — they live in the
// bundle entry (index.js, referenced by package.json "main"). expo-router's entry
// eagerly requires the whole app/ route tree via `_ctx` BEFORE this module's body
// runs, so a polyfill imported here installs thousands of modules too late to
// help anything the route tree pulls in. See index.js for the measurements.
//
// configure-core MUST be the first import here — it calls
// configureCore(appConfig) at module-init time so every other module in the
// static-import graph sees the registered config on its first read.
import '~/lib/configure-core'
import '~/global.css'
import { AppErrorBoundary } from '@tinycld/core/components/AppErrorBoundary'
import { NewVersionToast } from '@tinycld/core/components/NewVersionToast'
import { BundleSentinel } from '@tinycld/core/lib/bundle-sentinel'
import { installFatalRollbackHandler } from '@tinycld/core/lib/install-fatal-rollback'
import { initSentry } from '@tinycld/core/lib/sentry'
import { useAppUpdates } from '@tinycld/core/lib/use-app-updates'
import { useChunkLoadRecovery } from '@tinycld/core/lib/use-chunk-load-recovery'
import { useVersionCheck } from '@tinycld/core/lib/use-version-check'
import { Slot, usePathname } from 'expo-router'
import { BlankScreen, ConnectSlot, GateFailedScreen } from '~/lib/gate-screens'
import { MarkBundleHealthy } from '~/lib/use-mark-bundle-healthy'
import { useServerAddressGate } from '~/lib/use-server-address-gate'

initSentry()
// Install AFTER initSentry so the fatal handler chains Sentry's global handler
// rather than clobbering it. Catches non-render fatals (which the ErrorBoundary
// can't see) → reports to Sentry + reverts a not-yet-healthy crashing OTA bundle.
installFatalRollbackHandler()

// Expo Router renders a route module's exported `ErrorBoundary` (wrapping its
// subtree in <Try>) whenever a descendant throws during render. Exporting it
// from the ROOT layout makes the whole app's <Slot /> fall back here instead of
// white-screening; AppErrorBoundary also reports the error to Sentry.
export { AppErrorBoundary as ErrorBoundary }

export default function Layout() {
    const pathname = usePathname()
    const state = useServerAddressGate(pathname)
    useVersionCheck()
    useChunkLoadRecovery()
    useAppUpdates()

    if (state.status === 'resolving') return <BlankScreen />
    if (state.status === 'failed') return <GateFailedScreen error={state.error} />
    if (state.status === 'unresolved') {
        // The two routes whose whole job is to RESOLVE an address must render
        // rather than blank-screen: /connect sets a server's, /pick-org sets an
        // org's. Must stay in step with the gate's redirect exemptions
        // (use-server-address-gate.ts) — a route exempt there but blanked here
        // shows nothing at all.
        const resolvesAddress = pathname === '/connect' || pathname === '/pick-org'
        return resolvesAddress ? <ConnectSlot /> : <BlankScreen />
    }

    const { Providers } = state
    return (
        <Providers>
            <MarkBundleHealthy />
            <BundleSentinel />
            <Slot />
            <NewVersionToast />
        </Providers>
    )
}
