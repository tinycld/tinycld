// @vitest-environment happy-dom
import { act, cleanup, renderHook } from '@testing-library/react'
import { log } from '@tinycld/core/lib/logger'
import { createElement, type ReactNode } from 'react'
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { ScreenStub, setRouterPathname, usePathname } from '../../../../tests/expo-router-stub'
import { installBrowserBackGuard, useLeaveGuard } from '../use-leave-guard'

/**
 * The browser half of the leave guard: the one shared `popstate` listener and
 * the guards it answers for. happy-dom stands in for the browser. Its history
 * fires `popstate` at once; a real browser fires it a little later, so the
 * guard's own `history.go(n)` is played back after `browser.delay` ms, which
 * also lets a test make that `popstate` slow. The user's own Back and Forward
 * (`userGo`) move at once. happy-dom has no Navigation API; the guard holds
 * browser history moves only where the API exists, so the file installs a
 * fake of the part the guard reads, and one block removes it to check that
 * without it a move goes straight through. `routerSaw` is a later `popstate`
 * listener: it stands in for expo-router's, which must not see a move the
 * guard holds or a restore it makes, and it moves the router's path to the
 * URL it is handed, as expo-router does.
 *
 * The router's path is shared by every mounted screen, as `useUrl` is in the
 * app: `visit` moves it to the pushed screen's path first, then pushes the
 * entry, so a covered screen re-renders with the covering screen's path.
 */

const browser = {
    delay: 10,
    userGo: (_delta: number) => {},
    /** Every `history.go`, `back` and `forward` the guard makes, as a delta. */
    guardMoves: [] as number[],
}
const routerSaw = vi.fn()

/** The hook's IN_APP_PASS_MS: how long a confirmed in-app move waits for its `popstate`. */
const IN_APP_PASS_MS = 500

type HeldAction = { data: { action: { type: string } } }

/**
 * The in-app removal the screen's `usePreventRemove` holds back, while it
 * prevents one. As in expo-router 58, the callback gets `repeat` and the hook
 * returns `disablePrevention`. The guard does not use `repeat`: on Discard it
 * turns prevention off and dispatches the held action through the screen's
 * navigation with the action's own `source`.
 */
const preventRemove = vi.hoisted(() => ({
    onPrevent: null as null | ((event: HeldAction) => void),
    disablePrevention: (() => {}) as () => void,
}))
vi.mock('expo-router/react-navigation', async () => {
    const { useNavigation } = await import('expo-router')
    return {
        usePreventRemove: (
            isPreventing: boolean,
            onPrevent: (event: HeldAction & { repeat: () => void }) => void
        ) => {
            const navigation = useNavigation() as { dispatch: (action: unknown) => void }
            preventRemove.onPrevent = isPreventing
                ? event =>
                      onPrevent({
                          data: event.data,
                          repeat: () => navigation.dispatch(event.data.action),
                      })
                : null
            return preventRemove.disablePrevention
        },
    }
})

/** A screen the guard runs in, at `pathname`; `focused` is read at the event. */
function screenAt(pathname: string) {
    const screen = {
        pathname,
        focused: true,
        navigation: { isFocused: () => screen.focused, dispatch: vi.fn() },
    }
    return screen
}

/** Renders the guard in `screen`; `renders.count` counts the hook's renders. */
function renderGuard(screen: ReturnType<typeof screenAt>, isDirty: boolean) {
    const renders = { count: 0 }
    const wrapper = ({ children }: { children: ReactNode }) =>
        createElement(ScreenStub, { value: screen }, children)
    const hook = renderHook(
        ({ dirty }) => {
            renders.count += 1
            return useLeaveGuard(dirty)
        },
        { initialProps: { dirty: isDirty }, wrapper }
    )
    return Object.assign(hook, { renders })
}

/** Moves the router's path, which every mounted screen's `useUrl` reads. */
function routeTo(pathname: string) {
    act(() => {
        setRouterPathname(pathname)
    })
}

// Entry ids stay unique across tests: happy-dom keeps one history per file.
let run = 0
function entryId(name: string) {
    return `${run}:${name}`
}

