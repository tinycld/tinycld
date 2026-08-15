import type { EditorResult } from '../types'
import type { SurfaceId } from './warm-editor-store'

export interface WarmEditorLease {
    /** False only when no singleton provider is mounted at all. */
    isWarm: boolean
    /**
     * Whether the one editor has finished booting.
     *
     * Distinguishes "nobody is editing" from "this surface holds the instance
     * but it is not usable yet" — a surface that acquires during the boot. Both
     * render the read view, so a caller rarely needs this; it is here for one
     * that wants to say something different while waiting.
     */
    ready: boolean
    /**
     * Who holds the one instance, or null when nobody is editing.
     *
     * For a surface that must tell "someone else took it" from "it is free" —
     * a parent-owned session stays open through a steal, and reclaims the
     * editor once it falls idle rather than stranding the user in a read view.
     */
    holder: SurfaceId | null
    acquire(): void
    release(): void
    /**
     * The editor, non-null ONLY while this surface holds a usable instance.
     *
     * Null while idle, while displaced by another surface's acquire, and while
     * a held editor is still booting. To a caller those are the same case: it
     * has no editor to render, so it renders its read view.
     */
    result: EditorResult | null
    /**
     * Bumped on every handover, so a caller can tell that the instance it read
     * from has since been reconfigured for another surface.
     *
     * A read from the shared editor is async, and a handover during that read
     * makes the resolved content belong to the incoming surface — writing it
     * would put one surface's text on another's record. Compare this before and
     * after the await and discard the read if it moved. Meaningful on BOTH
     * platforms now that one instance is shared app-wide.
     */
    generation: number
}
