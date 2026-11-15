import type { RecordState } from '../hooks/useReportRecord'

/** The back link of a report opened from the hub. */
export const HUB_BACK_LABEL = 'Reports'

/**
 * The back link of a report opened from another one (`from`, D3). With the source's
 * title it names the source. While the title loads, or when its read failed, it
 * reads as the hub's (no blank, no spinner) but still returns to the source. A
 * source the reader cannot view has no record: the link goes to the hub.
 */
export function reportBack(
    state: RecordState,
    title: string | undefined
): { label: string; to: 'source' | 'hub' } {
    if (state === 'missing') return { label: HUB_BACK_LABEL, to: 'hub' }
    if (state !== 'ready' || !title) return { label: HUB_BACK_LABEL, to: 'source' }
    return { label: title, to: 'source' }
}
