import { useSetupContinueStore } from '@tinycld/core/lib/stores/setup-continue-store'
import { useCallback, useEffect, useRef } from 'react'

interface SetupBackButtonProps {
    onPress: () => void
    isVisible?: boolean
}

/**
 * The Back of a setup step that has parts of its own. Like SetupContinueButton
 * it renders nothing where it is placed: the wizard shell draws it in its
 * footer, to the left of Continue. A step leaves it out (or passes
 * isVisible false) on its first part.
 */
export function SetupBackButton({ onPress, isVisible = true }: SetupBackButtonProps) {
    const publishBack = useSetupContinueStore(s => s.publishBack)
    const latest = useRef(onPress)
    useEffect(() => {
        latest.current = onPress
    })
    const press = useCallback(() => latest.current(), [])
    useEffect(() => {
        if (!isVisible) return
        publishBack({ onPress: press })
        return () => publishBack(null)
    }, [publishBack, press, isVisible])
    return null
}
