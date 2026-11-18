import { log } from '@tinycld/core/lib/logger'
import { useUrl } from '@tinycld/core/lib/use-url'
import { useFocusEffect, useNavigation } from 'expo-router'
import { type NavigationAction, usePreventRemove } from 'expo-router/react-navigation'
import {
    type Dispatch,
    type RefObject,
    type SetStateAction,
    useCallback,
    useEffect,
    useRef,
    useState,
} from 'react'
import { Platform } from 'react-native'

/**
 * What the Discard confirm holds back: an in-app navigation action, which
 * Discard replays with its own `source` and `target` (see `useLeaveGuard`),
 * or a move of the browser's Back or Forward button (`delta` steps, negative
 * for Back). `leaving`: the user picked Discard, so nothing is held and the
 * guard is off until the screen is focused again.
 */
type Held =
    | { kind: 'action'; action: NavigationAction }
    | { kind: 'browser'; delta: number }
    | { kind: 'leaving' }

/**
 * Asks before a dirty editor is left. Every in-app way out (the header's Back,
 * hardware back, a swipe) removes the screen, which `usePreventRemove` holds
 * back until the user picks Discard. Discard turns removal prevention off and
 * replays the held action with the `source` it had, none when it had none,
 * not through `usePreventRemove`'s `repeat`: a screen's `navigation.dispatch`
 * stamps the screen's route key as the `source` of an action without one, and
 * a parent stack's move (a `router.replace` to one of its routes, or its pop)
 * has none; the parent stack does not know the nested screen's key, so it
 * would do nothing. The action keeps its `target`, so an action the screen's
 * own navigator does not handle goes up to the navigator it names. On web
 * expo-router also reduces the browser's Back and Forward through removal
 * prevention: it moves the browser back by itself and reports the blocked move
 * as a `RESET` action, whose replay would replace the history entry instead of
 * moving it. So `useBrowserHistoryGuard` holds those moves first, through the
 * shared `popstate` listener the app installs at start
 * (installBrowserBackGuard), and Discard turns removal prevention off before
 * it replays the move, or expo-router would block the replay too.
 * `usePreventRemove` also asks before a reload or tab close on web
 * (`beforeunload`). The guard's effect reaches the browser's events, which
 * only exist outside React: it arms the guard in the shared listener's list
 * (and, until the guard takes its entry, listens for the Navigation API's
 * `currententrychange`). On native it does nothing.
 *
 * After Discard the guard stays off while the screen is left. A move that
 * keeps the screen mounted (a browser Forward covers it) turns it on again
 * when the screen is focused again.
 *
 * The hook calls `useUrl` on every host, native too, so the host re-renders
 * on every URL change, also when the change is not on its own screen.
 *
 * The browser history half needs the Navigation API (`window.navigation`),
 * whose entries say exactly where a `popstate` went: the guard puts the URL
 * back and Discard replays the same move. Every current browser has it. A
 * browser without it gets no browser history guard: its Back and Forward
 * buttons leave a dirty screen without asking. In-app moves, reload and tab
 * close still ask there.
 *
 * Only the focused screen's guard holds a browser move. A move made while
 * another screen covers the dirty one (a screen pushed over it, or another
 * package's tab) goes through, even when it removes the dirty screen: a
 * covered screen is frozen (`freezeOnBlur`), so it cannot show its confirm,
 * and a switch to a package tab visited before replaces the dirty screen's
 * history entry instead of pushing one, so that entry then shows the tab's
 * URL: a hold would put the user back on the tab, not on the dirty screen.
 *
 * With the Navigation API, a browser move off the focused dirty screen goes
 * through when that screen has no entry to put the user back on: its URL
 * never showed the screen's path, so it never took one, or the browser
 * dropped its entry (session history has a cap). A move also goes through
 * inside the short pass window after a confirmed in-app move (see
 * `Expected`). Guards look for a dropped entry when they arm and at each
 * popstate that no guard caused or waits for (`registry.expected` is null);
 * a guard then takes the next entry that shows its screen while it is
 * focused, such as the slot expo-router replaces with the screen's URL on an
 * in-app return. A drop after the guard's last arm, such as the push that
 * brings the screen back being the one that drops its entry, is found only
 * at the popstate of the next browser move, too late to hold it. The first
 * move let through because the focused screen has no entry, whether it never
 * took one or the browser dropped it, logs one warning in the page's life
 * (`forms.leave-guard`). Later ones, moves made while the dirty screen is
 * covered, and moves in the pass window log nothing.
 */
