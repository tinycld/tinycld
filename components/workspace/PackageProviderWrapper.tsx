import { packageProviders } from '@tinycld/core/lib/packages/derive-components'
import {
    type ComponentType,
    type LazyExoticComponent,
    type ReactNode,
    Suspense,
} from 'react'

type ProviderComp =
    | ComponentType<{ children: ReactNode }>
    | LazyExoticComponent<ComponentType<{ children: ReactNode }>>

const stableProviderChain: ProviderComp[] = Object.values(packageProviders).filter(
    (p): p is ProviderComp => p != null
)

export function PackageProviderWrapper({ children }: { children: ReactNode }) {
    const chain = stableProviderChain.reduceRight<ReactNode>(
        (acc, Provider, i) => <Provider key={i}>{acc}</Provider>,
        children
    )
    return <Suspense fallback={null}>{chain}</Suspense>
}
