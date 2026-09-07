import { useNavigation, useRouter } from 'expo-router'
import { useEffect, useRef } from 'react'

/**
 * Keep one piece of shared UI state (a Zustand store field) and the route's
 * params in step, without the two ever fighting.
 *
 * The store stays the mechanism — every component keeps reading and writing
 * it — and the URL becomes its projection, so the state is linkable and
 * survives a reload. Boards' open card (`/a/boards/PL-12`) and drive's file
 * preview (`?file=&preview=1`) are this shape; so is any selected-item,
 * open-panel or active-tab state a package wants in the address bar.
 *
 * TWO DIRECTIONS, EACH FIRING ONLY ON ITS OWN SIDE CHANGING. That is the whole
 * design, and it is what every hand-rolled version of this got wrong: two
 * effects with the other side's values in their dependency arrays, so the URL
 * resolving ran the store->URL effect in the same commit, with a render-time
 * store value that predated the write it had just caused — it read "nothing
 * set", rewrote the URL without the value, and the two fought forever (React
 * #185 on every deep link).
 *
 *   URL -> store  an effect keyed on `urlValue`: a fresh load, a paste, a
 *                 back/forward, or the echo of a write below.
 *   store -> URL  a store SUBSCRIPTION, not an effect: it sees the value as
 *                 written, never a stale render's copy, and it never runs
 *                 because the URL changed.
 *
 * `syncedRef` is what the URL currently reflects, as far as this hook knows.
 * Either direction records it when it acts, so the other can tell an echo of
 * its own write from a genuine change.
 *
 * Writes go through `setParams`, never `replace`: REPLACE mints a new route
 * key and remounts the screen, while SET_PARAMS keeps the key and swaps the
 * params. The web history layer treats a same-route param change as a
 * replace, so Back leaves the screen rather than walking the state's history.
 */
export interface UrlStateSyncOptions<T> {
    /**
     * Off until whatever the URL resolves against has loaded. Neither
     * direction runs before this — a URL naming a record that has not synced
     * must not be read as "nothing", and the store must not be written out to
     * a URL that has not been read yet.
     */
    isReady: boolean
    /**
     * What the URL says the value is, resolved by the caller during render.
     * `undefined` means the URL names something that cannot be resolved yet
     * and neither direction should act; use `null` (or the type's own
     * "nothing") for a URL that names nothing.
     *
     * Must be a stable value — a primitive, or memoized — because it is the
     * URL->store effect's only dependency.
     */
    urlValue: T | undefined
    /** The route's current params, as plain strings ('' for absent). */
    params: Record<string, string>
    /**
     * The params that spell `value`. Name EVERY key the URL may carry, with
     * `undefined` for the ones this value does not use, so a write clears
     * whichever spelling the URL had before.
     */
    format: (value: T) => Record<string, string | undefined>
    read: () => T
    write: (value: T) => void
    /** The store's subscription; the listener re-reads through `read`. */
    subscribe: (listener: () => void) => () => void
    isEqual?: (a: T, b: T) => boolean
    /**
     * Called INSTEAD of `setParams` when this route is not the focused one —
     * a screen stays mounted under a route pushed over it, and `setParams`
     * would then change the covering route's params. Boards navigates back to
     * the board with the card peeked, which pops the page; omit it to write
     * this route's own params regardless.
     */
    onChangeWhileBlurred?: (value: T) => void
}

export function useUrlStateSync<T>(options: UrlStateSyncOptions<T>): void {
    const router = useRouter()
    const navigation = useNavigation()
    const { isReady, urlValue, subscribe } = options
    const isEqual = options.isEqual ?? Object.is

    // The latest options, read at event time by the subscription below so it
    // never needs to re-subscribe on a render — and never closes over a stale
    // `params` or `format`.
    const latestRef = useRef(options)
    latestRef.current = options

    // What the URL reflects. `null` is "unknown" — nothing has been read yet.
    const syncedRef = useRef<{ value: T } | null>(null)

    // URL -> store.
    useEffect(() => {
        if (!isReady || urlValue === undefined) return
        syncedRef.current = { value: urlValue }
        const { read, write } = latestRef.current
        if (!isEqual(read(), urlValue)) write(urlValue)
    }, [isReady, urlValue, isEqual])

    // store -> URL.
    useEffect(() => {
        if (!isReady) return
        const sync = () => {
            const { read, format, params, onChangeWhileBlurred } = latestRef.current
            const value = read()
            if (syncedRef.current && isEqual(syncedRef.current.value, value)) return
            // Nothing has been read from the URL yet, so there is nothing to
            // diverge from: record the store's value and wait for the URL side.
            if (!syncedRef.current) {
                syncedRef.current = { value }
                return
            }
            syncedRef.current = { value }
            if (onChangeWhileBlurred && !navigation.isFocused()) {
                onChangeWhileBlurred(value)
                return
            }
            const want = format(value)
            const inStep = Object.entries(want).every(
                ([key, spelled]) => (params[key] ?? '') === (spelled ?? '')
            )
            if (inStep) return
            router.setParams(want)
        }
        // A change made before this subscription attached — a value written
        // in the same commit the data arrived in.
        sync()
        return subscribe(sync)
    }, [isReady, subscribe, isEqual, router, navigation])
}
