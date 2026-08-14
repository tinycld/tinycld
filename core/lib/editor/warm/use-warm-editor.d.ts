import type { UseRichEditorOptions } from '../rich/options'
import type { WarmEditorLease } from './types'
import type { SurfaceId } from './warm-editor-store'

/**
 * Platform-neutral declaration for the warm editor lease.
 *
 * Mirrors `rich/use-rich-editor.d.ts`: the implementations live in
 * `use-warm-editor.web.ts` (always cold — web builds Tiptap in-process) and
 * `use-warm-editor.native.ts` (leases the parked WebView). Consumers import the
 * bare specifier, the bundler picks the variant, and this file is what typecheck
 * resolves — so it IS the cross-platform contract.
 */
export declare function useWarmEditor(
    surfaceId: SurfaceId,
    options: UseRichEditorOptions
): WarmEditorLease

export type { WarmEditorLease }
