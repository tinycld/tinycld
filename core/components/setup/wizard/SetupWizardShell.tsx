import { useBreakpoint } from '@tinycld/core/components/workspace/useBreakpoint'
import {
    SETUP_CONTINUE_TEST_ID,
    SETUP_SKIP_TEST_ID,
    setupStepTestId,
} from '@tinycld/core/lib/setup/step-ids'
import type { WizardSummary } from '@tinycld/core/lib/setup/wizard-logic'
import { useSetupContinueStore } from '@tinycld/core/lib/stores/setup-continue-store'
import { Button, ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
import type { ReactNode } from 'react'
import { ScrollView, Text, View } from 'react-native'
import { StepList } from './StepList'

export interface SetupWizardShellProps {
    /**
     * 'claim' is the pre-auth code + owner-account pair; its progress comes
     * from a synthetic summary.
     */
    phase: 'claim' | 'setup'
    summary: WizardSummary | null
    currentStepId: string | null
    onSkip: (() => void) | null
    /** Opens another step from the list; absent when steps cannot be revisited. */
    onOpenStep?: (id: string) => void
    children: ReactNode
}

const EMPTY_SUMMARY: WizardSummary = {
    steps: [],
    nextStepId: null,
    doneCount: 0,
    total: 0,
    isSettled: true,
}

const EYEBROW_CLASS = 'text-[11px] font-semibold uppercase tracking-wider text-muted-foreground'

function positionLabelOf(props: SetupWizardShellProps): string {
    const { summary, currentStepId } = props
    if (!summary) return ''
    const index = summary.steps.findIndex(s => s.id === currentStepId)
    if (index === -1) return ''
    return `${index + 1} of ${summary.total}`
}

function useSetupWizardLayout(props: SetupWizardShellProps) {
    const isPhone = useBreakpoint() === 'mobile'
    return {
        summary: props.summary ?? EMPTY_SUMMARY,
        positionLabel: positionLabelOf(props),
        eyebrow: props.phase === 'claim' ? 'New server' : 'Organization setup',
        // Claim and Done have no registry step, so they carry no step id.
        stepTestId: props.currentStepId ? setupStepTestId(props.currentStepId) : undefined,
        // On a phone the card is the screen; elsewhere it floats on the backdrop.
        pageClassName: isPhone ? 'flex-grow' : 'flex-grow items-center justify-center px-6 py-10',
        cardClassName: isPhone
            ? 'flex-1 bg-background'
            : 'w-full max-w-[640px] rounded-2xl border border-border bg-background shadow-lg',
        bodyClassName: isPhone ? 'px-5 py-6 gap-6' : 'px-10 py-9 gap-7',
    }
}

type SetupWizardLayout = ReturnType<typeof useSetupWizardLayout>

function PositionLabel({ label }: { label: string }) {
    if (!label) return null
    return <Text className="text-[13px] text-muted-foreground">{label}</Text>
}

function ShellHeader({
    layout,
    props,
}: {
    layout: SetupWizardLayout
    props: SetupWizardShellProps
}) {
    return (
        <View className="gap-4">
            <View className="flex-row items-center justify-between">
                <Text className={EYEBROW_CLASS}>{layout.eyebrow}</Text>
                <PositionLabel label={layout.positionLabel} />
            </View>
            <StepList
                summary={layout.summary}
                currentStepId={props.currentStepId}
                onOpen={props.onOpenStep}
            />
            <View className="h-px bg-border" />
        </View>
    )
}

function SkipButton({ onPress }: { onPress: (() => void) | null }) {
    if (!onPress) return <View />
    return (
        <Button
            variant="link"
            size="sm"
            className="px-0"
            onPress={onPress}
            accessibilityLabel="Skip this step"
            testID={SETUP_SKIP_TEST_ID}
        >
            <ButtonText className="text-[13px] text-muted-foreground">Skip this step</ButtonText>
        </Button>
    )
}

/** The Continue the screen's SetupContinueButton published; nothing until one has. */
function ContinueButton() {
    const action = useSetupContinueStore(s => s.action)
    if (!action) return null
    const ContinueAction = action.requiresServer ? ServerActionButton : Button
    return (
        <ContinueAction
            size="lg"
            className="min-h-11"
            onPress={action.onPress}
            isDisabled={action.isDisabled}
            testID={SETUP_CONTINUE_TEST_ID}
        >
            <ButtonText className="text-[15px] font-semibold">{action.label}</ButtonText>
        </ContinueAction>
    )
}

/** Skip on the left, Continue on the right; hidden when the screen has neither. */
function ShellFooter({ props }: { props: SetupWizardShellProps }) {
    const hasContinue = useSetupContinueStore(s => s.action !== null)
    if (!props.onSkip && !hasContinue) return null
    return (
        <View className="flex-row items-center justify-between border-t border-border pt-4">
            <SkipButton onPress={props.onSkip} />
            <ContinueButton />
        </View>
    )
}

/**
 * Frame for every wizard screen: one card, centred on the page, with the
 * step list along its top, the step's form in the middle and Skip / Continue
 * at its foot. Package steps render inside the same card.
 */
export function SetupWizardShell(props: SetupWizardShellProps) {
    const layout = useSetupWizardLayout(props)
    return (
        <ScrollView
            className="flex-1 bg-surface-secondary"
            contentContainerClassName={layout.pageClassName}
        >
            <View className={layout.cardClassName}>
                <View testID={layout.stepTestId} className={layout.bodyClassName}>
                    <ShellHeader layout={layout} props={props} />
                    {props.children}
                    <ShellFooter props={props} />
                </View>
            </View>
        </ScrollView>
    )
}
