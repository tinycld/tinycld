import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useNavigateBack } from '@tinycld/core/lib/use-navigate-back'
import { ArrowLeft } from 'lucide-react-native'
import type { ReactNode } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'

// The frame every single-topic settings screen shares: tab title, a back
// arrow to the settings hub, and a width-capped scrolling body.
export function SettingsScreen({ title, children }: { title: string; children: ReactNode }) {
    const orgHref = useOrgHref()
    const navigateBack = useNavigateBack(() => orgHref('settings'))
    const foregroundColor = useThemeColor('foreground')

    return (
        <ScrollView className="flex-1 bg-background" contentContainerStyle={{ flexGrow: 1 }}>
            <DocumentTitle pkg="Settings" title={title} />
            <View className="p-5 max-w-[600px] gap-6">
                <View className="flex-row gap-3 items-center">
                    <Pressable onPress={navigateBack}>
                        <ArrowLeft size={24} color={foregroundColor} />
                    </Pressable>
                    <Text className="text-foreground text-[22px] font-bold">{title}</Text>
                </View>
                {children}
            </View>
        </ScrollView>
    )
}
