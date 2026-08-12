import { QueryCache, QueryClient } from '@tanstack/react-query'
import { type MergedPackageSchema, tinycldConfig } from '@tinycld/app-generated/tinycld-config'
import { captureException } from '@tinycld/core/lib/errors'
import { buildPackageStores } from '@tinycld/core/lib/packages/derive-stores'
import type { Schema, Users } from '@tinycld/core/types/pbSchema'
import { BasicIndex, createCollection, createReactProvider, setLogger } from 'pbtsdb'
import PocketBase, { AsyncAuthStore } from 'pocketbase'
import { Platform } from 'react-native'
import { clearAuthBlob, parseAuthBlob, readAuthBlob, writeAuthBlob } from './auth-storage'
import { PB_SERVER_ADDR } from './config'
import { getResolvedAddress, subscribeResolvedAddress } from './server-address'
import { createReachabilityTracker } from './server-reachability'
import { shareTokenHeaders } from './share-token'
import { useConnectivityStore } from './stores/connectivity-store'
import type { UserSession } from './types'

export { eq } from '@tanstack/db'

// MergedSchema = core's Schema intersected with each linked package's schema.
// MergedPackageSchema is a PRECOMPUTED LITERAL intersection from the generated
// config (NOT derived from the config values) to avoid a circular type
// reference through coreStores. buildPackageStores wires stores from the config
// values at runtime.
type MergedSchema = Schema & MergedPackageSchema

if (Platform.OS !== 'web') {
    // Only polyfill EventSource on native — the browser has its own
    import('react-native-sse').then(mod => {
        global.EventSource = mod.default as unknown as typeof global.EventSource
    })
}

export { PB_SERVER_ADDR }

// The auth blob is keyed by server (`pb_auth:<serverKey>`) so several servers
// can hold sessions at once. That makes reading it depend on the RESOLVED
// ADDRESS, which is a boot-ordering change, not just a key rename: hydration
// used to fire on a setTimeout(0) at module eval, which can run before the gate
// has resolved anything — and at that moment getResolvedAddress() is still null
// (exactly why PB_SERVER_ADDR throws pre-gate). So we await the address instead
// of racing it.
//
// Awaiting is still deferred off module evaluation, which the original
// setTimeout(0) existed to guarantee: touching AsyncStorage during module eval
// crashes on React Native before the bridge is ready (AsyncStorage v3+). The
// await below provides that deferral — the address is resolved by the gate,
// which by construction runs after the bridge is up. (The old code expressed
// this as a `typeof window !== 'undefined'` guard around the storage read; the
// deferral, not the presence of `window`, is what actually made it safe.)
//
// Resolves to null rather than waiting forever when no address ever arrives —
// which is a real state, not a theoretical one: a fresh install with no cached
// address sits on /connect until the user picks a server. `authStoreReady` must
// still settle there, because auth-store's hydrate() awaits it before setting
// hasHydrated, and a promise that never resolves would strand the app on a
// spinner with nothing logged. There is no session to hydrate in that state
// anyway; once the user connects, the JS context is already reloaded (or the
// gate mounts the tree fresh), so hydration re-runs with an address.
const ADDRESS_WAIT_MS = 10_000

function whenAddressResolved(): Promise<string | null> {
    const current = getResolvedAddress()
    if (current) return Promise.resolve(current)
    return new Promise<string | null>(resolve => {
        const settle = (address: string | null) => {
            clearTimeout(timer)
            unsubscribe()
            resolve(address)
        }
        const timer = setTimeout(() => settle(null), ADDRESS_WAIT_MS)
        const unsubscribe = subscribeResolvedAddress(() => {
            const address = getResolvedAddress()
            if (address) settle(address)
        })
    })
}

// The address hydration read its blob from. Captured so save/clear write the
// SAME key the session was loaded from, even if the address changes underneath
// (a switch reloads the JS context, but a write racing that reload must not
// land under the new server's key).
let authAddress: string | null = null

const store = new AsyncAuthStore({
    save: async serialized => {
        if (!authAddress) return
        await writeAuthBlob(authAddress, serialized)
    },
    clear: async () => {
        if (!authAddress) return
        await clearAuthBlob(authAddress)
    },
})

