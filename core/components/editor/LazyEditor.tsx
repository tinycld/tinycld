import {
    type ComponentType,
    forwardRef,
    type ReactNode,
    useCallback,
    useEffect,
    useImperativeHandle,
    useMemo,
    useRef,
    useState,
} from 'react'
import { Pressable } from 'react-native'
import type { UseRichEditorOptions } from '../../lib/editor/rich/options'
import type { EditorCommands, EditorHandle, EditorToolbarState } from '../../lib/editor/types'
import { useWarmEditor } from '../../lib/editor/warm'
import type { SurfaceId } from '../../lib/editor/warm/warm-editor-store'
import { captureException } from '../../lib/errors'
import { isNoOpEdit } from './commit-policy'

/** The coordinate fields a React Native press event carries. */
export interface PressPoint {
    pageX?: number
    pageY?: number
}

/**
 * The viewport point a press landed on, or undefined when there is none.
 *
 * `pageX/pageY` are viewport-relative on RN-Web — the same space
 * `posAtCoords` reads — so they can be handed to the editor unchanged.
 *
 * Everything here is optional on purpose. A press does not always arrive with
 * coordinates, or even with an event: an accessibility activation, a synthetic
 * `onPress()` from a test, and a keyboard-triggered press all reach this with
 * nothing to read. Each of those means "put the caret at the end", which is the
 * behavior this whole path replaced for real pointer presses only.
 *
 * Exported because a surface whose swap is decided by a PARENT has to capture
 * the point itself: the editor is not mounted when the press happens, so the
 * press target below never sees it. Cards' comment list is the case.
 */
export function pressPoint(event?: {
    nativeEvent?: PressPoint
}): { x: number; y: number } | undefined {
    const { pageX, pageY } = event?.nativeEvent ?? {}
    if (typeof pageX !== 'number' || typeof pageY !== 'number') return undefined
    return { x: pageX, y: pageY }
}

export interface LazyEditorSlots {
    EditorComponent: ComponentType
    commands: EditorCommands
    toolbarState: EditorToolbarState
    submit: () => void
    cancel: () => void
}

export interface LazyEditorHeaderState {
    /** Drives the label ⇄ toolbar swap; the row itself is always rendered. */
    isEditing: boolean
    /** Null while idle — there is no editor to drive a toolbar with. */
    slots: LazyEditorSlots | null
}

/**
 * Drive the surface from OUTSIDE its own chrome.
 *
 * For a caller that decides elsewhere that this surface should be editing: a
 * comment composer whose Reply button lives in the activity list, a keyboard
 * shortcut registered by the screen. `slots` already carries submit/cancel, but
 * only while editing — and the whole point here is to reach a surface that is
 * not.
 *
 * Stable across renders, so it is safe to hold in a ref or a dependency array.
 */
export interface LazyEditorHandle {
    /**
     * Start editing: take the shared instance, and put the caret in it.
     *
     * The imperative form of pressing the read view. Idempotent — calling it on
     * a session that is already open reclaims the editor without resetting the
     * revert baseline, so a repeated press cannot turn half-typed text into the
     * thing Escape reverts to.
     *
     * `at` is the viewport point the user pressed. The read view and the
     * editing surface occupy the same box, so that point is where the caret
     * belongs — without it, clicking into the middle of a paragraph dropped the
     * caret at the very end and the reader had to click a second time. Omitted
     * by callers with no press to speak of (a Reply button, a shortcut), which
     * still get the caret at the end.
     */
    edit: (at?: { x: number; y: number }) => void
    /** Commit through the ordinary path, guards and all. */
    submit: () => void
    /** Discard and end the session, as Escape does. */
    cancel: () => void
    /**
     * Whether this surface is editing AND holds a usable editor.
     *
     * A method rather than a field because the handle outlives the render that
     * produced it; a boolean would be a snapshot that silently went stale.
     */
    isEditing: () => boolean
}

