import AsyncStorage from '@react-native-async-storage/async-storage'
import { captureException } from './errors'
import { pb } from './pocketbase'
import { reloadJsContext } from './reload-js-context'
import { getResolvedAddress, setResolvedAddress } from './server-address'
import { setActiveServer } from './servers'

// Persisted state that belongs to whichever server was active when it was
// written, and would otherwise be read back against a DIFFERENT server after a
// switch. The reload gives a fresh module graph, so in-memory leakage is moot —
// anything on disk is not.
//
//  - tinycld_sidebar_open holds `lastPackageHref`: deep-links to RECORD IDS on
//    the other server. logout() wipes it explicitly for exactly this reason
//    (auth-store.ts), but a switch never calls logout, so that wipe never runs.
//  - tinycld:anon-id is one share-link identity reused across unrelated servers.
//  - firstRun:* would suppress a new server's first-run guidance because a
//    different server already ran it.
//
// These are CLEARED rather than scoped per server. Scoping would preserve more
// (switching back would restore your sidebar state), but it means every package
// store in every sibling repo has to learn a server-scoped persist name — a
// coordinated multi-repo change for UI polish. Clearing is one place, and is
// strictly safe against cross-server leakage.
const CLEAR_ON_SWITCH_KEYS = ['tinycld_sidebar_open', 'tinycld:anon-id']

// Prefixes whose every key is per-server. Matched against the full AsyncStorage
// keyspace so a package's own `firstRun:<pkg>` entries are covered without this
// module having to enumerate packages it must not depend on.
const CLEAR_ON_SWITCH_PREFIXES = ['firstRun:']

// The pb file token (['pb-files-token'] in use-authed-file-url.ts) is a
// credential held in the React Query cache, whose keys carry neither server nor
// user. It is memory-only, so the reload clears it — no disk cleanup needed, but
// it is the reason a refused switch must NOT leave the address changed.
async function clearPerServerState(): Promise<void> {
    try {
        const keys = await AsyncStorage.getAllKeys()
        const doomed = keys.filter(
            key =>
                CLEAR_ON_SWITCH_KEYS.includes(key) ||
                CLEAR_ON_SWITCH_PREFIXES.some(prefix => key.startsWith(prefix))
        )
        if (doomed.length > 0) await AsyncStorage.multiRemove(doomed)
    } catch (err) {
        // Never block a switch on cleanup — the worst case is stale UI state,
        // and refusing the switch here would be a worse outcome than that.
        captureException('switch-server.clearPerServerState', err)
    }
}

// switchToServer makes `origin` the active server and restarts the JS context so
// its bundle and a clean module graph load.
//
// Why a reload rather than an in-place swap: `pb` follows the address for free
// (PB_SERVER_ADDR is a live Proxy the SDK re-reads per call), but the pbtsdb
// collections, the QueryClient and PBTSDBProvider are module-eval singletons
// bound to the old server, and clearStores() is terminal — nothing re-creates
// them. Only a fresh module graph rebinds them. On the hosted hosting router
// this is doubly required: each org runs a different build with a different
// package set, so continuing on the previous bundle would be wrong regardless.
//
// Deliberately does NOT call disconnectServer(). That would sign the user out of
// the server they are leaving — the entire point is that its session survives —
// and it carries an ordering hazard (PB's RealtimeService auto-reconnects and
// reconnect reads PB_SERVER_ADDR, so clearing the address first trips the
// "not resolved" guard). We never null the address, so that hazard cannot arise.
//
// A switch either completes or is refused. If the JS context cannot be
// restarted, the resolved address is left pointing at the PREVIOUS server so the
// running module graph stays coherent, and the error propagates. The saved
// active pointer is written first and deliberately left in place: a manual
// restart then completes the switch through the normal launch path.
export async function switchToServer(origin: string): Promise<void> {
    // Captured before anything moves: this is the server the RUNNING module
    // graph is bound to, and the address we must restore if the reload refuses.
    const previous = getResolvedAddress()
    const target = await setActiveServer(origin)

    await clearPerServerState()

    // Stop the old server's realtime stream and in-flight requests BEFORE
    // repointing, so its EventSource cannot retry against the new host.
    try {
        pb.realtime.unsubscribe()
        pb.cancelAllRequests()
    } catch (err) {
        captureException('switch-server.teardownRealtime', err)
    }

    // Point `pb` at the new server and rebind the native bundle store (via
    // setResolvedAddress → bindBundleStoreToServer → AppUpdater.setActiveServer),
    // so the pre-bridge loader picks the right bundle at the next launch.
    setResolvedAddress(target)

    try {
        await reloadJsContext()
    } catch (err) {
        // Restore the running context to the server whose module graph is
        // actually live. Without this the app runs with `pb` on the new server
        // and every collection still bound to the old one — a half-switch that
        // presents as success.
        if (previous) setResolvedAddress(previous)
        throw err
    }
}
