import { useQueryClient } from '@tanstack/react-query'
import { OrgBrandingSection } from '@tinycld/core/components/settings/OrgBrandingSection'
import { SetupContinueButton } from '@tinycld/core/components/setup/wizard/SetupContinueButton'
import { StepHeading } from '@tinycld/core/components/setup/wizard/StepHeading'
import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import { pb } from '@tinycld/core/lib/pocketbase'
import { SETUP_WORKSPACE_NAME_TEST_ID } from '@tinycld/core/lib/setup/step-ids'
import type { SetupStepProps } from '@tinycld/core/lib/setup/types'
import { ORG_INFO_QUERY_KEY } from '@tinycld/core/lib/use-org-info'
import { TextInput, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { View } from 'react-native'
import { useChosenOrgName } from '../use-workspace-summary'

const workspaceSchema = z.object({
    name: z.string().trim().min(1, 'Enter a name').max(255),
})

type WorkspaceForm = z.infer<typeof workspaceSchema>

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
    const save = useMutation({
        mutationFn: async ({ name }: WorkspaceForm) => {
            await pb.send('/api/org-info/name', { method: 'POST', body: { name } })
        },
        onSuccess: async () => {
            await queryClient.invalidateQueries({ queryKey: ORG_INFO_QUERY_KEY })
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
        <View>
            <StepHeading
                title="Your organization"
                lead="The name and logo appear on every screen of the app, on the sign-in screen, and in invite emails."
            />
            <TextInput
                control={control}
                name="name"
                label="Organization name"
                testID={SETUP_WORKSPACE_NAME_TEST_ID}
                placeholder="Harbor Dental"
                onSubmitEditing={onSubmit}
            />
            <View className="mb-5">
                <OrgBrandingSection variant="field" />
            </View>
            <SetupContinueButton onPress={onSubmit} isDisabled={isPending} />
        </View>
    )
}
