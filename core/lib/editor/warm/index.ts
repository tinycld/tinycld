/**
 * Public surface of the one app-wide editor instance.
 *
 * Consumers import from `@tinycld/core/lib/editor/warm`; core's exports map
 * resolves that through this file, via its lib wildcard's index entry.
 *
 * No platform split lives here any more. The singleton and its lease are ONE
 * file each, running identically on web and native — the previous `.web`/
 * `.native` variants left web on a stub, so every handover branch was
 * unreachable from CI.
 */

export { createDraftStore, type DraftStore } from './draft-store'
export {
    EditorSingletonProvider,
    type EditorSingletonValue,
    useDraftStore,
    useEditorSingleton,
} from './editor-singleton'
export type { WarmEditorLease } from './types'
export { useEditorNeeded } from './use-editor-needed'
export { useWarmEditor } from './use-warm-editor'
export {
    createWarmEditorStore,
    type SurfaceId,
    type WarmEditorStore,
    type WarmSnapshot,
} from './warm-editor-store'
