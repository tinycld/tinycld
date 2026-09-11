import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { OrgBrandingSection } from '@tinycld/core/components/settings/OrgBrandingSection'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useNavigateBack } from '@tinycld/core/lib/use-navigate-back'
import { ArrowLeft, Image as ImageIcon } from 'lucide-react-native'
import { Pressable, ScrollView, Text, View } from 'react-native'

export default function OrganizationSettings() {
    const orgHref = useOrgHref()
    const navigateBack = useNavigateBack(() => orgHref('settings'))
    const fgColor = useThemeColor('foreground')

    return (
        <View className="flex-1 bg-background">
            <DocumentTitle pkg="Settings" title="Organization" />
            <View className="flex-row gap-3 items-center p-5 pb-0">
                <Pressable onPress={navigateBack}>
                    <ArrowLeft size={24} color={fgColor} />
                </Pressable>
                <ImageIcon size={24} color={fgColor} />
                <Text className="text-[22px] font-bold text-foreground">Organization</Text>
            </View>
            <ScrollView className="flex-1" contentContainerStyle={{ flexGrow: 1 }}>
                <View className="p-4 max-w-[600px] gap-6">
                    <OrgBrandingSection />
                </View>
            </ScrollView>
        </View>
    )
}
