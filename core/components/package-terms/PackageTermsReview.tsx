import type { PackageTermsEntry } from '@tinycld/core/lib/packages/use-package-terms'
import { useTermsDecision } from '@tinycld/core/lib/packages/use-package-terms'
import { ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
import { ScrollView, Text, View } from 'react-native'
import { TermsDocument } from './TermsDocument'

/**
 * What the owner sees in place of a package whose terms are not accepted:
 * the terms, Agree and Decline. Agree has no keyboard shortcut on purpose:
 * an acceptance of terms is a deliberate press.
 */
export function PackageTermsReview({ entry }: { entry: PackageTermsEntry }) {
    const { decide, isPending } = useTermsDecision()
    return (
        <View testID="pkg-terms-review" className="flex-1 bg-background">
            <ScrollView
                className="flex-1"
                contentContainerClassName="w-full max-w-[720px] self-center gap-3 p-5"
            >
                <Text className="text-xl font-bold text-foreground">
                    {entry.name}: terms and conditions
                </Text>
                <Text className="text-sm text-muted-foreground">
                    Nobody in this workspace can use {entry.name} until you agree to these terms.
                </Text>
                <DeclinedNote isVisible={entry.isDeclined} />
                <TermsDocument slug={entry.slug} />
            </ScrollView>
            <View className="flex-row justify-end gap-3 border-t border-border p-4">
                <DeclineButton
                    isVisible={!entry.isDeclined}
                    isDisabled={isPending}
                    onPress={() => decide({ entry, decision: 'declined' })}
                />
                <ServerActionButton
                    testID="pkg-terms-agree"
                    isDisabled={isPending}
                    onPress={() => decide({ entry, decision: 'accepted' })}
                >
                    <ButtonText>Agree</ButtonText>
                </ServerActionButton>
            </View>
        </View>
    )
}

function DeclinedNote({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <Text
            testID="pkg-terms-declined"
            className="rounded-md border border-border bg-surface-secondary p-3 text-sm text-foreground"
        >
            You declined these terms. The package stays off until you agree.
        </Text>
    )
}

function DeclineButton({
    isVisible,
    isDisabled,
    onPress,
}: {
    isVisible: boolean
    isDisabled: boolean
    onPress: () => void
}) {
    if (!isVisible) return null
    return (
        <ServerActionButton
            testID="pkg-terms-decline"
            variant="outline"
            isDisabled={isDisabled}
            onPress={onPress}
        >
            <ButtonText>Decline</ButtonText>
        </ServerActionButton>
    )
}
