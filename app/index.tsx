import { AuthGate } from '@tinycld/core/components/workspace/AuthGate'
import { SkeletonLayout } from '@tinycld/core/components/workspace/SkeletonLayout'
import { useAuth } from '@tinycld/core/lib/auth'
import { trace } from '@tinycld/core/lib/debug-trace'
import { CONNECT_HREF, useOrgHref } from '@tinycld/core/lib/org-routes'
import { getResolvedAddress } from '@tinycld/core/lib/server-address'
import { useSortedPackagesResult } from '@tinycld/core/lib/use-sorted-packages'
import { Redirect, router } from 'expo-router'
import { useEffect } from 'react'
import { usePackagePrefetch } from '~/lib/route-prefetch'
import { useLaunchTermsPrompt } from '~/lib/use-launch-terms-prompt'

export default function Index() {
    const auth = useAuth({ throwIfAnon: false })
    const address = getResolvedAddress()
    const hasServer = !!address

    trace('Index render', {
        isLoggedIn: auth.isLoggedIn,
        isInitializing: auth.isInitializing,
        hasServer,
    })

    useEffect(() => {
        // Only the no-server case needs an imperative nav. The logged-in landing
        // is a declarative <Redirect> in LandingRedirect below — NOT an
        // imperative push to '/': pushing it from here (already at '/')
        // re-entered this same route and drove React Navigation into an
        // infinite setState loop (React #185).
        if (!auth.isLoggedIn && !auth.isInitializing && !hasServer) {
            trace('Index replace /connect')
            router.replace(CONNECT_HREF)
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
// before the real package list arrives, and then for the landing package's
// route chunks and sidebar, so they mount without a Suspense fallback (see
// lib/route-prefetch.ts). An owner with undecided package terms is sent to
// them first, as from the org root.
function LandingRedirect() {
    const { packages: sorted, isReady } = useSortedPackagesResult()
    const termsPrompt = useLaunchTermsPrompt()
    const orgHref = useOrgHref()
    const landing = isReady ? (sorted[0]?.slug ?? 'settings') : null
    const landingLoaded = usePackagePrefetch(landing)
    if (!landing || !landingLoaded || !termsPrompt.isReady) return <SkeletonLayout />
    if (termsPrompt.href) return <Redirect href={termsPrompt.href} />
    return <Redirect href={orgHref(landing as never)} />
}
