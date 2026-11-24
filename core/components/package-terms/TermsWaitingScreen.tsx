import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Hourglass } from 'lucide-react-native'
import { Text, View } from 'react-native'

/** What everybody but the owner sees in place of a package whose terms are not accepted. */
export function TermsWaitingScreen({ name }: { name: string }) {
    const mutedColor = useThemeColor('muted-foreground')
    return (
        <View
            testID="pkg-terms-waiting"
            className="flex-1 items-center justify-center gap-3 bg-background p-6"
        >
            <Hourglass size={28} color={mutedColor} />
            <Text className="text-center text-base font-bold text-foreground">
                Waiting for the owner to accept the terms
            </Text>
            <Text className="max-w-[420px] text-center text-sm leading-5 text-muted-foreground">
                {name} cannot be used until the owner of this workspace accepts its terms and
                conditions. Ask the owner to open {name}.
            </Text>
        </View>
    )
}
