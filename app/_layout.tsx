// polyfill-dom-shim must run before anything that pulls in prosemirror-view
// (tentap → @tiptap/core → @tiptap/pm/view). Something in our Expo SDK 55
// stack now installs a partial `document` on Hermes that breaks
// prosemirror-view's top-level browser sniff. The shim fills the missing
// `documentElement.style` slot. See lib/polyfill-dom-shim.ts for the why.
import '~/lib/polyfill-dom-shim'
// polyfill-crypto MUST run before configure-core: @tanstack/db's collection
// constructor calls crypto.randomUUID() at module init, and Hermes has no
// global crypto.
import '~/lib/polyfill-crypto'
// configure-core MUST be the first import after the polyfill — it calls
// configureCore(appConfig) at module-init time so every other module in the
// static-import graph sees the registered config on its first read.
import '~/lib/configure-core'
import '~/global.css'
import { AppErrorBoundary } from '@tinycld/core/components/AppErrorBoundary'
import { NewVersionToast } from '@tinycld/core/components/NewVersionToast'
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
        return pathname === '/connect' ? <ConnectSlot /> : <BlankScreen />
    }

    const { Providers } = state
    return (
        <Providers>
            <MarkBundleHealthy />
            <Slot />
            <NewVersionToast />
        </Providers>
    )
}
