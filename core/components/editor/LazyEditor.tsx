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
export function LazyEditor({
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
    testID,
    accessibilityLabel = 'Edit',
}: LazyEditorProps) {
    const [isEditing, setIsEditing] = useState(false)
    const [isDialogOpen, setIsDialogOpen] = useState(false)
    // The revert/no-op baseline, snapshotted when the session opens so a
    // realtime update mid-edit cannot become the comparison target.
    const baselineRef = useRef(value)
    const hasFocusedRef = useRef(false)
    const settledRef = useRef(false)

    const lease = useWarmEditor(surfaceId, {
        ...editorOptions,
        contentFormat,
        initialContent: value,
    })

    const startEditing = useCallback(() => {
        baselineRef.current = value
        hasFocusedRef.current = false
        settledRef.current = false
        lease.acquire()
        setIsEditing(true)
    }, [lease, value])

    const endSession = useCallback(() => {
        settledRef.current = true
        lease.release()
        setIsEditing(false)
    }, [lease])

    if (!isEditing) {
        if (!canEdit) return <>{readView}</>
        return (
            <Pressable
                testID={testID}
                onPress={startEditing}
                accessibilityRole="button"
                accessibilityLabel={accessibilityLabel}
            >
                {readView}
            </Pressable>
        )
    }

    return (
        <LazyEditorSession
            lease={lease}
            value={value}
            contentFormat={contentFormat}
            editorOptions={editorOptions}
            commitOnBlur={commitOnBlur}
            isDialogOpen={isDialogOpen}
            setDialogOpen={setIsDialogOpen}
            baselineRef={baselineRef}
            hasFocusedRef={hasFocusedRef}
            settledRef={settledRef}
            onCommit={onCommit}
            onCancel={onCancel}
            endSession={endSession}
            renderEditor={renderEditor}
        />
    )
}

/**
 * The live editing session, split out so the fallback `useRichEditor` below is
 * only ever called while editing — a hook cannot sit behind the swap's branch.
 */
function LazyEditorSession({
    lease,
    value,
    contentFormat,
    editorOptions,
    commitOnBlur,
    isDialogOpen,
    setDialogOpen,
    baselineRef,
    hasFocusedRef,
    settledRef,
    onCommit,
    onCancel,
    endSession,
    renderEditor,
}: {
    lease: ReturnType<typeof useWarmEditor>
    value: string
    contentFormat: 'markdown' | 'html'
    editorOptions: UseRichEditorOptions
    commitOnBlur: boolean
    isDialogOpen: boolean
    setDialogOpen: (open: boolean) => void
    baselineRef: { current: string }
    hasFocusedRef: { current: boolean }
    settledRef: { current: boolean }
    onCommit: (content: string) => void
    onCancel?: () => void
    endSession: () => void
    renderEditor: (slots: LazyEditorSlots) => ReactNode
}) {
    const readContent = useCallback(
        async (editor: EditorHandle): Promise<string> =>
            contentFormat === 'markdown'
                ? ((await editor.getMarkdown?.()) ?? '')
                : editor.getHTML(),
        [contentFormat]
    )

    const submitRef = useRef<() => void>(() => {})
    const blurRef = useRef<() => void>(() => {})

    // The warm instance when one is available; otherwise this surface mounts
    // its own and pays the cold start. Warm is an optimization, never a
    // correctness dependency.
    const own = useRichEditor({
        ...editorOptions,
        contentFormat,
        initialContent: value,
        autofocus: true,
        onFocus: () => {
            hasFocusedRef.current = true
        },
        onBlur: () => blurRef.current(),
        onSubmitShortcut: () => submitRef.current(),
    })
    const active = lease.result ?? own

    const submit = useCallback(() => {
        if (settledRef.current) return
        void (async () => {
            let content: string
            try {
                content = (await readContent(active.editor)).trim()
            } catch (err) {
                captureException('editor.lazy.readContent', err)
                return
            }
            if (onCancel && isNoOpEdit(content, baselineRef.current)) {
                settledRef.current = true
                endSession()
                onCancel()
                return
            }
            settledRef.current = true
            onCommit(content)
            endSession()
        })()
    }, [active.editor, readContent, onCommit, onCancel, endSession, baselineRef, settledRef])

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

    const cancel = useCallback(() => {
        settledRef.current = true
        endSession()
        onCancel?.()
    }, [endSession, onCancel, settledRef])

    return (
        <>
            {renderEditor({
                EditorComponent: active.EditorComponent,
                commands: active.commands,
                toolbarState: active.toolbarState,
                submit,
                cancel,
                setDialogOpen,
            })}
        </>
    )
}
