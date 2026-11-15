import { reportPanels } from '@tinycld/app-generated/report-panels'
import { lazyChunk, type PreloadableComponent } from '@tinycld/core/lib/lazy-chunk'
import { createElement } from 'react'
import type { ReportPanelProps } from '../hooks/usePanelTitle'
import type { ReportPanelEntry } from './record-panel'

export interface RegisteredPanel {
    slug: string
    path: string
    Component: PreloadableComponent<ReportPanelProps>
    /** Drops a failed chunk, so the next record opened in this panel loads it again. */
    reload: () => void
}

/**
 * A panel's lazy component that can load again (R14). lazyChunk keeps its first
 * promise for the session, so a chunk that failed once (offline) would fail every
 * later record of the panel. The panel's error boundary calls reload after it
 * catches, when the failed panel has left the screen, so no render starts a load
 * that the same render then drops.
 */
export function reloadableChunk(
    load: ReportPanelEntry['load']
): Pick<RegisteredPanel, 'Component' | 'reload'> {
    let chunk = lazyChunk(load)
    const Panel = (props: ReportPanelProps) => createElement(chunk, props)
    return {
        Component: Object.assign(Panel, { preload: () => chunk.preload() }),
        reload: () => {
            chunk = lazyChunk(load)
        },
    }
}

// The one reader of the generated list (R8): the pure rules take the panels as an
// argument, so tests need no generated file. Each package's chunk loads when a
// record first opens in its panel.
export const registeredPanels: readonly RegisteredPanel[] = reportPanels.map(entry => ({
    slug: entry.slug,
    path: entry.path,
    ...reloadableChunk(entry.load),
}))
