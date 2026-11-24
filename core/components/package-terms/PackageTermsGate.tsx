import { LoadingState } from '@tinycld/core/components/LoadingState'
import {
    type PackageTermsEntry,
    usePackageTermsList,
    useTermsBlocked,
} from '@tinycld/core/lib/packages/use-package-terms'
import { ScopeProvider, useShortcutScope } from '@tinycld/core/lib/shortcuts/scopes'
import { useWorkspaceStore } from '@tinycld/core/lib/stores/workspace-store'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { holdNativeFocusOut } from '@tinycld/core/ui/overlay/hold-native-focus'
import { setInert } from '@tinycld/core/ui/overlay/set-inert'
import { type ReactNode, useCallback } from 'react'
import { Platform, StyleSheet, View, type ViewInstance } from 'react-native'
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

/**
 * Whether the package screens are covered, and the props for the View around
 * them. While the cover shows, the screen under it stays mounted (so the other
 * packages keep their frozen state) but is out of play: no pointer, no focus,
 * no assistive tech. Web uses `inert`, set through the ref because
 * react-native's View props do not type it. Native uses pointerEvents and the
 * accessibility props, and the ref keeps the focus out of the screens
 * (`holdNativeFocusOut`) until the cover goes: React calls the release the ref
 * returns when the cover state changes or the View unmounts.
 */
export function useCoveredContentProps() {
    const isCovered = useActivePkgTermsBlocked()
    const ref = useCallback(
        (node: ViewInstance | null) => {
            if (node == null) return undefined
            if (Platform.OS === 'web') {
                setInert(node as unknown as HTMLElement, isCovered)
                return undefined
            }
            return isCovered ? holdNativeFocusOut(node) : undefined
        },
        [isCovered]
    )
    const viewProps = {
        ref,
        pointerEvents: isCovered ? 'none' : 'auto',
        importantForAccessibility: isCovered ? 'no-hide-descendants' : 'auto',
        accessibilityElementsHidden: isCovered,
    } as const
    return { isCovered, viewProps }
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
            <CoverScope>
                <CoverBody cover={cover} />
            </CoverScope>
        </View>
    )
}

/**
 * Holds the keyboard while the cover shows. A `modal` scope stays above the
 * covered screen's scope whichever of the two is pushed first, so the
 * package's own shortcuts cannot run under the cover.
 */
function CoverScope({ children }: { children: ReactNode }) {
    const scopeOwner = useShortcutScope('modal')
    return <ScopeProvider owner={scopeOwner}>{children}</ScopeProvider>
}

function CoverBody({ cover }: { cover: Cover }) {
    if (cover.kind === 'review') return <PackageTermsReview entry={cover.entry} />
    if (cover.kind === 'waiting') return <TermsWaitingScreen name={cover.name} />
    return (
        <View className="flex-1" testID="pkg-terms-loading">
            <LoadingState />
        </View>
    )
}