/**
 * The part of the Navigation API the guard reads: each entry's key, id, index
 * and URL, the entry the browser is on (from expo-router's
 * `history.state.id`, so it follows every push and traversal), and the
 * `currententrychange` event, which fires on every push, replace and
 * traversal as in a browser (before the traversal's `popstate`). `order`
 * holds this test's history slots, oldest first, by expo-router's
 * `history.state.id`; `visit` keeps it in step with the history, dropping the
 * forward slots a push replaces as a browser does. A test models the
 * browser's session-history cap by removing slots from `order`; happy-dom's
 * own history still holds them, so no test moves onto a removed slot.
 *
 * As in a browser, an entry's `key` names its slot in the history and a
 * replace (`history.replaceState` too) keeps it; only the entry's `id` is new.
 * expo-router's replace keeps `history.state.id` as well, so the slot is
 * found again after a replace.
 *
 * The fake's limit: it ties an entry's key to `history.state.id`, which
 * matches expo-router, whose replace keeps the id. A `replaceState` that
 * changes the id would make the fake report `currentEntry` null, where a
 * browser keeps the slot's key.
 */
const order: string[] = []
const urls = new Map<string, string>()
/** The Navigation API `id` of each slot's current entry; a replace gives a new one. */
const entryIds = new Map<string, string>()
let nextEntryId = 0

function newEntryId(slot: string) {
    nextEntryId += 1
    entryIds.set(slot, `entry:${nextEntryId}`)
}

function entryOf(slot: string) {
    return {
        key: `key:${slot}`,
        id: entryIds.get(slot) ?? null,
        index: order.indexOf(slot),
        url: urls.get(slot) ?? null,
    }
}

function currentId() {
    const state: unknown = window.history.state
    const id = state && typeof state === 'object' && 'id' in state ? state.id : null
    return typeof id === 'string' && order.includes(id) ? id : null
}

class FakeNavigation extends EventTarget {
    /** The `currententrychange` listeners added and not yet removed. */
    readonly watchers = new Set<EventListenerOrEventListenerObject>()
    addEventListener(...args: Parameters<EventTarget['addEventListener']>) {
        const [type, listener] = args
        if (type === 'currententrychange' && listener) this.watchers.add(listener)
        super.addEventListener(...args)
    }
    removeEventListener(...args: Parameters<EventTarget['removeEventListener']>) {
        const [type, listener] = args
        if (type === 'currententrychange' && listener) this.watchers.delete(listener)
        super.removeEventListener(...args)
    }
    entries() {
        return order.map(entryOf)
    }
    get currentEntry() {
        const id = currentId()
        return id === null ? null : entryOf(id)
    }
}
const fakeNavigation = new FakeNavigation()
let shownEntry: ReturnType<typeof entryOf> | null = null

function announceEntryChange(navigationType: 'push' | 'replace' | 'traverse') {
    const from = shownEntry
    shownEntry = fakeNavigation.currentEntry
    const event = Object.assign(new Event('currententrychange'), { navigationType, from })
    fakeNavigation.dispatchEvent(event)
}

function setNavigationApi(isPresent: boolean) {
    if (isPresent) {
        Object.defineProperty(window, 'navigation', { configurable: true, value: fakeNavigation })
    } else {
        Reflect.deleteProperty(window, 'navigation')
    }
}

// The browser has the API from the start, before the app installs its
// listener. This listener is added before that one, so the entry change of a
// traversal is announced before the guard's `popstate` listener runs.
setNavigationApi(true)
window.addEventListener('popstate', () => announceEntryChange('traverse'))

/**
 * Pushes a screen the way expo-router does: the router's path moves to the
 * screen's `path` (and every mounted screen renders), then the history entry
 * is pushed at `url` with `history.state = { id }`. `url` differs from `path`
 * only where the router's path decodes what the URL keeps encoded.
 *
 * `keepFrom` models a push that hits the browser's session-history cap: the
 * slots older than `keepFrom` are dropped as part of the push, after the
 * screen renders and before the entry change is announced, as in a browser.
 */
function visit(name: string, path: string, url = path, { keepFrom }: { keepFrom?: string } = {}) {
    routeTo(path)
    const current = currentId()
    if (current !== null) order.length = order.indexOf(current) + 1
    urls.set(entryId(name), new URL(url, window.location.href).href)
    newEntryId(entryId(name))
    order.push(entryId(name))
    if (keepFrom) order.splice(0, order.indexOf(keepFrom))
    act(() => {
        window.history.pushState({ id: entryId(name) }, '', url)
    })
}