export function useLeaveGuard(isDirty: boolean) {
    // The navigation held back while the Discard confirm is open: local UI state.
    const [held, setHeld] = useState<Held | null>(null)
    const guarding = isDirty && held?.kind !== 'leaving'
    const navigation = useNavigation()
    const disablePrevention = usePreventRemove(guarding, ({ data }) =>
        setHeld({ kind: 'action', action: data.action })
    )
    useBrowserHistoryGuard(guarding, setHeld)
    useFocusEffect(
        useCallback(() => {
            setHeld(current => (current?.kind === 'leaving' ? null : current))
        }, [])
    )
    const discard = () => {
        if (!held || held.kind === 'leaving') return
        setHeld({ kind: 'leaving' })
        if (held.kind === 'browser') {
            disablePrevention()
            replayBrowserMove(held.delta)
            return
        }
        // The held action moves the browser history too (expo-router's
        // `history.go(n)` fires a `popstate`), which no guard may swallow.
        passInAppMove()
        disablePrevention()
        // An explicit `source` key wins over the route key the screen's
        // dispatch stamps first, so the action reaches its navigator as held.
        navigation.dispatch({ ...held.action, source: held.action.source })
    }
    const isConfirming = held?.kind === 'action' || held?.kind === 'browser'
    return { isConfirming, cancel: () => setHeld(null), discard }
}

interface ArmedGuard {
    /**
     * The focused URL's path when the guard armed: the screen's own path
     * while it is focused, the covering screen's path while it is covered.
     * The guard takes its entry only while focused, when this is its own.
     */
    pathname: string
    /**
     * The key of the screen's Navigation API entry, kept by the screen across
     * arms, so it is taken again only when the browser drops it.
     */
    entryKey: RefObject<string | null>
    /** Whether a re-arm or popstate found the screen's entry dropped, and no entry has been taken since. */
    entryDropped: RefObject<boolean>
    isFocused: () => boolean
    hold: (delta: number) => void
    /**
     * Takes the screen's entry on the Navigation API's `currententrychange`
     * until the URL shows the screen; null once the entry is taken.
     */
    watch: (() => void) | null
}

/**
 * The next `popstate` the shared listener expects:
 * - `pass`: a confirmed move, which it lets through to expo-router. A
 *   confirmed in-app navigation's own `history.go(n)` may move no history,
 *   and then the browser fires no `popstate`, so that pass clears after
 *   IN_APP_PASS_MS. A replayed browser move always moves, so it waits.
 * - `restore`: the guard's own `go(n)` back to its entry, which it swallows;
 *   then the guard asks.
 */
type Expected =
    | { kind: 'pass'; timer: ReturnType<typeof setTimeout> | null }
    | { kind: 'restore'; guard: ArmedGuard; delta: number }

const IN_APP_PASS_MS = 500

interface GuardRegistry {
    /** The armed guards, most recently armed last (a re-arm moves a guard to the end). */
    armed: ArmedGuard[]
    expected: Expected | null
    isInstalled: boolean
    hasWarned: boolean
    /**
     * Whether a move was let through because the focused dirty guard has no
     * entry in the history: it never took one, or a re-arm or popstate found
     * it dropped and no entry has been taken since.
     */
    hasWarnedNoEntry: boolean
    /** The newest module's handler, which the installed listener calls. */
    onPopState: (event: PopStateEvent) => void
}

// Kept on globalThis, not in module scope: a Fast Refresh re-runs this module
// but cannot re-add the listener installed at app start (see
// installBrowserBackGuard), which must keep reaching the current guards.
const globalHost: typeof globalThis & { __tinycldLeaveGuards?: GuardRegistry } = globalThis
globalHost.__tinycldLeaveGuards ??= {
    armed: [],
    expected: null,
    isInstalled: false,
    hasWarned: false,
    hasWarnedNoEntry: false,
    onPopState: () => {},
}
const registry = globalHost.__tinycldLeaveGuards

