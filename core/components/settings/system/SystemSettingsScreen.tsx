// Owner-gated shell for a system-settings screen: back arrow, title, and the
// role gate. These values are deployment-wide (they configure the whole server,
// not one organization), so they sit behind `isOwner` — the same bar as Packages
// and Build History. The collection rules are looser (owner OR admin), but we
// keep the UI tighter than the rule rather than widening the rule.

import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useIsSettingManaged } from '@tinycld/core/lib/use-managed-settings'
import { useNavigateBack } from '@tinycld/core/lib/use-navigate-back'
import { ArrowLeft } from 'lucide-react-native'
import type { ReactNode } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'

export function SystemSettingsScreen({
    title,
    testID,
    keyPrefix,
    children,
}: {
    title: string
    testID: string
    /**
     * The system_settings namespace this screen edits (e.g. `'vapid.'`).
     *
     * When the deployment's operator owns that namespace, the screen renders an
     * explanation instead of the form. The link to get here is already hidden;
     * this closes the deep link, which still resolves. Without it the form would
     * render, save, report success and change nothing — reads resolve through
     * the provider, not the row the form wrote.
     */
    keyPrefix?: string
    children: ReactNode
}) {
    const orgHref = useOrgHref()
    const navigateBack = useNavigateBack(() => orgHref('settings'))
    const { isOwner, isReady } = useCurrentRole()
    const fgColor = useThemeColor('foreground')
    const isManaged = useIsSettingManaged(keyPrefix)

    // Gated on isReady too, so a cold-load owner isn't bounced by the
    // transient null role.
    if (!isReady) return null

    if (isManaged) {
        return (
            <View className="flex-1 p-5 items-center justify-center bg-background">
                <DocumentTitle pkg="Settings" title={title} />
                <Text className="text-muted-foreground text-base text-center" testID={testID}>
                    {title} is configured by your hosting provider.
                </Text>
            </View>
        )
    }

    if (!isOwner) {
        return (
            <View className="flex-1 p-5 items-center justify-center bg-background">
                <DocumentTitle pkg="Settings" title={title} />
                <Text className="text-muted-foreground text-base">
                    Only owners can change system settings.
                </Text>
            </View>
        )
    }

    return (
        <ScrollView className="flex-1 bg-background" contentContainerStyle={{ flexGrow: 1 }}>
            <DocumentTitle pkg="Settings" title={title} />
            <View className="p-5 w-full gap-4" style={{ maxWidth: 1040 }} testID={testID}>
                <View className="flex-row gap-3 items-center">
                    <Pressable onPress={navigateBack}>
                        <ArrowLeft size={24} color={fgColor} />
                    </Pressable>
                    <Text className="text-foreground text-[22px] font-bold">{title}</Text>
                </View>
                {children}
            </View>
        </ScrollView>
    )
}
