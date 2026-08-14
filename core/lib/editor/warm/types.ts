import type { EditorResult } from '../types'

export interface WarmEditorLease {
    /** False on web, and whenever no warm host is mounted. */
    isWarm: boolean
    acquire(): void
    release(): void
    /** Non-null only while this surface holds the instance. */
    result: EditorResult | null
    /**
     * Bumped on every handover, so a caller can tell that the instance it read
     * from has since been reconfigured for another surface.
     *
     * A read from the shared editor is async, and a handover during that read
     * makes the resolved content belong to the incoming surface — writing it
     * would put one surface's text on another's record. Compare this before and
     * after the await and discard the read if it moved. Always 0 on web, where
     * each surface owns its editor and no handover exists.
     */
    generation: number
}
