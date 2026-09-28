import { useMutation } from '@tinycld/core/lib/mutations'
import { appHref } from '@tinycld/core/lib/org-routes'
import { SETUP_DONE_TEST_ID } from '@tinycld/core/lib/setup/step-ids'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { useRouter } from 'expo-router'
import { Check } from 'lucide-react-native'
import { Text, View } from 'react-native'
import { useWorkspaceSummary } from '../use-workspace-summary'
import { doneHeadingOf, doneSummaryOf } from './done-summary'

function useDoneStep(complete: () => Promise<void>) {
    const router = useRouter()
    const workspace = useWorkspaceSummary()
    const open = useMutation({
        mutationFn: complete,
        onSuccess: () => router.replace(appHref('')),
    })
    return {
        ...doneHeadingOf(workspace.name),
        summary: doneSummaryOf(workspace),
        onOpen: () => open.mutate(),
        isPending: open.isPending,
    }
}

/** The last screen: a summary of what was set up, then a button that marks setup complete and opens the workspace. */
export function DoneStep({ complete }: { complete: () => Promise<void> }) {
    const { heading, buttonLabel, summary, onOpen, isPending } = useDoneStep(complete)
    const onPrimary = useThemeColor('primary-foreground')
    return (
        <View testID={SETUP_DONE_TEST_ID} className="items-center gap-6 py-6">
            <View className="size-16 items-center justify-center rounded-full bg-primary">
                <Check size={30} color={onPrimary} strokeWidth={3} />
            </View>
            <View className="items-center gap-2">
                <Text className="text-center text-[30px] font-bold leading-9 tracking-tight text-foreground">
                    {heading}
                </Text>
                <Text className="text-center text-[15px] text-muted-foreground">{summary}</Text>
            </View>
            <Button size="lg" className="min-h-11" onPress={onOpen} isDisabled={isPending}>
                <ButtonText className="text-[15px] font-semibold">{buttonLabel}</ButtonText>
            </Button>
        </View>
    )
}
