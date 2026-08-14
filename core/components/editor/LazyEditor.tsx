import { type ComponentType, type ReactNode, useCallback, useRef, useState } from 'react'
import { Pressable } from 'react-native'
import { useRichEditor } from '../../lib/editor/rich'
import type { UseRichEditorOptions } from '../../lib/editor/rich/options'
import type { EditorCommands, EditorHandle, EditorToolbarState } from '../../lib/editor/types'
import { useWarmEditor } from '../../lib/editor/warm'
import type { SurfaceId } from '../../lib/editor/warm/warm-editor-store'
import { captureException } from '../../lib/errors'
import { isNoOpEdit, shouldCommitOnBlur } from './commit-policy'

export interface LazyEditorSlots {
    EditorComponent: ComponentType
    commands: EditorCommands
    toolbarState: EditorToolbarState
    submit: () => void
    cancel: () => void
    /** Tell the swap a dialog holds the focus, so a blur is not the session ending. */
    setDialogOpen: (open: boolean) => void
}

export interface LazyEditorHeaderState {
    /** Drives the label ⇄ toolbar swap; the row itself is always rendered. */
    isEditing: boolean
    /** Null while idle — there is no editor to drive a toolbar with. */
    slots: LazyEditorSlots | null
}

export interface LazyEditorRenderSlots {
    /** The row above the surface, or null when no `renderHeader` was given. */
    header: ReactNode
    /** The read view, or the editing surface once a session opens. */
    body: ReactNode
}

export interface LazyEditorProps {
    /** Shown while idle. The consumer's component — core never interprets content. */
    readView: ReactNode
    /** Current persisted content, in `contentFormat`. */
    value: string
    contentFormat: 'markdown' | 'html'
    editorOptions: UseRichEditorOptions
    surfaceId: SurfaceId
    canEdit: boolean
    /** True for an edit of existing content; false for a composer. */
    commitOnBlur?: boolean
    onCommit: (content: string) => void
    /** Absent when there is nothing to revert (a collaborative description). */
    onCancel?: () => void
    /** The consumer's chrome around the editing surface. */
    renderEditor: (slots: LazyEditorSlots) => ReactNode
    /**
     * A row drawn ABOVE the editing surface, placed by the caller rather than
     * nested in the returned tree.
     *
     * Only meaningful through {@link useLazyEditor}. A card description draws
     * its formatting toolbar into a row that must stay a direct child of the
     * scroll view for `stickyHeaderIndices` to pin it, so the two halves cannot
     * be one tree. Given no `renderHeader`, the header slot is null.
     */
    renderHeader?: (state: LazyEditorHeaderState) => ReactNode
    testID?: string
    accessibilityLabel?: string
}

/**
 * Renders content, and swaps in a real editor when someone starts editing.
 *
 * Two jobs, both previously hand-rolled per consumer:
 *
 *  - **The swap.** The read view IS the boot placeholder, so an edit never
 *    shows an empty box while the editor initializes. On native the editor is
 *    the package's warm instance when one is available, which turns a ~1135 ms
 *    cold start into a ~34 ms reconfiguration.
 *  - **The commit rules.** See commit-policy.ts — each clause protects a write,
 *    and a blur COMMITS, so getting them wrong loses or invents user text.
 *
 * Deliberately NOT format-aware. `readView` is the consumer's, content crosses
 * as an opaque string, and `contentFormat` only selects which channel to read
 * back through — so mail's HTML surfaces use this exactly as cards' markdown
 * ones do.
 */
export function LazyEditor(props: LazyEditorProps) {
    return <>{useLazyEditor(props).body}</>
}

/**
 * The slot-returning form of {@link LazyEditor}.
 *
 * Same swap, same commit rules; the only difference is that the header comes
 * back separately instead of nested, for a caller that must place the two halves
 * in different parents. `LazyEditor` is this hook with the header dropped.
 */
