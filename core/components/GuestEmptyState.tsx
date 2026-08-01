import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { router } from 'expo-router'
import { Share2 } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'

// The landing screen for a guest with zero accessible packages. Before this
// existed, such a guest was silently redirected to Settings — which shows
// only "Personal" — with no explanation of why the app looks empty and no
// hint that their share links still work.
export function GuestEmptyState() {
    const mutedColor = useThemeColor('muted-foreground')
    const orgHref = useOrgHref()

    return (
        <View
            testID="guest-empty-state"
            className="flex-1 bg-background items-center justify-center gap-3 p-6"
        >
            <Share2 size={28} color={mutedColor} />
            <Text className="text-foreground" style={{ fontSize: 16, fontWeight: '700' }}>
                Nothing has been shared with you yet
            </Text>
            <Text
                className="text-muted-foreground text-center"
                style={{ fontSize: 13, lineHeight: 19, maxWidth: 440 }}
            >
                You’re signed in as a guest, so you only see apps an owner or admin has specifically
                granted you. If someone sent you a share link, open that link directly — it keeps
                working without any of this.
            </Text>
            <Pressable
                onPress={() => router.replace(orgHref('settings'))}
                className="rounded-md border border-border"
                style={{ paddingVertical: 8, paddingHorizontal: 14, marginTop: 4 }}
            >
                <Text className="text-foreground" style={{ fontSize: 13, fontWeight: '600' }}>
                    Personal settings
                </Text>
            </Pressable>
        </View>
    )
}
