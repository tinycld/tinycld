import { packageProviders } from '@tinycld/core/lib/packages/derive-components'
import {
    type ComponentType,
    type LazyExoticComponent,
    type ReactNode,
    Suspense,
    useMemo,
} from 'react'

type ProviderComp =
    | ComponentType<{ children: ReactNode }>
    | LazyExoticComponent<ComponentType<{ children: ReactNode }>>

const stableProviderChain: { slug: string; Provider: ProviderComp }[] = Object.entries(
    packageProviders
)
    .filter((entry): entry is [string, ProviderComp] => entry[1] != null)
    .map(([slug, Provider]) => ({ slug, Provider }))

export function PackageProviderWrapper({ children }: { children: ReactNode }) {
    // Rebuild the provider-chain elements only when `children` changes. A bare
    // reduceRight on every render gives each provider node a fresh element
    // identity, so React can't bail out and reconciles the whole subtree —
    // including the PackageTabs navigator and every mounted-but-frozen screen —
    // on every tab navigation (the layout above re-renders twice per nav). Memoizing on
    // a stable `children` lets React skip this entire subtree when a parent
    // re-renders for an unrelated reason.
    const chain = useMemo(
        () =>
            stableProviderChain.reduceRight<ReactNode>(
                (acc, { slug, Provider }) => <Provider key={slug}>{acc}</Provider>,
                children
            ),
        [children]
    )
    return <Suspense fallback={null}>{chain}</Suspense>
}
