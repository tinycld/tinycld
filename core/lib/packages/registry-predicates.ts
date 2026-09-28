import { eq, or, type Ref, type WithVirtualProps } from '@tanstack/db'
import type { PkgRegistry } from '@tinycld/core/types/pbSchema'

/**
 * The `pkg_registry.status` values that mean "this package is live in this
 * deployment", and the predicates the reading hooks build from them.
 *
 * Shared because under pbtsdb's on-demand sync a live query's cache key IS its
 * filter: a warm-up preload that sends a different predicate warms a key nobody
 * ever reads. `preloadStores` mirrors these exactly.
 */
export const ACTIVE_PKG_STATUSES = ['bundled', 'installed'] as const

/**
 * `status = 'bundled' || status = 'installed'` — usePackages reads this one.
 *
 * The row is typed from the query builder's own ref branch (what a `.where()`
 * callback hands its caller), not as a plain `{ status: string }`: the refs
 * carry a brand that a structural shape does not satisfy.
 */
export function isActivePkg(row: Ref<WithVirtualProps<PkgRegistry, string | number>>) {
    return or(eq(row.status, ACTIVE_PKG_STATUSES[0]), eq(row.status, ACTIVE_PKG_STATUSES[1]))
}
