import {
    type PackageTermsEntry,
    usePackageTermsList,
    useTermsBlocked,
} from '@tinycld/core/lib/packages/use-package-terms'
import { useWorkspaceStore } from '@tinycld/core/lib/stores/workspace-store'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { StyleSheet, View } from 'react-native'
import { PackageTermsReview } from './PackageTermsReview'
import { TermsWaitingScreen } from './TermsWaitingScreen'

/**
 * True when the ACTIVE package is closed for its terms. Also true while the
 * state of a package that declares terms is not known yet, so its screen and
 * sidebar never show first. Shared by the cover below, WorkspaceLayout and
 * MobileDrawer, which hide the package sidebar for the same state.
 */
export function useActivePkgTermsBlocked(): boolean {
    const activePkgSlug = useWorkspaceStore(s => s.activePkgSlug)
    const { blocked } = useTermsBlocked()
    return activePkgSlug != null && blocked.has(activePkgSlug)
}

type Cover =
    | { kind: 'none' }
    | { kind: 'loading' }
    | { kind: 'review'; entry: PackageTermsEntry }
    | { kind: 'waiting'; name: string }

function useCover(): Cover {
    const activePkgSlug = useWorkspaceStore(s => s.activePkgSlug)
    const isBlocked = useActivePkgTermsBlocked()
    const { entries, isReady } = usePackageTermsList()
    const { isOwner } = useCurrentRole()
    if (!isBlocked) return { kind: 'none' }
    const entry = entries.find(e => e.slug === activePkgSlug)
    if (!isReady || !entry) return { kind: 'loading' }
    return isOwner ? { kind: 'review', entry } : { kind: 'waiting', name: entry.name }
}

/**
 * Covers the package content area while the owner has not accepted the
 * package's terms. On top of the Tabs navigator, as PackageAccessDenied is, so
 * the other packages keep their frozen state.
 */
export function PackageTermsGate() {
    const cover = useCover()
    if (cover.kind === 'none') return null
    return (
        <View className="bg-background" style={StyleSheet.absoluteFill}>
            <CoverBody cover={cover} />
        </View>
    )
}

function CoverBody({ cover }: { cover: Cover }) {
    if (cover.kind === 'review') return <PackageTermsReview entry={cover.entry} />
    if (cover.kind === 'waiting') return <TermsWaitingScreen name={cover.name} />
    return null
}
