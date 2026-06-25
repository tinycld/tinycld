import { DemoFollowUpModal } from '@tinycld/core/components/DemoFollowUpModal'
import { DemoIntroModal } from '@tinycld/core/components/DemoIntroModal'
import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { HelpDrawer } from '@tinycld/core/components/help/HelpDrawer'
import { HelpSearchPalette } from '@tinycld/core/components/help/HelpSearchPalette'
import { NotifyContextSync } from '@tinycld/core/components/NotifyContextSync'
import { AuthGate } from '@tinycld/core/components/workspace/AuthGate'
import { ImportNotifier } from '@tinycld/core/components/workspace/ImportNotifier'
import { SkeletonLayout } from '@tinycld/core/components/workspace/SkeletonLayout'
import { WorkspaceLayout } from '@tinycld/core/components/workspace/WorkspaceLayout'
import { useAuth } from '@tinycld/core/lib/auth'
import { trace } from '@tinycld/core/lib/debug-trace'
import { useHelpSearchShortcut } from '@tinycld/core/lib/help/use-help-search-shortcut'
import { markNavMilestone } from '@tinycld/core/lib/nav-perf'
import { useWorkspaceStore } from '@tinycld/core/lib/stores/workspace-store'
import { useOrgInfo } from '@tinycld/core/lib/use-org-info'
import { OrgSlugProvider } from '@tinycld/core/lib/use-org-slug'
import { useGlobalSearchParams, usePathname } from 'expo-router'
import { useEffect } from 'react'

export default function OrgLayout() {
    const { orgSlug = '' } = useGlobalSearchParams<{ orgSlug: string }>()

    trace('OrgLayout mount', { orgSlug })

    return (
        <OrgSlugProvider slug={orgSlug}>
            <OrgLayoutInner />
        </OrgSlugProvider>
    )
}

function OrgLayoutInner() {
    const auth = useAuth({ throwIfAnon: false })
    const isReady = !auth.isInitializing && auth.isLoggedIn
    const { org } = useOrgInfo()
    // Bind ⌘/ globally so the help palette is reachable from any
    // org-scoped screen — not just package detail screens. The hook
    // is a no-op on native.
    useHelpSearchShortcut()

    if (!isReady) {
        return (
            <>
                <SkeletonLayout />
                {!auth.isInitializing && !auth.isLoggedIn && <AuthGate />}
            </>
        )
    }

    return (
        <>
            <DocumentTitle title={org?.name} includeOrg={false} />
            <ActivePkgSync />
            <ImportNotifier />
            <NotifyContextSync />
            <WorkspaceLayout isReady={isReady} />
            <DemoIntroModal />
            <DemoFollowUpModal />
            <HelpDrawer />
            <HelpSearchPalette />
        </>
    )
}

function ActivePkgSync() {
    const pathname = usePathname()

    useEffect(() => {
        const match = pathname.match(/^\/a\/[^/]+\/([^/?]+)/)
        const slug = match?.[1] ?? null
        const nextSlug = slug === 'settings' || slug === 'help' ? null : slug
        if (nextSlug) markNavMilestone(nextSlug, 'route-committed')

        const state = useWorkspaceStore.getState()
        if (state.isDrawerOpen) state.setDrawerOpen(false)
        if (state.activePkgSlug !== nextSlug) state.setActivePkgSlug(nextSlug)
    }, [pathname])

    return null
}