// AsyncAuthStore's own `initial` option runs through a private
// fire-and-forget _loadInitial that swallows errors, races consumers, and
// gives no readiness signal. Instead we hydrate explicitly: read storage,
// parse, call store.save(). authStoreReady resolves only after that
// completes, so anyone awaiting it sees a settled pb.authStore — the contract
// every existing `await authStoreReady` caller depends on is unchanged.
export const authStoreReady = whenAddressResolved().then(async address => {
    if (!address) return
    authAddress = address

    // Migrates a legacy flat `pb_auth` into this server's scoped key on first
    // run — without which every existing user is signed out on upgrade.
    const storedAuth = await readAuthBlob(address)
    if (!storedAuth) return

    const parsed = parseAuthBlob(storedAuth)
    if (!parsed) {
        // Corrupt stored auth — leave the store empty and let the user
        // re-authenticate. Clearing storage avoids replaying the corruption
        // on next boot.
        await clearAuthBlob(address)
        return
    }
    if (parsed.token) {
        store.save(parsed.token, (parsed.record ?? parsed.model) as never)
    }
})

export const pb = new PocketBase(PB_SERVER_ADDR, store)

pb.autoCancellation(false)

// A share-link visitor is unauthenticated, so every row they may read is
// authorized by a token rather than by `@request.auth.id`. Attaching it here
// covers every REST call the SDK makes; the realtime half rides
// `subscribeOptions` on the collection factory below, and both arrive at the
// rule as `@request.headers.x_share_token` because PocketBase snakecases header
// names identically on the two paths.
pb.beforeSend = (url, options) => {
    const headers = shareTokenHeaders()
    if (headers) {
        options.headers = { ...options.headers, ...headers }
    }
    return { url, options }
}

// The server-unreachable signal (which drives the offline overlay) is
// derived from pb.send outcomes via a rolling sustained-failure tracker.
// See server-reachability.ts for the rationale — in short, a single blip,
// an aborted request, or a failed auth-refresh must NOT flip the badge;
// only several genuine network failures in quick succession do.
const reachability = createReachabilityTracker()

const origSend = pb.send.bind(pb)
pb.send = (async <T>(path: string, options: Parameters<typeof origSend>[1]) => {
    try {
        const result = (await origSend(path, options)) as T
        reachability.record(path, true, null, Date.now())
        // Any successful request proves the server is reachable — recover the
        // signal regardless of tracker streak state (the health-probe poll or
        // another path may have flipped it).
        if (!useConnectivityStore.getState().isServerReachable) {
            useConnectivityStore.getState().setServerReachable(true)
        }
        return result
    } catch (err) {
        if (reachability.record(path, false, err, Date.now()) === 'down') {
            useConnectivityStore.getState().setServerReachable(false)
        }
        throw err
    }
}) as typeof pb.send

export function usePocketBase() {
    return pb
}

// Tear down the live PB session, then drop the resolved address so the
// next gate pass routes to the picker. Order matters: PB's RealtimeService
// auto-reconnects on EventSource error, and reconnect reads PB_SERVER_ADDR
// — clearing the address before disconnecting realtime trips the "address
// not resolved" guard. The auth-store's logout already does the realtime
// teardown + auth clear + resetSessionState (stores + query cache); we
// just need to add the address clear.
export async function disconnectServer() {
    const { useAuthStore } = await import('./stores/auth-store')
    useAuthStore.getState().logout()
    pb.cancelAllRequests()
    const { clearCached, setResolvedAddress } = await import('./server-address')
    await clearCached()
    setResolvedAddress(null)
}

// Route pbtsdb's internal logs somewhere useful. Errors go to Sentry so a
// failed subscription/sync is observable in production instead of being
// silently dropped — the previous empty if/else bodies discarded EVERYTHING,
// errors included, which is the bug this fixes. The debug/info/warn levels are
// subscription-lifecycle chatter with no production value, so they're
// intentionally dropped rather than logged (no unguarded console shipped).
setLogger({
    debug: () => {},
    info: () => {},
    warn: () => {},
    error: (msg, context) => {
        // Stable grouping key; the varying message rides in the Error and the
        // pbtsdb context object rides in `extra`.
        captureException('pbtsdb.error', new Error(msg), context as Record<string, unknown>)
    },
})

