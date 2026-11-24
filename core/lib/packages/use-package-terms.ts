import { eq, inArray } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useAuth } from '@tinycld/core/lib/auth'
import { captureException, errorToString } from '@tinycld/core/lib/errors'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { notify } from '@tinycld/core/lib/notify'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { enabledStatusFor } from '@tinycld/core/lib/setup/set-package-enabled'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { newRecordId } from 'pbtsdb/core'
import { useMemo } from 'react'
import { packageRegistry } from './static-registry'
import {
    blockedSlugsOf,
    openEntriesOf,
    type PendingTerms,
    pendingTermsOf,
    type TermsState,
    termsStateOf,
} from './terms-state'

/**
 * The packages of this build that declare terms (manifest `terms`). Static, so
 * a deployment with no such package sends no terms query at all, and a package
 * with terms is closed from the first render, before the registry has loaded.
 */
export const TERMS_PACKAGES = packageRegistry
    .filter(p => p.terms)
    .map(p => ({ slug: p.slug, name: p.name }))

const TERMS_SLUGS = TERMS_PACKAGES.map(p => p.slug)
const HAS_TERMS = TERMS_SLUGS.length > 0
const NO_SLUGS: ReadonlySet<string> = new Set()

const TERMS_SLUG_SET: ReadonlySet<string> = new Set(TERMS_SLUGS)

/** True for a package of this build that declares terms. Static: reads no store. */
export const declaresTerms = (slug: string) => TERMS_SLUG_SET.has(slug)

const nameOf = (slug: string) => TERMS_PACKAGES.find(p => p.slug === slug)?.name ?? slug

/** The registry rows of the packages that declare terms. One filter, so every caller shares one subscription. */
function useTermsRows() {
    const [pkgRegistry] = useStore('pkg_registry')
    const { data: rows = [], isReady } = useLiveQuery({
        queryKey: ['core', 'pkg-terms', 'rows'],
        query: query =>
            HAS_TERMS
                ? query
                      .from({ p: pkgRegistry })
                      .where(({ p }) => inArray(p.slug, TERMS_SLUGS))
                      .select(({ p }) => ({
                          id: p.id,
                          slug: p.slug,
                          status: p.status,
                          terms_version: p.terms_version,
                          terms_accepted_version: p.terms_accepted_version,
                      }))
                : null,
    })
    return { rows, isReady: !HAS_TERMS || isReady }
}

export interface TermsBlocked {
    /** Slugs of the packages nobody may use yet. Empty when no package declares terms. */
    blocked: ReadonlySet<string>
    stateOf: (slug: string) => TermsState
    isReady: boolean
}

const NOTHING_BLOCKED: TermsBlocked = { blocked: NO_SLUGS, stateOf: () => 'none', isReady: true }

function useNothingBlocked(): TermsBlocked {
    return NOTHING_BLOCKED
}

function useBlockedFromRegistry(): TermsBlocked {
    const { rows, isReady } = useTermsRows()
    return useMemo(
        () => ({
            blocked: blockedSlugsOf(TERMS_SLUGS, rows, isReady),
            stateOf: slug => termsStateOf(rows.find(r => r.slug === slug)),
            isReady,
        }),
        [rows, isReady]
    )
}

/**
 * Which packages are closed until the owner accepts their terms.
 *
 * Chosen once for the build: with no package that declares terms this is a
 * constant and reads no store, so every surface that asks (the rail, help,
 * settings, search, sidebar slots) behaves as it did before terms existed.
 */
export const useTermsBlocked: () => TermsBlocked = HAS_TERMS
    ? useBlockedFromRegistry
    : useNothingBlocked

/** The package of a settings or help group. Module-level, so useTermsOpen gets a stable reference. */
export const slugOfGroup = (group: { pkgSlug: string }) => group.pkgSlug

/** The package of an entry that carries its slug. */
export const slugOfEntry = (entry: { slug: string }) => entry.slug

