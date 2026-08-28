import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { OAuthClientsSection } from '@tinycld/core/components/settings/OAuthClientsSection'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useNavigateBack } from '@tinycld/core/lib/use-navigate-back'
import { ArrowLeft, KeyRound } from 'lucide-react-native'
import { Pressable, ScrollView, Text, View } from 'react-native'

export default function OAuthClientsSettings() {
    const orgHref = useOrgHref()
    const navigateBack = useNavigateBack(() => orgHref('settings'))
    const { isAdmin, isReady } = useCurrentRole()

    const fgColor = useThemeColor('foreground')
    const mutedColor = useThemeColor('muted-foreground')

    // Gate on isReady before refusing: `isAdmin` is false both while the role
    // query loads and when the user genuinely lacks the role, so acting on the
    // transient value flashes "access required" at a legitimate admin who
    // deep-links on a cold load.
    if (isReady && !isAdmin) {
        return (
            <View className="flex-1 items-center justify-center p-5 bg-background">
                <DocumentTitle pkg="Settings" title="OAuth clients" />
                <View className="items-center gap-3 rounded-xl bg-surface-secondary border border-border px-6 py-8">
                    <KeyRound size={28} color={mutedColor} />
                    <Text className="text-foreground text-[15px] font-semibold">
                        Admin access required
                    </Text>
                    <Text className="text-muted-foreground text-[13px] text-center">
                        Only admins and owners can manage OAuth clients.
                    </Text>
                </View>
            </View>
        )
    }

    return (
        <>
            <DocumentTitle pkg="Settings" title="OAuth clients" />
            <ScrollView contentContainerStyle={{ flexGrow: 1 }} className="bg-background">
                <View className="flex-1 gap-6 p-5" style={{ maxWidth: 820 }}>
                    <View className="flex-row items-center gap-3">
                        <Pressable
                            onPress={navigateBack}
                            hitSlop={12}
                            className="rounded-full"
                            style={{ padding: 6 }}
                        >
                            <ArrowLeft size={22} color={fgColor} />
                        </Pressable>
                        <View className="flex-1 gap-0.5">
                            <Text
                                className="text-muted-foreground"
                                style={{ fontSize: 11, letterSpacing: 0.6 }}
                            >
                                Settings
                            </Text>
                            <Text
                                className="text-foreground"
                                style={{ fontSize: 24, fontWeight: '800' }}
                            >
                                OAuth clients
                            </Text>
                        </View>
                    </View>

                    <OAuthClientsSection isVisible={isReady && isAdmin} />
                </View>
            </ScrollView>
        </>
    )
}