// A global query-error handler so failed reads are observable in Sentry — useQuery
// surfaces errors via state rather than throwing, so without this a read failure is
// silent (the component shows its own error UI, but nothing is logged). The queryKey
// is the grouping context. Mutations report via their own onError/handleMutationErrorsWithForm.
const queryClient = new QueryClient({
    queryCache: new QueryCache({
        onError: (error, query) => {
            captureException('query.error', error, { queryKey: query.queryKey })
        },
    }),
})

// `subscribeOptions` is a GETTER, called at subscribe time rather than read
// once: a subscription is re-established on reconnect and whenever a
// collection's subscriber count rises from zero, so a captured value would go
// stale exactly when it matters — most sharply at OTP sign-in, where the token
// stops being sent and the membership half of the rule takes over.
const newCollection = createCollection<MergedSchema>(pb, queryClient, {
    subscribeOptions: () => {
        const headers = shareTokenHeaders()
        return headers ? { headers } : undefined
    },
})

const indexing = {
    collectionOptions: {
        autoIndex: 'eager' as const,
        defaultIndexType: BasicIndex,
    },
}

const users = newCollection('users', {
    omitOnInsert: ['created', 'updated', 'password', 'tokenKey'],
    ...indexing,
})

const settings = newCollection('settings', {
    omitOnInsert: ['created', 'updated'],
    ...indexing,
})

const user_preferences = newCollection('user_preferences', {
    omitOnInsert: ['created', 'updated'],
    expand: { user: users },
    ...indexing,
})

const labels = newCollection('labels', {
    omitOnInsert: ['created', 'updated'],
    expand: { user: users },
    ...indexing,
})

const label_assignments = newCollection('label_assignments', {
    omitOnInsert: ['created', 'updated'],
    expand: { label: labels, user: users },
    ...indexing,
})

const org_pkg_access = newCollection('org_pkg_access', {
    omitOnInsert: ['created', 'updated'],
    expand: { user: users },
    ...indexing,
})

// Read-only from the client: creation/approval/revocation all go through Go
// handlers in core/server/oauth (grants carry credential material a client must
// never write). Registered so the Connected apps settings screen can read a
// user's own grants live via useOrgLiveQuery — the oauth_grants list/view rule
// already scopes reads to `user = @request.auth.id`.
const oauth_grants = newCollection('oauth_grants', {
    ...indexing,
})

const pkg_registry = newCollection('pkg_registry', {
    omitOnInsert: ['created', 'updated'],
    ...indexing,
})

// Build-history catalog for the Admin panel's Builds tab. Read-only from the
// client (the build pipeline writes it server-side); registered so the tab can
// read it via the pbtsdb store instead of the raw PocketBase client.
const pkg_build = newCollection('pkg_build', {
    omitOnInsert: ['created', 'updated'],
    ...indexing,
})

// System-wide configuration (Sentry, web-push, mail provider creds). Read/written
// by the /admin Settings console; the server reads it through SystemConfig. See
// the create_system_settings migration.
const system_settings = newCollection('system_settings', {
    omitOnInsert: ['created', 'updated'],
    ...indexing,
})

const audit_logs = newCollection('audit_logs', {
    omitOnInsert: ['created', 'updated'],
    expand: { actor: users },
    ...indexing,
})

const pkg_install_log = newCollection('pkg_install_log', {
    omitOnInsert: ['created', 'updated'],
    expand: { initiated_by: users },
    ...indexing,
})

const notifications = newCollection('notifications', {
    omitOnInsert: ['created', 'updated'],
    expand: { user: users },
    ...indexing,
})
export const notificationsCollection = notifications

const rules = newCollection('rules', {
    omitOnInsert: ['created', 'updated'],
    expand: { owner: users },
    ...indexing,
})

const rule_runs = newCollection('rule_runs', {
    omitOnInsert: ['created', 'updated'],
    expand: { rule: rules },
    ...indexing,
})

// NOTE: the `comment_mentions` store registration was removed in the new
// standalone-core layout because the collection is created by a feature
// package's migration (drive's create_comment_mentions), not core's own — so
// core can't unconditionally register it when that package isn't linked. The
// comment-mutations factory takes the collection as a parameter, so nothing in
// core hard-depends on a core-owned store here. Re-introducing comments as a
// first-class core feature (with its own migration) is tracked as a follow-up.

