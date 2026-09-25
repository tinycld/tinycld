import { useMutation } from '@tinycld/core/lib/mutations'
import { appHref } from '@tinycld/core/lib/org-routes'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { useRouter } from 'expo-router'
import { Text, View } from 'react-native'
import { useWorkspacePreview } from '../use-workspace-preview'
import { WorkspacePreview } from '../WorkspacePreview'
import { doneHeadingOf, doneSummaryOf } from './done-summary'

function useDoneStep(complete: () => Promise<void>) {
    const router = useRouter()
    const model = useWorkspacePreview()
    const open = useMutation({
        mutationFn: complete,
        onSuccess: () => router.replace(appHref('')),
    })
    return {
        model,
        ...doneHeadingOf(model.name),
        summary: doneSummaryOf({
            appCount: model.apps.length,
            memberCount: model.memberCount,
        }),
        onOpen: () => open.mutate(),
        isPending: open.isPending,
    }
}

/** The last screen: the finished workspace, then a button that marks setup complete and opens it. */
export function DoneStep({ complete }: { complete: () => Promise<void> }) {
    const { model, heading, buttonLabel, summary, onOpen, isPending } = useDoneStep(complete)
    return (
        <View className="w-full items-center gap-3.5">
            <View className="w-full max-w-[420px] items-center">
                <WorkspacePreview model={model} isNewApps={false} />
            </View>
            <Text className="text-center text-2xl font-bold text-foreground">{heading}</Text>
            <Text className="text-center text-sm text-muted-foreground">{summary}</Text>
            <Button onPress={onOpen} isDisabled={isPending}>
                <ButtonText>{buttonLabel}</ButtonText>
            </Button>
        </View>
    )
}
