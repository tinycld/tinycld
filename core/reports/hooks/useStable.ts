import { useState } from 'react'

// The value kept from an earlier render while it is equal in content, so a new
// array or object with the same content does not redraw every row.
export function useStable<T>(value: T): T {
    const [kept, setKept] = useState(value)
    if (kept === value || JSON.stringify(kept) === JSON.stringify(value)) return kept
    setKept(value)
    return value
}
