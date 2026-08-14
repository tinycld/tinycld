import type { ReactNode } from 'react'
import type { UseRichEditorOptions } from '../rich/options'
import type { DraftStore } from './draft-store'

/**
 * No warm host on web, so no draft store either — the composer keeps its own
 * mounted editor and its draft survives exactly as it did before.
 */
export function useDraftStore(): DraftStore | null {
    return null
}

/**
 * A pass-through on web. Kept so a package mounts the same component on both
 * platforms and the e2e suite walks the same tree.
 */
export function WarmEditorHost({
    children,
}: {
    options: UseRichEditorOptions
    children: ReactNode
}) {
    return <>{children}</>
}