/**
 * Replaces the entry the browser is on, as expo-router's `setParams` and
 * `replace` on the same screen do: the router's path moves to `path`, then
 * the entry is replaced at `url` with the same `history.state.id`. The slot,
 * and so the entry's key, stays; the entry gets a new id.
 */
function replaceCurrent(path: string, url = path) {
    const current = currentId()
    if (current === null) throw new Error('no entry to replace')
    routeTo(new URL(path, window.location.href).pathname)
    urls.set(current, new URL(url, window.location.href).href)
    newEntryId(current)
    act(() => {
        window.history.replaceState({ id: current }, '', url)
    })
}

/**
 * The warn-once flags of the guards' shared registry. The registry lives on
 * globalThis for the whole file, so each test starts with them cleared and no
 * test depends on which test warned first.
 */
interface WarnFlags {
    hasWarned: boolean
    hasWarnedNoEntry: boolean
}
function resetWarnFlags() {
    const host: typeof globalThis & { __tinycldLeaveGuards?: WarnFlags } = globalThis
    const registry = host.__tinycldLeaveGuards
    if (!registry) return
    registry.hasWarned = false
    registry.hasWarnedNoEntry = false
}

/** The entry key each armed guard holds as its own, read from the shared registry. */
interface TakenEntries {
    armed: { entryKey: { current: string | null } }[]
}
function takenEntries() {
    const host: typeof globalThis & { __tinycldLeaveGuards?: TakenEntries } = globalThis
    return host.__tinycldLeaveGuards?.armed.map(guard => guard.entryKey.current) ?? []
}

/** The route the router renders, which a held move must leave the URL on. */
function renderedRoute() {
    return renderHook(() => usePathname())
}

function leaveGuardWarnings(warn: { mock: { calls: unknown[][] } }) {
    return warn.mock.calls.filter(([context]) => context === 'forms.leave-guard')
}

async function userGo(delta: number) {
    await act(async () => {
        browser.userGo(delta)
    })
}

async function settle(ms = 1000) {
    await act(async () => {
        vi.advanceTimersByTime(ms)
    })
}

function here() {
    return window.location.pathname
}

function installSlowHistory() {
    const history = window.history
    const go = history.go.bind(history)
    browser.userGo = go
    history.go = (delta?: number) => {
        browser.guardMoves.push(delta ?? 0)
        setTimeout(() => go(delta), browser.delay)
    }
    history.back = () => history.go(-1)
    history.forward = () => history.go(1)
    const pushState = history.pushState.bind(history)
    const replaceState = history.replaceState.bind(history)
    history.pushState = (...args: Parameters<History['pushState']>) => {
        pushState(...args)
        announceEntryChange('push')
    }
    history.replaceState = (...args: Parameters<History['replaceState']>) => {
        replaceState(...args)
        announceEntryChange('replace')
    }
}

function removeSlowHistory() {
    for (const method of ['go', 'back', 'forward', 'pushState', 'replaceState']) {
        Reflect.deleteProperty(window.history, method)
    }
}

beforeEach(() => {
    run += 1
    order.length = 0
    urls.clear()
    entryIds.clear()
    shownEntry = null
    browser.delay = 10
    browser.guardMoves = []
    preventRemove.onPrevent = null
    preventRemove.disablePrevention = vi.fn()
    resetWarnFlags()
    vi.useFakeTimers()
    installSlowHistory()
})

afterEach(async () => {
    await settle()
    cleanup()
    setRouterPathname(null)
    removeSlowHistory()
    vi.useRealTimers()
    vi.restoreAllMocks()
    routerSaw.mockReset()
})

// Runs first, before any test in this file installs the listener.
describe('before the shared listener is installed', () => {
    it('warns once that the guard is not installed, not on every arm', async () => {
        const warn = vi.spyOn(log, 'warn').mockImplementation(() => {})
        visit('editor', '/editor')
        const first = renderGuard(screenAt('/editor'), true)
        first.rerender({ dirty: false })
        first.rerender({ dirty: true })
        renderGuard(screenAt('/editor'), true)
        await settle()
        expect(leaveGuardWarnings(warn)).toHaveLength(1)
    })
})

