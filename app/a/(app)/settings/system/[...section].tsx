// Package-contributed SYSTEM settings panels (manifest `systemSettings`),
// addressed under /settings/system/<pkgSlug>/<panelSlug>.
//
// A separate route tree from settings/[...section].tsx, which resolves the
// org-scoped `settings` panels, because the two namespaces can collide: mail
// declares the slug `provider` in BOTH. Resolving them through one route would
// make ('mail','provider') ambiguous — the org-scoped panel would always win and
// the system panel would have no addressable URL.

import { SystemSettingsScreen } from '@tinycld/core/components/settings/system/SystemSettingsScreen'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { packageSystemSettings } from '@tinycld/core/lib/packages/derive-components'
import { resolvePanel } from '@tinycld/core/lib/packages/resolve-panel'
import {
    declaresTerms,
    slugOfGroup,
    useTermsBlocked,
    useTermsOpen,
} from '@tinycld/core/lib/packages/use-package-terms'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useLocalSearchParams, useRouter } from 'expo-router'
import { Suspense, useMemo } from 'react'
import { Pressable, Text, View } from 'react-native'

export default function PackageSystemSettingsSection() {
    const router = useRouter()
    const orgHref = useOrgHref()
    const primaryColor = useThemeColor('primary')
    const params = useLocalSearchParams<{ section: string[] }>()
    const segments = params.section ?? []
    const [pkgSlug, panelSlug] = segments

    // A package whose terms the owner has not accepted has no panels.
    const groups = useTermsOpen(packageSystemSettings, slugOfGroup)
    const match = useMemo(
        () => resolvePanel(groups, pkgSlug, panelSlug),
        [groups, pkgSlug, panelSlug]
    )
    const { isReady: termsReady } = useTermsBlocked()

    // Wait for the terms state: until then every package with terms is closed.
    // Only for such a package, so no other panel unmounts if the state is lost.
    if (declaresTerms(pkgSlug ?? '') && !termsReady) return null

    if (!match) {
        return (
            <SystemSettingsScreen title="Settings not found" testID="settings-system-not-found">
                <Pressable onPress={() => router.push(orgHref('settings'))}>
                    <Text style={{ fontSize: 15, color: primaryColor }}>Back to Settings</Text>
                </Pressable>
            </SystemSettingsScreen>
        )
    }

    const { group, panel } = match
    const PanelComponent = panel.Component

    return (
        <SystemSettingsScreen
            title={`${group.packageName} — ${panel.label}`}
            testID={`settings-system-${pkgSlug}-${panelSlug}`}
            keyPrefix={panel.keyPrefix}
        >
            <Suspense fallback={null}>
                <View className="gap-4 p-5 rounded-2xl bg-surface-secondary border border-border">
                    <PanelComponent />
                </View>
            </Suspense>
        </SystemSettingsScreen>
    )
}
