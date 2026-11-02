import { Platform } from 'react-native'

export type ParentMessage =
    | { type: 'tinycld:form-resize'; height: number }
    | { type: 'tinycld:form-submitted'; form: string }

/** Posts to the embedding page. Holds no field values: any origin may read it. */
export function postToParent(message: ParentMessage) {
    if (Platform.OS !== 'web' || typeof window === 'undefined' || window.parent === window) return
    window.parent.postMessage(message, '*')
}

/**
 * Reports the form's content height to the embedding page. It is the content
 * wrapper's height, not the document's: the app shell pins html and body to
 * the viewport, so the document never measures taller than the iframe.
 */
export function reportHeight(height: number) {
    postToParent({ type: 'tinycld:form-resize', height: Math.ceil(height) })
}
