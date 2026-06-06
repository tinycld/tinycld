import { packageSidebars } from '@tinycld/core/lib/packages/derive-components'
import { usePackage } from '@tinycld/core/lib/packages/use-packages'
import { useWorkspaceStore } from '@tinycld/core/lib/stores/workspace-store'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Suspense } from 'react'
import { Platform, View } from 'react-native'
import { SkeletonSidebar } from './SkeletonLayout'

interface PackageSidebarProps {
    width: number
}

export function PackageSidebar({ width }: PackageSidebarProps) {
    const activePkgSlug = useWorkspaceStore(s => s.activePkgSlug)
    const isSidebarOpen = useWorkspaceStore(s => s.isSidebarOpen)
    const pkg = usePackage(activePkgSlug ?? '')
    const sidebarBg = useThemeColor('sidebar-background')

    if (!pkg) return null

    const SidebarComponent = packageSidebars[pkg.slug]
    const targetWidth = isSidebarOpen ? width : 0

    return (
        <View
            className={`overflow-hidden border-border ${isSidebarOpen ? 'border-r' : ''}`}
            style={[
                { width: targetWidth, backgroundColor: sidebarBg },
                Platform.OS === 'web'
                    ? ({
                          transition:
                              'width 0.25s cubic-bezier(0.4, 0, 0.2, 1), border-right-width 0.25s cubic-bezier(0.4, 0, 0.2, 1)',
                      } as object)
                    : undefined,
            ]}
        >
            <View style={{ width, flex: 1, minHeight: 0 }}>
                {SidebarComponent ? (
                    <Suspense fallback={<SkeletonSidebar width={width} />}>
                        {/*
                            testID="package-sidebar-mounted" is the
                            stable signal e2e helpers use to know the
                            lazy-loaded sidebar chunk has actually
                            rendered (not just the Suspense skeleton).
                            It lives INSIDE the Suspense boundary so
                            the testID only attaches once Suspense
                            unsuspends — i.e. when the real sidebar
                            component is mounted. Don't move it
                            outside the boundary or the gate becomes
                            a no-op.
                        */}
                        <View
                            testID="package-sidebar-mounted"
                            style={{ width, flex: 1, minHeight: 0 }}
                        >
                            <SidebarComponent isCollapsed={false} />
                        </View>
                    </Suspense>
                ) : null}
            </View>
        </View>
    )
}
