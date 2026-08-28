import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { BuildHistoryTab } from '@tinycld/core/components/setup/BuildHistoryTab'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { pb } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useNavigateBack } from '@tinycld/core/lib/use-navigate-back'
import { ArrowLeft } from 'lucide-react-native'
import { Pressable, ScrollView, Text, View } from 'react-native'

// Owner-only, matching the install manager it reports on: a build row's revert
// action re-deploys the artifact the whole deployment runs. Gated on isReady
// too, so a cold-load owner isn't bounced by the transient null role.
export default function BuildHistorySettings() {
    const orgHref = useOrgHref()
    const navigateBack = useNavigateBack(() => orgHref('settings'))
    const { isOwner, isReady } = useCurrentRole()
    const fgColor = useThemeColor('foreground')

    if (!isReady) return null

    if (!isOwner) {
        return (
            <View className="flex-1 p-5 items-center justify-center bg-background">
                <DocumentTitle pkg="Settings" title="Build History" />
                <Text className="text-muted-foreground text-base">
                    Only owners can view build history.
                </Text>
            </View>
        )
    }

    return (
        <ScrollView className="flex-1 bg-background" contentContainerStyle={{ flexGrow: 1 }}>
            <DocumentTitle pkg="Settings" title="Build History" />
            <View
                className="p-5 w-full gap-4"
                style={{ maxWidth: 1040 }}
                testID="settings-section-build-history"
            >
                <View className="flex-row gap-3 items-center">
                    <Pressable onPress={navigateBack}>
                        <ArrowLeft size={24} color={fgColor} />
                    </Pressable>
                    <Text className="text-foreground text-[22px] font-bold">Build History</Text>
                </View>
                <BuildHistoryTab isVisible pb={pb} />
            </View>
        </ScrollView>
    )
}