export interface LazyEditorRenderSlots {
    /** The row above the surface, or null when no `renderHeader` was given. */
    header: ReactNode
    /** The read view, or the editing surface once a session opens. */
    body: ReactNode
    /** Drive the surface from outside its chrome. See {@link LazyEditorHandle}. */
    handle: LazyEditorHandle
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
    /**
     * Write when ANOTHER surface takes the shared editor.
     *
     * There is one editor app-wide, so clicking a second surface hands it over
     * unconditionally — the first surface keeps its session but loses the
     * instance. This decides what happens to what it was holding: an edit of
     * existing content commits (the reader clicked away from a finished edit),
     * while a composer keeps its text as a stashed draft via `onRelease`.
     *
     * Being displaced is unambiguous — someone else is editing now — which is
     * why the session hangs off it rather than off focus. Losing focus happens
     * for reasons that have nothing to do with finishing: a toolbar's overflow
     * menu, a link dialog, a click on the panel behind.
     */
    commitOnDisplace?: boolean
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
     * Viewport point to put the caret at when a `startOpen` session opens.
     *
     * The press-target branch below captures this itself, but a surface whose
     * swap is decided by a PARENT never renders that branch: it is mounted
     * already editing, after the press it should honour. Cards' inline comment
     * edit is the case — without this the caret went to the end of the comment
     * however far up someone clicked.
     */
    openAt?: { x: number; y: number }
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
export const LazyEditor = forwardRef<LazyEditorHandle, LazyEditorProps>(
    function LazyEditor(props, ref) {
        const { body, handle } = useLazyEditor(props)
        // The SAME object the hook returns, so the two forms cannot drift.
        useImperativeHandle(ref, () => handle, [handle])
        return <>{body}</>
    }
)

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
    commitOnDisplace = false,
    onCommit,
    onCancel,
    onRelease,
    startOpen = false,
    openAt,
    stayOpenOnCommit = false,
    renderEditor,
    renderHeader,
    testID,
    accessibilityLabel = 'Edit',
}: LazyEditorProps): LazyEditorRenderSlots {
    const [isEditing, setIsEditing] = useState(startOpen)
    // The revert/no-op baseline, snapshotted when the session opens so a
    // realtime update mid-edit cannot become the comparison target.
    const baselineRef = useRef(value)
    const settledRef = useRef(false)

    const submitRef = useRef<() => void>(() => {})

    // The consumer's own focus handlers are CHAINED rather than replaced: a
    // caller uses them to drive its chrome (a card description swaps its section
    // label for a formatting toolbar on focus), and silently dropping them
    // leaves that chrome permanently in its idle state.
    //
    // `submitRef` answers ⌘↵ through here, so BOTH the warm instance and the
    // cold fallback must receive these. Giving them only to `own` left the
    // policy inert on the warm path, which is the only path native ever takes.
    const chainedOptions: UseRichEditorOptions = {
        ...editorOptions,
        contentFormat,
        initialContent: value,
        onFocus: () => {
            // The editor has genuinely landed and taken the caret, so the swap
            // is over and blur handling comes back on. Keyed on the real focus
            // event rather than on the effect that requests focus: the request
            // can be made against an instance tiptap is about to replace, and
            // only the event says one actually holds it.
            isSwappingRef.current = false
            editorOptions.onFocus?.()
        },
        // Chained, but no longer a session event.
        //
        // Losing focus used to END the session — and for an edit of existing
        // content, WRITE it. That made every focusable control a hazard: a
        // toolbar's overflow menu, a dialog, anything portalled. A session now
        // ends only when another surface takes the editor, on Escape, or when
        // the caller closes it. The consumer's own handler still fires, because
        // it drives chrome that legitimately follows focus.
        onBlur: () => editorOptions.onBlur?.(),
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
    // The last editor this surface actually held.
    //
    // `activeRef` goes null the moment another surface takes over, but a
    // DISPLACED surface still has to read its own uncommitted text out of the
    // instance it was using — that read is the only copy. Blur got away with
    // reading `activeRef` because it fired during the DOM move, while the
    // holding was still ours; an effect runs after the store has already moved
    // on. Held separately so it survives that.
    const lastHeldRef = useRef<typeof active>(null)
    const lastHeldGenerationRef = useRef<number | null>(null)
    /** Which holding's text has already been stashed — see endSession. */
    const stashedGenerationRef = useRef<number | null>(null)
    if (active != null) {
        lastHeldRef.current = active
        lastHeldGenerationRef.current = lease.generation
    }

    // Opening a session and RECLAIMING the instance for one already open are
    // different things, and only the first may touch the guards.
    //
    // `baselineRef` is the revert target, snapshotted when the session opens so
    // a realtime update mid-edit cannot become it. Re-snapshotting on a surface
    // that is already editing would capture the user's half-typed text instead,
    // and a later Escape would "revert" to that rather than to what was
    // persisted — so the reclaim path leaves all three alone.
    const openSession = useCallback(() => {
        // A fresh session has not held the editor yet, whatever the last one did.
        hasHeldRef.current = false
        baselineRef.current = value
        settledRef.current = false
        // Set BEFORE the acquire, which is what moves the DOM node and so what
        // raises the blur this suppresses.
        isSwappingRef.current = true
        lease.acquire()
        setIsEditing(true)
    }, [lease, value])

    // Take the instance back without disturbing a session in progress. Focus
    // follows from the generation effect below, which fires on every acquire.
    const reclaim = useCallback(() => {
        isSwappingRef.current = true
        lease.acquire()
    }, [lease])

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
    // A parent-owned session that LOSES the instance to a steal does not get it
    // back here — that is `handle.edit()`, driven by the caller, because the
    // editor falls idle for a moment during every ordinary handover and
    // reclaiming on idle would rip it out of the surface the user just clicked.
    const acquire = lease.acquire
    useEffect(() => {
        if (!startOpen) return
        isSwappingRef.current = true
        acquire()
    }, [startOpen, acquire])

    // The warm instance parks with autofocus off — focusing a parked editor
    // would open the keyboard over a card nobody is editing — so the surface
    // that acquires it has to take the caret itself. Without this, tapping a
    // description on native swaps in an editor with no cursor and the user has
    // to tap a second time.
    //
    // Keyed on the generation rather than isEditing so a handover refocuses the
    // incoming surface too.
    // Where the press that opened this session landed, consumed by the focus
    // effect below. A ref rather than state because it must not cause a render:
    // it is read once, when the caret is taken, and is meaningless after that.
    //
    // Seeded from `openAt` for a parent-owned surface, which is mounted after
    // the press it should honour and so never runs the press target that would
    // otherwise fill this in.
    const pendingFocusPointRef = useRef<{ x: number; y: number } | null>(openAt ?? null)
    const leaseResult = lease.result

    // True from the moment a swap is requested until the editor has landed in
    // this surface and been focused.
    //
    // Taking the shared editor MOVES its DOM node (@tiptap/react's EditorContent
    // `append`s the live ProseMirror element into its new parent and parks it in
    // a detached div on the way out), and removing a focused element from the
    // document blurs it. That blur is MACHINERY, not the user leaving — but the
    // handler below cannot tell the difference, and for a surface with no
    // blur-commit it means `endSession`, which releases the editor and closes
    // the session that was just opened. Clicking a description therefore swapped
    // the editor in and immediately threw it away again, leaving no caret.
    //
    // Losing focus mid-move is fine; the focus effect re-takes it once the node
    // has landed. This flag is what keeps the blur in between from being read as
    // intent.
    const isSwappingRef = useRef(false)
    // In an EFFECT, and then again after the next frame.
    //
    // Two things defeat a single synchronous focus here:
    //
    //  - A microtask (what this used to be) runs BEFORE React commits, so the
    //    ProseMirror node is not in the document yet and the call does nothing.
    //  - Acquiring the shared editor MOVES its DOM node: the provider renders it
    //    in the parked viewport while nobody holds it and the surface renders it
    //    once someone does, so React unmounts one and mounts the other. Removing
    //    a focused element blurs it, and that removal happens right after this
    //    effect — the caret appeared and vanished in the same frame.
    //
    // Focus is therefore taken HERE, after the commit — which is what makes a
    // click place the caret at all, and what gives `posAtCoords` a laid-out view
    // to resolve the click point against.
    //
    // Keyed on the EDITOR HANDLE, not on the generation. One acquire produces
    // several handles: tiptap tears its instance down and rebuilds it, and each
    // rebuild is a NEW ProseMirror node — the one focused a moment ago is
    // discarded, still holding the caret nobody can see. Keying on the
    // generation meant focusing the first of those and skipping every later one
    // (the guard read "already focused this generation"), so the caret reliably
    // landed on a node that was about to be thrown away.
    //
    // Re-focusing per handle is safe because it is not per RENDER: the handle
    // only changes when tiptap actually replaced the instance, and the identity
    // check below stops a repeat for one we have already focused.
    //
    // The blur each rebuild raises is ignored while the swap is in flight — see
    // isSwappingRef, which clears on the real focus event. Losing focus mid-swap
    // does not matter; landing it on the node that survives does.
    const focusedEditorRef = useRef<EditorHandle | null>(null)
    useEffect(() => {
        if (!isEditing || leaseResult == null) {
            // Cleared whenever this surface does not HOLD an editor, not merely
            // when its session closes.
            //
            // There is one editor app-wide, so re-acquiring hands back the very
            // same object — and the identity check below would then read it as
            // "already focused" and skip the caret. A surface whose session
            // outlives its holding hits exactly that: a composer and an inline
            // comment edit are parent-owned (`startOpen`), so releasing the
            // editor leaves `isEditing` true, and every re-acquire after the
            // first silently declined to focus. Descriptions were unaffected
            // only because their session closes with the holding.
            focusedEditorRef.current = null
            if (!isEditing) {
                pendingFocusPointRef.current = null
                // No session, so nothing is in flight. Clearing here is what
                // stops a swap that never completed (an acquire this surface
                // lost to another) from leaving blur-handling disabled for good
                // — which would silently stop committing an inline edit.
                isSwappingRef.current = false
            }
            return
        }
        const editor = leaseResult.editor
        if (focusedEditorRef.current === editor) return
        focusedEditorRef.current = editor
        // Kept until the caret is placed, NOT cleared per attempt: the point
        // describes the press that opened the session, and a rebuild an instant
        // later still wants the caret where the user pressed.
        const at = pendingFocusPointRef.current
        editor.focus(at ?? 'end')
    }, [isEditing, leaseResult])

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
            // Falls back to the last instance this surface held: a displaced
            // surface no longer holds one, and its text would otherwise be
            // unreadable — see lastHeldRef.
            const held = activeRef.current ?? lastHeldRef.current
            // Stash ONCE per holding.
            //
            // A parent-owned surface re-arms `settledRef` after a handover so it
            // can send again, which makes that flag useless as a guard here —
            // and every later pass reads an editor that now belongs to someone
            // else, so it reads EMPTY and `stash` treats empty as a clear. The
            // first pass saved the draft; the second and third deleted it.
            const alreadyStashed = stashedGenerationRef.current === lastHeldGenerationRef.current
            if (stash && onRelease && held && !alreadyStashed) {
                stashedGenerationRef.current = lastHeldGenerationRef.current
                const editor = held.editor
                // The generation at the moment this surface still held the
                // instance. A blur caused BY a steal runs after the editor has
                // already been reconfigured for the incoming surface, so reading
                // it now returns THEIR text — the composer would stash the
                // comment someone just clicked into and redisplay it as its own
                // draft. Discard the read if the instance moved.
                // The generation the text we are about to read belongs to.
                //
                // Snapshotted when this surface HELD the editor, not when it
                // noticed it had lost it: a displacement is observed after the
                // store has already moved on, so comparing against the live
                // `lease.generation` discards every displaced read as stale —
                // which is precisely the text this exists to save.
                const generationAtStash = lastHeldGenerationRef.current ?? lease.generation
                void (async () => {
                    try {
                        const content = await readContent(editor)
                        // Was the instance still ours when the read started? A
                        // handover REBUILDS the editor, so a generation past the
                        // one our text belonged to means these bytes are the
                        // incoming surface's — the composer would otherwise
                        // stash the comment someone just clicked into.
                        if (lease.generation > generationAtStash + 1) return
                        onRelease(content)
                    } catch (err) {
                        // The session still ends — a broken read is not a
                        // reason to trap the user in an editor. But it IS a lost
                        // draft, so the surface is told rather than left to
                        // re-seed from a stash that never arrived: onRelease
                        // with the baseline restores what was last persisted
                        // instead of silently showing an empty composer.
                        captureException('editor.lazy.readOnRelease', err, { surfaceId })
                        if (lease.generation <= generationAtStash + 1) {
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

    // Another surface took the editor.
    //
    // This is what "clicking a second comment finishes the first" actually is.
    // It used to happen only as a SIDE EFFECT of blur: handing the instance over
    // moves its DOM node, the move blurs it, and the blur handler committed or
    // stashed. That made every stray focus loss — a toolbar menu, a dialog —
    // indistinguishable from a genuine handover, which is the bug this whole
    // change exists to fix. Said directly here instead.
    //
    // A NULL holder is not a steal. The editor is unheld between a release and
    // the next acquire, and before this surface's own `startOpen` acquire lands;
    // treating that as a handover would commit an edit nobody left, including
    // during the boot.
    // Whether this surface has actually held the editor during this session.
    //
    // Being displaced means LOSING something, so a surface that never had it
    // cannot be displaced. Without this the effect fires on the render where a
    // surface has just acquired but the store's holder has not propagated yet —
    // it reads someone else's id and commits the session that was opening,
    // settling it before a caret ever landed.
    // Held means it actually had an EDITOR, not merely that the store named it.
    //
    // `holder` is set the instant a surface acquires, but `result` stays null
    // until the singleton has booted — so a surface can be the holder with
    // nothing to type into. Arming on `holder` alone let a displacement land on
    // a session that had never rendered an editor, which ended it before the
    // boot could hand one over. It only showed up under load, where the boot is
    // slow enough to lose the race.
    const hasHeldRef = useRef(false)
    if (lease.holder === surfaceId && lease.result != null) hasHeldRef.current = true

    const holder = lease.holder
    useEffect(() => {
        if (!isEditing || settledRef.current) return
        if (!hasHeldRef.current) return
        if (holder == null || holder === surfaceId) return
        if (commitOnDisplace) {
            submitRef.current()
            return
        }
        endSession({ stash: true })
        // A parent-owned session outlives the handover — the caller that mounted
        // it decides when editing is over — so the guards `endSession` just set
        // have to come back with it. Without this the composer's NEXT send is
        // refused as already-settled and silently does nothing.
        if (startOpen) settledRef.current = false
    }, [isEditing, holder, surfaceId, commitOnDisplace, endSession, startOpen])

    // The surface is going away with an unfinished edit.
    //
    // The displacement effect above cannot see this case. A surface whose
    // session is owned by a PARENT — cards mounts one inline comment editor, for
    // whichever id is being edited — is UNMOUNTED by the same commit that hands
    // the editor on, so it never renders again to notice it was displaced.
    // Clicking a second comment is exactly that: `setEditingCommentId(B)` tears
    // down A, and A's edit would be lost.
    //
    // Blur happened to cover it, because the DOM move fires synchronously before
    // React unmounts anything. That is the accident this replaces.
    //
    // Empty deps and read through a ref, matching `useWarmEditor`'s own release:
    // this must fire on unmount and nothing else. Depending on the values would
    // re-run the cleanup on every handover and commit a live session.
    const commitOnUnmountRef = useRef<() => void>(() => {})
    commitOnUnmountRef.current = () => {
        if (!commitOnDisplace || settledRef.current || !isEditing) return
        if (!hasHeldRef.current) return
        submitRef.current()
    }
    useEffect(() => () => commitOnUnmountRef.current(), [])

    // Everything the handle calls, read at call time rather than captured.
    //
    // A handle is HELD across renders — a keyboard shortcut registered once, a
    // parent effect that fires much later — so a captured closure would call
    // into a stale render's state. The object identity below therefore stays
    // fixed while its behavior stays current, which is what lets a caller put it
    // in a dependency array without re-running on every keystroke.
    const setPendingFocusPoint = useCallback((at: { x: number; y: number } | null) => {
        pendingFocusPointRef.current = at
    }, [])

    const handleFnsRef = useRef({
        openSession,
        reclaim,
        submit,
        cancel,
        setPendingFocusPoint,
        isEditing: false,
        isSessionOpen: false,
    })
    handleFnsRef.current = {
        openSession,
        reclaim,
        submit,
        cancel,
        setPendingFocusPoint,
        // What a CALLER means by "is it editing": holding a usable editor. A
        // displaced surface whose session is still open has nothing to type
        // into, so it reads false.
        isEditing: isEditing && active != null,
        // Whether a session exists at all, displaced or not. Distinct from the
        // above, and the one `edit()` must branch on — a displaced session is
        // still a session, and re-opening it would reset its revert baseline.
        isSessionOpen: isEditing,
    }

    const handle = useMemo<LazyEditorHandle>(
        () => ({
            edit: (at?: { x: number; y: number }) => {
                const fns = handleFnsRef.current
                // Recorded before either branch, because both end in an acquire
                // and it is the acquire's focus that consumes this.
                fns.setPendingFocusPoint(at ?? null)
                // A session already open — INCLUDING one displaced by a steal,
                // which is the case this exists for — only takes the instance
                // back. Re-opening would snapshot the user's half-typed text as
                // the thing Escape reverts to, and clear the settled guard on a
                // surface that may have already committed.
                if (fns.isSessionOpen) fns.reclaim()
                else fns.openSession()
            },
            submit: () => handleFnsRef.current.submit(),
            cancel: () => handleFnsRef.current.cancel(),
            isEditing: () => handleFnsRef.current.isEditing,
        }),
        []
    )

    submitRef.current = submit

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
                    // Through the handle, not openSession: a DISPLACED session
                    // renders this branch too, and it must reclaim the instance
                    // rather than re-open — otherwise tapping back into a
                    // composer would make its half-typed draft the revert target.
                    //
                    // The press point rides along so the caret lands where the
                    // reader pressed. `pageX/pageY` are viewport coordinates on
                    // RN-Web, which is what posAtCoords wants; a native press
                    // has them too and simply falls back to 'end' there.
                    onPress={event => handle.edit(pressPoint(event))}
                    accessibilityRole="button"
                    accessibilityLabel={accessibilityLabel}
                >
                    {readView}
                </Pressable>
            ) : (
                readView
            ),
            handle,
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
    }

    return {
        header: renderHeader?.({ isEditing: true, slots }) ?? null,
        body: renderEditor(slots),
        handle,
    }
}