/** A per-package list (settings groups, help groups, search packages) without the closed packages. */
export function useTermsOpen<T>(entries: readonly T[], slugOf: (entry: T) => string): T[] {
    const { blocked } = useTermsBlocked()
    return useMemo(() => openEntriesOf(entries, slugOf, blocked), [entries, slugOf, blocked])
}

export interface PackageTermsEntry {
    /** The pkg_registry row id. */
    id: string
    slug: string
    name: string
    version: string
    state: Exclude<TermsState, 'none'>
    isDeclined: boolean
    isEnabled: boolean
}

/**
 * Every package with terms and its state, for the owner: the Package terms
 * screen, the launch prompt and the setup wizard. The decisions query runs
 * only for the owner; nobody else may read the collection.
 */
export function usePackageTermsList(): {
    entries: PackageTermsEntry[]
    pending: PendingTerms[]
    isReady: boolean
} {
    const { rows, isReady: rowsReady } = useTermsRows()
    const { isOwner, isReady: roleReady } = useCurrentRole()
    const [decisions] = useStore('pkg_terms_decisions')
    const shouldReadDecisions = HAS_TERMS && isOwner
    const { data: declined = [], isReady: declinedReady } = useLiveQuery({
        queryKey: ['core', 'pkg-terms', 'declined', shouldReadDecisions],
        query: query =>
            shouldReadDecisions
                ? query
                      .from({ d: decisions })
                      .where(({ d }) => eq(d.decision, 'declined'))
                      .select(({ d }) => ({ pkg: d.pkg, terms_version: d.terms_version }))
                : null,
    })
    return useMemo(() => {
        const pending = pendingTermsOf(rows, declined, nameOf)
        const entries = rows.flatMap((r): PackageTermsEntry[] => {
            const state = termsStateOf(r)
            if (state === 'none') return []
            return [
                {
                    id: r.id,
                    slug: r.slug,
                    name: nameOf(r.slug),
                    version: r.terms_version,
                    state,
                    isDeclined: pending.some(p => p.slug === r.slug && p.isDeclined),
                    isEnabled: r.status !== 'disabled',
                },
            ]
        })
        return {
            entries,
            pending,
            isReady: !HAS_TERMS || (rowsReady && roleReady && (!isOwner || declinedReady)),
        }
    }, [rows, declined, rowsReady, roleReady, isOwner, declinedReady])
}

export interface TermsDecisionInput {
    entry: Pick<PackageTermsEntry, 'id' | 'slug' | 'version' | 'isEnabled'>
    decision: 'accepted' | 'declined'
    /** The setup wizard turns the app on with the acceptance. */
    shouldEnable?: boolean
}

/**
 * Records the owner's decision. The server checks the row and, for an
 * acceptance, writes pkg_registry.terms_accepted_version; the registry query
 * above then reads the package as accepted.
 */
export function useTermsDecision(
    options: { onDecided?: (input: TermsDecisionInput) => void } = {}
) {
    const [decisions, pkgRegistry] = useStore('pkg_terms_decisions', 'pkg_registry')
    const { user } = useAuth()
    const decide = useMutation<void, Error, TermsDecisionInput>({
        mutationFn: mutation(function* (input: TermsDecisionInput) {
            yield decisions.insert({
                id: newRecordId(),
                pkg: input.entry.slug,
                terms_version: input.entry.version,
                decision: input.decision,
                user: user.id,
            })
            if (input.decision === 'accepted' && input.shouldEnable && !input.entry.isEnabled) {
                yield pkgRegistry.update(input.entry.id, draft => {
                    draft.status = enabledStatusFor(true)
                })
            }
        }),
        onSuccess: (_data, input) => options.onDecided?.(input),
        onError: err => {
            captureException('core.pkg-terms', err)
            notify.emit({
                event: 'mutation.error',
                title: 'Could not save your decision',
                body: errorToString(err),
                data: { operation: 'core.pkg-terms', error: errorToString(err) },
            })
        },
    })
    return { decide: decide.mutate, isPending: decide.isPending }
}
