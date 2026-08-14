import type { EditorResult } from '../types'

export interface WarmEditorLease {
    /** False on web, and whenever no warm host is mounted. */
    isWarm: boolean
    acquire(): void
    release(): void
    /** Non-null only while this surface holds the instance. */
    result: EditorResult | null
}