function awaitPopState(next: Expected) {
    clearExpected()
    registry.expected = next
}

function clearExpected() {
    const expected = registry.expected
    if (expected?.kind === 'pass' && expected.timer) clearTimeout(expected.timer)
    registry.expected = null
}

function navigationApi(): Navigation | undefined {
    return 'navigation' in window ? window.navigation : undefined
}

function indexOfKey(api: Navigation, key: string | null): number | undefined {
    if (!key) return undefined
    return api.entries().find(e => e.key === key)?.index
}

function trimSlash(path: string) {
    return path.length > 1 ? path.replace(/\/+$/, '') : path
}

/** A malformed escape (a lone `%`) stays as the URL has it. */
function decodedSegment(segment: string) {
    try {
        return decodeURIComponent(segment)
    } catch {
        return segment
    }
}

/**
 * Whether the URL shows `pathname`, expo-router's path. expo-router encodes
 * `/ ? # %` and space inside a path param, and its path has the param
 * decoded, so the URL is decoded one segment at a time: `decodeURI` of the
 * whole path keeps `%2F`, `%3F` and `%23`.
 */
function isShowing(pathname: string) {
    const here = window.location.pathname
    const wanted = trimSlash(pathname)
    const decoded = here.split('/').map(decodedSegment).join('/')
    return trimSlash(here) === wanted || trimSlash(decoded) === wanted
}

/**
 * Takes the screen's entry once the URL shows the screen. A screen renders,
 * and may be dirty, before expo-router pushes its entry, so the entry the
 * browser is on at the first arm may still be the previous screen's. A
 * covered screen's path is the covering screen's URL, so it waits for focus.
 */
function takeEntryIfShown(guard: ArmedGuard, api: Navigation): boolean {
    if (guard.entryKey.current) return true
    const now = api.currentEntry
    if (!now || !guard.isFocused() || !isShowing(guard.pathname)) return false
    guard.entryKey.current = now.key
    guard.entryDropped.current = false
    return true
}

/**
 * Makes sure the guard has its entry or watches for it. A replace keeps a
 * slot's key, but the browser caps session history and drops the oldest
 * entries, so a mounted guard's entry can be gone. Runs at each arm and at
 * each popstate that no guard caused or waits for, so a drop is found only
 * there. Then the guard has no entry until the URL shows its screen again
 * while it is focused, as on expo-router's replace at an in-app return. At a
 * popstate it takes only the entry the move went to.
 */
function seekEntry(guard: ArmedGuard, api: Navigation) {
    if (guard.entryKey.current && indexOfKey(api, guard.entryKey.current) === undefined) {
        guard.entryKey.current = null
        guard.entryDropped.current = true
    }
    if (takeEntryIfShown(guard, api)) {
        stopWatching(guard, api)
        return
    }
    if (guard.watch) return
    // The push or replace that shows the screen announces itself with
    // `currententrychange`.
    const watch = () => {
        if (takeEntryIfShown(guard, api)) stopWatching(guard, api)
    }
    guard.watch = watch
    api.addEventListener('currententrychange', watch)
}

function stopWatching(guard: ArmedGuard, api: Navigation) {
    if (guard.watch) api.removeEventListener('currententrychange', guard.watch)
    guard.watch = null
}

/**
 * Logs once when a move is let through because the focused dirty guard has
 * no entry to put the user back on.
 */
function warnIfNoEntry() {
    if (registry.hasWarnedNoEntry) return
    const guard = registry.armed.find(g => g.isFocused() && !g.entryKey.current)
    if (!guard) return
    registry.hasWarnedNoEntry = true
    const reason = guard.entryDropped.current
        ? 'its entry left the history'
        : 'the URL never showed the screen'
    log.warn('forms.leave-guard', `browser move not held: ${reason}`, {
        pathname: guard.pathname,
        url: window.location.pathname,
    })
}

/**
 * Installs the one `popstate` listener the leave guards share. It must run
 * before expo-router's own listener, which renders a browser Back
 * synchronously, so the screen and its guard are gone before any later
 * listener runs. The browser runs the listeners of one phase in the order
 * they were added, so the app's root layout calls this at module load,
 * before the navigation container mounts and adds expo-router's. Web only; a
 * second call does nothing.
 */
