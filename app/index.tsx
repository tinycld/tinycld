import { AuthGate } from '@tinycld/core/components/workspace/AuthGate'
import { SkeletonLayout } from '@tinycld/core/components/workspace/SkeletonLayout'
import { isApexAddress } from '@tinycld/core/lib/apex'
import { useAuth } from '@tinycld/core/lib/auth'
import { trace } from '@tinycld/core/lib/debug-trace'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { getResolvedAddress } from '@tinycld/core/lib/server-address'
import { useSortedPackagesResult } from '@tinycld/core/lib/use-sorted-packages'
import { Redirect, router } from 'expo-router'
import { useEffect } from 'react'
import { Platform } from 'react-native'

export default function Index() {
    const auth = useAuth({ throwIfAnon: false })
    const address = getResolvedAddress()
    const hasServer = !!address

    trace('Index render', {
        isLoggedIn: auth.isLoggedIn,
        isInitializing: auth.isInitializing,
        hasServer,
    })

    // A device that connected to the multi-org apex before this was caught at
    // admission has the apex cached as its server, where signing in can never
    // succeed. Recover by sending it to the picker.
    //
    // Deliberately does NOT gate the render: the overwhelmingly common case is
    // an ordinary single-tenant server, which must show its login form
    // immediately rather than waiting on a network round-trip. So the sign-in
    // panel renders straight away and this redirects out from under it only if
    // the address actually answers as an apex. Web is exempt — it resolves
    // same-origin, and there the apex serves the router's own finder page
    // rather than this bundle.
    useEffect(() => {
        if (Platform.OS === 'web' || !address) return
        if (auth.isLoggedIn || auth.isInitializing) return
        let cancelled = false
        isApexAddress(address).then(atApex => {
            if (cancelled || !atApex) return
            trace('Index replace /pick-org', { address })
            router.replace('/pick-org')
        })
        return () => {
            cancelled = true
        }
    }, [address, auth.isLoggedIn, auth.isInitializing])

    useEffect(() => {
        // Only the no-server case needs an imperative nav. The logged-in landing
        // is a declarative <Redirect> in LandingRedirect below — NOT an
        // imperative navigateToOrg(): single-org collapsed the org path to '/',
        // so pushing it from here (already at '/') re-entered this same route
        // and drove React Navigation into an infinite setState loop (React #185).
        if (!auth.isLoggedIn && !auth.isInitializing && !hasServer) {
            trace('Index replace /connect')
            router.replace('/connect')
        }
    }, [auth.isLoggedIn, hasServer, auth.isInitializing])

    // Logged in: redirect into the workspace. Rendered in a child so the
    // package/role hooks (which throw for anonymous users) only run once auth is
    // established — hooks can't be called conditionally in this component.
    if (auth.isLoggedIn) return <LandingRedirect />

    if (auth.isInitializing || !hasServer) {
        return <SkeletonLayout />
    }

    return (
        <>
            <SkeletonLayout />
            <AuthGate />
        </>
    )
}

// The single authoritative '/' landing for a logged-in user: redirect to the
// first accessible package (or settings when there is none). app/(app)/index.tsx
// can't own '/' while app/index.tsx exists, so the landing redirect lives here.
// Waits for isReady so the default-deny empty window doesn't bounce to settings
// before the real package list arrives.
function LandingRedirect() {
    const { packages: sorted, isReady } = useSortedPackagesResult()
    const orgHref = useOrgHref()
    if (!isReady) return <SkeletonLayout />
    const landing = sorted[0]?.slug ?? 'settings'
    return <Redirect href={orgHref(landing as never)} />
}
