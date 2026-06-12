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
import { NewVersionToast } from '@tinycld/core/components/NewVersionToast'
import { initSentry } from '@tinycld/core/lib/sentry'
import { useAppUpdates } from '@tinycld/core/lib/use-app-updates'
import { useChunkLoadRecovery } from '@tinycld/core/lib/use-chunk-load-recovery'
import { useVersionCheck } from '@tinycld/core/lib/use-version-check'
import { Slot, usePathname } from 'expo-router'
import { BlankScreen, ConnectSlot, GateFailedScreen } from '~/lib/gate-screens'
import { MarkBundleHealthy } from '~/lib/use-mark-bundle-healthy'
import { useServerAddressGate } from '~/lib/use-server-address-gate'

initSentry()

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
