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
import { installBrowserBackGuard } from '@tinycld/core/components/forms-builder/use-leave-guard'
import { NewVersionToast } from '@tinycld/core/components/NewVersionToast'
import { useAuth } from '@tinycld/core/lib/auth'
import { BundleSentinel } from '@tinycld/core/lib/bundle-sentinel'
import { EditorSingletonProvider } from '@tinycld/core/lib/editor/warm'
import { installFatalRollbackHandler } from '@tinycld/core/lib/install-fatal-rollback'
import { CONNECT_HREF } from '@tinycld/core/lib/org-routes'
import { usePackageProviders } from '@tinycld/core/lib/packages/provider-loader'
import { initSentry } from '@tinycld/core/lib/sentry'
import { acceptServerHandoff } from '@tinycld/core/lib/server-handoff'
import { useAppUpdates } from '@tinycld/core/lib/use-app-updates'
import { useChunkLoadRecovery } from '@tinycld/core/lib/use-chunk-load-recovery'
import { useUrl } from '@tinycld/core/lib/use-url'
import { useVersionCheck } from '@tinycld/core/lib/use-version-check'
import { Slot } from 'expo-router'
import { View } from 'react-native'
import { BlankScreen, ConnectSlot, GateFailedScreen } from '~/lib/gate-screens'
import { MarkBundleHealthy } from '~/lib/use-mark-bundle-healthy'
import { useServerAddressGate } from '~/lib/use-server-address-gate'

initSentry()
// Install AFTER initSentry so the fatal handler chains Sentry's global handler
// rather than clobbering it. Catches non-render fatals (which the ErrorBoundary
// can't see) → reports to Sentry + reverts a not-yet-healthy crashing OTA bundle.
installFatalRollbackHandler()
// The bundle entry already took the handoff out of the URL (lib/capture-server-
// handoff.ts); saving it waits for configure-core, which the probe needs.
acceptServerHandoff()
// Before the navigation container mounts and adds expo-router's own popstate
// listener: the browser runs window listeners in the order they were added,
// so a dirty editor's leave guard sees the browser Back first.
installBrowserBackGuard()

// Expo Router renders a route module's exported `ErrorBoundary` (wrapping its
// subtree in <Try>) whenever a descendant throws during render. Exporting it
// from the ROOT layout makes the whole app's <Slot /> fall back here instead of
// white-screening; AppErrorBoundary also reports the error to Sentry.
export { AppErrorBoundary as ErrorBoundary }

export default function Layout() {
    const { pathname } = useUrl()
    const state = useServerAddressGate(pathname)
    useVersionCheck()
    useChunkLoadRecovery()
    useAppUpdates()

    if (state.status === 'resolving') return <BlankScreen />
    if (state.status === 'failed') return <GateFailedScreen error={state.error} />
    if (state.status === 'unresolved') {
        // /connect's whole job is to RESOLVE an address, so it must render
        // rather than blank-screen. Must stay in step with the gate's redirect
        // exemptions (use-server-address-gate.ts) — a route exempt there but
        // blanked here shows nothing at all.
        const resolvesAddress = pathname === CONNECT_HREF
        return resolvesAddress ? <ConnectSlot /> : <BlankScreen />
    }

    const { Providers } = state
    return (
        <Providers>
            <MarkBundleHealthy />
            <BundleSentinel />
            {/* Above the route tree so the one editor outlives any package's
                section — leaving Cards and returning re-uses it instead of
                re-paying the ~1135 ms boot. Constructs nothing until a package
                calls useEditorNeeded(), so an app whose user never opens an
                editing package pays nothing for this. */}
            <EditorSingletonProvider>
                <ReadySlot />
            </EditorSingletonProvider>
            <NewVersionToast />
        </Providers>
    )
}

// The route tree mounts only once the auth store has hydrated AND every
// package provider module has loaded.
//
// Expo Router derives the browser URL from the navigators that are MOUNTED,
// not from the navigation state. app/a/(app)/_layout cannot mount the package
// tabs until it knows who is signed in (package screens require a user), and
// the tabs sit inside the package providers, which are dynamic imports. With
// the tree mounted before either settles there is a window where the root
// navigator is up and the tabs are not — and the URL sync writes the deepest
// mounted route, the bare app root, over a deep link like
// /a/boards?focused=HOME-1. The state itself keeps the link and the URL comes
// back once the tabs mount, but the address bar visibly bounces through /a on
// every cold load. Waiting here means the tabs mount in the same commit as the
// root navigator, so the sync only ever sees the whole tree. Both waits are
// short — one storage read, one chunk fetch that starts alongside it.
function ReadySlot() {
    const { isInitializing } = useAuth({ throwIfAnon: false })
    const providers = usePackageProviders()
    if (providers.status === 'failed') return <GateFailedScreen error={providers.error} />
    if (isInitializing || providers.status === 'loading') {
        return <View className="flex-1 bg-background" />
    }
    return <Slot />
}
