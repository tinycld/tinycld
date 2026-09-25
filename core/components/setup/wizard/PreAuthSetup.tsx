import { useState } from 'react'
import { ClaimServerStep } from './ClaimServerStep'
import { CreateOwnerStep } from './CreateOwnerStep'
import { claimSummary } from './claim-summary'
import { SetupWizardShell } from './SetupWizardShell'
import { formatSetupCode } from './setup-api'

function usePreAuthSetup() {
    const [verifiedCode, setVerifiedCode] = useState<string | null>(null)
    const [rejection, setRejection] = useState<string | undefined>(undefined)
    const [ghostInitials, setGhostInitials] = useState('')
    return {
        verifiedCode,
        rejection,
        ghostInitials,
        setGhostInitials,
        onVerified: (code: string) => {
            setRejection(undefined)
            setVerifiedCode(code)
        },
        onCodeRejected: (message: string) => {
            setRejection(message)
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
                preview="server-log"
                code={formatSetupCode(initialCode ?? '')}
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
            preview="ghost"
            code=""
            ghostInitials={s.ghostInitials}
        >
            <CreateOwnerStep
                code={s.verifiedCode}
                onNameChange={s.setGhostInitials}
                onCodeRejected={s.onCodeRejected}
            />
        </SetupWizardShell>
    )
}
