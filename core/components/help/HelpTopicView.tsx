import { Text, View } from 'react-native'
import { substituteHelpTokens } from '../../lib/help/tokens'
import type { HelpTopic } from '../../lib/help/types'
import { MarkdownRenderer, openHelpLink } from './MarkdownRenderer'

interface Props {
    topic: HelpTopic
    showTitle?: boolean
}

export function HelpTopicView({ topic, showTitle = true }: Props) {
    return (
        <View>
            {showTitle && (
                <View className="mb-3">
                    <Text className="text-2xl font-bold text-foreground">{topic.title}</Text>
                    <Text className="text-sm text-muted-foreground mt-1">{topic.summary}</Text>
                </View>
            )}
            {/* The help-specific behaviors are now opt-in; a help topic opts
                into all of them. `openHelpLink` is module-level, so the
                renderer cache keys to a single stable slot. */}
            <MarkdownRenderer body={substituteHelpTokens(topic.body)} onLinkPress={openHelpLink} />
        </View>
    )
}
