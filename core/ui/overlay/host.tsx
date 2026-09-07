import {
    createContext,
    type ReactNode,
    useCallback,
    useContext,
    useId,
    useLayoutEffect,
    useMemo,
    useState,
} from 'react'
import { createPortal } from 'react-dom'
import { Platform, StyleSheet, View } from 'react-native'

/**
 * Where a floating layer renders.
 *
 * `root` is the app root: dialogs, and on web every popover and menu. `sheet`
 * is the mobile chrome's content region, which ends at the top of the tab
 * bar — a bottom sheet rests on the bottom edge of ITS host, so rendering it
 * there is what keeps it above the bar. A sheet falls back to `root` when no
 * sheet host is mounted (desktop, a public share page).
 *
 * Popovers and menus on native use an RN `Modal` instead of a host (see
 * PopoverLayer): a full-screen, status-bar-translucent Modal shares
 * `measureInWindow`'s coordinate space and stacks above every host.
 */
export type OverlayHostName = 'root' | 'sheet'

interface HostItem {
    id: string
    node: ReactNode
}

interface HostActions {
    registerHost: (host: OverlayHostName, node: HTMLElement | null) => void
    unregisterHost: (host: OverlayHostName) => void
    mount: (host: OverlayHostName, id: string, node: ReactNode) => void
    unmount: (host: OverlayHostName, id: string) => void
}

interface HostState {
    /** Which hosts are mounted right now. */
    hosts: Partial<Record<OverlayHostName, true>>
    /** Web: the DOM element a host renders into. */
    domNodes: Partial<Record<OverlayHostName, HTMLElement>>
}

/** Native: the elements each host currently renders. */
type HostItems = Partial<Record<OverlayHostName, HostItem[]>>

// Three contexts, not one. A portal publishing its content on native writes
// `items`; if that write reached the portal itself through a shared context,
// the portal would re-render, republish, and loop. Actions never change,
// host state changes only when a host mounts, and only OverlayHost reads
// items.
const ActionsContext = createContext<HostActions | null>(null)
const StateContext = createContext<HostState | null>(null)
const ItemsContext = createContext<HostItems>({})

/**
 * Mounts the registry and the root host. Wrap the app once, above every
 * screen; MobileLayout mounts the `sheet` host inside its content region.
 */
export function OverlayProvider({ children }: { children: ReactNode }) {
    const [hosts, setHosts] = useState<HostState['hosts']>({})
    const [domNodes, setDomNodes] = useState<HostState['domNodes']>({})
    const [items, setItems] = useState<HostItems>({})

    const actions = useMemo<HostActions>(
        () => ({
            registerHost: (host, node) => {
                setHosts(prev => (prev[host] ? prev : { ...prev, [host]: true }))
                if (node) {
                    setDomNodes(prev => (prev[host] === node ? prev : { ...prev, [host]: node }))
                }
            },
            unregisterHost: host => {
                setHosts(prev => {
                    if (!prev[host]) return prev
                    const { [host]: _gone, ...rest } = prev
                    return rest
                })
                setDomNodes(prev => {
                    if (!prev[host]) return prev
                    const { [host]: _gone, ...rest } = prev
                    return rest
                })
            },
            mount: (host, id, node) => {
                setItems(prev => {
                    const list = prev[host] ?? []
                    const existing = list.find(item => item.id === id)
                    // The same element again is a no-op; that is what stops a
                    // republish from cascading.
                    if (existing?.node === node) return prev
                    const next = existing
                        ? list.map(item => (item.id === id ? { id, node } : item))
                        : [...list, { id, node }]
                    return { ...prev, [host]: next }
                })
            },
            unmount: (host, id) => {
                setItems(prev => {
                    const list = prev[host]
                    if (!list?.some(item => item.id === id)) return prev
                    return { ...prev, [host]: list.filter(item => item.id !== id) }
                })
            },
        }),
        []
    )
    const state = useMemo<HostState>(() => ({ hosts, domNodes }), [hosts, domNodes])

    return (
        <ActionsContext.Provider value={actions}>
            <StateContext.Provider value={state}>
                <ItemsContext.Provider value={items}>
                    {children}
                    <OverlayHost name="root" />
                </ItemsContext.Provider>
            </StateContext.Provider>
        </ActionsContext.Provider>
    )
}

