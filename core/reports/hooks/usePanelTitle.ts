import { createContext, createElement, type ReactNode, use, useEffect } from 'react'
import { useReportUIStore } from '../stores/report-ui-store'

/**
 * The props core gives a package's record panel (manifest `reportPanels`, D5):
 * the path segments of the row's `open` after `<slug>/<path>/`. For `open`
 * `gizmos/history/c/1` and the panel path `history`, the segments are
 * `['c', '1']`.
 */
export type ReportPanelProps = {
    segments: string[]
}

// The record path the report's record panel shows. On the package's own screen
// the same component runs outside the provider (null), and the hook does nothing
// (R13).
const PanelRecord = createContext<string | null>(null)

/** Wraps a package's panel component inside the report's record panel. */
export function PanelTitleProvider({ open, children }: { open: string; children: ReactNode }) {
    return createElement(PanelRecord.Provider, { value: open }, children)
}

/** Names the record a report's side panel shows; the panel's header shows the name. */
export function usePanelTitle(title: string): void {
    const open = use(PanelRecord)
    const setRecordTitle = useReportUIStore(s => s.setRecordTitle)
    // A child telling its host what to show, not state derived in render. The
    // title is filed under its record path, so it never shows for another record.
    useEffect(() => {
        if (open !== null) setRecordTitle(open, title)
    }, [open, title, setRecordTitle])
}
