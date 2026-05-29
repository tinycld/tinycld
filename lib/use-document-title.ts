import { useEffect, useRef } from 'react'
import { Platform } from 'react-native'

// Stack-based document.title manager. The newest mounted screen wins. We use a
// stack (not bare assignment) because Expo Router can unmount the previous
// screen *after* mounting the next one, so a naive `useEffect` cleanup would
// clobber the new title with the old screen's pop.

const BASE = 'TinyCld'

type Entry = { id: number; title: string }
const stack: Entry[] = []
let nextId = 1

function apply() {
    if (Platform.OS !== 'web') return
    if (typeof document === 'undefined') return
    const top = stack[stack.length - 1]
    document.title = top ? `${BASE}: ${top.title}` : BASE
}

export function useDocumentTitle(suffix: string | null | undefined) {
    const idRef = useRef<number | null>(null)

    // Allocate the stack entry once per mount, free it on unmount. Keeping
    // allocation in its own effect (empty deps) means suffix changes only
    // update the existing entry rather than popping/re-pushing.
    useEffect(() => {
        if (Platform.OS !== 'web') return
        idRef.current = nextId++
        return () => {
            const id = idRef.current
            if (id === null) return
            const i = stack.findIndex(e => e.id === id)
            if (i !== -1) stack.splice(i, 1)
            idRef.current = null
            apply()
        }
    }, [])

    useEffect(() => {
        if (Platform.OS !== 'web') return
        const id = idRef.current
        if (id === null) return

        const trimmed = typeof suffix === 'string' ? suffix.trim() : ''
        const i = stack.findIndex(e => e.id === id)

        if (!trimmed) {
            if (i !== -1) stack.splice(i, 1)
            apply()
            return
        }

        if (i === -1) stack.push({ id, title: trimmed })
        else stack[i].title = trimmed
        apply()
    }, [suffix])
}
