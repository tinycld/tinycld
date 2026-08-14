import type { UseRichEditorOptions } from '../rich/options'
import type { WarmEditorLease } from './types'
import type { SurfaceId } from './warm-editor-store'

/**
 * Web builds Tiptap in-process, so there is no browser cold start to hide and
 * nothing to keep warm. The lease exists only so LazyEditor's call sites are
 * identical on both platforms; reporting cold sends the consumer down its own
 * useRichEditor path.
 */
export function useWarmEditor(
    _surfaceId: SurfaceId,
    _options: UseRichEditorOptions
): WarmEditorLease {
    return {
        isWarm: false,
        acquire: () => {},
        release: () => {},
        result: null,
        // Constant: each surface owns its editor here, so there is no handover
        // that could invalidate an in-flight read.
        generation: 0,
    }
}
