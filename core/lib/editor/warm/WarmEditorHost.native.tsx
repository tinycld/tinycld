import {
    createContext,
    type ReactNode,
    useContext,
    useMemo,
    useRef,
    useSyncExternalStore,
} from 'react'
import { View } from 'react-native'
import { useRichEditor } from '../rich'
import type { UseRichEditorOptions } from '../rich/options'
import type { EditorResult } from '../types'
import { createDraftStore, type DraftStore } from './draft-store'
import { createWarmEditorStore, type WarmEditorStore } from './warm-editor-store'

interface WarmContextValue {
    store: WarmEditorStore
    drafts: DraftStore
    result: EditorResult
    setOptions: (options: UseRichEditorOptions) => void
}

const WarmContext = createContext<WarmContextValue | null>(null)

export function useWarmContext(): WarmContextValue | null {
    return useContext(WarmContext)
}

/**
 * The composer draft store for the surrounding warm host.
 *
 * Returns null when no host is mounted, which is also the web case — a consumer
 * with no store keeps whatever draft behavior it had before.
 */
export function useDraftStore(): DraftStore | null {
    return useContext(WarmContext)?.drafts ?? null
}

/**
 * Keeps ONE WebView editor booted and parked, ready to be handed to whichever
 * surface the user starts editing.
 *
 * The cost this removes is measured: creating a WebView editor is a browser cold
 * start plus a 0.86 MB bundle parse — 1135 ms of the 1186 ms an edit used to
 * take. That work is configuration-independent and finishes before the init
 * payload is even sent, so a page booted in advance can be reconfigured for a
 * new surface in the remaining ~34 ms.
 *
 * Mount this where the package's editing surfaces live — for cards, the route
 * layout, so it warms on entering the section and stays warm across boards and
 * cards. NOT a manifest `provider`: PackageProviderWrapper builds its chain at
 * module load and wraps the whole app, which would boot a WebView at launch for
 * anyone who has the package installed but never opens it.
 */
export function WarmEditorHost({
    options,
    children,
}: {
    options: UseRichEditorOptions
    children: ReactNode
}) {
    const storeRef = useRef<WarmEditorStore | null>(null)
    if (storeRef.current === null) storeRef.current = createWarmEditorStore()
    const store = storeRef.current

    const draftsRef = useRef<DraftStore | null>(null)
    if (draftsRef.current === null) draftsRef.current = createDraftStore()
    const drafts = draftsRef.current

    // The live configuration, held in a ref and read through the store's
    // generation: putting it in state would re-render this provider (and so the
    // whole section) on every handover.
    const optionsRef = useRef<UseRichEditorOptions>(options)
    const snapshot = useSyncExternalStore(store.subscribe, store.getSnapshot)

    const result = useRichEditor({
        ...optionsRef.current,
        generation: snapshot.generation,
        // Never on acquire: the surface decides when to take the caret, and
        // focusing a parked editor would open the keyboard over a card nobody
        // is editing.
        autofocus: false,
    })

    const value = useMemo<WarmContextValue>(
        () => ({
            store,
            drafts,
            result,
            setOptions: next => {
                optionsRef.current = next
            },
        }),
        [store, drafts, result]
    )

    return (
        <WarmContext.Provider value={value}>
            {children}
            <WarmEditorViewport isParked={snapshot.holder === null}>
                <result.EditorComponent />
            </WarmEditorViewport>
        </WarmContext.Provider>
    )
}

/**
 * Where the WebView lives while nobody is editing.
 *
 * Deliberately NOT `display: none` or a zero-size box: an unlaid-out WebView may
 * never paint on iOS, and a page that never paints never finishes booting —
 * which would defeat the entire point of warming it. It is therefore kept at a
 * real size and pushed off-viewport instead.
 */
function WarmEditorViewport({ isParked, children }: { isParked: boolean; children: ReactNode }) {
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
