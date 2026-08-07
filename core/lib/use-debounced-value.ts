import { useEffect, useState } from 'react'

// Debounce a value: only surface the latest after `delayMs` of quiet. Genuine
// timer side-effect (not a server-data sync), so it stays in an effect.
export function useDebouncedValue<T>(value: T, delayMs: number): T {
    const [debounced, setDebounced] = useState(value)
    useEffect(() => {
        const timer = setTimeout(() => setDebounced(value), delayMs)
        return () => clearTimeout(timer)
    }, [value, delayMs])
    return debounced
}