const coreStores = {
    users,
    settings,
    user_preferences,
    labels,
    label_assignments,
    org_pkg_access,
    pkg_registry,
    pkg_build,
    audit_logs,
    pkg_install_log,
    notifications,
    rules,
    rule_runs,
    system_settings,
    oauth_grants,
}
export type CoreStores = typeof coreStores

const stores = {
    ...coreStores,
    ...buildPackageStores(tinycldConfig, newCollection, coreStores),
}

const { Provider: PBTSDBProvider, useStore } = createReactProvider(stores)

export function getUserFromAuthStore(): UserSession | null {
    const authRecord = pb.authStore.record as Users | null
    const authToken = pb.authStore.token

    if (!authRecord || !authToken || !pb.authStore.isValid) {
        return null
    }

    const metadata = (authRecord as Users & { metadata?: Record<string, unknown> }).metadata
    return {
        id: authRecord.id,
        name: authRecord.name,
        email: authRecord.email,
        isDemo: !!(authRecord as Users & { is_demo?: boolean }).is_demo,
        isBetaTester: !!metadata?.isBetaTester,
    }
}

// PocketBase auth tokens carry a JWT `exp` (default 7-day duration) and are
// never renewed on their own — the SDK doesn't auto-refresh, and expiry doesn't
// fire authStore.onChange. Left alone, a long-open session silently crosses exp:
// the token string is still present (so naive presence checks read "logged in"),
// but every request now carries a dead token, so reads return nothing and the
// pbtsdb collections drain to empty — the "stale session" state where fetches
// yield no records and any create fails for want of org context.
//
// refreshAuth reconciles that. It mints a fresh token when the current one is
// still valid (extending an active session indefinitely), and on any failure —
// including a token already past exp — clears the auth store so the app routes
// to the login gate instead of running as a broken half-session. authRefresh()
// saves the new token, which DOES fire onChange, keeping the auth-store user in
// sync. Returns whether a valid session remains afterward.
export async function refreshAuth(): Promise<boolean> {
    if (!pb.authStore.token) return false
    try {
        await pb.collection('users').authRefresh()
        return pb.authStore.isValid
    } catch (err) {
        // A network blip must not sign the user out; only an authoritative
        // rejection (expired/revoked token → 401/403) should. On a transient
        // failure we keep whatever token we have and let the next tick retry.
        if (isAuthRejection(err)) {
            pb.authStore.clear()
            return false
        }
        return pb.authStore.isValid
    }
}

// True when the error is PocketBase rejecting our credentials (401/403), as
// opposed to a network/timeout failure. Only the former means the token is
// genuinely dead and the session must be cleared.
function isAuthRejection(err: unknown): boolean {
    const status = (err as { status?: number } | null)?.status
    return status === 401 || status === 403
}

export async function seedUser(userRecord: Users) {
    await stores.users.preload()
    stores.users.utils?.writeUpsert(userRecord)
}

export async function preloadStores() {
    await Promise.all([
        stores.users.preload(),
        stores.org_pkg_access.preload(),
        stores.pkg_registry.preload(),
    ])
}

export async function clearStores() {
    for (const s of Object.values(stores)) {
        // cleanup() on an already cleaned-up collection throws an invalid
        // status-transition error, and repeated teardowns are a legal path
        // (sign out, then "change server" runs logout() a second time).
        if (s.status === 'cleaned-up') continue
        await s.cleanup()
    }
}

// Single teardown point for everything cached per-session in memory. On web
// the SPA survives logout → next login in the same tab (no reload), so
// anything left behind leaks across users on the same device: the pbtsdb
// stores still hold the previous user's rows, and the React Query cache still
// holds their file token (['pb-files-token'] in use-authed-file-url.ts) —
// which authorizes file reads as that user. Runs from the auth-store's
// logout(), which disconnectServer() also routes through.
//
// Push subscription teardown is NOT done here: it needs the userId and an
// authorized PB session, both of which are gone by the time resetSessionState
// runs (logout() clears pb.authStore first). The auth-store's logout() tears
// down push explicitly, before the auth clear — see teardownPushOnLogout.
export async function resetSessionState() {
    await clearStores()
    queryClient.clear()
}

export { PBTSDBProvider, queryClient, stores, useStore }
