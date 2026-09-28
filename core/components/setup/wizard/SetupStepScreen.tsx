import { appHref } from '@tinycld/core/lib/org-routes'
import { paramToStepId, stepIdToParam } from '@tinycld/core/lib/setup/registry'
import type { LoadedSetupStep, SetupStepProps, StepStatus } from '@tinycld/core/lib/setup/types'
import { useSetupSteps } from '@tinycld/core/lib/setup/use-setup-steps'
import { summarizeWizard, type WizardSummary } from '@tinycld/core/lib/setup/wizard-logic'
import { Redirect, useRouter } from 'expo-router'
import type { ComponentType } from 'react'
import { SetupWizardShell } from './SetupWizardShell'
import { DoneStep } from './steps/DoneStep'
import { StepStatusChain, useWizardActions, type WizardActions } from './use-setup-wizard'

const NEXT_HREF = appHref('setup/next')

type StepScreen =
    | { kind: 'loading' }
    | { kind: 'redirect'; href: string }
    | {
          kind: 'step'
          summary: WizardSummary
          currentStepId: string
          Component: ComponentType<SetupStepProps>
          next: () => void
          onSkip: (() => void) | null
          openStep: (id: string) => void
      }
    | {
          kind: 'done'
          summary: WizardSummary
          complete: () => Promise<void>
          openStep: (id: string) => void
      }

// A failed save has already been reported by its mutation (toast + log), so
// the person simply stays on the step and can try again.
function stayOnFailure() {}

function useStepScreen(
    param: string,
    steps: LoadedSetupStep[],
    statuses: StepStatus[],
    actions: WizardActions
): StepScreen {
    const router = useRouter()
    // Each action awaits its single write before navigating; see useWizardActions.
    // Steps are pushed, not replaced, so the browser's back button returns to
    // the previous step.
    const thenGo = (write: () => Promise<void>, href: string) => () => {
        write().then(() => router.push(href), stayOnFailure)
    }
    const openStep = (id: string) => router.push(appHref(`setup/${stepIdToParam(id)}`))

    if (!actions.state) return { kind: 'redirect', href: appHref('') }
    const summary = summarizeWizard(statuses, actions.state)

    if (param === 'next') {
        if (!summary.isSettled) return { kind: 'loading' }
        const target = summary.nextStepId
            ? `setup/${stepIdToParam(summary.nextStepId)}`
            : 'setup/done'
        return { kind: 'redirect', href: appHref(target) }
    }

    if (param === 'done') return { kind: 'done', summary, complete: actions.complete, openStep }

    const id = paramToStepId(param)
    const step = steps.find(s => s.id === id)
    const status = statuses.find(s => s.id === id)
    if (step && status && status.isVisible === undefined) return { kind: 'loading' }
    if (!step || !status?.isVisible) return { kind: 'redirect', href: NEXT_HREF }

    return {
        kind: 'step',
        summary,
        currentStepId: id,
        Component: step.Component,
        next: thenGo(() => actions.continueStep(status), NEXT_HREF),
        onSkip: thenGo(() => actions.skip(id), NEXT_HREF),
        openStep,
    }
}

function StepScreenBody({
    param,
    steps,
    statuses,
    actions,
}: {
    param: string
    steps: LoadedSetupStep[]
    statuses: StepStatus[]
    actions: WizardActions
}) {
    const screen = useStepScreen(param, steps, statuses, actions)
    if (screen.kind === 'loading') return null
    if (screen.kind === 'redirect') return <Redirect href={screen.href} />
    if (screen.kind === 'done') {
        return (
            <SetupWizardShell
                phase="setup"
                summary={screen.summary}
                currentStepId={null}
                onSkip={null}
                onOpenStep={screen.openStep}
            >
                <DoneStep complete={screen.complete} />
            </SetupWizardShell>
        )
    }
    const { Component } = screen
    return (
        <SetupWizardShell
            phase="setup"
            summary={screen.summary}
            currentStepId={screen.currentStepId}
            onSkip={screen.onSkip}
            onOpenStep={screen.openStep}
        >
            <Component next={screen.next} />
        </SetupWizardShell>
    )
}

/** One signed-in wizard screen: a registry step, `next` (resume) or `done`. */
export function SetupStepScreen({ param }: { param: string }) {
    const { steps } = useSetupSteps()
    const actions = useWizardActions()
    // Loading is sub-second, so there is no skeleton.
    if (!steps || !actions.isReady) return null
    return (
        <StepStatusChain steps={steps}>
            {statuses => (
                <StepScreenBody param={param} steps={steps} statuses={statuses} actions={actions} />
            )}
        </StepStatusChain>
    )
}