export function installBrowserBackGuard() {
    if (Platform.OS !== 'web' || registry.isInstalled) return
    registry.isInstalled = true
    window.addEventListener('popstate', event => registry.onPopState(event))
}

/**
 * A browser Back or Forward has already moved the history when `popstate`
 * fires. When the move leaves a dirty screen, the listener stops the event
 * before expo-router sees it and moves back to where the user was (that
 * entry is still there, so its URL and `history.state.id`, expo-router's
 * history index, are restored), then the guard asks. Without the Navigation
 * API no guard is armed, so every move goes on to expo-router.
 */
registry.onPopState = (event: PopStateEvent) => {
    const api = navigationApi()
    const expected = registry.expected
    if (expected) {
        clearExpected()
        if (expected.kind === 'restore') {
            event.stopImmediatePropagation()
            expected.guard.hold(expected.delta)
        }
        return
    }
    if (!api) return
    for (const guard of registry.armed) seekEntry(guard, api)
    if (holdForFocused(api)) {
        event.stopImmediatePropagation()
        return
    }
    warnIfNoEntry()
}

/** The focused screen's guard holds any move off its entry. */
function holdForFocused(api: Navigation): boolean {
    const guard = registry.armed.filter(g => g.entryKey.current && g.isFocused()).at(-1)
    const now = api.currentEntry
    if (!guard || !now || now.key === guard.entryKey.current) return false
    const guarded = indexOfKey(api, guard.entryKey.current)
    if (guarded === undefined) return false
    restore(guard, now.index - guarded)
    return true
}

/** Puts the browser back where the user was before a move of `delta` steps, then the guard asks. */
function restore(guard: ArmedGuard, delta: number) {
    awaitPopState({ kind: 'restore', guard, delta })
    window.history.go(-delta)
}

function arm(guard: ArmedGuard, api: Navigation) {
    if (!registry.isInstalled && !registry.hasWarned) {
        registry.hasWarned = true
        log.warn('forms.leave-guard', 'browser Back guard not installed', {
            fix: 'call installBrowserBackGuard() at app start',
        })
    }
    registry.armed.push(guard)
    seekEntry(guard, api)
}

function disarm(guard: ArmedGuard, api: Navigation) {
    stopWatching(guard, api)
    const index = registry.armed.indexOf(guard)
    if (index !== -1) registry.armed.splice(index, 1)
}

/** Lets a held browser move through: the listener passes it on to expo-router. */
function replayBrowserMove(delta: number) {
    if (Platform.OS !== 'web') return
    awaitPopState({ kind: 'pass', timer: null })
    window.history.go(delta)
}

function passInAppMove() {
    if (Platform.OS !== 'web') return
    awaitPopState({ kind: 'pass', timer: setTimeout(clearExpected, IN_APP_PASS_MS) })
}

/**
 * Holds the browser's Back and Forward buttons on web while the screen is
 * dirty, where the browser has the Navigation API. The guard stays armed
 * while another screen covers this one, so each popstate still finds a drop
 * of its entry, but it holds only while focused; the listener reads
 * the focus at the event (a frozen screen, freezeOnBlur, may not have
 * rendered its blur yet).
 */
function useBrowserHistoryGuard(isDirty: boolean, setHeld: Dispatch<SetStateAction<Held | null>>) {
    const navigation = useNavigation()
    // The URL's path while the screen is focused, the one its entry shows.
    const { pathname } = useUrl()
    // The screen's entry, kept for its life: when a covering screen goes back
    // in-app, the screen re-arms before the browser history follows.
    const entryKey = useRef<string | null>(null)
    const entryDropped = useRef(false)
    useEffect(() => {
        if (Platform.OS !== 'web' || !isDirty) return
        const api = navigationApi()
        if (!api) return
        const guard: ArmedGuard = {
            pathname,
            entryKey,
            entryDropped,
            isFocused: () => navigation.isFocused(),
            hold: delta => setHeld({ kind: 'browser', delta }),
            watch: null,
        }
        arm(guard, api)
        return () => disarm(guard, api)
    }, [isDirty, navigation, pathname, setHeld])
}
