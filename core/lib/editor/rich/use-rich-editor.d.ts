import type { EditorResult } from '../types'
import type { UseRichEditorOptions } from './options'

/**
 * Platform-neutral declaration for the shared editor.
 *
 * The implementations live in `use-rich-editor.web.tsx` (Tiptap in the DOM) and
 * `use-rich-editor.native.tsx` (Tiptap inside a WebView via TenTap). Consumers
 * import the bare specifier and the bundler picks the variant; this file is what
 * typecheck resolves, so it IS the cross-platform contract — a capability that
 * only exists on one platform must be optional in `EditorResult`.
 */
export declare function useRichEditor(options?: UseRichEditorOptions): EditorResult

export type { UseRichEditorOptions }
