import { HelpIcon } from '@tinycld/core/components/help/HelpIcon'
import type { HelpTopicId } from '@tinycld/core/lib/help/types'
import { Text, View } from 'react-native'

function StepHelp({ topic }: { topic: HelpTopicId | undefined }) {
    if (!topic) return null
    return <HelpIcon topic={topic} />
}

/**
 * The title and lead of a setup step. Package steps use it too, so every
 * screen of the wizard opens with the same type.
 */
export function StepHeading({
    title,
    lead,
    helpTopic,
}: {
    title: string
    lead: string
    helpTopic?: HelpTopicId
}) {
    return (
        <View className="mb-7 gap-2.5">
            <View className="flex-row items-center gap-2.5">
                <Text className="text-[30px] font-bold leading-9 tracking-tight text-foreground">
                    {title}
                </Text>
                <StepHelp topic={helpTopic} />
            </View>
            <Text className="max-w-[460px] text-[15px] leading-6 text-muted-foreground">
                {lead}
            </Text>
        </View>
    )
}
