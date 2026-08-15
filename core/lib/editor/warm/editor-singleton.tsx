import {
    createContext,
    type ReactNode,
    useCallback,
    useContext,
    useEffect,
    useMemo,
    useRef,
    useState,
    useSyncExternalStore,
} from 'react'
import { View } from 'react-native'
import { useRichEditor } from '../rich'
import type { UseRichEditorOptions } from '../rich/options'
import type { EditorResult } from '../types'
import { createDraftStore, type DraftStore } from './draft-store'
import { createWarmEditorStore, type WarmEditorStore } from './warm-editor-store'

export interface EditorSingletonValue {
    store: WarmEditorStore
    drafts: DraftStore
    /** The one editor, or null until a package has declared need and it boots. */
    result: EditorResult | null
    /** Declare that this section may edit. Idempotent; never disposes. */
    declareNeed: () => void
    setOptions: (options: UseRichEditorOptions) => void
}

const EditorSingletonContext = createContext<EditorSingletonValue | null>(null)

export function useEditorSingleton(): EditorSingletonValue | null {
    return useContext(EditorSingletonContext)
}

/**
 * The composer draft store.
 *
 * Null only when no provider is mounted at all. Unlike the previous per-section
 * host this is app-wide and platform-neutral, so a consumer gets the same store
 * on web and native.
 */
export function useDraftStore(): DraftStore | null {
    return useContext(EditorSingletonContext)?.drafts ?? null
}

/**
 * ONE editor for the whole app, booted lazily and never disposed.
 *
 * Mounted above the route tree, so the instance outlives any package's section:
 * leaving Cards and coming back re-uses the same editor rather than re-paying
 * the boot. On native that boot is a browser cold start plus a 0.86 MB bundle
 * parse — 1135 of the 1186 ms an edit used to take — and the per-section host
 * this replaces paid it again on every re-entry.
 *
 * Nothing is constructed until a package calls `useEditorNeeded()`. An app whose
 * user never opens an editing package pays nothing, which is why this can sit at
 * the root where a manifest `provider` (built at module load, wrapping everyone)
 * could not.
 *
 * The SAME file runs on both platforms. That is the point: the previous split
 * left web with a stub host and a no-op lease, so every handover branch —
 * staleness guards, refocus, draft stash — was unreachable from CI, which runs
 * on web.
 */
export function EditorSingletonProvider({ children }: { children: ReactNode }) {
    const storeRef = useRef<WarmEditorStore | null>(null)
    if (storeRef.current === null) storeRef.current = createWarmEditorStore()
    const store = storeRef.current

    const draftsRef = useRef<DraftStore | null>(null)
    if (draftsRef.current === null) draftsRef.current = createDraftStore()
    const drafts = draftsRef.current

    // The latch. Once true it never goes back — "never disposed" is the whole
    // guarantee, and a section unmounting must not tear the editor down.
    const [isBooted, setIsBooted] = useState(false)
    const declareNeed = useCallback(() => setIsBooted(true), [])

    const value = useMemo<EditorSingletonValue>(
        () => ({ store, drafts, result: null, declareNeed, setOptions: () => {} }),
        [store, drafts, declareNeed]
    )

    // Before any declaration there is no editor and no hook to build one — the
    // provider is a pass-through carrying only the latch. Splitting the booted
    // half into its own component is what keeps `useRichEditor` from being
    // called at all until then, since a hook cannot sit behind a branch.
    if (!isBooted) {
        return (
            <EditorSingletonContext.Provider value={value}>
                {children}
            </EditorSingletonContext.Provider>
        )
    }

    return (
        <BootedEditor store={store} drafts={drafts} declareNeed={declareNeed}>
            {children}
        </BootedEditor>
    )
}

function BootedEditor({
    store,
    drafts,
    declareNeed,
    children,
}: {
    store: WarmEditorStore
    drafts: DraftStore
    declareNeed: () => void
    children: ReactNode
}) {
    // The live configuration, held in a ref and read through the store's
    // generation: putting it in state would re-render this provider — and so
    // the entire route tree beneath it — on every handover.
    const optionsRef = useRef<UseRichEditorOptions>({})
    const snapshot = useSyncExternalStore(store.subscribe, store.getSnapshot)

    const result = useRichEditor({
        ...optionsRef.current,
        generation: snapshot.generation,
        // Never on acquire: the surface decides when to take the caret, and
        // focusing a parked editor would open the keyboard over a card nobody
        // is editing.
        autofocus: false,
    })

    // Published to the store rather than read straight off `result`, so a
    // surface learns the editor became usable through the same subscription it
    // already uses for the holder — no second channel to keep in step.
    // A variant that never reports readiness (isReady is optional on the
    // contract) counts as ready once mounted — withholding forever would leave
    // every surface stuck on its read view.
    const isReady = result.isReady ?? true
    useEffect(() => {
        store.setReady(isReady)
    }, [store, isReady])

    // `setOptions` is stable on its own, so the only member of this value that
    // changes identity is `result`. That matters: a consumer derives its
    // acquire/release callbacks from this object, and acquiring bumps the
    // generation, which rebuilds the editor, which produces a new `result`. If
    // that fed back into the callbacks' identity, an effect depending on
    // acquire would re-acquire and spin — which it did, at ~50 generations a
    // second.
    const setOptions = useCallback((next: UseRichEditorOptions) => {
        optionsRef.current = next
    }, [])

    // A parked editor must not keep the last surface's text.
    //
    // It stays mounted (off-viewport) so it never re-pays the boot, which means
    // its content is still in the document — a released comment editor left the
    // comment's own words sitting in the DOM, where anything reading the page by
    // text found them twice. It belongs to nobody while parked, so it holds
    // nothing.
    //
    // Not under collaboration: there the Yjs document is the source of truth and
    // clearing would propagate the deletion to every other client.
    const isParked = snapshot.holder === null
    const editorHandle = result.editor
    const isCollab = !!optionsRef.current.collab
    useEffect(() => {
        if (isParked && !isCollab) editorHandle.setContent('')
    }, [isParked, isCollab, editorHandle])

    const value = useMemo<EditorSingletonValue>(
        () => ({ store, drafts, result, declareNeed, setOptions }),
        [store, drafts, result, declareNeed, setOptions]
    )

    return (
        <EditorSingletonContext.Provider value={value}>
            {children}
            <ParkedEditorViewport isParked={snapshot.holder === null}>
                <result.EditorComponent />
            </ParkedEditorViewport>
        </EditorSingletonContext.Provider>
    )
}

/**
 * Where the editor lives while nobody is editing.
 *
 * Deliberately NOT `display: none` or a zero-size box: an unlaid-out WebView may
 * never paint on iOS, and a page that never paints never finishes booting —
 * which would defeat the entire point. It is therefore kept at a real size and
 * pushed off-viewport instead. Web parks the same way rather than unmounting, so
 * both platforms hold the instance identically.
 *
 * Renders NOTHING once a surface holds the instance, because the holder renders
 * `EditorComponent` itself. Both at once mounts the same editor in two places:
 * on web that is literally one tiptap DOM node, which moves to whichever
 * mounted last and leaves an empty duplicate behind — an invisible box that sat
 * over the comment list and swallowed every click meant for a comment.
 */
function ParkedEditorViewport({ isParked, children }: { isParked: boolean; children: ReactNode }) {
    if (!isParked) return null
    return (
        <View
            style={{ position: 'absolute', left: -10000, top: 0, width: 320, height: 200 }}
            pointerEvents="none"
            accessibilityElementsHidden
            importantForAccessibility="no-hide-descendants"
        >
            {children}
        </View>
    )
}
