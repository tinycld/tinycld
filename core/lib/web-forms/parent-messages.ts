import { useEffect } from 'react'
import { Platform } from 'react-native'

export type ParentMessage =
    | { type: 'tinycld:form-resize'; height: number }
    | { type: 'tinycld:form-submitted'; form: string }

/** Posts to the embedding page. Holds no field values: any origin may read it. */
export function postToParent(message: ParentMessage) {
    if (Platform.OS !== 'web' || typeof window === 'undefined' || window.parent === window) return
    window.parent.postMessage(message, '*')
}

/** Reports the document height to the embedding page whenever it changes. */
export function useReportHeight() {
    // Subscribes to a DOM API; it syncs no React state.
    useEffect(() => {
        if (Platform.OS !== 'web' || typeof ResizeObserver === 'undefined') return
        const el = document.documentElement
        const report = () => postToParent({ type: 'tinycld:form-resize', height: el.scrollHeight })
        const observer = new ResizeObserver(report)
        observer.observe(document.body)
        report()
        return () => observer.disconnect()
    }, [])
}