export function useLazyEditor({
    readView,
    value,
    contentFormat,
    editorOptions,
    surfaceId,
    canEdit,
    commitOnBlur = false,
    onCommit,
    onCancel,
    renderEditor,
    renderHeader,
    testID,
    accessibilityLabel = 'Edit',
}: LazyEditorProps): LazyEditorRenderSlots {
    const [isEditing, setIsEditing] = useState(false)
    const [isDialogOpen, setIsDialogOpen] = useState(false)
    // The revert/no-op baseline, snapshotted when the session opens so a
    // realtime update mid-edit cannot become the comparison target.
    const baselineRef = useRef(value)
    const hasFocusedRef = useRef(false)
    const settledRef = useRef(false)

    const submitRef = useRef<() => void>(() => {})
    const blurRef = useRef<() => void>(() => {})

    // The consumer's own focus handlers are CHAINED rather than replaced: a
    // caller uses them to drive its chrome (a card description swaps its section
    // label for a formatting toolbar on focus), and silently dropping them
    // leaves that chrome permanently in its idle state.
    //
    // These wrappers carry the entire commit policy — hasFocusedRef gates
    // shouldCommitOnBlur, blurRef fires the blur-commit, submitRef answers ⌘↵ —
    // so BOTH the warm instance and the cold fallback must receive them. Giving
    // them only to `own` left the policy inert on the warm path, which is the
    // only path native ever takes: blur never committed and ⌘↵ never submitted.
    const chainedOptions: UseRichEditorOptions = {
        ...editorOptions,
        contentFormat,
        initialContent: value,
        onFocus: () => {
            hasFocusedRef.current = true
            editorOptions.onFocus?.()
        },
        onBlur: () => {
            editorOptions.onBlur?.()
            blurRef.current()
        },
        onSubmitShortcut: () => {
            editorOptions.onSubmitShortcut?.()
            submitRef.current()
        },
    }

    const lease = useWarmEditor(surfaceId, chainedOptions)

    // The cold fallback, used when no warm host is mounted (always on web, and
    // on native before the host boots). Warm is an optimization, never a
    // correctness dependency.
    //
    // Called unconditionally, including while idle: a hook cannot sit behind the
    // swap's branch. On web it is the same Tiptap instance the previous
    // hand-rolled swaps mounted.
    const own = useRichEditor({ ...chainedOptions, autofocus: true })
    const active = lease.result ?? own

    const startEditing = useCallback(() => {
        baselineRef.current = value
        hasFocusedRef.current = false
        settledRef.current = false
        lease.acquire()
        setIsEditing(true)
    }, [lease, value])

    // The warm instance parks with autofocus off — focusing a parked editor
    // would open the keyboard over a card nobody is editing — so the surface
    // that acquires it has to take the caret itself. Without this, tapping a
    // description on native swaps in an editor with no cursor and the user has
    // to tap a second time.
    //
    // Keyed on the generation rather than isEditing so a handover refocuses the
    // incoming surface too. The cold fallback sets autofocus: true and needs
    // none of this, hence the isWarm guard.
    const focusedGenerationRef = useRef<number | null>(null)
    if (isEditing && lease.isWarm && lease.result != null) {
        if (focusedGenerationRef.current !== lease.generation) {
            focusedGenerationRef.current = lease.generation
            // Deferred: the editor is reconfigured during this commit, and
            // focusing before it has applied the new content moves the caret in
            // the outgoing surface's document.
            queueMicrotask(() => lease.result?.editor.focus('end'))
        }
    } else if (!isEditing) {
        focusedGenerationRef.current = null
    }

    const endSession = useCallback(() => {
        settledRef.current = true
        lease.release()
        setIsEditing(false)
    }, [lease])

    const readContent = useCallback(
        async (editor: EditorHandle): Promise<string> =>
            contentFormat === 'markdown'
                ? ((await editor.getMarkdown?.()) ?? '')
                : editor.getHTML(),
        [contentFormat]
    )

    const submit = useCallback(() => {
        if (settledRef.current) return
        // Claimed BEFORE the await, not after. On native every surface shares
        // one editor, so a tap on another surface during the read reconfigures
        // it under us: the content that resolves then belongs to the incoming
        // surface, and writing it through onCommit would put one card's text on
        // another card's record. Settling up front makes the second caller a
        // no-op, and the generation check below discards the stale read.
        settledRef.current = true
        const generationAtRead = lease.generation
        void (async () => {
            let content: string
            try {
                content = (await readContent(active.editor)).trim()
            } catch (err) {
                captureException('editor.lazy.readContent', err)
                endSession()
                return
            }
            // A handover bumps the generation, so a mismatch means these bytes
            // came from an editor that has already been handed to someone else.
            // Dropping the write is the only safe move: we cannot tell whose
            // text this is, and the outgoing surface keeps its persisted value.
            if (lease.generation !== generationAtRead) {
                captureException(
                    'editor.lazy.staleRead',
                    new Error('editor was handed over mid-submit; dropping the read'),
                    { surfaceId, generationAtRead, generationNow: lease.generation }
                )
                endSession()
                return
            }
            if (onCancel && isNoOpEdit(content, baselineRef.current)) {
                endSession()
                onCancel()
                return
            }
            onCommit(content)
            endSession()
        })()
    }, [active.editor, readContent, onCommit, onCancel, endSession, lease.generation, surfaceId])

    const cancel = useCallback(() => {
        settledRef.current = true
        endSession()
        onCancel?.()
    }, [endSession, onCancel])

    submitRef.current = submit
    blurRef.current = () => {
        if (
            shouldCommitOnBlur({
                commitOnBlur,
                hasFocused: hasFocusedRef.current,
                isSettled: settledRef.current,
                isDialogOpen,
            })
        ) {
            submit()
            return
        }
        // A surface with no blur-commit still ends its session on blur — the
        // caller decides whether that text was worth keeping.
        if (!commitOnBlur && hasFocusedRef.current && !isDialogOpen) endSession()
    }

    if (!isEditing) {
        return {
            // Rendered even while idle: the row reserves its height, so swapping
            // a toolbar into it does not shift the prose under the reader's
            // finger at the moment they tap it.
            header: renderHeader?.({ isEditing: false, slots: null }) ?? null,
            body: canEdit ? (
                <Pressable
                    testID={testID}
                    onPress={startEditing}
                    accessibilityRole="button"
                    accessibilityLabel={accessibilityLabel}
                >
                    {readView}
                </Pressable>
            ) : (
                readView
            ),
        }
    }

    // One set of slots feeds both halves, so the header's toolbar and the body's
    // surface are driven by the SAME editor rather than two mounts of it.
    const slots: LazyEditorSlots = {
        EditorComponent: active.EditorComponent,
        commands: active.commands,
        toolbarState: active.toolbarState,
        submit,
        cancel,
        setDialogOpen: setIsDialogOpen,
    }

    return {
        header: renderHeader?.({ isEditing: true, slots }) ?? null,
        body: renderEditor(slots),
    }
}
