/**
 * The props core gives a package's record panel (manifest `reportPanels`, D5):
 * the path segments of the row's `open` after `<slug>/<path>/`. For `open`
 * `gizmos/history/c/1` and the panel path `history`, the segments are
 * `['c', '1']`.
 */
export type ReportPanelProps = {
    segments: string[]
}
