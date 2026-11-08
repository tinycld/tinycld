import { useCallback, useEffect, useRef, useState } from 'react'

const FLASH_MS = 150

/**
 * A keyboard shortcut presses the button with no pointer on it, so the user
 * gets no sign of which control acted. Show the pressed state for a moment.
 * Local, synchronous UI state: no other component reads it.
 */
export function usePressFlash() {
    const [isOn, setIsOn] = useState(false)
    const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

    useEffect(
        () => () => {
            if (timer.current) clearTimeout(timer.current)
        },
        []
    )

    const flash = useCallback(() => {
        if (timer.current) clearTimeout(timer.current)
        setIsOn(true)
        timer.current = setTimeout(() => {
            timer.current = null
            setIsOn(false)
        }, FLASH_MS)
    }, [])

    return { isOn, flash }
}
