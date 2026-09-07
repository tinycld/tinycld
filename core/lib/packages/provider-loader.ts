import { log } from '@tinycld/core/lib/logger'
import { type ComponentType, useSyncExternalStore } from 'react'
import type { PackageProviderLoader, PackageProviderProps } from './config-types'
import { packageProviders } from './derive-components'

export interface LoadedPackageProvider {
    slug: string
    Provider: ComponentType<PackageProviderProps>
}

export type PackageProvidersState =
    | { status: 'loading' }
    | { status: 'ready'; providers: LoadedPackageProvider[] }
    | { status: 'failed'; error: string }

let state: PackageProvidersState = { status: 'loading' }
let inFlight: Promise<LoadedPackageProvider[]> | null = null
const listeners = new Set<() => void>()

function publish(next: PackageProvidersState) {
    state = next
    for (const notify of listeners) notify()
}

/**
 * Resolve every package's provider module. Idempotent: one load per boot,
 * shared by every caller.
 *
 * Rejects (as well as publishing 'failed') so a chunk-load failure still
 * surfaces as an unhandled rejection, which is what useChunkLoadRecovery
 * listens for to reload a tab whose asset hashes were pruned by a deploy.
 */
export function loadPackageProviders(): Promise<LoadedPackageProvider[]> {
    if (!inFlight) {
        const loaders = Object.entries(packageProviders).filter(
            (entry): entry is [string, PackageProviderLoader] => entry[1] != null
        )
        inFlight = Promise.all(
            loaders.map(async ([slug, loader]) => ({
                slug,
                Provider: (await loader.load()).default,
            }))
        ).then(
            providers => {
                publish({ status: 'ready', providers })
                return providers
            },
            (err: unknown) => {
                log.error('core.packages.provider-load', err)
                publish({
                    status: 'failed',
                    error: err instanceof Error ? err.message : String(err),
                })
                throw err
            }
        )
    }
    return inFlight
}

function subscribe(listener: () => void) {
    listeners.add(listener)
    // Starts the load from the first subscriber's effect rather than at module
    // init, so importing this module has no side effect.
    void loadPackageProviders()
    return () => {
        listeners.delete(listener)
    }
}

function getSnapshot() {
    return state
}

/** The provider load, live. Subscribing starts it. */
export function usePackageProviders(): PackageProvidersState {
    return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}

/**
 * The loaded providers, for render paths that only run behind the root
 * layout's gate. Throwing rather than returning an empty list: rendering the
 * workspace without its providers would silently drop package context.
 */
export function getLoadedPackageProviders(): LoadedPackageProvider[] {
    if (state.status !== 'ready') {
        throw new Error(
            'Package providers are not loaded. The workspace must mount behind the root layout gate, which awaits loadPackageProviders().'
        )
    }
    return state.providers
}
