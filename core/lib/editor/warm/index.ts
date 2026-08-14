/**
 * Public surface of the warm editor instance.
 *
 * Consumers import from `@tinycld/core/lib/editor/warm`; core's exports map
 * resolves that through this file, via its lib wildcard's index entry. The
 * platform split happens one level down — the bundler picks the `.web` or
 * `.native` variant, and typecheck resolves the matching `.d.ts`.
 */

export { createDraftStore, type DraftStore } from './draft-store'
export type { WarmEditorLease } from './types'
export { useWarmEditor } from './use-warm-editor'
export { useDraftStore, WarmEditorHost } from './WarmEditorHost'
export {
    createWarmEditorStore,
    type SurfaceId,
    type WarmEditorStore,
    type WarmSnapshot,
} from './warm-editor-store'
