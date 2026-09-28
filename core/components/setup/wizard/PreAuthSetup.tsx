import { useState } from 'react'
import { ClaimServerStep } from './ClaimServerStep'
import { CreateOwnerStep } from './CreateOwnerStep'
import { claimSummary } from './claim-summary'
import { SetupWizardShell } from './SetupWizardShell'

function usePreAuthSetup() {
    const [verifiedCode, setVerifiedCode] = useState<string | null>(null)
    const [rejection, setRejection] = useState<string | undefined>(undefined)
    return {
        verifiedCode,
        rejection,
        onVerified: (code: string) => {
            setRejection(undefined)
            setVerifiedCode(code)
        },
        onCodeRejected: (message: string) => {
            setRejection(message)
            setVerifiedCode(null)
        },
        // The only other claim step is the code screen; the account screen
        // is only reachable after it, so opening it here means going back.
        backToCode: () => {
            setRejection(undefined)
            setVerifiedCode(null)
        },
    }
}

/** The two screens before an owner exists: prove access, then create the owner. */
export function PreAuthSetup({ initialCode }: { initialCode: string | undefined }) {
    const s = usePreAuthSetup()
    if (s.verifiedCode === null) {
        return (
            <SetupWizardShell
                phase="claim"
                summary={claimSummary(false)}
                currentStepId="claim:code"
                onFinishLater={null}
                onSkip={null}
            >
                <ClaimServerStep
                    initialCode={initialCode}
                    notice={s.rejection}
                    onVerified={s.onVerified}
                />
            </SetupWizardShell>
        )
    }
    return (
        <SetupWizardShell
            phase="claim"
            summary={claimSummary(true)}
            currentStepId="claim:account"
            onFinishLater={null}
            onSkip={null}
            onOpenStep={s.backToCode}
        >
            <CreateOwnerStep code={s.verifiedCode} onCodeRejected={s.onCodeRejected} />
        </SetupWizardShell>
    )
}
