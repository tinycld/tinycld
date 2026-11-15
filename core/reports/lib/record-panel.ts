import type { ComponentType } from 'react'
import type { ReportPanelProps } from '../hooks/usePanelTitle'

/** One package's record panel, as the generator lists it (lib/generated/report-panels.ts). */
export type ReportPanelEntry = {
    slug: string
    path: string
    load: () => Promise<{ default: ComponentType<ReportPanelProps> }>
}
