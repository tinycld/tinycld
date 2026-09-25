import { useQueryClient } from '@tanstack/react-query'
import { OrgBrandingSection } from '@tinycld/core/components/settings/OrgBrandingSection'
import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import { pb } from '@tinycld/core/lib/pocketbase'
import { useSetupPreviewStore } from '@tinycld/core/lib/setup/setup-preview-store'
import type { SetupStepProps } from '@tinycld/core/lib/setup/types'
import { ORG_INFO_QUERY_KEY } from '@tinycld/core/lib/use-org-info'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { TextInput, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { useEffect } from 'react'
import { Text, View } from 'react-native'
import { useChosenOrgName } from '../use-workspace-preview'

const workspaceSchema = z.object({
    name: z.string().trim().min(1, 'Enter a name').max(255),
})

type WorkspaceForm = z.infer<typeof workspaceSchema>

function setDraftName(value: string | null) {
    useSetupPreviewStore.getState().setDraftName(value)
}

function useSaveWorkspace(next: () => void) {
    const queryClient = useQueryClient()
    const chosenName = useChosenOrgName()
    const form = useForm<WorkspaceForm>({
        resolver: zodResolver(workspaceSchema),
        // `values` fills the field once org info arrives on a cold deep link;
        // keepDirtyValues stops that from overwriting what was already typed.
        values: { name: chosenName },
        resetOptions: { keepDirtyValues: true },
    })
    // An unsaved name must not linger in the preview after leaving the step
    // (Skip, Finish later), so the draft goes when the step unmounts.
    useEffect(() => () => setDraftName(null), [])
    const save = useMutation({
        mutationFn: async ({ name }: WorkspaceForm) => {
            await pb.send('/api/org-info/name', { method: 'POST', body: { name } })
        },
        onSuccess: async () => {
            await queryClient.invalidateQueries({ queryKey: ORG_INFO_QUERY_KEY })
            // The draft stays until the step unmounts: the saved name counts
            // only once `next` has acknowledged the step, and clearing the
            // draft first would blank the preview in between.
            next()
        },
        onError: handleMutationErrorsWithForm({
            setError: form.setError,
            getValues: form.getValues,
            operation: 'setup.workspace',
        }),
    })
    return {
        control: form.control,
        onSubmit: form.handleSubmit(data => save.mutate(data)),
        isPending: save.isPending,
    }
}

// Acknowledged-only: every new server already has a name (PocketBase's
// "Acme"), so the stored name cannot tell whether anyone chose one.
export default function WorkspaceStep({ next }: SetupStepProps) {
    const { control, onSubmit, isPending } = useSaveWorkspace(next)
    return (
        <View className="max-w-[440px] gap-1">
            <Text className="text-2xl font-bold text-foreground">Your workspace</Text>
            <Text className="mb-3 text-sm text-muted-foreground">
                People see this name and logo when they sign in and in invite emails.
            </Text>
            <TextInput
                control={control}
                name="name"
                label="Workspace name"
                placeholder="Harbor Dental"
                onValueChange={setDraftName}
                onSubmitEditing={onSubmit}
            />
            <View className="mb-4">
                <OrgBrandingSection />
            </View>
            <Button className="self-start" onPress={onSubmit} isDisabled={isPending}>
                <ButtonText>Continue</ButtonText>
            </Button>
        </View>
    )
}