describe('with the shared listener installed', () => {
    beforeAll(() => {
        installBrowserBackGuard()
        window.addEventListener('popstate', event => {
            routerSaw(event)
            setRouterPathname(decodeURIComponent(window.location.pathname))
        })
    })

    describe('without the Navigation API', () => {
        beforeEach(() => setNavigationApi(false))
        afterEach(() => setNavigationApi(true))

        it('lets a browser Back from a dirty editor straight through to the router', async () => {
            visit('list', '/list')
            visit('editor', '/editor')
            const { result } = renderGuard(screenAt('/editor'), true)

            await userGo(-1)
            expect(routerSaw).toHaveBeenCalledTimes(1)
            expect(result.current.isConfirming).toBe(false)
            expect(browser.guardMoves).toEqual([])
            expect(vi.getTimerCount()).toBe(0)

            await settle()
            expect(here()).toBe('/list')
            expect(result.current.isConfirming).toBe(false)
            expect(routerSaw).toHaveBeenCalledTimes(1)
            expect(browser.guardMoves).toEqual([])
        })
    })

    describe('with the Navigation API', () => {
        it('puts a browser Forward back on the guarded entry and replays it on Discard', async () => {
            visit('list', '/list')
            visit('editor', '/editor')
            visit('over', '/over')
            await userGo(-1)
            routerSaw.mockReset()
            const { result } = renderGuard(screenAt('/editor'), true)

            await userGo(1)
            await settle()
            expect(here()).toBe('/editor')
            expect(result.current.isConfirming).toBe(true)
            expect(routerSaw).not.toHaveBeenCalled()

            await act(async () => result.current.discard())
            await settle()
            expect(here()).toBe('/over')
            expect(result.current.isConfirming).toBe(false)
            expect(routerSaw).toHaveBeenCalledTimes(1)
        })

        it('puts a 2-step Back back on the guarded entry and replays both steps on Discard', async () => {
            visit('a', '/a')
            visit('b', '/b')
            visit('editor', '/editor')
            const { result } = renderGuard(screenAt('/editor'), true)

            await userGo(-2)
            await settle()
            expect(here()).toBe('/editor')
            expect(result.current.isConfirming).toBe(true)
            expect(routerSaw).not.toHaveBeenCalled()

            // expo-router reduces the replayed move through removal
            // prevention, so Discard turns it off before the move replays.
            await act(async () => result.current.discard())
            expect(preventRemove.disablePrevention).toHaveBeenCalledTimes(1)
            expect(preventRemove.onPrevent).toBeNull()
            expect(routerSaw).not.toHaveBeenCalled()
            await settle()
            expect(here()).toBe('/a')
            expect(result.current.isConfirming).toBe(false)
            expect(routerSaw).toHaveBeenCalledTimes(1)
        })

        it('lets only the focused guard answer a popstate', async () => {
            visit('list', '/list')
            visit('b', '/b')
            const covered = screenAt('/b')
            const coveredGuard = renderGuard(covered, true)
            // A screen pushed over B: from that push on, B's navigation says
            // it is not focused.
            covered.focused = false
            visit('a', '/a')
            const top = renderGuard(screenAt('/a'), true)
            // B arms again after the top screen, so it is the most recently
            // armed guard: being armed last must not make it the one that asks.
            coveredGuard.rerender({ dirty: false })
            coveredGuard.rerender({ dirty: true })

            await userGo(-1)
            await settle()
            expect(top.result.current.isConfirming).toBe(true)
            expect(coveredGuard.result.current.isConfirming).toBe(false)
            expect(here()).toBe('/a')
        })

        it('takes its entry from the entry change when dirty before the URL follows', async () => {
            visit('prev', '/prev')
            // Dirty on its first render: the router already shows the screen,
            // but its effects run before expo-router pushes the screen's entry.
            routeTo('/editor')
            const { result, renders } = renderGuard(screenAt('/editor'), true)
            const rendered = renders.count
            // Nothing polls for the entry: the entry change of the push is
            // the only way the guard learns it.
            expect(vi.getTimerCount()).toBe(0)
            visit('editor', '/editor')
            expect(renders.count).toBe(rendered)
            expect(vi.getTimerCount()).toBe(0)

            await userGo(-1)
            await settle()
            expect(result.current.isConfirming).toBe(true)
            expect(here()).toBe('/editor')
            expect(routerSaw).not.toHaveBeenCalled()
        })

        // expo-router encodes `/`, `?`, `#`, `%` and space inside a path param
        // (its encodeURISegment), and the router's path has the param decoded,
        // so the URL shows the screen only when read one decoded segment at a time.
        it.each([
            ['/', '%2F'],
            ['?', '%3F'],
            ['#', '%23'],
            ['%', '%25'],
            [' ', '%20'],
        ])('takes its entry and holds a browser Back when a path param holds "%s" (%s in the URL)', async (char, encoded) => {
            const warn = vi.spyOn(log, 'warn').mockImplementation(() => {})
            const url = `/files/a${encoded}b`
            visit('list', '/list')
            visit('doc', `/files/a${char}b`, url)
            const { result } = renderGuard(screenAt(`/files/a${char}b`), true)

            await userGo(-1)
            await settle()
            expect(result.current.isConfirming).toBe(true)
            expect(routerSaw).not.toHaveBeenCalled()
            expect(here()).toBe(url)
            expect(leaveGuardWarnings(warn)).toHaveLength(0)
        })

        it('warns once and lets moves through when its path never matches the URL', async () => {
            const warn = vi.spyOn(log, 'warn').mockImplementation(() => {})
            visit('list', '/list')
            // The router's path and the URL name different places altogether,
            // so the guard never takes an entry.
            visit('doc', '/files/a', '/elsewhere/a')
            const first = renderGuard(screenAt('/files/a'), true)
            await settle()
            expect(leaveGuardWarnings(warn)).toHaveLength(0)

            await userGo(-1)
            await settle()
            expect(first.result.current.isConfirming).toBe(false)
            expect(routerSaw).toHaveBeenCalledTimes(1)
            expect(here()).toBe('/list')
            expect(leaveGuardWarnings(warn)).toHaveLength(1)
            first.unmount()

            // A second unheld move, from another screen that never matches.
            visit('doc2', '/files/b', '/elsewhere/b')
            const second = renderGuard(screenAt('/files/b'), true)
            await userGo(-1)
            await settle()
            expect(second.result.current.isConfirming).toBe(false)
            expect(routerSaw).toHaveBeenCalledTimes(2)
            expect(here()).toBe('/list')
            expect(leaveGuardWarnings(warn)).toHaveLength(1)
        })

        // A replace keeps the entry's key (its slot in the history) and gives
        // it a new id, so the entry the guard took is still there to go back to.
        // A stack `router.replace` is not a case: it gives the route a new key
        // and removes the screen, so the same hook cannot outlive it.
        it('holds a browser Back after a setParams replaces its entry', async () => {
            const warn = vi.spyOn(log, 'warn').mockImplementation(() => {})
            const path = '/customers/c1'
            const replaced = '/customers/c1?tab=notes'
            visit('list', '/list')
            visit('editor', path)
            const { result } = renderGuard(screenAt(path), true)
            const before = fakeNavigation.currentEntry
            replaceCurrent(replaced)
            const after = fakeNavigation.currentEntry
            expect(after?.key).toBe(before?.key)
            expect(after?.id).not.toBe(before?.id)

            await userGo(-1)
            await settle()
            expect(result.current.isConfirming).toBe(true)
            expect(routerSaw).not.toHaveBeenCalled()
            expect(window.location.pathname + window.location.search).toBe(replaced)
            expect(currentId()).toBe(entryId('editor'))
            expect(leaveGuardWarnings(warn)).toHaveLength(0)

            await act(async () => result.current.discard())
            await settle()
            expect(here()).toBe('/list')
            expect(result.current.isConfirming).toBe(false)
            expect(routerSaw).toHaveBeenCalledTimes(1)
        })

        it('takes no entry from an entry change while not focused, and takes one once focused', async () => {
            visit('list', '/list')
            routeTo('/editor')
            const editor = screenAt('/editor')
            editor.focused = false
            const { result } = renderGuard(editor, true)
            // The URL shows the screen's path, but the screen is not focused.
            visit('early', '/editor')
            editor.focused = true
            visit('editor', '/editor')

            // Had it taken 'early', this Back would land on its own entry and pass.
            await userGo(-1)
            await settle()
            expect(result.current.isConfirming).toBe(true)
            expect(routerSaw).not.toHaveBeenCalled()
            expect(here()).toBe('/editor')
            expect(currentId()).toBe(entryId('editor'))
        })

        it('stops watching for its entry when it becomes clean', async () => {
            visit('prev', '/prev')
            routeTo('/editor')
            const guard = renderGuard(screenAt('/editor'), true)
            expect(fakeNavigation.watchers.size).toBe(1)

            guard.rerender({ dirty: false })
            expect(fakeNavigation.watchers.size).toBe(0)
            visit('editor', '/editor')
            await userGo(-1)
            await settle()
            expect(guard.result.current.isConfirming).toBe(false)
            expect(routerSaw).toHaveBeenCalledTimes(1)
            expect(here()).toBe('/prev')
        })

        it('stops watching for its entry when it unmounts', async () => {
            visit('prev', '/prev')
            routeTo('/editor')
            const guard = renderGuard(screenAt('/editor'), true)
            expect(fakeNavigation.watchers.size).toBe(1)

            guard.unmount()
            expect(fakeNavigation.watchers.size).toBe(0)
            visit('editor', '/editor')
            await userGo(-1)
            await settle()
            expect(routerSaw).toHaveBeenCalledTimes(1)
            expect(here()).toBe('/prev')
        })

        it('stops watching once it takes its entry', () => {
            visit('prev', '/prev')
            routeTo('/editor')
            renderGuard(screenAt('/editor'), true)
            expect(fakeNavigation.watchers.size).toBe(1)
            visit('editor', '/editor')
            expect(fakeNavigation.watchers.size).toBe(0)
        })

        it('holds a browser Back again once an in-app pass expires with no popstate', async () => {
            visit('list', '/list')
            visit('editor', '/editor')
            const editor = screenAt('/editor')
            const first = renderGuard(editor, true)
            // An in-app removal, held and then confirmed. Its own history move
            // moved nothing, so no popstate comes for the pass.
            await act(async () =>
                preventRemove.onPrevent?.({ data: { action: { type: 'GO_BACK' } } })
            )
            expect(first.result.current.isConfirming).toBe(true)
            await act(async () => first.result.current.discard())
            expect(editor.navigation.dispatch).toHaveBeenCalledTimes(1)
            expect(preventRemove.onPrevent).toBeNull()
            expect(first.result.current.isConfirming).toBe(false)
            // The confirmed removal takes the screen away; the editor opened
            // again on the same entry is dirty with a new guard.
            first.unmount()
            const { result } = renderGuard(screenAt('/editor'), true)
            await settle(IN_APP_PASS_MS + 100)

            await userGo(-1)
            await settle()
            expect(result.current.isConfirming).toBe(true)
            expect(here()).toBe('/editor')
            expect(routerSaw).not.toHaveBeenCalled()
        })

        it('lets a replayed move through even when its popstate comes after 500 ms', async () => {
            visit('list', '/list')
            visit('editor', '/editor')
            const { result } = renderGuard(screenAt('/editor'), true)
            await userGo(-1)
            await settle()
            expect(result.current.isConfirming).toBe(true)

            browser.delay = 600
            await act(async () => result.current.discard())
            await settle()
            expect(here()).toBe('/list')
            expect(result.current.isConfirming).toBe(false)
            expect(routerSaw).toHaveBeenCalledTimes(1)
        })

        it('swallows its own restore even when its popstate comes after 500 ms', async () => {
            visit('list', '/list')
            visit('editor', '/editor')
            const { result } = renderGuard(screenAt('/editor'), true)
            browser.delay = 600

            await userGo(-1)
            await settle()
            expect(here()).toBe('/editor')
            expect(result.current.isConfirming).toBe(true)
            expect(routerSaw).not.toHaveBeenCalled()
        })

        /**
         * A dirty editor at /editor, covered by a screen at /over pushed over
         * it. The push moves the router's path, so the editor renders with
         * /over and its guard arms again while covered.
         */
        function coveredEditor() {
            visit('list', '/list')
            visit('editor', '/editor')
            const editor = screenAt('/editor')
            const guard = renderGuard(editor, true)
            const rendered = guard.renders.count
            editor.focused = false
            visit('over', '/over')
            expect(guard.renders.count).toBeGreaterThan(rendered)
            return Object.assign(guard, { editor })
        }

        it('lets a covering screen go Back to the dirty editor without asking', async () => {
            const { result } = coveredEditor()
            await userGo(-1)
            await settle()
            expect(here()).toBe('/editor')
            expect(result.current.isConfirming).toBe(false)
            expect(routerSaw).toHaveBeenCalledTimes(1)
        })

        it('keeps its entry while covered, so a browser Back after the cover pops still asks', async () => {
            const covered = coveredEditor()
            await userGo(-1)
            await settle()
            expect(here()).toBe('/editor')
            expect(routerSaw).toHaveBeenCalledTimes(1)
            covered.editor.focused = true

            await userGo(-1)
            await settle()
            expect(covered.result.current.isConfirming).toBe(true)
            expect(here()).toBe('/editor')
            expect(routerSaw).toHaveBeenCalledTimes(1)
        })

        // A covered screen is frozen and cannot show its confirm, so only the
        // focused guard holds: a skip past the covered editor goes through.
        it('lets a 2-step Back from a covering screen through, past the dirty editor', async () => {
            const warn = vi.spyOn(log, 'warn').mockImplementation(() => {})
            const { result } = coveredEditor()
            await userGo(-2)
            await settle()
            expect(result.current.isConfirming).toBe(false)
            expect(routerSaw).toHaveBeenCalledTimes(1)
            expect(browser.guardMoves).toEqual([])
            expect(here()).toBe('/list')
            expect(leaveGuardWarnings(warn)).toHaveLength(0)
        })

        // A switch to a package tab visited before is a replace, not a push:
        // the tab's screen takes over the editor's slot, key and all. A
        // regression guard only: the removed covered hold let this move
        // through too.
        it('lets a browser Back through when a tab switch replaced the dirty editor entry', async () => {
            const warn = vi.spyOn(log, 'warn').mockImplementation(() => {})
            visit('list', '/list')
            visit('editor', '/editor')
            const editor = screenAt('/editor')
            const { result } = renderGuard(editor, true)
            await settle()
            expect(takenEntries()).toEqual([`key:${entryId('editor')}`])
            editor.focused = false
            replaceCurrent('/tab')

            await userGo(-1)
            await settle()
            expect(result.current.isConfirming).toBe(false)
            expect(routerSaw).toHaveBeenCalledTimes(1)
            expect(browser.guardMoves).toEqual([])
            expect(here()).toBe('/list')
            expect(leaveGuardWarnings(warn)).toHaveLength(0)
        })

        /**
         * A dirty editor whose entry the browser dropped. A browser caps its
         * session history (about 50 entries in Chrome) and drops the oldest
         * entries first, so enough pushes over the editor drop its slot and
         * every older one; here the cap leaves only over1 and over2. Then an
         * in-app return to the editor: expo-router's memory history clamps
         * its go(n) to its own items, which still hold the editor, so it calls
         * the browser's go(-2). The browser cannot go that far back, so it
         * does nothing and fires no popstate; when none comes, expo-router
         * replaces the slot the move started on (over2) with the editor's URL.
         * The fake makes no move for that go (happy-dom's history still holds
         * the dropped slots and would move onto them) and replaces over2.
         * Its own entry gone, the guard has no entry; the replace shows
         * /editor on over2 while the editor is focused, so it takes over2.
         */
        function droppedEditor() {
            visit('list', '/list')
            visit('editor', '/editor')
            const editor = screenAt('/editor')
            const guard = renderGuard(editor, true)
            editor.focused = false
            visit('over1', '/over1')
            visit('over2', '/over2')
            order.splice(0, order.indexOf(entryId('over1')))
            editor.focused = true
            replaceCurrent('/editor')
            expect(currentId()).toBe(entryId('over2'))
            expect(here()).toBe('/editor')
            return Object.assign(guard, { editor })
        }

        it('takes the slot that shows its screen when its own entry left the history, and holds a browser Back there', async () => {
            const warn = vi.spyOn(log, 'warn').mockImplementation(() => {})
            const { result } = droppedEditor()
            await settle()
            // Taken at the replace's entry change, before any popstate.
            expect(takenEntries()).toEqual([`key:${entryId('over2')}`])
            expect(browser.guardMoves).toEqual([])

            await userGo(-1)
            await settle()
            expect(result.current.isConfirming).toBe(true)
            expect(routerSaw).not.toHaveBeenCalled()
            expect(browser.guardMoves).toEqual([1])
            expect(here()).toBe('/editor')
            expect(currentId()).toBe(entryId('over2'))
            expect(leaveGuardWarnings(warn)).toHaveLength(0)

            await act(async () => result.current.cancel())
            await userGo(-1)
            await settle()
            expect(result.current.isConfirming).toBe(true)
            expect(routerSaw).not.toHaveBeenCalled()
            expect(here()).toBe('/editor')
            expect(currentId()).toBe(entryId('over2'))

            await act(async () => result.current.discard())
            await settle()
            expect(here()).toBe('/over1')
            expect(result.current.isConfirming).toBe(false)
            expect(routerSaw).toHaveBeenCalledTimes(1)
            expect(leaveGuardWarnings(warn)).toHaveLength(0)
        })

        it('lets an in-app Back from a sub-screen to the editor through, and keeps the slot it took', async () => {
            const dropped = droppedEditor()
            const rendered = renderedRoute()
            await settle()
            dropped.editor.focused = false
            visit('over3', '/over3')
            // The in-app header Back: expo-router renders the editor first,
            // then calls history.go(-1).
            dropped.editor.focused = true
            routeTo('/editor')
            await userGo(-1)
            await settle()
            expect(dropped.result.current.isConfirming).toBe(false)
            expect(routerSaw).toHaveBeenCalledTimes(1)
            expect(browser.guardMoves).toEqual([])
            expect(here()).toBe('/editor')
            expect(currentId()).toBe(entryId('over2'))
            expect(takenEntries()).toEqual([`key:${entryId('over2')}`])

            // A browser Back from the editor is held on the slot it took.
            await userGo(-1)
            await settle()
            expect(dropped.result.current.isConfirming).toBe(true)
            expect(routerSaw).toHaveBeenCalledTimes(1)
            expect(currentId()).toBe(entryId('over2'))
            expect(here()).toBe('/editor')

            await act(async () => dropped.result.current.cancel())
            expect(rendered.result.current).toBe('/editor')
            expect(here()).toBe(rendered.result.current)
        })

        it('leaves the URL on the rendered route after a Cancel on a hold whose entry the browser dropped', async () => {
            const dropped = droppedEditor()
            const rendered = renderedRoute()
            await userGo(-1)
            await settle()
            expect(dropped.result.current.isConfirming).toBe(true)

            await act(async () => dropped.result.current.cancel())
            await settle()
            expect(dropped.result.current.isConfirming).toBe(false)
            expect(routerSaw).not.toHaveBeenCalled()
            expect(rendered.result.current).toBe('/editor')
            expect(here()).toBe(rendered.result.current)
            expect(currentId()).toBe(entryId('over2'))
        })

        // The documented limit: the guard looks for a dropped entry only at a
        // re-arm or a popstate. Here the push that shows the editor again is
        // the one that drops its entry: the re-arm runs when the router's
        // path moves, before the push, and the push drops the editor's slot
        // before it announces its entry change. So nothing sees the drop until
        // the user's browser Back, which goes through and logs the warning.
        // A guard that also looked at the push's entry change would find the
        // drop there and take 'again', and this Back would be held.
        it('lets a browser Back through, with one warning, when the return push drops its entry after the re-arm', async () => {
            const warn = vi.spyOn(log, 'warn').mockImplementation(() => {})
            visit('list', '/list')
            visit('editor', '/editor')
            const editor = screenAt('/editor')
            const { result } = renderGuard(editor, true)
            editor.focused = false
            visit('over1', '/over1')
            editor.focused = true
            visit('again', '/editor', '/editor', { keepFrom: entryId('over1') })
            expect(fakeNavigation.entries().map(entry => entry.key)).toEqual([
                `key:${entryId('over1')}`,
                `key:${entryId('again')}`,
            ])

            await userGo(-1)
            await settle()
            expect(result.current.isConfirming).toBe(false)
            expect(routerSaw).toHaveBeenCalledTimes(1)
            expect(here()).toBe('/over1')
            expect(leaveGuardWarnings(warn).map(([, message]) => message)).toEqual([
                'browser move not held: its entry left the history',
            ])
        })
    })
})
