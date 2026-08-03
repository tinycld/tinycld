import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { ServersSection } from '@tinycld/core/components/settings/ServersSection'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useNavigateBack } from '@tinycld/core/lib/use-navigate-back'
import { ArrowLeft, Server } from 'lucide-react-native'
import { Pressable, ScrollView, Text, View } from 'react-native'

// Saved servers live at the top level of Settings rather than under Personal:
// they are device/connection scope, not a personal preference — and this is the
// one settings screen that still means something when you are NOT signed in to
// the server you are currently pointed at.
//
// The quick-switch affordance is in the app chrome (More drawer on phone, user
// menu on tablet). This screen is the full management surface, and the only place
// that offers Remove.
export default function ServersSettings() {
    const orgHref = useOrgHref()
    const navigateBack = useNavigateBack(() => orgHref('settings'))
    const fgColor = useThemeColor('foreground')

    return (
        <View className="flex-1 bg-background">
            <DocumentTitle pkg="Settings" title="Servers" />
            <View className="flex-row gap-3 items-center p-5 pb-0">
                <Pressable onPress={navigateBack}>
                    <ArrowLeft size={24} color={fgColor} />
                </Pressable>
                <Server size={24} color={fgColor} />
                <Text className="text-[22px] font-bold text-foreground">Servers</Text>
            </View>
            <ScrollView className="flex-1" contentContainerStyle={{ padding: 16 }}>
                <View className="max-w-[600px] w-full">
                    <ServersSection />
                </View>
            </ScrollView>
        </View>
    )
}
