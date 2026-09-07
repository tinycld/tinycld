import { getLoadedPackageProviders } from '@tinycld/core/lib/packages/provider-loader'
import { type ReactNode, useMemo } from 'react'

/**
 * Wraps the package area in every package's app-wide provider.
 *
 * The providers come from the loader, already resolved: the root layout mounts
 * the route tree only after loadPackageProviders() settles, so nothing here can
 * suspend. That matters beyond speed — a Suspense fallback here once let the
 * root navigator mount while the package tabs were still pending, and Expo
 * Router's URL sync wrote the bare app root over the deep link that was loading.
 */
export function PackageProviderWrapper({ children }: { children: ReactNode }) {
    // Rebuild the provider-chain elements only when `children` changes. A bare
    // reduceRight on every render gives each provider node a fresh element
    // identity, so React can't bail out and reconciles the whole subtree —
    // including the PackageTabs navigator and every mounted-but-frozen screen —
    // on every tab navigation (the layout above re-renders twice per nav). Memoizing on
    // a stable `children` lets React skip this entire subtree when a parent
    // re-renders for an unrelated reason.
    return useMemo(
        () =>
            getLoadedPackageProviders().reduceRight<ReactNode>(
                (acc, { slug, Provider }) => <Provider key={slug}>{acc}</Provider>,
                children
            ),
        [children]
    )
}
