import { type ComponentType, type ReactNode, useCallback, useEffect, useRef, useState } from 'react'
import { Pressable } from 'react-native'
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
    /**
     * Shown whenever this surface does not hold a usable editor — idle,
     * displaced by another surface, or still booting. The consumer's component;
     * core never interprets content.
     *
     * Effectively REQUIRED. Null renders an invisible box the user cannot get
     * back into, which is precisely what made a displaced composer look
     * broken. A composer with no prose to show still has something to render:
     * its stashed draft as static text, or its placeholder when empty.
     */
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
    /**
     * The session is ending WITHOUT a commit — here is what was in the editor.
     *
     * For a surface that has no commit semantics and therefore nowhere else to
     * put uncommitted text. A comment composer is the case: it does not write on
     * blur, and before the shared editor its half-typed draft survived only
     * because its own editor stayed mounted. Stash it here and seed `value` from
     * the stash to get that back.
     *
     * Not called on a commit — `onCommit` already carries the content.
     */
    onRelease?: (content: string) => void
    /**
     * True while a dialog the editor opened holds the focus, so a blur is not
     * the session ending — the editor must survive until the picked image or
     * link lands in it.
     *
     * Two ways to say this, and a caller should pick one. `slots.setDialogOpen`
     * suits a caller that learns about the dialog from inside the rendered
     * chrome. This prop suits one that already knows: a card description owns
     * both dialogs' open state itself, and pushing it back up through an effect
     * only to receive it again is the useState+useEffect pairing the style
     * guide names as the signal to switch primitives. When supplied, this wins.
     */
    isDialogOpen?: boolean
    /**
     * Open the session on mount instead of waiting for a press.
     *
     * For a surface whose swap is decided ELSEWHERE: a comment composer that a
     * collapsed row already expanded, or an inline edit whose parent tracks
     * which single comment is open. Those callers mount this component only when
     * editing has already begun, so an idle read view would be a second, stale
     * source of truth about whether a session exists.
     *
     * Such a surface also never closes itself on a blur: the parent decides
     * when editing is over, since it is the one that will unmount this. A blur
     * still RELEASES the shared instance and stashes the draft; it just does not
     * end the session.
     *
     * This does NOT imply a distinct rendering. A `startOpen` surface that does
     * not hold the editor renders `readView` like any other — the difference is
     * only that it opened its session without waiting for a tap.
     */
    startOpen?: boolean
    /**
     * Change this to say "the caret belongs in this surface now".
     *
     * Only meaningful with `startOpen`. Such a session stays open when another
     * surface steals the editor, so the caller needs a way to claim it back on
     * an explicit user action — a comment composer keys this on its reply
     * target, so pressing Reply on a comment brings the editor back to the
     * composer that the reply will be written in.
     */
    acquireToken?: string | number
    /**
     * Keep the session open after a commit, clearing the editor instead.
     *
     * A composer sends and stays — the next comment goes in the same box, and
     * ending the session would unmount the editor only to rebuild it. An edit of
     * existing content is the opposite: it ends at its first commit.
     */
    stayOpenOnCommit?: boolean
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
 * Renders content, and swaps in the app's one editor when someone starts
 * editing.
 *
 * **It renders exactly two things: `readView`, or the editor.** Holding a
 * usable instance renders the editor; everything else — idle, displaced by
 * another surface's acquire, or waiting on a boot that has not finished —
 * renders `readView`. There is no third rendering and no loading state here,
 * because what the read view SHOWS is the caller's decision: prose, a stashed
 * draft, a placeholder. Core never interprets it, exactly as it never
 * interprets content.
 *
 * That makes `readView` effectively required. A caller passing null gets an
 * invisible box the user cannot get back into.
 *
 * Two jobs, both previously hand-rolled per consumer:
 *
 *  - **The swap.** The read view IS the boot placeholder, so an edit never
 *    shows an empty box while the editor initializes. The editor is the app's
 *    single instance, which turns a ~1135 ms cold start into a ~34 ms
 *    reconfiguration.
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
    onRelease,
    isDialogOpen: dialogOpenProp,
    startOpen = false,
    acquireToken,
    stayOpenOnCommit = false,
    renderEditor,
    renderHeader,
    testID,
    accessibilityLabel = 'Edit',
}: LazyEditorProps): LazyEditorRenderSlots {
    const [isEditing, setIsEditing] = useState(startOpen)
    const [dialogOpenState, setIsDialogOpen] = useState(false)
    // The prop wins when given, so a caller that already owns the dialog state
    // does not have to echo it back through setDialogOpen.
    const isDialogOpen = dialogOpenProp ?? dialogOpenState
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

    // The ONLY editor. There is no fallback: a second instance is what let one
    // surface keep editing while another held the shared one, and on web it hid
    // every handover branch from CI. Null whenever this surface does not hold a
    // usable editor — idle, displaced by a steal, or still booting — and all
    // three render `readView`.
    const active = lease.result
    // The current holding, for callbacks that can outlive the render they were
    // created in (a dialog's Save, a blur arriving after a steal). Reading the
    // captured `active` there would test a holding this surface no longer has.
    const activeRef = useRef(active)
    activeRef.current = active

    const startEditing = useCallback(() => {
        baselineRef.current = value
        hasFocusedRef.current = false
        settledRef.current = false
        lease.acquire()
        setIsEditing(true)
    }, [lease, value])

    // A session that opened on mount still has to TAKE the instance. Without
    // this it renders an editing surface while holding no lease, so `result` is
    // null and it silently falls back to a cold editor — the exact cost this
    // whole mechanism exists to remove.
    //
    // In an effect, not during render: acquire mutates the shared store and
    // notifies every other surface subscribed to it.
    // Re-run when the lease's acquire changes identity, which it does when the
    // singleton finishes booting. A surface that mounts DURING the boot — the
    // composer on a freshly opened card — would otherwise acquire once against
    // a provider that had no editor yet and never try again, leaving it stuck
    // on its read view for the life of the card.
    // `acquireToken` re-runs it on demand. A parent-owned session survives a
    // steal (#193), so a composer can be open while another surface holds the
    // editor; when the parent does something that means "the caret belongs here
    // now" — pressing Reply, which targets THIS composer — it changes the token
    // and the instance comes back. Without it the composer stays open with no
    // editor in it and no way in but a tap.
    //
    // Deliberately caller-driven rather than a blanket "reclaim whenever idle":
    // the editor falls idle for a moment during every ordinary handover, and
    // grabbing it there would rip it out of the surface the user just clicked.
    const acquire = lease.acquire
    // Null when this surface should not claim the instance at all, and a NEW
    // value each time the caller says the caret belongs here again. Reading it
    // in the effect makes it a genuine dependency rather than one the linter can
    // prove does nothing — the token IS the trigger.
    const claim = startOpen ? `${surfaceId}:${acquireToken ?? ''}` : null
    useEffect(() => {
        if (claim) acquire()
    }, [claim, acquire])

    // The warm instance parks with autofocus off — focusing a parked editor
    // would open the keyboard over a card nobody is editing — so the surface
    // that acquires it has to take the caret itself. Without this, tapping a
    // description on native swaps in an editor with no cursor and the user has
    // to tap a second time.
    //
    // Keyed on the generation rather than isEditing so a handover refocuses the
    // incoming surface too.
    const focusedGenerationRef = useRef<number | null>(null)
    if (isEditing && lease.result != null) {
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

    const readContent = useCallback(
        async (editor: EditorHandle): Promise<string> =>
            contentFormat === 'markdown'
                ? ((await editor.getMarkdown?.()) ?? '')
                : editor.getHTML(),
        [contentFormat]
    )

    /**
     * End the session and give the instance back.
     *
     * `stash` is for the one path that leaves text behind: a surface with no
     * commit semantics blurring away. A commit or a cancel has already decided
     * what happens to the content, and stashing there would resurrect a comment
     * that was just sent.
     */
    const endSession = useCallback(
        ({ stash }: { stash?: boolean } = {}) => {
            settledRef.current = true
            // Read BEFORE the release, while this surface still holds the
            // editor. With no second editor to fall back on, this async read is
            // the ONLY copy of an uncommitted draft — the composer's text used
            // to survive because its own editor stayed mounted, and it no
            // longer does.
            const held = activeRef.current
            if (stash && onRelease && held) {
                const editor = held.editor
                // The generation at the moment this surface still held the
                // instance. A blur caused BY a steal runs after the editor has
                // already been reconfigured for the incoming surface, so reading
                // it now returns THEIR text — the composer would stash the
                // comment someone just clicked into and redisplay it as its own
                // draft. Discard the read if the instance moved.
                const generationAtStash = lease.generation
                void (async () => {
                    try {
                        const content = await readContent(editor)
                        if (lease.generation !== generationAtStash) return
                        onRelease(content)
                    } catch (err) {
                        // The session still ends — a broken read is not a
                        // reason to trap the user in an editor. But it IS a lost
                        // draft, so the surface is told rather than left to
                        // re-seed from a stash that never arrived: onRelease
                        // with the baseline restores what was last persisted
                        // instead of silently showing an empty composer.
                        captureException('editor.lazy.readOnRelease', err, { surfaceId })
                        if (lease.generation === generationAtStash) {
                            onRelease(baselineRef.current)
                        }
                    }
                })()
            }
            lease.release()
            // A parent-owned session (startOpen) does NOT close itself: the
            // caller that mounted it decides when editing is over, and a
            // composer has no read view to fall back to — closing would leave
            // an empty box with no way back into it. The instance is still
            // released above, so a handover works; only the swap is withheld.
            if (!startOpen) setIsEditing(false)
        },
        [lease, onRelease, readContent, surfaceId, startOpen]
    )

    const submit = useCallback(() => {
        if (settledRef.current) return
        // Read through the ref, not the captured `active`: a submit handler can
        // outlive the render that produced it — a dialog's Save button holds the
        // one it was given when it opened — and the captured value would still
        // point at an editor this surface has since lost. What matters is
        // whether it holds one NOW.
        const held = activeRef.current
        // No editor means nothing to read, and reading nothing would resolve to
        // '' — writing that through onCommit would blank the user's content.
        // Ending without committing keeps the persisted value, which is the
        // only safe answer when the text is not reachable.
        if (!held) {
            captureException(
                'editor.lazy.submitWithoutEditor',
                new Error('submit with no editor held; dropping the write'),
                { surfaceId }
            )
            endSession()
            return
        }
        // Claimed BEFORE the await, not after. On native every surface shares
        // one editor, so a tap on another surface during the read reconfigures
        // it under us: the content that resolves then belongs to the incoming
        // surface, and writing it through onCommit would put one card's text on
        // another card's record. Settling up front makes the second caller a
        // no-op, and the generation check below discards the stale read.
        settledRef.current = true
        const generationAtRead = lease.generation
        // Captured now, so the async block below reads from the instance this
        // surface held when submit was pressed rather than from whatever the
        // lease points at after a handover.
        const editor = held.editor
        void (async () => {
            let content: string
            try {
                content = (await readContent(editor)).trim()
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
            if (stayOpenOnCommit) {
                // A composer sends and stays: the next comment goes in the same
                // box. Clearing rather than ending is also what keeps the editor
                // mounted, so the second comment does not re-pay the boot.
                if (contentFormat === 'markdown') editor.setMarkdown?.('')
                else editor.setContent('')
                // The session continues, so its guards reset with it — otherwise
                // the next comment would be refused as already-settled.
                settledRef.current = false
                baselineRef.current = ''
                return
            }
            endSession()
        })()
    }, [
        readContent,
        onCommit,
        onCancel,
        endSession,
        lease.generation,
        surfaceId,
        stayOpenOnCommit,
        contentFormat,
    ])

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
        // A surface with no blur-commit still ends its session on blur — and
        // this is the one exit that leaves uncommitted text, so it is the one
        // that stashes.
        if (!commitOnBlur && hasFocusedRef.current && !isDialogOpen) {
            endSession({ stash: true })
            // A parent-owned session survived that call, so the guards it just
            // set have to come back with it: `settled` would otherwise refuse
            // the next send, and `hasFocused` must re-arm so a LATER blur can
            // stash again. Re-focusing the composer resets the latter anyway;
            // clearing it here keeps a blurred-but-open surface from stashing
            // twice off one focus.
            if (startOpen) {
                settledRef.current = false
                hasFocusedRef.current = false
            }
        }
    }

    // The whole rule, in one condition. Wanting to edit is not enough — this
    // surface must actually HOLD a usable editor, because there is only one and
    // someone else may have it. Idle, displaced by a steal, and still booting
    // are the same case here: no editor, so the read view.
    //
    // Pressing it re-acquires, which is what makes a displaced surface
    // recoverable rather than dead. `startOpen` no longer implies a distinct
    // rendering; it only decides whether the session opened without a tap.
    if (!isEditing || active == null) {
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
