import { useSetupContinueStore } from '@tinycld/core/lib/stores/setup-continue-store'
import { useCallback, useEffect, useRef } from 'react'

interface SetupContinueButtonProps {
    onPress: () => void
    isDisabled?: boolean
    label?: string
}

/**
 * The button that finishes a setup step. It renders nothing where it is
 * placed: the wizard shell draws it in its footer, next to Skip, and this
 * component only tells the shell what the button does. Package steps use it
 * too, so e2e helpers can advance any step by its id instead of by its label.
 */
export function SetupContinueButton({
    onPress,
    isDisabled = false,
    label = 'Continue',
}: SetupContinueButtonProps) {
    const publish = useSetupContinueStore(s => s.publish)
    // Steps pass a new onPress closure on every render. The footer gets one
    // stable callback that reads the latest, so the slot is re-published only
    // when the label or disabled state changes, never on every keystroke.
    const latest = useRef(onPress)
    useEffect(() => {
        latest.current = onPress
    })
    const press = useCallback(() => latest.current(), [])
    useEffect(() => {
        publish({ label, onPress: press, isDisabled })
        return () => publish(null)
    }, [publish, label, press, isDisabled])
    return null
}