function useActions(): HostActions {
    const actions = useContext(ActionsContext)
    if (!actions) throw new Error('Overlay surfaces need <OverlayProvider> above them')
    return actions
}

function useHostState(): HostState {
    const state = useContext(StateContext)
    if (!state) throw new Error('Overlay surfaces need <OverlayProvider> above them')
    return state
}

/**
 * A place layers can render into. Fills its parent and lets pointers through
 * where no layer is drawn, so a host mounted inside the mobile content region
 * costs that region nothing while no sheet is open.
 */
export function OverlayHost({ name }: { name: OverlayHostName }) {
    const { registerHost, unregisterHost } = useActions()
    const items = useContext(ItemsContext)
    const setRef = useCallback(
        (node: View | null) => {
            if (node)
                registerHost(name, Platform.OS === 'web' ? (node as unknown as HTMLElement) : null)
        },
        [name, registerHost]
    )
    useLayoutEffect(() => () => unregisterHost(name), [name, unregisterHost])
    const hostItems = items[name] ?? []

    return (
        <View
            ref={setRef}
            pointerEvents="box-none"
            style={[StyleSheet.absoluteFill, name === 'root' ? ROOT_HOST_STYLE : undefined]}
            // An empty host must stay in the tree so a layer can portal into
            // it on the very next commit.
            collapsable={false}
        >
            {hostItems.map(item => (
                // Siblings, in mount order: a later layer paints over an
                // earlier one, which is the stacking a nested dialog needs
                // with no z-index in sight.
                <HostItemBoundary key={item.id}>{item.node}</HostItemBoundary>
            ))}
        </View>
    )
}

function HostItemBoundary({ children }: { children: ReactNode }) {
    return <>{children}</>
}

// Above gluestack's own overlay layer (9999 — its drawers still live there,
// and a menu opened from inside one must paint over it), the sidebar drawer
// (201) and the mobile sheet chrome (250); below the toast renderer
// (10000), which must stay readable over any dialog. `fixed` so the root
// host covers the viewport, not the scrolled document; it is a web-only
// position react-native's types do not know. The sheet host stays absolute:
// it must fill its region of the mobile chrome, not the screen.
const ROOT_HOST_STYLE: object | undefined =
    Platform.OS === 'web' ? { zIndex: 9999, position: 'fixed' } : undefined

/**
 * Renders `children` inside a host.
 *
 * Web: a React portal, so `contains()` and the consumer's contexts both work
 * as they would in place. Native: the host renders the element for us, so a
 * consumer's own React contexts do not reach it — a layer's content must be
 * self-contained (forms, stores and queries are; a bespoke provider around
 * the consumer is not).
 */
export function OverlayPortal({
    host = 'root',
    children,
}: {
    host?: OverlayHostName
    children: ReactNode
}) {
    const { mount, unmount } = useActions()
    const { hosts, domNodes } = useHostState()
    const id = useId()
    const resolvedHost: OverlayHostName = host === 'sheet' && hosts.sheet ? 'sheet' : 'root'

    // Re-published on every render so the host shows the latest children.
    useLayoutEffect(() => {
        if (Platform.OS === 'web') return
        mount(resolvedHost, id, children)
    })
    useLayoutEffect(() => {
        if (Platform.OS === 'web') return
        return () => unmount(resolvedHost, id)
    }, [unmount, resolvedHost, id])

    if (Platform.OS !== 'web') return null
    const node = domNodes[resolvedHost]
    if (!node) return null
    return createPortal(children, node)
}

/** Whether a `sheet` host is mounted — the mobile chrome is up. */
export function useHasSheetHost(): boolean {
    return useHostState().hosts.sheet === true
}
