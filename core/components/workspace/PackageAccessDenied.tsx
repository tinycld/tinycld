import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { usePackage } from '@tinycld/core/lib/packages/use-packages'
import { useWorkspaceStore } from '@tinycld/core/lib/stores/workspace-store'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { usePkgAccessResult } from '@tinycld/core/lib/use-pkg-access'
import { router } from 'expo-router'
import { Lock } from 'lucide-react-native'
import { Pressable, StyleSheet, Text, View } from 'react-native'

// True when the ACTIVE package resolved to access 'none' for the signed-in
// user. Waits for isReady: access reads 'none' while the override row loads,
// and flashing a denial at every member on every package switch is worse
// than a moment of skeleton. False for app-owned areas (settings/help/
// admin), whose slugs resolve no package and are never access-gated.
// Shared by the overlay below and WorkspaceLayout (which hides the package
// sidebar for the same state — it sits outside this overlay).
export function useActivePkgDenied(): boolean {
    const activePkgSlug = useWorkspaceStore(s => s.activePkgSlug)
    const pkg = usePackage(activePkgSlug ?? '')
    const { access, isReady } = usePkgAccessResult(activePkgSlug ?? '')
    return pkg != null && isReady && access === 'none'
}

// Covers the package content area when the signed-in user's package access
// resolves to 'none'. Access rows only drive NAVIGATION (the rail hides
// denied packages), so before this overlay a denied user's bookmark rendered
// the full package UI — over empty collections for a guest, which read as
// data loss. Sits on top of the Tabs navigator rather than replacing it, so
// other packages' frozen state survives a visit to a denied one.
export function PackageAccessDenied() {
    const activePkgSlug = useWorkspaceStore(s => s.activePkgSlug)
    const pkg = usePackage(activePkgSlug ?? '')
    const isDenied = useActivePkgDenied()
    const mutedColor = useThemeColor('muted-foreground')

    if (!isDenied || pkg == null) return null

    const label = pkg.nav?.label ?? pkg.slug

    return (
        <View
            testID="pkg-access-denied"
            className="bg-background items-center justify-center gap-3 p-6"
            style={StyleSheet.absoluteFill}
        >
            <Lock size={28} color={mutedColor} />
            <Text className="text-foreground" style={{ fontSize: 16, fontWeight: '700' }}>
                You don’t have access to {label}
            </Text>
            <Text
                className="text-muted-foreground text-center"
                style={{ fontSize: 13, lineHeight: 19, maxWidth: 420 }}
            >
                An owner or admin can grant you access from Settings → Members. Anything shared with
                you directly — like a share link — keeps working without it.
            </Text>
            <GoToSettingsButton />
        </View>
    )
}

function GoToSettingsButton() {
    const orgHref = useOrgHref()

    return (
        <Pressable
            onPress={() => router.replace(orgHref('settings'))}
            className="rounded-md border border-border"
            style={{ paddingVertical: 8, paddingHorizontal: 14, marginTop: 4 }}
        >
            <Text className="text-foreground" style={{ fontSize: 13, fontWeight: '600' }}>
                Go to Settings
            </Text>
        </Pressable>
    )
}
