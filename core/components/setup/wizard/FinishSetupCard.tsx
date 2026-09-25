import { appHref } from '@tinycld/core/lib/org-routes'
import { useSetupSteps } from '@tinycld/core/lib/setup/use-setup-steps'
import { useSetupWizardState } from '@tinycld/core/lib/setup/use-setup-wizard-state'
import { summarizeWizard } from '@tinycld/core/lib/setup/wizard-logic'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { useRouter } from 'expo-router'
import { Text, View } from 'react-native'
import { finishSetupSubtitleOf } from './finish-setup-summary'
import { StepStatusChain } from './use-setup-wizard'

/**
 * Stays at the top of Settings until the wizard is finished. "Finish later"
 * on a step only sets `dismissedAt` — this card is the one way back in, so it
 * must outlive that dismissal until `completedAt` is actually set.
 */
export function FinishSetupCard() {
    const { isAdmin } = useCurrentRole()
    const { state, update } = useSetupWizardState()
    const { steps } = useSetupSteps()

    if (!isAdmin || !state || state.completedAt) return null
    if (!steps) return null

    return (
        <StepStatusChain steps={steps}>
            {statuses => (
                <FinishSetupCardBody
                    summary={summarizeWizard(statuses, state)}
                    onContinue={() => update(s => ({ ...s, dismissedAt: undefined }))}
                />
            )}
        </StepStatusChain>
    )
}

function FinishSetupCardBody({
    summary,
    onContinue,
}: {
    summary: { doneCount: number; total: number }
    onContinue: () => Promise<void>
}) {
    const router = useRouter()

    const handleContinue = async () => {
        await onContinue()
        router.push(appHref('setup/next'))
    }

    return (
        <View className="mb-5 flex-row items-center justify-between gap-3 rounded-xl border border-border bg-surface-secondary p-4">
            <View className="gap-0.5">
                <Text className="text-base font-semibold text-foreground">Finish setup</Text>
                <Text className="text-sm text-muted-foreground">
                    {finishSetupSubtitleOf(summary)}
                </Text>
            </View>
            <Button onPress={handleContinue}>
                <ButtonText>Continue</ButtonText>
            </Button>
        </View>
    )
}
