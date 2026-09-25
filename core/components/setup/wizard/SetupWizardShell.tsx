import type { WizardSummary } from '@tinycld/core/lib/setup/wizard-logic'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import type { ReactNode } from 'react'
import { ScrollView, Text, useWindowDimensions, View } from 'react-native'
import { ProgressSegments } from './ProgressSegments'
import { ServerLogPreview } from './ServerLogPreview'
import { ghostPreviewModel, type PreviewModel, useWorkspacePreview } from './use-workspace-preview'
import { WorkspacePreview } from './WorkspacePreview'
import { WorkspacePreviewStrip } from './WorkspacePreviewStrip'

type PreviewKind = 'workspace' | 'server-log' | 'ghost'

export interface SetupWizardShellProps {
    /**
     * 'claim' is the pre-auth code + owner-account pair; its progress comes
     * from a synthetic summary and its label is always "Claim this server".
     */
    phase: 'claim' | 'setup'
    summary: WizardSummary | null
    currentStepId: string | null
    onFinishLater: (() => void) | null
    onSkip: (() => void) | null
    preview: PreviewKind
    code: string
    /** Shown as the only avatar on the ghost preview, before any user exists. */
    ghostInitials?: string
    children: ReactNode
}

// The breakpoint is read in JS, not with `md:` classes, so the phone layout is
// chosen the same way on web and native.
const PHONE_MAX_WIDTH = 768

const EMPTY_SUMMARY: WizardSummary = {
    steps: [],
    nextStepId: null,
    doneCount: 0,
    total: 0,
    isSettled: true,
}

// Apps chosen on this step are shown as "new" so each toggle is visible on the
// rail; on every other step they are part of the settled workspace.
const APPS_STEP_ID = 'core:apps'

function stepLabelOf(props: SetupWizardShellProps): string {
    const { phase, summary, currentStepId } = props
    if (phase === 'claim' || !summary) return 'Claim this server'
    const index = summary.steps.findIndex(s => s.id === currentStepId)
    if (index === -1) return `${Math.min(summary.doneCount + 1, summary.total)} of ${summary.total}`
    return `${summary.steps[index].label} · ${index + 1} of ${summary.total}`
}

function useSetupWizardLayout(props: SetupWizardShellProps, model: PreviewModel) {
    const { width } = useWindowDimensions()
    const isPhone = width < PHONE_MAX_WIDTH
    return {
        model,
        isNewApps: props.currentStepId === APPS_STEP_ID,
        summary: props.summary ?? EMPTY_SUMMARY,
        stepLabel: stepLabelOf(props),
        showStrip: isPhone,
        formClassName: isPhone ? 'w-full' : 'w-[52%]',
        preview: props.preview,
    }
}

type SetupWizardLayout = ReturnType<typeof useSetupWizardLayout>

function FinishLaterButton({ onPress }: { onPress: (() => void) | null }) {
    if (!onPress) return null
    return (
        <Button variant="link" size="sm" onPress={onPress} accessibilityLabel="Finish setup later">
            <ButtonText>Finish later</ButtonText>
        </Button>
    )
}

function SkipButton({ onPress }: { onPress: (() => void) | null }) {
    if (!onPress) return null
    return (
        <Button
            variant="link"
            className="self-start"
            onPress={onPress}
            accessibilityLabel="Skip this step"
        >
            <ButtonText>Skip</ButtonText>
        </Button>
    )
}

function PreviewBody({ layout, code }: { layout: SetupWizardLayout; code: string }) {
    if (layout.preview === 'server-log') return <ServerLogPreview code={code} />
    return <WorkspacePreview model={layout.model} isNewApps={layout.isNewApps} />
}

function PreviewPane({ layout, code }: { layout: SetupWizardLayout; code: string }) {
    if (layout.showStrip) return null
    return (
        <View className="flex-1 items-center justify-center border-l border-border bg-surface-secondary p-5">
            <PreviewBody layout={layout} code={code} />
        </View>
    )
}

/**
 * Frame for every wizard step: progress along the top, the step's form on the
 * left and a live miniature of the workspace on the right (a strip on phones).
 */
export function SetupWizardShell(props: SetupWizardShellProps) {
    if (props.preview === 'workspace') return <LiveShell {...props} />
    // Pre-auth screens never show live org data (the name there is the server
    // default), and there is nobody signed in to read it as, so they do not
    // subscribe to it at all.
    return <ShellFrame props={props} model={ghostPreviewModel(props.ghostInitials)} />
}

function LiveShell(props: SetupWizardShellProps) {
    const model = useWorkspacePreview()
    return <ShellFrame props={props} model={model} />
}

function ShellFrame({ props, model }: { props: SetupWizardShellProps; model: PreviewModel }) {
    const layout = useSetupWizardLayout(props, model)
    return (
        <View className="flex-1 bg-background">
            <WorkspacePreviewStrip
                model={layout.model}
                isNewApps={layout.isNewApps}
                isVisible={layout.showStrip}
            />
            <View className="flex-row items-center gap-3 border-b border-border px-4 py-2.5">
                <ProgressSegments summary={layout.summary} currentStepId={props.currentStepId} />
                <Text className="text-[11px] text-muted-foreground">{layout.stepLabel}</Text>
                <FinishLaterButton onPress={props.onFinishLater} />
            </View>
            <View className="flex-1 flex-row">
                <ScrollView className={layout.formClassName} contentContainerClassName="p-6 gap-4">
                    {props.children}
                    <SkipButton onPress={props.onSkip} />
                </ScrollView>
                <PreviewPane layout={layout} code={props.code} />
            </View>
        </View>
    )
}
