import type { ReactNode } from 'react'
import type { UseRichEditorOptions } from '../rich/options'
import type { DraftStore } from './draft-store'

/**
 * Platform-neutral declaration for the warm host.
 *
 * `WarmEditorHost.native.tsx` mounts one parked WebView and provides the warm
 * context; `WarmEditorHost.web.tsx` is a pass-through. Both take the same props,
 * so a package mounts this component unconditionally.
 */
export declare function WarmEditorHost(props: {
    options: UseRichEditorOptions
    children: ReactNode
}): ReactNode

/** The surrounding host's composer draft store, or null when there is none. */
export declare function useDraftStore(): DraftStore | null
