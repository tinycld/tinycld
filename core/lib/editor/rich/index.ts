/**
 * Public surface of the shared rich-text editor.
 *
 * Consumers import from `@tinycld/core/lib/editor/rich`; core's exports map
 * resolves that through this file, via its lib wildcard's index entry. The
 * platform split happens one level down — the bundler picks
 * `use-rich-editor.web.tsx` or `.native.tsx`, and typecheck resolves
 * `use-rich-editor.d.ts`.
 */

export type { RichEditorCollabOptions } from './extensions'
export { buildRichEditorExtensions } from './extensions'
export { findDamagedTableRows, repairMarkdown } from './markdown-repair'
export type { EditorContentFormat, UseRichEditorOptions } from './options'
export { setContentWhenReady } from './set-content-when-ready'
export type {
    TriggerAnchor,
    TriggerConfig,
    TriggerItem,
    TriggerState,
} from './triggers'
export { CLOSED_TRIGGER_STATE, createTriggerExtension } from './triggers'
export { useRichEditor } from './use-rich-editor'
