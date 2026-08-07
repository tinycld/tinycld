import { asyncStorage, create, persist } from '@tinycld/core/lib/store'

/**
 * How long the drawer edge-swipe stays suppressed after a package drag ends.
 * On a physical phone a fingertip pressed into the left edge of the glass can
 * micro-lift as it reverses direction: iOS ends the touch and starts a new one
 * a few ms later, and that new touch is born inside the edge strip — to the
 * drawer it looks exactly like a deliberate edge swipe, so a card dragged to
 * the left edge and back would fling the drawer open. The grace window
 * swallows that re-touch; a real edge swipe moments later still works.
 */
const EDGE_SWIPE_RESUME_GRACE_MS = 400

let edgeSwipeResumeTimer: ReturnType<typeof setTimeout> | null = null

interface WorkspaceStoreState {
    isSidebarOpen: boolean
    isDrawerOpen: boolean
    isMoreOpen: boolean
    isNotificationsOpen: boolean
    isEdgeSwipeSuspended: boolean
    activePkgSlug: string | null
    // Per-package "last visited href" map. Packages may persist the
    // last deep-link the user opened (e.g. a calc file path) so the
    // sidebar/rail can re-link straight back to that file the next
    // time the package is reopened. Persisted across reloads.
    lastPackageHref: Record<string, string>
    toggleSidebar: () => void
    setSidebarOpen: (open: boolean) => void
    toggleDrawer: () => void
    setDrawerOpen: (open: boolean) => void
    /**
     * Packages with their own drag interactions (board cards, grid tiles)
     * call this with `true` at drag start and `false` at drag end so the
     * mobile drawer's edge-swipe can't hijack a touch near the left edge
     * mid-drag. Resuming is deferred by EDGE_SWIPE_RESUME_GRACE_MS.
     */
    setEdgeSwipeSuspended: (suspended: boolean) => void
    setMoreOpen: (open: boolean) => void
    setNotificationsOpen: (open: boolean) => void
    setActivePkgSlug: (slug: string | null) => void
    setLastPackageHref: (slug: string, href: string) => void
    clearLastPackageHref: (slug: string) => void
}

export const useWorkspaceStore = create<WorkspaceStoreState>()(
    persist(
        set => ({
            isSidebarOpen: true,
            isDrawerOpen: false,
            isMoreOpen: false,
            isNotificationsOpen: false,
            isEdgeSwipeSuspended: false,
            activePkgSlug: null,
            lastPackageHref: {},

            toggleSidebar: () => set(s => ({ isSidebarOpen: !s.isSidebarOpen })),
            setSidebarOpen: open => set({ isSidebarOpen: open }),
            toggleDrawer: () => set(s => ({ isDrawerOpen: !s.isDrawerOpen })),
            setDrawerOpen: open => set({ isDrawerOpen: open }),
            setEdgeSwipeSuspended: suspended => {
                if (edgeSwipeResumeTimer) {
                    clearTimeout(edgeSwipeResumeTimer)
                    edgeSwipeResumeTimer = null
                }
                if (suspended) {
                    set({ isEdgeSwipeSuspended: true })
                    return
                }
                edgeSwipeResumeTimer = setTimeout(() => {
                    edgeSwipeResumeTimer = null
                    set({ isEdgeSwipeSuspended: false })
                }, EDGE_SWIPE_RESUME_GRACE_MS)
            },
            setMoreOpen: open => set({ isMoreOpen: open }),
            setNotificationsOpen: open => set({ isNotificationsOpen: open }),
            setActivePkgSlug: slug => set({ activePkgSlug: slug }),
            setLastPackageHref: (slug, href) =>
                set(s => ({ lastPackageHref: { ...s.lastPackageHref, [slug]: href } })),
            clearLastPackageHref: slug =>
                set(s => {
                    if (!(slug in s.lastPackageHref)) return s
                    const next = { ...s.lastPackageHref }
                    delete next[slug]
                    return { lastPackageHref: next }
                }),
        }),
        {
            // Key kept as 'tinycld_sidebar_open' for historical reasons even
            // though it now also persists `lastPackageHref` (see partialize).
            // Renaming to something accurate (e.g. 'tinycld_workspace') would
            // orphan every existing user's stored sidebar/last-href state on
            // upgrade — Zustand's version/migrate can't carry state across a
            // key rename — and this state is trivial enough that the silent
            // drop isn't worth it. Left as-is intentionally.
            name: 'tinycld_sidebar_open',
            storage: asyncStorage,
            partialize: s => ({
                isSidebarOpen: s.isSidebarOpen,
                lastPackageHref: s.lastPackageHref,
            }),
        }
    )
)
