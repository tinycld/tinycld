import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Users } from 'lucide-react-native'
import { Text, View } from 'react-native'

// Shown in place of the Invite button when every seat is in use.
export function LimitReachedNotice({
    isVisible,
    used,
    limit,
}: {
    isVisible: boolean
    used: number
    limit: number
}) {
    const warningColor = useThemeColor('warning')
    if (!isVisible) return null
    return (
        <View
            testID="members-limit-reached"
            className="flex-row items-start gap-2.5 rounded-xl p-3 bg-surface-secondary border border-border"
        >
            <Users size={16} color={warningColor} style={{ marginTop: 2 }} />
            <View className="flex-1 gap-0.5">
                <Text className="text-foreground" style={{ fontSize: 13, fontWeight: '700' }}>
                    User limit reached ({used} of {limit})
                </Text>
                <Text className="text-muted-foreground" style={{ fontSize: 12.5 }}>
                    Remove a user to invite someone.
                </Text>
            </View>
        </View>
    )
}
